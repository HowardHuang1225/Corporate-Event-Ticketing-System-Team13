package repository

import (
	"time"
	"ticketing-system/backend/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EventRepository struct {
	db *gorm.DB
}

func (r *EventRepository) List(status string, role string, ticketType string, startFrom string, startTo string) ([]model.Event, error) {
	var events []model.Event
	q := r.db.Preload("TicketTypes").Preload("Creator")
	if status != "" {
		if status == "ended" {
			q = q.Where("status = 'ended' OR (status IN ('published','closed') AND end_time <= ?)", time.Now())
		} else if status == "published" || status == "closed" {
			q = q.Where("status = ? AND end_time > ?", status, time.Now())
		} else {
			q = q.Where("status = ?", status)
		}
	}
	if role != "event_manager" {
		q = q.Where("status IN ('published','closed')").Where("end_time > ?", time.Now())
	}
	if ticketType != "" {
		q = q.Where("events.id IN (SELECT event_id FROM ticket_types WHERE name = ?)", ticketType)
	}
	if startFrom != "" {
		if t, err := time.Parse(time.RFC3339, startFrom); err == nil {
			q = q.Where("start_time >= ?", t)
		}
	}
	if startTo != "" {
		if t, err := time.Parse(time.RFC3339, startTo); err == nil {
			q = q.Where("start_time <= ?", t)
		}
	}
	err := q.Order("created_at desc").Find(&events).Error
	return events, err
}

func (r *EventRepository) FindByID(id string) (model.Event, error) {
	var event model.Event
	err := r.db.Preload("TicketTypes").Preload("Creator").First(&event, "id = ?", id).Error
	return event, err
}

func (r *EventRepository) TicketTypeIDs(eventID uuid.UUID) ([]string, error) {
	var ids []string
	err := r.db.Model(&model.TicketType{}).Where("event_id = ?", eventID).Pluck("id", &ids).Error
	return ids, err
}
