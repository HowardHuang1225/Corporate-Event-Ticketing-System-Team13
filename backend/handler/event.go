package handler

import (
	"context"
	"net/http"
	"time"

	"ticketing-system/backend/model"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EventHandler struct {
	db    *gorm.DB
	redis *redis.Client
}

func NewEventHandler(db *gorm.DB, redis *redis.Client) *EventHandler {
	return &EventHandler{db: db, redis: redis}
}

type CreateEventRequest struct {
	Title               string    `json:"title" binding:"required"`
	Description         string    `json:"description"`
	Venue               string    `json:"venue" binding:"required"`
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

func (h *EventHandler) ListEvents(c *gin.Context) {
	var events []model.Event
	q := h.db.Preload("TicketTypes").Preload("Creator")
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	// Employees only see published/closed
	if c.GetString("role") == "employee" {
		q = q.Where("status IN ('published','closed')")
	}
	q.Order("created_at desc").Find(&events)
	c.JSON(http.StatusOK, okResp(events))
}

func (h *EventHandler) GetEvent(c *gin.Context) {
	var event model.Event
	if err := h.db.Preload("TicketTypes").Preload("Creator").First(&event, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Event not found"))
		return
	}
	c.JSON(http.StatusOK, okResp(event))
}

func (h *EventHandler) CreateEvent(c *gin.Context) {
	var req CreateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errResp("VALIDATION_ERROR", err.Error()))
		return
	}

	creatorID, _ := uuid.Parse(c.GetString("user_id"))
	max := req.MaxTicketsPerPerson
	if max == 0 {
		max = 1
	}

	event := model.Event{
		Title:               req.Title,
		Description:         req.Description,
		Venue:               req.Venue,
		StartTime:           req.StartTime,
		EndTime:             req.EndTime,
		ApplyDeadline:       req.ApplyDeadline,
		Status:              "draft",
		RegionRestriction:   req.RegionRestriction,
		MaxTicketsPerPerson: max,
		CreatedBy:           creatorID,
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		for _, tt := range req.TicketTypes {
			t := model.TicketType{
				EventID:    event.ID,
				Name:       tt.Name,
				TotalQuota: tt.TotalQuota,
				Remaining:  tt.TotalQuota,
			}
			if err := tx.Create(&t).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "Failed to create event"))
		return
	}

	h.db.Preload("TicketTypes").First(&event, event.ID)
	c.JSON(http.StatusCreated, okResp(event))
}

func (h *EventHandler) UpdateEvent(c *gin.Context) {
	var event model.Event
	if err := h.db.First(&event, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Event not found"))
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, errResp("VALIDATION_ERROR", err.Error()))
		return
	}
	// Remove protected fields
	delete(updates, "id")
	delete(updates, "created_by")
	delete(updates, "status")
	h.db.Model(&event).Updates(updates)

	// Invalidate event cache if needed
	h.invalidateEventCache(c.Request.Context(), event.ID.String())

	c.JSON(http.StatusOK, okResp(event))
}

func (h *EventHandler) invalidateEventCache(ctx context.Context, eventID string) {
	// Find all ticket types for this event and clear their loaded flag
	var ttIDs []string
	h.db.Model(&model.TicketType{}).Where("event_id = ?", eventID).Pluck("id", &ttIDs)
	for _, id := range ttIDs {
		h.redis.Del(ctx, "inventory_loaded:"+id)
	}
}

func (h *EventHandler) PublishEvent(c *gin.Context) {
	var event model.Event
	if err := h.db.First(&event, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Event not found"))
		return
	}
	if event.Status != "draft" {
		c.JSON(http.StatusBadRequest, errResp("INVALID_STATUS", "Only draft events can be published"))
		return
	}
	h.db.Model(&event).Update("status", "published")
	c.JSON(http.StatusOK, okResp(event))
}

func (h *EventHandler) CloseEvent(c *gin.Context) {
	var event model.Event
	if err := h.db.First(&event, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Event not found"))
		return
	}
	h.db.Model(&event).Update("status", "closed")
	c.JSON(http.StatusOK, okResp(event))
}
