package handler

import (
	"context"
	"net/http"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/pkg"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type TicketHandler struct {
	db    *gorm.DB
	redis *redis.Client
}

func NewTicketHandler(db *gorm.DB, redis *redis.Client) *TicketHandler {
	return &TicketHandler{db: db, redis: redis}
}

// ────────────────────────────────────────────────────────────
// POST /v1/applications   Apply for ticket (EMPLOYEE)
// ────────────────────────────────────────────────────────────
type ApplyRequest struct {
	EventID        string `json:"event_id" binding:"required"`
	TicketTypeID   string `json:"ticket_type_id" binding:"required"`
	Quantity       int    `json:"quantity" binding:"required,min=1"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
}

func (h *TicketHandler) Apply(c *gin.Context) {
	var req ApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errResp("VALIDATION_ERROR", err.Error()))
		return
	}

	userID, _ := uuid.Parse(c.GetString("user_id"))

	// ── Idempotency check ──
	var existing model.Application
	if err := h.db.Where("idempotency_key = ? AND user_id = ?", req.IdempotencyKey, userID.String()).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, okResp(existing))
		return
	}

	// ── Duplicate application check ──
	var dup model.Application
	if err := h.db.Where(
		"user_id = ? AND event_id = ? AND ticket_type_id = ? AND status NOT IN ('cancelled','rejected')",
		userID.String(), req.EventID, req.TicketTypeID,
	).First(&dup).Error; err == nil {
		c.JSON(http.StatusConflict, errResp("DUPLICATE_APPLICATION", "You already have an active application for this ticket type"))
		return
	}

	// ── Fetch & validate event ──
	var event model.Event
	if err := h.db.First(&event, "id = ?", req.EventID).Error; err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Event not found"))
		return
	}
	if event.Status != "published" {
		c.JSON(http.StatusBadRequest, errResp("EVENT_NOT_AVAILABLE", "Event is not accepting applications"))
		return
	}
	if time.Now().After(event.ApplyDeadline) {
		c.JSON(http.StatusBadRequest, errResp("APPLY_DEADLINE_PASSED", "Application deadline has passed"))
		return
	}

	// ── Region restriction check ──
	if event.RegionRestriction != nil && *event.RegionRestriction != "" {
		var user model.User
		h.db.First(&user, "id = ?", userID.String())
		if user.Region != *event.RegionRestriction {
			c.JSON(http.StatusForbidden, errResp("NOT_ELIGIBLE", "You are not eligible due to region restriction (restricted to "+*event.RegionRestriction+")"))
			return
		}
	}
	if req.Quantity > event.MaxTicketsPerPerson {
		c.JSON(http.StatusBadRequest, errResp("VALIDATION_ERROR", "Quantity exceeds max tickets per person"))
		return
	}

	// ════════════════════════════════════════════════════════
	// ANTI-OVERSELL: Redis distributed lock + Optimistic lock
	// ════════════════════════════════════════════════════════
	lockKey := "ticket_type:" + req.TicketTypeID
	ctx := context.Background()

	acquired, err := pkg.AcquireLock(ctx, h.redis, lockKey, 10*time.Second)
	if err != nil || !acquired {
		c.JSON(http.StatusServiceUnavailable, errResp("BUSY", "System is busy, please retry"))
		return
	}
	defer pkg.ReleaseLock(ctx, h.redis, lockKey)

	eventID, _ := uuid.Parse(req.EventID)
	ticketTypeID, _ := uuid.Parse(req.TicketTypeID)

	for attempt := 0; attempt < 3; attempt++ {
		var tt model.TicketType
		if err := h.db.First(&tt, "id = ?", ticketTypeID).Error; err != nil {
			c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Ticket type not found"))
			return
		}
		if tt.Remaining < req.Quantity {
			c.JSON(http.StatusConflict, errResp("TICKET_SOLD_OUT", "Not enough tickets remaining"))
			return
		}

		// CAS update with version check
		result := h.db.Model(&model.TicketType{}).
			Where("id = ? AND version = ? AND remaining >= ?", tt.ID, tt.Version, req.Quantity).
			Updates(map[string]interface{}{
				"remaining": gorm.Expr("remaining - ?", req.Quantity),
				"version":   gorm.Expr("version + 1"),
			})
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "DB error"))
			return
		}
		if result.RowsAffected == 0 {
			continue // concurrent update, retry
		}

		app := model.Application{
			UserID:         userID,
			EventID:        eventID,
			TicketTypeID:   ticketTypeID,
			Quantity:       req.Quantity,
			Status:         "pending",
			IdempotencyKey: req.IdempotencyKey,
		}
		if err := h.db.Create(&app).Error; err != nil {
			// Rollback reservation
			h.db.Model(&model.TicketType{}).Where("id = ?", tt.ID).Updates(map[string]interface{}{
				"remaining": gorm.Expr("remaining + ?", req.Quantity),
				"version":   gorm.Expr("version + 1"),
			})
			c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "Failed to create application"))
			return
		}

		c.JSON(http.StatusCreated, okResp(app))
		return
	}

	c.JSON(http.StatusConflict, errResp("CONFLICT", "Too many concurrent requests, please retry"))
}

// ── GET /v1/applications/my ──
func (h *TicketHandler) MyApplications(c *gin.Context) {
	userID := c.GetString("user_id")
	var apps []model.Application
	h.db.Preload("Event").Preload("TicketType").Preload("Tickets").
		Where("user_id = ?", userID).
		Order("applied_at desc").
		Find(&apps)
	c.JSON(http.StatusOK, okResp(apps))
}

// ── GET /v1/applications   (Manager) ──
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

// ── POST /v1/applications/:id/approve ──
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
		if err := tx.Model(&app).Updates(map[string]interface{}{
			"status":      "approved",
			"reviewed_by": reviewerID,
			"reviewed_at": now,
		}).Error; err != nil {
			return err
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

// ── POST /v1/applications/:id/reject ──
type RejectRequest struct {
	Reason string `json:"reason"`
}

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
		if err := tx.Model(&app).Updates(map[string]interface{}{
			"status": "rejected", "reason": req.Reason,
			"reviewed_by": reviewerID, "reviewed_at": now,
		}).Error; err != nil {
			return err
		}
		// Return tickets to pool
		return tx.Model(&model.TicketType{}).Where("id = ?", app.TicketTypeID).
			Updates(map[string]interface{}{
				"remaining": gorm.Expr("remaining + ?", app.Quantity),
				"version":   gorm.Expr("version + 1"),
			}).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "Failed to reject"))
		return
	}
	c.JSON(http.StatusOK, okResp(app))
}

// ── GET /v1/tickets/my ──
func (h *TicketHandler) MyTickets(c *gin.Context) {
	userID := c.GetString("user_id")
	var tickets []model.Ticket
	h.db.Preload("Event").Preload("TicketType").
		Where("user_id = ?", userID).
		Order("issued_at desc").
		Find(&tickets)
	c.JSON(http.StatusOK, okResp(tickets))
}

// ── POST /v1/checkin  (Manager) ──
type CheckinRequest struct {
	QRToken string `json:"qr_token" binding:"required"`
}

func (h *TicketHandler) Checkin(c *gin.Context) {
	var req CheckinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errResp("VALIDATION_ERROR", err.Error()))
		return
	}
	checkerID, _ := uuid.Parse(c.GetString("user_id"))

	// Atomic: mark as used only if NOT already used
	result := h.db.Model(&model.Ticket{}).
		Where("qr_token = ? AND is_used = false", req.QRToken).
		Update("is_used", true)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "DB error"))
		return
	}

	if result.RowsAffected == 0 {
		var count int64
		h.db.Model(&model.Ticket{}).Where("qr_token = ?", req.QRToken).Count(&count)
		if count == 0 {
			c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Ticket not found"))
		} else {
			c.JSON(http.StatusConflict, errResp("ALREADY_CHECKED_IN", "This ticket has already been used"))
		}
		return
	}

	var ticket model.Ticket
	h.db.Preload("Event").Preload("TicketType").Preload("User").First(&ticket, "qr_token = ?", req.QRToken)

	checkin := model.Checkin{TicketID: ticket.ID, CheckedBy: checkerID}
	h.db.Create(&checkin)

	c.JSON(http.StatusOK, okResp(gin.H{
		"message": "✅ Check-in successful!",
		"ticket":  ticket,
	}))
}

// ── GET /v1/checkins ──
func (h *TicketHandler) ListCheckins(c *gin.Context) {
	var checkins []model.Checkin
	q := h.db.Preload("Ticket.Event").Preload("Checker")
	if eid := c.Query("event_id"); eid != "" {
		q = q.Joins("JOIN tickets ON checkins.ticket_id = tickets.id").
			Where("tickets.event_id = ?", eid)
	}
	q.Order("checked_at desc").Limit(100).Find(&checkins)
	c.JSON(http.StatusOK, okResp(checkins))
}
