package repository

import (
	"ticketing-system/backend/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EventRepository struct {
	db *gorm.DB
}

func (r *EventRepository) List(status string, role string) ([]model.Event, error) {
	var events []model.Event
	q := r.db.Preload("TicketTypes").Preload("Creator")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if role == "employee" {
		q = q.Where("status IN ('published','closed')")
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
