package auth

import (
	"net/http"

	"ticketing-system/backend/handler/shared"
	authsvc "ticketing-system/backend/service/auth"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *authsvc.Service
}

func New(service *authsvc.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Login(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	
	var req authsvc.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.Error("VALIDATION_ERROR", err.Error()))
		return
	}
	result, err := h.service.Login(req)
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(result))
}

func (h *Handler) Me(c *gin.Context) {
	user, err := h.service.Me(c.GetString("user_id"))
	if err != nil {
		shared.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, shared.OK(user))
}
