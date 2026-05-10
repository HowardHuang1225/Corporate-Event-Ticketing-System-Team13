package hr

import (
	"net/http"

	"ticketing-system/backend/handler/shared"
	reportsvc "ticketing-system/backend/service/report"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	reports *reportsvc.Service
}

func New(reports *reportsvc.Service) *Handler {
	return &Handler{reports: reports}
}

func (h *Handler) EventStats(c *gin.Context) {
	stats, err := h.reports.EventStats(c.Param("id"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(stats))
}

func (h *Handler) Overview(c *gin.Context) {
	overview, err := h.reports.Overview()
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(overview))
}

func (h *Handler) ExportEventCSV(c *gin.Context) {
	body, err := h.reports.ExportEventCSV(c.Param("id"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=event-stats.csv")
	c.String(http.StatusOK, body)
}
