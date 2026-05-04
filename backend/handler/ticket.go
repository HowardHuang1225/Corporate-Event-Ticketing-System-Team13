package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/pkg"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TicketHandler struct {
	db    *gorm.DB
	redis *redis.Client
}

func NewTicketHandler(db *gorm.DB, redis *redis.Client) *TicketHandler {
	return &TicketHandler{db: db, redis: redis}
}

type ApplyRequest struct {
	EventID        string `json:"event_id" binding:"required"`
	TicketTypeID   string `json:"ticket_type_id" binding:"required"`
	Quantity       int    `json:"quantity" binding:"required,min=1"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
}

// ────────────────────────────────────────────────────────────
// POST /v1/applications   Apply for ticket (EMPLOYEE)
// ────────────────────────────────────────────────────────────
func (h *TicketHandler) Apply(c *gin.Context) {
	var req ApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errResp("VALIDATION_ERROR", err.Error()))
		return
	}

	userID, _ := uuid.Parse(c.GetString("user_id"))
	eventID, _ := uuid.Parse(req.EventID)
	ticketTypeID, _ := uuid.Parse(req.TicketTypeID)

	var existing model.Application
	if err := h.db.Where("idempotency_key = ? AND user_id = ?", req.IdempotencyKey, userID.String()).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, okResp(existing))
		return
	}

	// 1. Redis Pre-decrement for inventory
	inventoryKey := "inventory:" + req.TicketTypeID
	ctx := context.Background()

	// Ensure Redis has the inventory count (Lazy loading with safety check)
	// We use a separate flag to ensure the inventory was properly loaded from DB
	loadedKey := "inventory_loaded:" + req.TicketTypeID
	if h.redis.Exists(ctx, loadedKey).Val() == 0 {
		lockKey := "init_lock:" + req.TicketTypeID
		if ok, _ := pkg.AcquireLock(ctx, h.redis, lockKey, 5*time.Second, 60*time.Second); ok {
			// Re-check after acquiring lock
			if h.redis.Exists(ctx, loadedKey).Val() == 0 {
				var tt model.TicketType
				if err := h.db.First(&tt, "id = ?", ticketTypeID).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Ticket type not found"))
					} else {
						c.JSON(http.StatusInternalServerError, errResp("DB_ERROR", "Database system busy"))
					}
					pkg.ReleaseLock(ctx, h.redis, lockKey)
					return
				}
				// Atomic initialization
				pipe := h.redis.TxPipeline()
				pipe.Set(ctx, inventoryKey, tt.Remaining, 24*time.Hour)
				pipe.Set(ctx, loadedKey, "1", 24*time.Hour)
				if _, err := pipe.Exec(ctx); err != nil {
					log.Printf("Redis init error: %v", err)
				}
			}
			pkg.ReleaseLock(ctx, h.redis, lockKey)
		}
	}

	// Atomic decrement in Redis
	newStock, err := h.redis.DecrBy(ctx, inventoryKey, int64(req.Quantity)).Result()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, errResp("BUSY", "Inventory system busy"))
		return
	}

	// 2. Check if we went below zero
	if newStock < 0 {
		h.redis.IncrBy(ctx, inventoryKey, int64(req.Quantity)) // Compensate

		// Double check idempotency before returning SOLD_OUT (with a small retry loop for DB lag)
		for checkAttempt := 0; checkAttempt < 3; checkAttempt++ {
			var existing model.Application
			if err := h.db.Where("idempotency_key = ? AND user_id = ?", req.IdempotencyKey, userID.String()).First(&existing).Error; err == nil {
				c.JSON(http.StatusOK, okResp(existing))
				return
			}
			time.Sleep(50 * time.Millisecond)
		}

		c.JSON(http.StatusConflict, errResp("TICKET_SOLD_OUT", "Not enough tickets remaining"))
		return
	}

	var createdApp model.Application
	var existingApp model.Application
	created := false

	// Cleanup on DB failure or idempotent retry
	success := false
	defer func() {
		if !success {
			h.redis.IncrBy(ctx, inventoryKey, int64(req.Quantity))
		}
	}()

	for attempt := 0; attempt < 5; attempt++ {
		created = false
		createdApp = model.Application{}
		existingApp = model.Application{}

		err = h.db.Transaction(func(tx *gorm.DB) error {
			lockName := fmt.Sprintf("apply:%s:%s", userID.String(), eventID.String())
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockName).Error; err != nil {
				return err
			}

			if err := tx.Where("idempotency_key = ? AND user_id = ?", req.IdempotencyKey, userID.String()).First(&existingApp).Error; err == nil {
				return nil
			} else if err != gorm.ErrRecordNotFound {
				return err
			}

			var event model.Event
			if err := tx.First(&event, "id = ?", eventID).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return cErr(http.StatusNotFound, "NOT_FOUND", "Event not found")
				}
				return err
			}
			if event.Status != "published" {
				return cErr(http.StatusBadRequest, "EVENT_NOT_AVAILABLE", "Event is not accepting applications")
			}
			if time.Now().After(event.ApplyDeadline) {
				return cErr(http.StatusBadRequest, "APPLY_DEADLINE_PASSED", "Application deadline has passed")
			}

			if event.RegionRestriction != nil && *event.RegionRestriction != "" {
				var user model.User
				if err := tx.First(&user, "id = ?", userID.String()).Error; err != nil {
					return err
				}
				if user.Region != *event.RegionRestriction {
					return cErr(http.StatusForbidden, "NOT_ELIGIBLE", "You are not eligible due to region restriction (restricted to "+*event.RegionRestriction+")")
				}
			}

			var issuedCount int64
			if err := tx.Model(&model.Ticket{}).
				Where("user_id = ? AND event_id = ?", userID.String(), eventID.String()).
				Count(&issuedCount).Error; err != nil {
				return err
			}

			var pendingCount int
			if err := tx.Model(&model.Application{}).
				Where("user_id = ? AND event_id = ? AND status = 'pending'", userID.String(), eventID.String()).
				Select("COALESCE(SUM(quantity), 0)").
				Scan(&pendingCount).Error; err != nil {
				return err
			}

			currentTotal := int(issuedCount) + pendingCount
			remainingAllowance := event.MaxTicketsPerPerson - currentTotal
			if req.Quantity > remainingAllowance {
				return cErr(http.StatusBadRequest, "EXCEEDS_MAX_TICKETS", fmt.Sprintf("You have already applied for %d tickets. The limit is %d. You can only apply for %d more.", currentTotal, event.MaxTicketsPerPerson, remainingAllowance))
			}

			res := tx.Model(&model.TicketType{}).
				Where("id = ? AND event_id = ? AND remaining >= ?", ticketTypeID, eventID, req.Quantity).
				Updates(map[string]interface{}{
					"remaining": gorm.Expr("remaining - ?", req.Quantity),
					"version":   gorm.Expr("version + 1"),
				})

			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return cErr(http.StatusConflict, "TICKET_SOLD_OUT", "Not enough tickets remaining")
			}

			app := model.Application{
				UserID:         userID,
				EventID:        eventID,
				TicketTypeID:   ticketTypeID,
				Quantity:       req.Quantity,
				Status:         "pending",
				IdempotencyKey: req.IdempotencyKey,
			}

			result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "idempotency_key"}},
				DoNothing: true,
			}).Create(&app)
			if result.Error != nil {
				return result.Error
			}

			if result.RowsAffected == 0 {
				if err := tx.Model(&model.TicketType{}).Where("id = ?", ticketTypeID).Updates(map[string]interface{}{
					"remaining": gorm.Expr("remaining + ?", req.Quantity),
					"version":   gorm.Expr("version + 1"),
				}).Error; err != nil {
					return err
				}
				if err := tx.Where("idempotency_key = ? AND user_id = ?", req.IdempotencyKey, userID.String()).First(&existingApp).Error; err != nil {
					return err
				}
				return nil
			}

			createdApp = app
			created = true
			return nil
		})

		if err == nil {
			if created {
				success = true
			}
			break
		} else if !isRetryableApplyError(err) {
			break
		}
		log.Printf("⚠️  DB Conflict, retrying... (attempt %d/5): %v", attempt+1, err)
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}

	if err != nil {
		if ce, ok := err.(*clientError); ok {
			c.JSON(ce.status, errResp(ce.code, ce.message))
			return
		}
		log.Printf("❌ Apply failed after retries: user=%s err=%v", userID, err)
		c.JSON(http.StatusServiceUnavailable, errResp("BUSY", "System busy, please retry"))
		return
	}

	if !created {
		c.JSON(http.StatusOK, okResp(existingApp))
		return
	}

	c.JSON(http.StatusCreated, okResp(createdApp))
}

type clientError struct {
	status  int
	code    string
	message string
}

func (e *clientError) Error() string {
	return e.message
}

func cErr(status int, code, message string) error {
	return &clientError{status: status, code: code, message: message}
}

func isRetryableApplyError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", "40P01", "55P03", "57014":
			return true
		}
	}
	return errors.Is(err, gorm.ErrInvalidTransaction)
}

// ────────────────────────────────────────────────────────────
// GET /v1/applications/my   List my applications (EMPLOYEE)
// ────────────────────────────────────────────────────────────
func (h *TicketHandler) MyApplications(c *gin.Context) {
	userID := c.GetString("user_id")
	var apps []model.Application
	h.db.Preload("Event").Preload("TicketType").Preload("Tickets").
		Where("user_id = ?", userID).
		Order("applied_at desc").
		Find(&apps)
	c.JSON(http.StatusOK, okResp(apps))
}

// ────────────────────────────────────────────────────────────
// GET /v1/applications   List all applications (MANAGER)
// ────────────────────────────────────────────────────────────
func (h *TicketHandler) ListApplications(c *gin.Context) {
	var apps []model.Application
	q := h.db.Preload("User").Preload("Event").Preload("TicketType")
	if eid := c.Query("event_id"); eid != "" {
		q = q.Where("event_id = ?", eid)
	}
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	q.Order("applied_at desc").Find(&apps)
	c.JSON(http.StatusOK, okResp(apps))
}

// ────────────────────────────────────────────────────────────
// POST /v1/applications/:id/approve   Approve application (MANAGER)
// ────────────────────────────────────────────────────────────
func (h *TicketHandler) ApproveApplication(c *gin.Context) {
	reviewerID, _ := uuid.Parse(c.GetString("user_id"))
	var app model.Application
	if err := h.db.Preload("Event").First(&app, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Application not found"))
		return
	}
	if app.Status != "pending" {
		c.JSON(http.StatusBadRequest, errResp("INVALID_STATUS", "Application is not pending"))
		return
	}

	now := time.Now()
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		// Use a WHERE clause to ensure we only update if it's still pending
		res := tx.Model(&model.Application{}).
			Where("id = ? AND status = 'pending'", app.ID).
			Updates(map[string]interface{}{
				"status":      "approved",
				"reviewed_by": reviewerID,
				"reviewed_at": now,
			})

		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return cErr(http.StatusConflict, "ALREADY_PROCESSED", "Application has already been approved or rejected")
		}

		for i := 0; i < app.Quantity; i++ {
			t := model.Ticket{
				ApplicationID: app.ID,
				UserID:        app.UserID,
				EventID:       app.EventID,
				TicketTypeID:  app.TicketTypeID,
				QRToken:       uuid.New().String(),
				ExpiresAt:     app.Event.EndTime,
			}
			if err := tx.Create(&t).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "Failed to approve"))
		return
	}

	h.db.Preload("Tickets").First(&app, app.ID)
	c.JSON(http.StatusOK, okResp(app))
}

type RejectRequest struct {
	Reason string `json:"reason"`
}

// ────────────────────────────────────────────────────────────
// POST /v1/applications/:id/reject   Reject application (MANAGER)
// ────────────────────────────────────────────────────────────
func (h *TicketHandler) RejectApplication(c *gin.Context) {
	reviewerID, _ := uuid.Parse(c.GetString("user_id"))
	var req RejectRequest
	c.ShouldBindJSON(&req)

	var app model.Application
	if err := h.db.First(&app, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Application not found"))
		return
	}
	if app.Status != "pending" {
		c.JSON(http.StatusBadRequest, errResp("INVALID_STATUS", "Application is not pending"))
		return
	}

	now := time.Now()
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Application{}).
			Where("id = ? AND status = 'pending'", app.ID).
			Updates(map[string]interface{}{
				"status":      "rejected",
				"reason":      req.Reason,
				"reviewed_by": reviewerID,
				"reviewed_at": now,
			})

		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return cErr(http.StatusConflict, "ALREADY_PROCESSED", "Application has already been approved or rejected")
		}

		// Return inventory to DB
		if err := tx.Model(&model.TicketType{}).Where("id = ?", app.TicketTypeID).
			Update("remaining", gorm.Expr("remaining + ?", app.Quantity)).Error; err != nil {
			return err
		}

		// Also return to Redis
		inventoryKey := "inventory:" + app.TicketTypeID.String()
		h.redis.IncrBy(context.Background(), inventoryKey, int64(app.Quantity))

		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "Failed to reject"))
		return
	}
	c.JSON(http.StatusOK, okResp(app))
}

// ────────────────────────────────────────────────────────────
// GET /v1/tickets/my   List my active tickets (EMPLOYEE)
// ────────────────────────────────────────────────────────────
func (h *TicketHandler) MyTickets(c *gin.Context) {
	userID := c.GetString("user_id")
	var tickets []model.Ticket
	h.db.Preload("Event").Preload("TicketType").
		Where("user_id = ?", userID).
		Order("issued_at desc").
		Find(&tickets)
	c.JSON(http.StatusOK, okResp(tickets))
}

// ────────────────────────────────────────────────────────────
// POST /v1/applications/:id/cancel   Cancel application (EMPLOYEE)
// ────────────────────────────────────────────────────────────
func (h *TicketHandler) CancelApplication(c *gin.Context) {
	userID, _ := uuid.Parse(c.GetString("user_id"))
	appID := c.Param("id")

	var app model.Application
	if err := h.db.First(&app, "id = ? AND user_id = ?", appID, userID.String()).Error; err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Application not found"))
		return
	}

	if app.Status == "cancelled" || app.Status == "rejected" {
		c.JSON(http.StatusConflict, errResp("INVALID_STATUS", "Application is already cancelled or rejected"))
		return
	}

	// If already approved, check if any tickets have been used
	if app.Status == "approved" {
		var usedCount int64
		h.db.Model(&model.Ticket{}).Where("application_id = ? AND is_used = true", app.ID).Count(&usedCount)
		if usedCount > 0 {
			c.JSON(http.StatusConflict, errResp("TICKETS_ALREADY_USED", "Cannot cancel application because some tickets have already been used. Please return unused tickets individually."))
			return
		}
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&app).Update("status", "cancelled").Error; err != nil {
			return err
		}

		if err := tx.Model(&model.TicketType{}).Where("id = ?", app.TicketTypeID).
			Updates(map[string]interface{}{
				"remaining": gorm.Expr("remaining + ?", app.Quantity),
				"version":   gorm.Expr("version + 1"),
			}).Error; err != nil {
			return err
		}

		if err := tx.Where("application_id = ?", app.ID).Delete(&model.Ticket{}).Error; err != nil {
			return err
		}

		// Sync Redis
		inventoryKey := "inventory:" + app.TicketTypeID.String()
		h.redis.IncrBy(context.Background(), inventoryKey, int64(app.Quantity))

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "Failed to cancel application"))
		return
	}

	c.JSON(http.StatusOK, okResp(gin.H{"message": "Application cancelled and tickets returned to pool"}))
}

// ────────────────────────────────────────────────────────────
// POST /v1/tickets/:id/cancel   Return/Cancel issued ticket (EMPLOYEE)
// ────────────────────────────────────────────────────────────
func (h *TicketHandler) CancelTicket(c *gin.Context) {
	userID, _ := uuid.Parse(c.GetString("user_id"))
	ticketID := c.Param("id")

	var ticket model.Ticket
	if err := h.db.Preload("Application").First(&ticket, "id = ? AND user_id = ?", ticketID, userID.String()).Error; err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Ticket not found"))
		return
	}

	if ticket.IsUsed {
		c.JSON(http.StatusConflict, errResp("ALREADY_USED", "Cannot return a ticket that has already been used"))
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.TicketType{}).Where("id = ?", ticket.TicketTypeID).
			Updates(map[string]interface{}{
				"remaining": gorm.Expr("remaining + 1"),
				"version":   gorm.Expr("version + 1"),
			}).Error; err != nil {
			return err
		}

		refundApp := model.Application{
			UserID:         ticket.UserID,
			EventID:        ticket.EventID,
			TicketTypeID:   ticket.TicketTypeID,
			Quantity:       1,
			Status:         "cancelled",
			IdempotencyKey: "refund-" + ticket.ID.String(),
			Reason:         pkg.Ptr("退票 (原票號: " + ticket.ID.String()[:8] + ")"),
		}
		if err := tx.Create(&refundApp).Error; err != nil {
			return err
		}

		if err := tx.Delete(&ticket).Error; err != nil {
			return err
		}

		// Sync Redis
		inventoryKey := "inventory:" + ticket.TicketTypeID.String()
		h.redis.IncrBy(context.Background(), inventoryKey, 1)

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "Failed to return ticket"))
		return
	}

	c.JSON(http.StatusOK, okResp(gin.H{"message": "Ticket returned successfully"}))
}

type CheckinRequest struct {
	QRToken string `json:"qr_token" binding:"required"`
}

// ────────────────────────────────────────────────────────────
// POST /v1/checkin   Scan ticket for check-in (MANAGER)
// ────────────────────────────────────────────────────────────
func (h *TicketHandler) Checkin(c *gin.Context) {
	var req CheckinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errResp("VALIDATION_ERROR", err.Error()))
		return
	}
	checkerID, _ := uuid.Parse(c.GetString("user_id"))

	var ticket model.Ticket
	now := time.Now()

	err := h.db.Transaction(func(tx *gorm.DB) error {
		// 1. Atomically update ticket status
		res := tx.Model(&model.Ticket{}).
			Where("qr_token = ? AND is_used = false AND expires_at > ?", req.QRToken, now).
			Update("is_used", true)

		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// If update failed, we need to find out why to return a good error
			if err := tx.Where("qr_token = ?", req.QRToken).First(&ticket).Error; err != nil {
				return cErr(http.StatusNotFound, "NOT_FOUND", "Ticket not found")
			}
			if ticket.IsUsed {
				return cErr(http.StatusConflict, "ALREADY_CHECKED_IN", "This ticket has already been used")
			}
			if now.After(ticket.ExpiresAt) {
				return cErr(http.StatusGone, "TICKET_EXPIRED", "This ticket has expired")
			}
			return cErr(http.StatusBadRequest, "INVALID_REQUEST", "Ticket is invalid")
		}

		// 2. Fetch full ticket info for record and response
		if err := tx.Preload("Event").Preload("TicketType").Preload("User").
			First(&ticket, "qr_token = ?", req.QRToken).Error; err != nil {
			return err
		}

		// 3. Create check-in record
		checkin := model.Checkin{TicketID: ticket.ID, CheckedBy: checkerID}
		if err := tx.Create(&checkin).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		if ce, ok := err.(*clientError); ok {
			c.JSON(ce.status, errResp(ce.code, ce.message))
			return
		}
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "Check-in failed: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, okResp(gin.H{
		"message": "Check-in successful!",
		"ticket":  ticket,
	}))
}

// ────────────────────────────────────────────────────────────
// GET /v1/checkins   List check-in records (MANAGER)
// ────────────────────────────────────────────────────────────
func (h *TicketHandler) ListCheckins(c *gin.Context) {
	var checkins []model.Checkin
	q := h.db.Preload("Ticket.Event").Preload("Ticket.TicketType").Preload("Ticket.User").Preload("Checker")
	if eid := c.Query("event_id"); eid != "" {
		q = q.Joins("JOIN tickets ON checkins.ticket_id = tickets.id").
			Where("tickets.event_id = ?", eid)
	}
	q.Order("checked_at desc").Limit(100).Find(&checkins)
	c.JSON(http.StatusOK, okResp(checkins))
}
