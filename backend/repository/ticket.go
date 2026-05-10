package repository

import (
	"ticketing-system/backend/model"

	"gorm.io/gorm"
)

type TicketRepository struct {
	db *gorm.DB
}

func (r *TicketRepository) ListMy(userID string) ([]model.Ticket, error) {
	var tickets []model.Ticket
	err := r.db.Preload("Event").Preload("TicketType").
		Where("user_id = ?", userID).
		Order("issued_at desc").
		Find(&tickets).Error
	return tickets, err
}

func (r *TicketRepository) ListCheckins(eventID string) ([]model.Checkin, error) {
	var checkins []model.Checkin
	q := r.db.Preload("Ticket.Event").Preload("Ticket.TicketType").Preload("Ticket.User").Preload("Checker")
	if eventID != "" {
		q = q.Joins("JOIN tickets ON checkins.ticket_id = tickets.id").
			Where("tickets.event_id = ?", eventID)
	}
	err := q.Order("checked_at desc").Limit(100).Find(&checkins).Error
	return checkins, err
}
