package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ticketing-system/backend/pkg"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const middlewareTestJWTSecret = "middleware-test-secret"

func TestMiddleware(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試缺少、格式錯誤、簽章錯誤 token 會回傳 UNAUTHORIZED",
			Target:      RejectUnauthorizedTokens,
		},
		{
			Description: "測試合法 token 會寫入 gin context",
			Target:      WriteClaimsToGinContext,
		},
		{
			Description: "測試角色不符會回傳 FORBIDDEN",
			Target:      RejectMismatchedRole,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func RejectUnauthorizedTokens(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("測試缺少、格式錯誤、簽章錯誤 token 會回傳 UNAUTHORIZED\n")
	utils.PrintTestProgress("==================================================\n")

	wrongSignatureToken, err := pkg.GenerateToken(uuid.New(), "EMP001", "employee", "wrong-secret")
	if err != nil {
		errs.Add("準備簽章錯誤 token", "產生 token 失敗：%v", err)
		return
	}

	tests := []struct {
		name       string
		authHeader string
		progress   string
	}{
		{
			name:       "缺少 Authorization header",
			authHeader: "",
			progress:   "沒有 Authorization header 時應回傳 UNAUTHORIZED。",
		},
		{
			name:       "token 格式錯誤",
			authHeader: "Bearer 這不是合法JWT",
			progress:   "Bearer 後方不是合法 JWT 格式時應回傳 UNAUTHORIZED。",
		},
		{
			name:       "token 簽章錯誤",
			authHeader: "Bearer " + wrongSignatureToken,
			progress:   "token 使用不同 secret 簽章時應回傳 UNAUTHORIZED。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			router := newAuthOnlyRouter()
			resp := performMiddlewareRequest(router, http.MethodGet, "/protected", tt.authHeader)

			if resp.Code != http.StatusUnauthorized {
				errs.Add(tt.progress, "預期狀態碼 401，實際為 %d，回應內容：%s", resp.Code, resp.Body.String())
				return
			}
			if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), "UNAUTHORIZED"); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func WriteClaimsToGinContext(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("測試合法 token 會寫入 gin context\n")
	utils.PrintTestProgress("==================================================\n")

	userID := uuid.New()
	token, err := pkg.GenerateToken(userID, "EMP777", "hr", middlewareTestJWTSecret)
	if err != nil {
		errs.Add("準備合法 token", "產生 token 失敗：%v", err)
		return
	}

	tests := []struct {
		name     string
		progress string
	}{
		{
			name:     "合法 token 寫入 context",
			progress: "合法 token 通過驗證後，應把 user_id、employee_id、role 寫入 gin context。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			router := gin.New()
			router.Use(AuthMiddleware(middlewareTestJWTSecret))
			router.GET("/protected", func(c *gin.Context) {
				contextUserID, exists := c.Get("user_id")
				if !exists {
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"error":   gin.H{"code": "CONTEXT_MISSING", "message": "user_id 不存在"},
					})
					return
				}
				contextUserIDString, ok := contextUserID.(string)
				if !ok {
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"error":   gin.H{"code": "CONTEXT_TYPE_ERROR", "message": "user_id 型別錯誤"},
					})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data": gin.H{
						"user_id":     contextUserIDString,
						"employee_id": c.GetString("employee_id"),
						"role":        c.GetString("role"),
					},
				})
			})

			resp := performMiddlewareRequest(router, http.MethodGet, "/protected", "Bearer "+token)
			if resp.Code != http.StatusOK {
				errs.Add(tt.progress, "預期狀態碼 200，實際為 %d，回應內容：%s", resp.Code, resp.Body.String())
				return
			}

			body, err := decodeContextResponse(resp.Body.Bytes())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if !body.Success {
				errs.Add(tt.progress, "預期 success=true，回應內容：%s", resp.Body.String())
				return
			}
			if body.Data.UserID != userID.String() {
				errs.Add(tt.progress, "預期 user_id 為 %q，實際為 %q", userID.String(), body.Data.UserID)
				return
			}
			if body.Data.EmployeeID != "EMP777" {
				errs.Add(tt.progress, "預期 employee_id 為 %q，實際為 %q", "EMP777", body.Data.EmployeeID)
				return
			}
			if body.Data.Role != "hr" {
				errs.Add(tt.progress, "預期 role 為 %q，實際為 %q", "hr", body.Data.Role)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func RejectMismatchedRole(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("測試角色不符會回傳 FORBIDDEN\n")
	utils.PrintTestProgress("==================================================\n")

	tests := []struct {
		name       string
		tokenRole  string
		allowRoles []string
		progress   string
	}{
		{
			name:       "員工不能存取活動管理者路由",
			tokenRole:  "employee",
			allowRoles: []string{"event_manager"},
			progress:   "employee 角色存取只允許 event_manager 的路由時應回傳 FORBIDDEN。",
		},
		{
			name:       "活動管理者不能存取人資路由",
			tokenRole:  "event_manager",
			allowRoles: []string{"hr"},
			progress:   "event_manager 角色存取只允許 hr 的路由時應回傳 FORBIDDEN。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			token, err := pkg.GenerateToken(uuid.New(), "EMP888", tt.tokenRole, middlewareTestJWTSecret)
			if err != nil {
				errs.Add(tt.progress, "產生 token 失敗：%v", err)
				return
			}

			router := gin.New()
			router.Use(AuthMiddleware(middlewareTestJWTSecret))
			router.GET("/role-protected", RequireRole(tt.allowRoles...), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"success": true})
			})

			resp := performMiddlewareRequest(router, http.MethodGet, "/role-protected", "Bearer "+token)
			if resp.Code != http.StatusForbidden {
				errs.Add(tt.progress, "預期狀態碼 403，實際為 %d，回應內容：%s", resp.Code, resp.Body.String())
				return
			}
			if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), "FORBIDDEN"); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func newAuthOnlyRouter() *gin.Engine {
	router := gin.New()
	router.Use(AuthMiddleware(middlewareTestJWTSecret))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	return router
}

func performMiddlewareRequest(router *gin.Engine, method, path, authHeader string) *httptest.ResponseRecorder {
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	router.ServeHTTP(resp, req)
	return resp
}

func decodeContextResponse(body []byte) (struct {
	Success bool `json:"success"`
	Data    struct {
		UserID     string `json:"user_id"`
		EmployeeID string `json:"employee_id"`
		Role       string `json:"role"`
	} `json:"data"`
}, error) {
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			UserID     string `json:"user_id"`
			EmployeeID string `json:"employee_id"`
			Role       string `json:"role"`
		} `json:"data"`
	}
	err := json.Unmarshal(body, &resp)
	return resp, err
}
