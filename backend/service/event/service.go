package event

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
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
	ImageURL            string    `json:"image_url"`
	DocumentURL         string    `json:"document_url"`
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
	ctx := context.Background()
	cacheKey := eventListCacheKey(status, role, ticketType, startFrom, startTo)
	cacheTTL := eventListCacheTTL()
	if s.redis != nil && cacheTTL > 0 {
		if raw, err := s.redis.Get(ctx, cacheKey).Result(); err == nil && raw != "" {
			var cached []model.Event
			if err := json.Unmarshal([]byte(raw), &cached); err == nil {
				for i := range cached {
					applyTimeBasedStatus(&cached[i])
				}
				return cached, nil
			}
		}
	}

	events, err := s.events.List(status, role, ticketType, startFrom, startTo)
	if err != nil {
		return nil, apperror.Internal("Failed to list events")
	}

	for i := range events {
		applyTimeBasedStatus(&events[i])
	}

	if s.redis != nil && cacheTTL > 0 {
		if payload, err := json.Marshal(events); err == nil {
			_ = s.redis.Set(ctx, cacheKey, payload, cacheTTL).Err()
		}
	}
	return events, nil
}

func (s *Service) Get(id string) (model.Event, error) {
	ctx := context.Background()
	cacheKey := "event:detail:" + id
	cacheTTL := eventListCacheTTL()

	// 1. 查 Redis
	if s.redis != nil && cacheTTL > 0 {
		if raw, err := s.redis.Get(ctx, cacheKey).Result(); err == nil && raw != "" {
			var cached model.Event
			if err := json.Unmarshal([]byte(raw), &cached); err == nil {
				applyTimeBasedStatus(&cached)
				return cached, nil
			}
		}
	}

	// 2. 查 DB
	event, err := s.events.FindByID(id)
	if err != nil {
		return model.Event{}, apperror.NotFound("Event not found")
	}

	// 3. 寫回 Redis
	if s.redis != nil && cacheTTL > 0 {
		if payload, err := json.Marshal(event); err == nil {
			_ = s.redis.Set(ctx, cacheKey, payload, cacheTTL).Err()
		}
	}

	return event, nil
}

// applyTimeBasedStatus 補償 GORM AfterFind hook 對 Redis 反序列化的 Event 不會觸發的問題
func applyTimeBasedStatus(e *model.Event) {
	now := time.Now()
	if (e.Status == "published" || e.Status == "closed") && !e.EndTime.IsZero() && now.After(e.EndTime) {
		e.Status = "ended"
	} else if e.Status == "published" && !e.ApplyDeadline.IsZero() && now.After(e.ApplyDeadline) {
		e.Status = "closed"
	}
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
		ImageURL:            req.ImageURL,
		DocumentURL:         req.DocumentURL,
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
	s.invalidateEventListCache(context.Background())
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
		event.ImageURL = req.ImageURL
		event.DocumentURL = req.DocumentURL
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
	s.invalidateEventListCache(context.Background())
	s.invalidateEventDetailCache(context.Background(), event.ID.String())
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
	s.invalidateEventListCache(context.Background())
	s.invalidateEventDetailCache(context.Background(), id)
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
	s.invalidateEventListCache(context.Background())
	s.invalidateEventDetailCache(context.Background(), id)
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
	s.invalidateEventListCache(context.Background())
	s.invalidateEventDetailCache(context.Background(), id)
	return event, nil
}

func (s *Service) CheckEligibility(eventID string, userID uuid.UUID) (Eligibility, error) {
	// 改用 s.Get() 以命中 Redis cache，減少 DB 查詢
	event, err := s.Get(eventID)
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

func eventListCacheTTL() time.Duration {
	raw := os.Getenv("EVENT_LIST_CACHE_TTL_SECONDS")
	if raw == "" {
		return 0
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func eventListCacheKey(status string, role string, ticketType string, startFrom string, startTo string) string {
	parts := []string{status, role, ticketType, startFrom, startTo}
	payload, _ := json.Marshal(parts)
	sum := sha1.Sum(payload)
	return "event:list:" + hex.EncodeToString(sum[:])
}

func (s *Service) invalidateEventListCache(ctx context.Context) {
	if s.redis == nil {
		return
	}
	var cursor uint64
	for {
		keys, next, err := s.redis.Scan(ctx, cursor, "event:list:*", 100).Result()
		if err != nil {
			return
		}
		if len(keys) > 0 {
			_ = s.redis.Del(ctx, keys...).Err()
		}
		cursor = next
		if cursor == 0 {
			return
		}
	}
}

func (s *Service) invalidateEventDetailCache(ctx context.Context, eventID string) {
	if s.redis == nil {
		return
	}
	_ = s.redis.Del(ctx, "event:detail:"+eventID).Err()
}
