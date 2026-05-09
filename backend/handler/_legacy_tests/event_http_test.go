package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"time"

	"ticketing-system/backend/model"

	"github.com/gin-gonic/gin"
)

func validCreateEventPayload(now time.Time) gin.H {
	return gin.H{
		"title":                  "Unit Test Company Day",
		"description":            "Created from event handler test",
		"venue":                  "Main Hall",
		"publish_time":           now.Add(48 * time.Hour).Format(time.RFC3339),
		"start_time":             now.Add(72 * time.Hour).Format(time.RFC3339),
		"apply_deadline":         now.Add(74 * time.Hour).Format(time.RFC3339),
		"end_time":               now.Add(76 * time.Hour).Format(time.RFC3339),
		"max_tickets_per_person": 2,
		"ticket_types": []gin.H{
			{"name": "一般票", "total_quota": 100},
			{"name": "眷屬票", "total_quota": 50},
		},
	}
}

func performEventJSON(router *gin.Engine, method, path string, body gin.H) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

type eventResponse struct {
	Success bool        `json:"success"`
	Data    model.Event `json:"data"`
}

func decodeEventResponse(body []byte) (eventResponse, error) {
	var resp eventResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return eventResponse{}, fmt.Errorf("failed to decode event response: %w", err)
	}
	return resp, nil
}

type eventListResponse struct {
	Success bool          `json:"success"`
	Data    []model.Event `json:"data"`
}

func decodeEventListResponse(body []byte) (eventListResponse, error) {
	var resp eventListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return eventListResponse{}, fmt.Errorf("failed to decode event list response: %w", err)
	}
	return resp, nil
}
