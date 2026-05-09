package report

import (
	"encoding/csv"
	"fmt"
	"strings"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	"ticketing-system/backend/service/apperror"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
}

func New(repos *repository.Repositories) *Service {
	return &Service{db: repos.DB}
}

type DeptStat struct {
	Department string `json:"department"`
	Count      int64  `json:"count"`
}

type RegionStat struct {
	Region string `json:"region"`
	Count  int64  `json:"count"`
}

type TypeStat struct {
	TicketTypeName string `json:"ticket_type_name"`
	Total          int64  `json:"total"`
	Approved       int64  `json:"approved"`
	Cancelled      int64  `json:"cancelled"`
	Active         int64  `json:"active"`
}

type EventOverview struct {
	EventID         string  `json:"event_id"`
	Title           string  `json:"title"`
	AppliedApps     int64   `json:"applied_apps"`
	AppliedTickets  int64   `json:"applied_tickets"`
	AppliedUsers    int64   `json:"applied_users"`
	ApprovedTickets int64   `json:"approved_tickets"`
	ApprovedUsers   int64   `json:"approved_users"`
	Cancelled       int64   `json:"cancelled"`
	TotalTickets    int64   `json:"total_tickets"`
	CheckedIn       int64   `json:"checked_in"`
	CheckInRate     float64 `json:"check_in_rate"`
}

func (s *Service) EventStats(eventID string) (gin.H, error) {
	var event model.Event
	if err := s.db.First(&event, "id = ?", eventID).Error; err != nil {
		return nil, apperror.NotFound("Event not found")
	}

	var appliedApps, appliedTickets, appliedUsers int64
	s.db.Model(&model.Application{}).Where("event_id = ?", eventID).Count(&appliedApps)
	s.db.Model(&model.Application{}).Where("event_id = ?", eventID).Select("COALESCE(SUM(quantity), 0)").Scan(&appliedTickets)
	s.db.Model(&model.Application{}).Where("event_id = ?", eventID).Distinct("user_id").Count(&appliedUsers)

	var approvedApps, approvedTickets, approvedUsers int64
	s.db.Model(&model.Application{}).Where("event_id = ? AND status = 'approved'", eventID).Count(&approvedApps)
	s.db.Model(&model.Application{}).Where("event_id = ? AND status = 'approved'", eventID).Select("COALESCE(SUM(quantity), 0)").Scan(&approvedTickets)
	s.db.Model(&model.Application{}).Where("event_id = ? AND status = 'approved'", eventID).Distinct("user_id").Count(&approvedUsers)

	var cancelledTickets int64
	s.db.Model(&model.Application{}).Where("event_id = ? AND status = 'cancelled'", eventID).Select("COALESCE(SUM(quantity), 0)").Scan(&cancelledTickets)

	var totalTickets, checkedInTickets, checkedInUsers int64
	s.db.Model(&model.Ticket{}).Where("event_id = ?", eventID).Count(&totalTickets)
	s.db.Model(&model.Checkin{}).
		Joins("JOIN tickets ON checkins.ticket_id = tickets.id").
		Where("tickets.event_id = ?", eventID).Count(&checkedInTickets)
	s.db.Model(&model.Checkin{}).
		Joins("JOIN tickets ON checkins.ticket_id = tickets.id").
		Where("tickets.event_id = ?", eventID).Distinct("tickets.user_id").Count(&checkedInUsers)

	checkInRate := 0.0
	if totalTickets > 0 {
		checkInRate = float64(checkedInTickets) / float64(totalTickets) * 100
	}

	var deptStats []DeptStat
	s.db.Model(&model.Application{}).
		Select("users.department, COUNT(DISTINCT users.id) as count").
		Joins("JOIN users ON applications.user_id = users.id").
		Where("applications.event_id = ? AND applications.status = 'approved'", eventID).
		Group("users.department").Scan(&deptStats)

	var regionStats []RegionStat
	s.db.Model(&model.Application{}).
		Select("users.region, COUNT(DISTINCT users.id) as count").
		Joins("JOIN users ON applications.user_id = users.id").
		Where("applications.event_id = ? AND applications.status = 'approved'", eventID).
		Group("users.region").Scan(&regionStats)

	var typeStats []TypeStat
	s.db.Model(&model.TicketType{}).
		Select(`
			ticket_types.name as ticket_type_name, 
			(SELECT COALESCE(SUM(quantity), 0) FROM applications WHERE applications.ticket_type_id = ticket_types.id AND applications.event_id = ?) as total,
			(SELECT COALESCE(SUM(quantity), 0) FROM applications WHERE applications.ticket_type_id = ticket_types.id AND applications.status = 'approved' AND applications.event_id = ?) as approved,
			(SELECT COALESCE(SUM(quantity), 0) FROM applications WHERE applications.ticket_type_id = ticket_types.id AND applications.status = 'cancelled' AND applications.event_id = ?) as cancelled,
			(SELECT COUNT(*) FROM tickets WHERE tickets.ticket_type_id = ticket_types.id AND tickets.event_id = ?) as active
		`, eventID, eventID, eventID, eventID).
		Where("ticket_types.event_id = ?", eventID).
		Scan(&typeStats)

	return gin.H{
		"event":              event,
		"applied_apps":       appliedApps,
		"applied_tickets":    appliedTickets,
		"applied_users":      appliedUsers,
		"approved_apps":      approvedApps,
		"approved_tickets":   approvedTickets,
		"cancelled_tickets":  cancelledTickets,
		"approved_users":     approvedUsers,
		"total_tickets":      totalTickets,
		"checked_in_tickets": checkedInTickets,
		"checked_in_users":   checkedInUsers,
		"check_in_rate":      checkInRate,
		"by_department":      deptStats,
		"by_region":          regionStats,
		"by_ticket_type":     typeStats,
	}, nil
}

