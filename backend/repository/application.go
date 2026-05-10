package repository

import (
	"ticketing-system/backend/model"

	"gorm.io/gorm"
)

type ApplicationRepository struct {
	db *gorm.DB
}

func (r *ApplicationRepository) ListMy(userID string) ([]model.Application, error) {
	var apps []model.Application
	err := r.db.Preload("Event").Preload("TicketType").Preload("Tickets").
		Where("user_id = ?", userID).
		Order("applied_at desc").
		Find(&apps).Error
	return apps, err
}

func (r *ApplicationRepository) List(eventID string, status string) ([]model.Application, error) {
	var apps []model.Application
	q := r.db.Preload("User").Preload("Event").Preload("TicketType")
	if eventID != "" {
		q = q.Where("event_id = ?", eventID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("applied_at desc").Find(&apps).Error
	return apps, err
}
