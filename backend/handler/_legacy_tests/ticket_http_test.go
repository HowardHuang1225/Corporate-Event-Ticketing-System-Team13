package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"

	"ticketing-system/backend/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func newTicketApplyRouter(db *gorm.DB, redisClient *redis.Client, user model.User) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", user.ID.String())
		c.Set("role", user.Role)
		c.Next()
	})
	router.POST("/applications", NewTicketHandler(db, redisClient).Apply)
	return router
}

func validApplyPayload(event model.Event, ticketType model.TicketType, quantity int) gin.H {
	return validApplyPayloadWithIDs(event.ID.String(), ticketType.ID.String(), quantity)
}

func validApplyPayloadWithIDs(eventID string, ticketTypeID string, quantity int) gin.H {
	return gin.H{
		"event_id":        eventID,
		"ticket_type_id":  ticketTypeID,
		"quantity":        quantity,
		"status":          "pending",
		"idempotency_key": "apply-test-" + uuid.New().String(),
	}
}

func performTicketJSON(router *gin.Engine, method string, path string, body gin.H) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

type applyResponse struct {
	Success bool              `json:"success"`
	Data    model.Application `json:"data"`
}

func decodeApplyResponse(body []byte) (applyResponse, error) {
	var resp applyResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return applyResponse{}, fmt.Errorf("failed to decode apply response: %w", err)
	}
	return resp, nil
}
