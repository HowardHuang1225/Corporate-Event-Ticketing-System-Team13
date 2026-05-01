package handler

import (
	"net/http"

	"ticketing-system/backend/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ReportHandler struct{ db *gorm.DB }

func NewReportHandler(db *gorm.DB) *ReportHandler { return &ReportHandler{db: db} }

type DeptStat struct {
	Department string `json:"department"`
	Count      int64  `json:"count"`
}
type TypeStat struct {
	TicketTypeName string `json:"ticket_type_name"`
	Total          int64  `json:"total"`
	Approved       int64  `json:"approved"`
}

func (h *ReportHandler) EventStats(c *gin.Context) {
	eventID := c.Param("id")

	var event model.Event
	if err := h.db.First(&event, "id = ?", eventID).Error; err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "Event not found"))
		return
	}

	var totalApplied, totalApproved, totalCheckedIn int64
	h.db.Model(&model.Application{}).Where("event_id = ?", eventID).Count(&totalApplied)
	h.db.Model(&model.Application{}).Where("event_id = ? AND status = 'approved'", eventID).Count(&totalApproved)
	h.db.Model(&model.Checkin{}).
		Joins("JOIN tickets ON checkins.ticket_id = tickets.id").
		Where("tickets.event_id = ?", eventID).Count(&totalCheckedIn)

	var deptStats []DeptStat
	h.db.Model(&model.Application{}).
		Select("users.department, COUNT(*) as count").
		Joins("JOIN users ON applications.user_id = users.id").
		Where("applications.event_id = ? AND applications.status = 'approved'", eventID).
		Group("users.department").Scan(&deptStats)

	var typeStats []TypeStat
	h.db.Model(&model.Application{}).
		Select("ticket_types.name as ticket_type_name, COUNT(*) as total, SUM(CASE WHEN applications.status = 'approved' THEN 1 ELSE 0 END) as approved").
		Joins("JOIN ticket_types ON applications.ticket_type_id = ticket_types.id").
		Where("applications.event_id = ?", eventID).
		Group("ticket_types.id, ticket_types.name").Scan(&typeStats)

	c.JSON(http.StatusOK, okResp(gin.H{
		"event":            event,
		"total_applied":    totalApplied,
		"total_approved":   totalApproved,
		"total_checked_in": totalCheckedIn,
		"by_department":    deptStats,
		"by_ticket_type":   typeStats,
	}))
}

func (h *ReportHandler) AllEventsOverview(c *gin.Context) {
	type EventOverview struct {
		EventID   string `json:"event_id"`
		Title     string `json:"title"`
		Applied   int64  `json:"applied"`
		Approved  int64  `json:"approved"`
		CheckedIn int64  `json:"checked_in"`
	}
	var overview []EventOverview
	h.db.Model(&model.Event{}).
		Select(`
			events.id as event_id, 
			events.title, 
			(SELECT COUNT(*) FROM applications WHERE applications.event_id = events.id) as applied,
			(SELECT COUNT(*) FROM applications WHERE applications.event_id = events.id AND applications.status = 'approved') as approved,
			(SELECT COUNT(*) FROM checkins JOIN tickets ON checkins.ticket_id = tickets.id WHERE tickets.event_id = events.id) as checked_in
		`).
		Order("events.start_time DESC").
		Scan(&overview)
	c.JSON(http.StatusOK, okResp(overview))
}
