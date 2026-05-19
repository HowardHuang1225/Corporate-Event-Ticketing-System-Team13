package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ticketing-system/backend/handler/shared"
	eventsvc "ticketing-system/backend/service/event"
	reportsvc "ticketing-system/backend/service/report"
	ticketsvc "ticketing-system/backend/service/ticket"

	"github.com/gin-gonic/gin"
)

func validateCreatePayload(bodyBytes []byte) error {
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return err
	}

	// 1. 檢查 status
	if statusVal, exists := body["status"]; exists {
		if statusVal == nil {
			return fmt.Errorf("status cannot be nil")
		}
		statusStr, ok := statusVal.(string)
		if !ok {
			return fmt.Errorf("status must be a string")
		}
		if statusStr != "draft" {
			return fmt.Errorf("invalid status: %s", statusStr)
		}
	}

	// 2. 檢查 title
	if titleVal, exists := body["title"]; exists {
		if titleVal == nil {
			return fmt.Errorf("title cannot be nil")
		}
		titleStr, ok := titleVal.(string)
		if !ok {
			return fmt.Errorf("title must be a string")
		}
		if strings.TrimSpace(titleStr) == "" {
			return fmt.Errorf("title cannot be empty or only spaces")
		}
	}

	// 3. 檢查 venue
	if venueVal, exists := body["venue"]; exists {
		if venueVal == nil {
			return fmt.Errorf("venue cannot be nil")
		}
		venueStr, ok := venueVal.(string)
		if !ok {
			return fmt.Errorf("venue must be a string")
		}
		if strings.TrimSpace(venueStr) == "" {
			return fmt.Errorf("venue cannot be empty or only spaces")
		}
	}

	// 4. 檢查 ticket_types 並統計 totalQuotaSum
	ticketTypesVal, exists := body["ticket_types"]
	if !exists || ticketTypesVal == nil {
		return fmt.Errorf("ticket_types is required")
	}
	ticketTypesSlice, ok := ticketTypesVal.([]any)
	if !ok || len(ticketTypesSlice) == 0 {
		return fmt.Errorf("ticket_types cannot be empty")
	}

	totalQuotaSum := 0
	names := make(map[string]bool)
	for _, item := range ticketTypesSlice {
		if item == nil {
			return fmt.Errorf("ticket_type item cannot be nil")
		}
		m, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("ticket_type item must be an object")
		}

		// 檢查欄位是否超標
		for k := range m {
			if k != "name" && k != "total_quota" {
				return fmt.Errorf("undefined field in ticket type: %s", k)
			}
		}

		// name 欄位驗證
		nameVal, nameExists := m["name"]
		if !nameExists || nameVal == nil {
			return fmt.Errorf("ticket_type name is required")
		}
		nameStr, ok := nameVal.(string)
		if !ok {
			return fmt.Errorf("ticket_type name must be a string")
		}
		trimmed := strings.TrimSpace(nameStr)
		if nameStr == "" || trimmed == "" {
			return fmt.Errorf("ticket_type name cannot be empty or only spaces")
		}
		if names[nameStr] {
			return fmt.Errorf("duplicate ticket_type name: %s", nameStr)
		}
		names[nameStr] = true

		// total_quota 欄位驗證
		quotaVal, quotaExists := m["total_quota"]
		if !quotaExists || quotaVal == nil {
			return fmt.Errorf("ticket_type total_quota is required")
		}
		quotaNum, ok := quotaVal.(float64)
		if !ok {
			return fmt.Errorf("ticket_type total_quota must be an integer")
		}
		if quotaNum != float64(int(quotaNum)) {
			return fmt.Errorf("ticket_type total_quota cannot be a float")
		}
		if int(quotaNum) <= 0 {
			return fmt.Errorf("ticket_type total_quota must be greater than 0")
		}
		totalQuotaSum += int(quotaNum)
	}

	// 5. 檢查 max_tickets_per_person，並確保其不大於總票數
	if val, exists := body["max_tickets_per_person"]; exists {
		if val == nil {
			return fmt.Errorf("max_tickets_per_person cannot be nil")
		}
		num, ok := val.(float64)
		if !ok {
			return fmt.Errorf("max_tickets_per_person must be an integer")
		}
		if num != float64(int(num)) {
			return fmt.Errorf("max_tickets_per_person cannot be a float")
		}
		maxTickets := int(num)
		if maxTickets <= 0 {
			return fmt.Errorf("max_tickets_per_person must be greater than 0")
		}
		if maxTickets > totalQuotaSum {
			return fmt.Errorf("max_tickets_per_person (%d) cannot be greater than the total quota of all ticket types (%d)", maxTickets, totalQuotaSum)
		}
	}

	return nil
}

type Handler struct {
	events  *eventsvc.Service
	tickets *ticketsvc.Service
	reports *reportsvc.Service
}

func New(events *eventsvc.Service, tickets *ticketsvc.Service, reports *reportsvc.Service) *Handler {
	return &Handler{events: events, tickets: tickets, reports: reports}
}

func (h *Handler) CreateEvent(c *gin.Context) {
	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, shared.Error("VALIDATION_ERROR", err.Error()))
		return
	}
	if err := validateCreatePayload(bodyBytes); err != nil {
		c.JSON(http.StatusBadRequest, shared.Error("VALIDATION_ERROR", err.Error()))
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

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
	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, shared.Error("VALIDATION_ERROR", err.Error()))
		return
	}
	if err := validateCreatePayload(bodyBytes); err != nil {
		c.JSON(http.StatusBadRequest, shared.Error("VALIDATION_ERROR", err.Error()))
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

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
