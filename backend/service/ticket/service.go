package ticket

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/pkg"
	"ticketing-system/backend/repository"
	"ticketing-system/backend/service/apperror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db           *gorm.DB
	redis        *redis.Client
	applications *repository.ApplicationRepository
	tickets      *repository.TicketRepository
}

func New(repos *repository.Repositories) *Service {
	return &Service{
		db:           repos.DB,
		redis:        repos.Redis,
		applications: repos.Applications,
		tickets:      repos.Tickets,
	}
}

type ApplyRequest struct {
	EventID        string `json:"event_id" binding:"required"`
	TicketTypeID   string `json:"ticket_type_id" binding:"required"`
	Quantity       int    `json:"quantity" binding:"required,min=1"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
}

type RejectRequest struct {
	Reason string `json:"reason"`
}

type CheckinRequest struct {
	QRToken string `json:"qr_token" binding:"required"`
}

type ApplyResult struct {
	Application model.Application
	Created     bool
}

type CheckinResult struct {
	Message string       `json:"message"`
	Ticket  model.Ticket `json:"ticket"`
}

func (s *Service) Apply(userID uuid.UUID, req ApplyRequest) (ApplyResult, error) {
	eventID, err := uuid.Parse(req.EventID)
	if err != nil {
		return ApplyResult{}, apperror.Validation("Invalid event_id")
	}
	ticketTypeID, err := uuid.Parse(req.TicketTypeID)
	if err != nil {
		return ApplyResult{}, apperror.Validation("Invalid ticket_type_id")
	}
	if s.redis == nil {
		return ApplyResult{}, apperror.Busy("Inventory system busy")
	}

	var existing model.Application
	if err := s.db.Where("idempotency_key = ? AND user_id = ?", req.IdempotencyKey, userID.String()).First(&existing).Error; err == nil {
		return ApplyResult{Application: existing, Created: false}, nil
	}

	inventoryKey := "inventory:" + req.TicketTypeID
	loadedKey := "inventory_loaded:" + req.TicketTypeID
	ctx := context.Background()

	if s.redis.Exists(ctx, loadedKey).Val() == 0 {
		lockKey := "init_lock:" + req.TicketTypeID
		if ok, _ := pkg.AcquireLock(ctx, s.redis, lockKey, 5*time.Second, 60*time.Second); ok {
			if s.redis.Exists(ctx, loadedKey).Val() == 0 {
				var ticketType model.TicketType
				if err := s.db.First(&ticketType, "id = ?", ticketTypeID).Error; err != nil {
					pkg.ReleaseLock(ctx, s.redis, lockKey)
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return ApplyResult{}, apperror.NotFound("Ticket type not found")
					}
					return ApplyResult{}, apperror.Busy("Database system busy")
				}
				pipe := s.redis.TxPipeline()
				pipe.Set(ctx, inventoryKey, ticketType.Remaining, 24*time.Hour)
				pipe.Set(ctx, loadedKey, "1", 24*time.Hour)
				if _, err := pipe.Exec(ctx); err != nil {
					log.Printf("Redis init error: %v", err)
				}
			}
			pkg.ReleaseLock(ctx, s.redis, lockKey)
		}
	}

	newStock, err := s.redis.DecrBy(ctx, inventoryKey, int64(req.Quantity)).Result()
	if err != nil {
		return ApplyResult{}, apperror.New(503, "BUSY", "Inventory system busy")
	}
	if newStock < 0 {
		s.redis.IncrBy(ctx, inventoryKey, int64(req.Quantity))
		for attempt := 0; attempt < 3; attempt++ {
			var existing model.Application
			if err := s.db.Where("idempotency_key = ? AND user_id = ?", req.IdempotencyKey, userID.String()).First(&existing).Error; err == nil {
				return ApplyResult{Application: existing, Created: false}, nil
			}
			time.Sleep(50 * time.Millisecond)
		}
		return ApplyResult{}, apperror.Conflict("TICKET_SOLD_OUT", "Not enough tickets remaining")
	}

	var createdApp model.Application
	var existingApp model.Application
	created := false
	success := false
	defer func() {
		if !success {
			s.redis.IncrBy(ctx, inventoryKey, int64(req.Quantity))
		}
	}()

	for attempt := 0; attempt < 5; attempt++ {
		created = false
		createdApp = model.Application{}
		existingApp = model.Application{}

		err = s.db.Transaction(func(tx *gorm.DB) error {
			lockName := fmt.Sprintf("apply:%s:%s", userID.String(), eventID.String())
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockName).Error; err != nil {
				return err
			}

			if err := tx.Where("idempotency_key = ? AND user_id = ?", req.IdempotencyKey, userID.String()).First(&existingApp).Error; err == nil {
				return nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			var event model.Event
			if err := tx.First(&event, "id = ?", eventID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return apperror.NotFound("Event not found")
				}
				return err
			}
			if event.Status != "published" {
				return apperror.New(400, "EVENT_NOT_AVAILABLE", "Event is not accepting applications")
			}
			if time.Now().After(event.ApplyDeadline) {
				return apperror.New(400, "APPLY_DEADLINE_PASSED", "Application deadline has passed")
			}

			if event.RegionRestriction != nil && *event.RegionRestriction != "" {
				var user model.User
				if err := tx.First(&user, "id = ?", userID.String()).Error; err != nil {
					return err
				}
				if user.Region != *event.RegionRestriction {
					return apperror.Forbidden("NOT_ELIGIBLE", "You are not eligible due to region restriction (restricted to "+*event.RegionRestriction+")")
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
				return apperror.New(400, "EXCEEDS_MAX_TICKETS", fmt.Sprintf("You have already applied for %d tickets. The limit is %d. You can only apply for %d more.", currentTotal, event.MaxTicketsPerPerson, remainingAllowance))
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
				return apperror.Conflict("TICKET_SOLD_OUT", "Not enough tickets remaining")
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
		}
		if !isRetryableApplyError(err) {
			break
		}
		log.Printf("DB conflict, retrying... (attempt %d/5): %v", attempt+1, err)
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}

	if err != nil {
		return ApplyResult{}, err
	}
	if !created {
		return ApplyResult{Application: existingApp, Created: false}, nil
	}
	return ApplyResult{Application: createdApp, Created: true}, nil
}

func (s *Service) MyApplications(userID string) ([]model.Application, error) {
	apps, err := s.applications.ListMy(userID)
	if err != nil {
		return nil, apperror.Internal("Failed to list applications")
	}
	return apps, nil
}

func (s *Service) ListApplications(eventID string, status string) ([]model.Application, error) {
	apps, err := s.applications.List(eventID, status)
	if err != nil {
		return nil, apperror.Internal("Failed to list applications")
	}
	return apps, nil
}

func (s *Service) ApproveApplication(applicationID string, reviewerID uuid.UUID) (model.Application, error) {
	var app model.Application
	if err := s.db.Preload("Event").First(&app, "id = ?", applicationID).Error; err != nil {
		return model.Application{}, apperror.NotFound("Application not found")
	}
	if app.Status != "pending" {
		return model.Application{}, apperror.New(400, "INVALID_STATUS", "Application is not pending")
	}

	now := time.Now()
	if err := s.db.Transaction(func(tx *gorm.DB) error {
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
			return apperror.Conflict("ALREADY_PROCESSED", "Application has already been approved or rejected")
		}

		for i := 0; i < app.Quantity; i++ {
			ticket := model.Ticket{
				ApplicationID: app.ID,
				UserID:        app.UserID,
				EventID:       app.EventID,
				TicketTypeID:  app.TicketTypeID,
				QRToken:       uuid.New().String(),
				ExpiresAt:     app.Event.EndTime,
			}
			if err := tx.Create(&ticket).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return model.Application{}, err
	}

	if err := s.db.Preload("Tickets").First(&app, app.ID).Error; err != nil {
		return model.Application{}, apperror.Internal("Failed to load application")
	}
	return app, nil
}

func (s *Service) RejectApplication(applicationID string, reviewerID uuid.UUID, req RejectRequest) (model.Application, error) {
	var app model.Application
	if err := s.db.First(&app, "id = ?", applicationID).Error; err != nil {
		return model.Application{}, apperror.NotFound("Application not found")
	}
	if app.Status != "pending" {
		return model.Application{}, apperror.New(400, "INVALID_STATUS", "Application is not pending")
	}

	now := time.Now()
	if err := s.db.Transaction(func(tx *gorm.DB) error {
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
			return apperror.Conflict("ALREADY_PROCESSED", "Application has already been approved or rejected")
		}
		if err := tx.Model(&model.TicketType{}).Where("id = ?", app.TicketTypeID).
			Update("remaining", gorm.Expr("remaining + ?", app.Quantity)).Error; err != nil {
			return err
		}
		s.returnInventory(app.TicketTypeID.String(), app.Quantity)
		return nil
	}); err != nil {
		return model.Application{}, err
	}
	return app, nil
}

func (s *Service) MyTickets(userID string) ([]model.Ticket, error) {
	tickets, err := s.tickets.ListMy(userID)
	if err != nil {
		return nil, apperror.Internal("Failed to list tickets")
	}
	return tickets, nil
}

func (s *Service) CancelApplication(applicationID string, userID uuid.UUID) error {
	var app model.Application
	if err := s.db.First(&app, "id = ? AND user_id = ?", applicationID, userID.String()).Error; err != nil {
		return apperror.NotFound("Application not found")
	}
	if app.Status == "cancelled" || app.Status == "rejected" {
		return apperror.Conflict("INVALID_STATUS", "Application is already cancelled or rejected")
	}
	if app.Status == "approved" {
		var usedCount int64
		s.db.Model(&model.Ticket{}).Where("application_id = ? AND is_used = true", app.ID).Count(&usedCount)
		if usedCount > 0 {
			return apperror.Conflict("TICKETS_ALREADY_USED", "Cannot cancel application because some tickets have already been used. Please return unused tickets individually.")
		}
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
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
		s.returnInventory(app.TicketTypeID.String(), app.Quantity)
		return nil
	})
	if err != nil {
		return apperror.Internal("Failed to cancel application")
	}
	return nil
}

func (s *Service) CancelTicket(ticketID string, userID uuid.UUID) error {
	var ticket model.Ticket
	if err := s.db.Preload("Application").First(&ticket, "id = ? AND user_id = ?", ticketID, userID.String()).Error; err != nil {
		return apperror.NotFound("Ticket not found")
	}
	if ticket.IsUsed {
		return apperror.Conflict("ALREADY_USED", "Cannot return a ticket that has already been used")
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
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
			Reason:         pkg.Ptr("Returned ticket " + ticket.ID.String()[:8]),
		}
		if err := tx.Create(&refundApp).Error; err != nil {
			return err
		}
		if err := tx.Delete(&ticket).Error; err != nil {
			return err
		}
		s.returnInventory(ticket.TicketTypeID.String(), 1)
		return nil
	})
	if err != nil {
		return apperror.Internal("Failed to return ticket")
	}
	return nil
}

func (s *Service) Checkin(req CheckinRequest, checkerID uuid.UUID) (CheckinResult, error) {
	var ticket model.Ticket
	now := time.Now()

	err := s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Ticket{}).
			Where("qr_token = ? AND is_used = false AND expires_at > ?", req.QRToken, now).
			Update("is_used", true)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			if err := tx.Where("qr_token = ?", req.QRToken).First(&ticket).Error; err != nil {
				return apperror.NotFound("Ticket not found")
			}
			if ticket.IsUsed {
				return apperror.Conflict("ALREADY_CHECKED_IN", "This ticket has already been used")
			}
			if now.After(ticket.ExpiresAt) {
				return apperror.New(410, "TICKET_EXPIRED", "This ticket has expired")
			}
			return apperror.New(400, "INVALID_REQUEST", "Ticket is invalid")
		}

		if err := tx.Preload("Event").Preload("TicketType").Preload("User").
			First(&ticket, "qr_token = ?", req.QRToken).Error; err != nil {
			return err
		}
		checkin := model.Checkin{TicketID: ticket.ID, CheckedBy: checkerID}
		if err := tx.Create(&checkin).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return CheckinResult{}, err
	}
	return CheckinResult{Message: "Check-in successful!", Ticket: ticket}, nil
}

func (s *Service) ListCheckins(eventID string) ([]model.Checkin, error) {
	checkins, err := s.tickets.ListCheckins(eventID)
	if err != nil {
		return nil, apperror.Internal("Failed to list check-ins")
	}
	return checkins, nil
}

func (s *Service) returnInventory(ticketTypeID string, quantity int) {
	if s.redis == nil {
		return
	}
	s.redis.IncrBy(context.Background(), "inventory:"+ticketTypeID, int64(quantity))
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
