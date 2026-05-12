package manager

import (
	"net/http"

	"ticketing-system/backend/handler/shared"
	eventsvc "ticketing-system/backend/service/event"
	reportsvc "ticketing-system/backend/service/report"
	ticketsvc "ticketing-system/backend/service/ticket"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	events  *eventsvc.Service
	tickets *ticketsvc.Service
	reports *reportsvc.Service
}

func New(events *eventsvc.Service, tickets *ticketsvc.Service, reports *reportsvc.Service) *Handler {
	return &Handler{events: events, tickets: tickets, reports: reports}
}

func (h *Handler) CreateEvent(c *gin.Context) {
	var req eventsvc.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.Error("VALIDATION_ERROR", err.Error()))
		return
	}
	userID, err := shared.UserID(c)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	event, err := h.events.Create(req, userID)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusCreated, shared.OK(event))
}

func (h *Handler) UpdateEvent(c *gin.Context) {
	var req eventsvc.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.Error("VALIDATION_ERROR", err.Error()))
		return
	}
	event, err := h.events.UpdateDraft(c.Param("id"), req)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(event))
}

func (h *Handler) DeleteEvent(c *gin.Context) {
	err := h.events.DeleteDraft(c.Param("id"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(nil))
}

func (h *Handler) PublishEvent(c *gin.Context) {
	event, err := h.events.Publish(c.Param("id"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(event))
}

func (h *Handler) CloseEvent(c *gin.Context) {
	event, err := h.events.Close(c.Param("id"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(event))
}

func (h *Handler) ListApplications(c *gin.Context) {
	apps, err := h.tickets.ListApplications(c.Query("event_id"), c.Query("status"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(apps))
}

func (h *Handler) ApproveApplication(c *gin.Context) {
	userID, err := shared.UserID(c)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	app, err := h.tickets.ApproveApplication(c.Param("id"), userID)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(app))
}

func (h *Handler) RejectApplication(c *gin.Context) {
	userID, err := shared.UserID(c)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	var req ticketsvc.RejectRequest
	_ = c.ShouldBindJSON(&req)
	app, err := h.tickets.RejectApplication(c.Param("id"), userID, req)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(app))
}

func (h *Handler) Checkin(c *gin.Context) {
	var req ticketsvc.CheckinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.Error("VALIDATION_ERROR", err.Error()))
		return
	}
	userID, err := shared.UserID(c)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	result, err := h.tickets.Checkin(req, userID)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(result))
}

func (h *Handler) ListCheckins(c *gin.Context) {
	checkins, err := h.tickets.ListCheckins(c.Query("event_id"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(checkins))
}

func (h *Handler) EventStats(c *gin.Context) {
	stats, err := h.reports.EventStats(c.Param("id"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(stats))
}
