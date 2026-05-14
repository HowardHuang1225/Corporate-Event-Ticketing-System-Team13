package testutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

func PerformLogin(router *gin.Engine, employeeID, password string) *httptest.ResponseRecorder {
	return PerformJSON(router, http.MethodPost, "/login", gin.H{
		"employee_id": employeeID,
		"password":    password,
	})
}

func PerformJSON(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func DecodeJSON[T any](body []byte) (T, error) {
	var resp T
	if err := json.Unmarshal(body, &resp); err != nil {
		return resp, fmt.Errorf("failed to decode JSON response: %w", err)
	}
	return resp, nil
}

type loginResponse struct {
	Success bool `json:"success"`
	Data    struct {
		AccessToken string `json:"access_token"`
		User        struct {
			ID         string `json:"id"`
			EmployeeID string `json:"employee_id"`
			Name       string `json:"name"`
			Email      string `json:"email"`
			Department string `json:"department"`
			Region     string `json:"region"`
			Role       string `json:"role"`
		} `json:"user"`
	} `json:"data"`
}

// decodeLoginResponse keeps response assertions type-safe and easy to read.
func DecodeLoginResponse(body []byte) (loginResponse, error) {
	return DecodeJSON[loginResponse](body)
}
