package event

import (
	"context"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	"ticketing-system/backend/service/apperror"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Service struct {
	db     *gorm.DB
	redis  *redis.Client
	events *repository.EventRepository
	users  *repository.UserRepository
}

func New(repos *repository.Repositories) *Service {
	return &Service{
		db:     repos.DB,
		redis:  repos.Redis,
		events: repos.Events,
		users:  repos.Users,
	}
}

type CreateRequest struct {
	Title               string    `json:"title" binding:"required"`
	Description         string    `json:"description"`
	Venue               string    `json:"venue" binding:"required"`
	PublishTime         time.Time `json:"publish_time" binding:"required"`
	StartTime           time.Time `json:"start_time" binding:"required"`
	EndTime             time.Time `json:"end_time" binding:"required"`
	ApplyDeadline       time.Time `json:"apply_deadline" binding:"required"`
	RegionRestriction   *string   `json:"region_restriction"`
	MaxTicketsPerPerson int       `json:"max_tickets_per_person"`
	TicketTypes         []struct {
		Name       string `json:"name" binding:"required"`
		TotalQuota int    `json:"total_quota" binding:"required,min=1"`
	} `json:"ticket_types" binding:"required,min=1"`
}

type Eligibility struct {
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason,omitempty"`
}

func (s *Service) List(status string, role string, ticketType string, startFrom string, startTo string) ([]model.Event, error) {
	events, err := s.events.List(status, role, ticketType, startFrom, startTo)
	if err != nil {
		return nil, apperror.Internal("Failed to list events")
	}
	return events, nil
}

func (s *Service) Get(id string) (model.Event, error) {
	event, err := s.events.FindByID(id)
	if err != nil {
		return model.Event{}, apperror.NotFound("Event not found")
	}
	return event, nil
}

func (s *Service) Create(req CreateRequest, creatorID uuid.UUID) (model.Event, error) {
	if err := validateTimeline(req.PublishTime, req.StartTime, req.ApplyDeadline, req.EndTime); err != nil {
		return model.Event{}, err
	}

	maxTickets := req.MaxTicketsPerPerson
	if maxTickets == 0 {
		maxTickets = 1
	}

	event := model.Event{
		Title:               req.Title,
		Description:         req.Description,
		Venue:               req.Venue,
		PublishTime:         req.PublishTime,
		StartTime:           req.StartTime,
		EndTime:             req.EndTime,
		ApplyDeadline:       req.ApplyDeadline,
		Status:              "draft",
		RegionRestriction:   req.RegionRestriction,
		MaxTicketsPerPerson: maxTickets,
		CreatedBy:           creatorID,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		for _, ticketType := range req.TicketTypes {
			record := model.TicketType{
				EventID:    event.ID,
				Name:       ticketType.Name,
				TotalQuota: ticketType.TotalQuota,
				Remaining:  ticketType.TotalQuota,
			}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Event{}, apperror.Internal("Failed to create event")
	}

	if err := s.db.Preload("TicketTypes").First(&event, event.ID).Error; err != nil {
		return model.Event{}, apperror.Internal("Failed to load created event")
	}
	return event, nil
}

func (s *Service) UpdateDraft(id string, req CreateRequest) (model.Event, error) {
	if err := validateTimeline(req.PublishTime, req.StartTime, req.ApplyDeadline, req.EndTime); err != nil {
		return model.Event{}, err
	}

	maxTickets := req.MaxTicketsPerPerson
	if maxTickets == 0 {
		maxTickets = 1
	}

	var event model.Event
	if err := s.db.Preload("TicketTypes").First(&event, "id = ?", id).Error; err != nil {
		return model.Event{}, apperror.NotFound("Event not found")
	}

	if event.Status != "draft" {
		return model.Event{}, apperror.New(400, "INVALID_STATUS", "Only draft events can be edited")
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Update Event
		event.Title = req.Title
		event.Description = req.Description
		event.Venue = req.Venue
		event.PublishTime = req.PublishTime
		event.StartTime = req.StartTime
		event.EndTime = req.EndTime
		event.ApplyDeadline = req.ApplyDeadline
		event.RegionRestriction = req.RegionRestriction
		event.MaxTicketsPerPerson = maxTickets
		if err := tx.Save(&event).Error; err != nil {
			return err
		}

		// Update TicketTypes: delete existing and create new ones
		if err := tx.Where("event_id = ?", event.ID).Delete(&model.TicketType{}).Error; err != nil {
			return err
		}

		for _, ticketType := range req.TicketTypes {
			record := model.TicketType{
				EventID:    event.ID,
				Name:       ticketType.Name,
				TotalQuota: ticketType.TotalQuota,
				Remaining:  ticketType.TotalQuota,
			}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return model.Event{}, apperror.Internal("Failed to update event")
	}

	if err := s.db.Preload("TicketTypes").First(&event, event.ID).Error; err != nil {
		return model.Event{}, apperror.Internal("Failed to load updated event")
	}
	s.invalidateInventoryCache(context.Background(), event.ID)
	return event, nil
}

func (s *Service) DeleteDraft(id string) error {
	var event model.Event
	if err := s.db.First(&event, "id = ?", id).Error; err != nil {
		return apperror.NotFound("Event not found")
	}
	if event.Status != "draft" {
		return apperror.New(400, "INVALID_STATUS", "Only draft events can be deleted")
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("event_id = ?", event.ID).Delete(&model.TicketType{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&event).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return apperror.Internal("Failed to delete event")
	}
	return nil
}

func (s *Service) Publish(id string) (model.Event, error) {
	var event model.Event
	if err := s.db.First(&event, "id = ?", id).Error; err != nil {
		return model.Event{}, apperror.NotFound("Event not found")
	}
	if event.Status != "draft" {
		return model.Event{}, apperror.New(400, "INVALID_STATUS", "Only draft events can be published")
	}
	if err := s.db.Model(&event).Update("status", "published").Error; err != nil {
		return model.Event{}, apperror.Internal("Failed to publish event")
	}
	event.Status = "published"
	return event, nil
}

func (s *Service) Close(id string) (model.Event, error) {
	var event model.Event
	if err := s.db.First(&event, "id = ?", id).Error; err != nil {
		return model.Event{}, apperror.NotFound("Event not found")
	}
	if err := s.db.Model(&event).Update("status", "closed").Error; err != nil {
		return model.Event{}, apperror.Internal("Failed to close event")
	}
	event.Status = "closed"
	return event, nil
}

func (s *Service) CheckEligibility(eventID string, userID uuid.UUID) (Eligibility, error) {
	event, err := s.events.FindByID(eventID)
	if err != nil {
		return Eligibility{}, apperror.NotFound("Event not found")
	}
	if event.Status != "published" {
		return Eligibility{Eligible: false, Reason: "Event is not accepting applications"}, nil
	}
	if time.Now().After(event.ApplyDeadline) {
		return Eligibility{Eligible: false, Reason: "Application deadline has passed"}, nil
	}
	return Eligibility{Eligible: true}, nil
}

func StatusAt(event model.Event, now time.Time) string {
	if event.Status == "published" && !event.ApplyDeadline.IsZero() && now.After(event.ApplyDeadline) {
		return "closed"
	}
	if event.Status == "closed" && !event.EndTime.IsZero() && now.After(event.EndTime) {
		return "ended"
	}
	return event.Status
}

func validateTimeline(publishTime, startTime, applyDeadline, endTime time.Time) error {
	if publishTime.IsZero() || startTime.IsZero() || applyDeadline.IsZero() || endTime.IsZero() {
		return apperror.Validation("publish_time, start_time, apply_deadline, and end_time are required")
	}
	if publishTime.After(startTime) || publishTime.After(applyDeadline) || startTime.After(endTime) || applyDeadline.After(endTime) {
		return apperror.Validation("publish_time <= (start_time, apply_deadline) <= end_time")
	}
	return nil
}

func (s *Service) invalidateInventoryCache(ctx context.Context, eventID uuid.UUID) {
	if s.redis == nil {
		return
	}
	ids, err := s.events.TicketTypeIDs(eventID)
	if err != nil {
		return
	}
	for _, id := range ids {
		s.redis.Del(ctx, "inventory_loaded:"+id)
	}
}
