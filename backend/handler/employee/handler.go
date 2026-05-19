package employee

import (
	"net/http"

	"ticketing-system/backend/handler/shared"
	eventsvc "ticketing-system/backend/service/event"
	ticketsvc "ticketing-system/backend/service/ticket"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	events  *eventsvc.Service
	tickets *ticketsvc.Service
}

func New(events *eventsvc.Service, tickets *ticketsvc.Service) *Handler {
	return &Handler{events: events, tickets: tickets}
}

func (h *Handler) ListEvents(c *gin.Context) {
	events, err := h.events.List(
		c.Query("status"),
		shared.Role(c),
		c.Query("ticket_type"),
		c.Query("start_from"),
		c.Query("start_to"),
	)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(events))
}

func (h *Handler) GetEvent(c *gin.Context) {
	event, err := h.events.Get(c.Param("id"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(event))
}

func (h *Handler) CheckEligibility(c *gin.Context) {
	userID, err := shared.UserID(c)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	result, err := h.events.CheckEligibility(c.Param("id"), userID)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(result))
}

func (h *Handler) Apply(c *gin.Context) {
	var req ticketsvc.ApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.Error("VALIDATION_ERROR", err.Error()))
		return
	}
	userID, err := shared.UserID(c)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	result, err := h.tickets.Apply(userID, req)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	status := http.StatusCreated
	if result.Queued {
		status = http.StatusAccepted
	} else if !result.Created {
		status = http.StatusOK
	}
	c.JSON(status, shared.OK(result.Application))
}

func (h *Handler) QueueStatus(c *gin.Context) {
	userID, err := shared.UserID(c)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	result, err := h.tickets.QueueStatus(userID, c.Param("idempotency_key"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(result))
}

func (h *Handler) MyApplications(c *gin.Context) {
	apps, err := h.tickets.MyApplications(c.GetString("user_id"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(apps))
}

func (h *Handler) CancelApplication(c *gin.Context) {
	userID, err := shared.UserID(c)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	if err := h.tickets.CancelApplication(c.Param("id"), userID); err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(gin.H{"message": "Application cancelled and tickets returned to pool"}))
}

func (h *Handler) MyTickets(c *gin.Context) {
	tickets, err := h.tickets.MyTickets(c.GetString("user_id"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(tickets))
}

func (h *Handler) CancelTicket(c *gin.Context) {
	userID, err := shared.UserID(c)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	if err := h.tickets.CancelTicket(c.Param("id"), userID); err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(gin.H{"message": "Ticket returned successfully"}))
}
