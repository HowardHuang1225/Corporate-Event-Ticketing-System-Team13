package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ticketing-system/backend/pkg"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestEmployeeCannotAccessManagerEventRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "route-test-secret"

	router := gin.New()
	Register(router, Dependencies{JWTSecret: secret})

	token, err := pkg.GenerateToken(uuid.New(), "EMP001", "employee", secret)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("POST /v1/events as employee status = %d, body = %s", resp.Code, resp.Body.String())
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if body.Error.Code != "FORBIDDEN" {
		t.Fatalf("error code = %q, want FORBIDDEN", body.Error.Code)
	}
}