func (s *Service) Overview() ([]EventOverview, error) {
	var overview []EventOverview
	err := s.db.Model(&model.Event{}).
		Select(`
			events.id as event_id, 
			events.title, 
			(SELECT COUNT(*) FROM applications WHERE applications.event_id = events.id) as applied_apps,
			(SELECT COALESCE(SUM(quantity), 0) FROM applications WHERE applications.event_id = events.id) as applied_tickets,
			(SELECT COUNT(DISTINCT user_id) FROM applications WHERE applications.event_id = events.id) as applied_users,
			(SELECT COALESCE(SUM(quantity), 0) FROM applications WHERE applications.event_id = events.id AND applications.status = 'approved') as approved_tickets,
			(SELECT COUNT(DISTINCT user_id) FROM applications WHERE applications.event_id = events.id AND applications.status = 'approved') as approved_users,
			(SELECT COALESCE(SUM(quantity), 0) FROM applications WHERE applications.event_id = events.id AND applications.status = 'cancelled') as cancelled,
			(SELECT COUNT(*) FROM tickets WHERE tickets.event_id = events.id) as total_tickets,
			(SELECT COUNT(*) FROM checkins JOIN tickets ON checkins.ticket_id = tickets.id WHERE tickets.event_id = events.id) as checked_in,
			COALESCE(
				(SELECT COUNT(*) FROM checkins JOIN tickets ON checkins.ticket_id = tickets.id WHERE tickets.event_id = events.id)::float / 
				NULLIF((SELECT COUNT(*) FROM tickets WHERE tickets.event_id = events.id), 0)::float * 100, 
				0
			) as check_in_rate
		`).
		Order("events.start_time DESC").
		Scan(&overview).Error
	if err != nil {
		return nil, apperror.Internal("Failed to load report overview")
	}
	return overview, nil
}

func (s *Service) ExportEventCSV(eventID string) (string, error) {
	stats, err := s.EventStats(eventID)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	writer := csv.NewWriter(&out)
	_ = writer.Write([]string{"metric", "value"})
	for _, key := range []string{
		"applied_apps",
		"applied_tickets",
		"applied_users",
		"approved_apps",
		"approved_tickets",
		"approved_users",
		"cancelled_tickets",
		"total_tickets",
		"checked_in_tickets",
		"checked_in_users",
		"check_in_rate",
	} {
		_ = writer.Write([]string{key, fmt.Sprint(stats[key])})
	}
	writer.Flush()
	return out.String(), nil
}
