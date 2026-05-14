package auth

import (
	"net/http"
	"testing"

	"ticketing-system/backend/pkg"
	"ticketing-system/backend/repository"
	service "ticketing-system/backend/service/auth"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

const loginTestJWTSecret = "login-test-secret"

func TestAuth(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試 demo 帳號是否能成功登入並且拿到正確的角色、廠區和 JWT claims",
			Target:      LoginDemoAccounts,
		},
		{
			Description: "測試不合法的帳號密碼",
			Target:      LoginInvalidCredentials,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func LoginDemoAccounts(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("測試 demo 帳號是否能成功登入並且拿到正確的角色、廠區和 JWT claims\n")
	utils.PrintTestProgress("==================================================\n")

	tx, cleanup, err := utils.BeginTestTransaction(t, paramDB)
	if err != nil {
		errs.Add("open login test database", "%v", err)
		return
	}

	// Keep the Docker Postgres database clean after the test run.
	t.Cleanup(cleanup)

	if _, err := utils.SeedTestRole(tx, demoAccounts, false); err != nil {
		errs.Add("seed demo users", "%v", err)
		return
	}

	repos := repository.New(tx, nil)
	handler := New(service.New(repos, loginTestJWTSecret))

	router := gin.New()
	router.POST("/login", handler.Login)

	tests := []struct {
		name       string
		employeeID string
		password   string
		wantRole   string
		wantRegion string
		progress   string
	}{
		{
			name:       "活動管理者帳號",
			employeeID: "MGR001",
			password:   "password",
			wantRole:   "event_manager",
			wantRegion: "台南廠",
			progress:   "活動管理者帳號登入成功後，應取得正確角色、廠區與 JWT claims。",
		},
		{
			name:       "台南員工帳號",
			employeeID: "EMP001",
			password:   "password",
			wantRole:   "employee",
			wantRegion: "台南廠",
			progress:   "台南員工帳號登入成功後，應取得正確角色、廠區與 JWT claims。",
		},
		{
			name:       "新竹員工帳號",
			employeeID: "EMP002",
			password:   "password",
			wantRole:   "employee",
			wantRegion: "新竹廠",
			progress:   "新竹員工帳號登入成功後，應取得正確角色、廠區與 JWT claims。",
		},
		{
			name:       "人資帳號",
			employeeID: "HR001",
			password:   "password",
			wantRole:   "hr",
			wantRegion: "台南廠",
			progress:   "人資帳號登入成功後，應取得正確角色、廠區與 JWT claims。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			resp := utils.PerformLogin(router, tt.employeeID, tt.password)

			if resp.Code != http.StatusOK {
				errs.Add(tt.progress, "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
				return
			}

			body, err := utils.DecodeLoginResponse(resp.Body.Bytes())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if !body.Success {
				errs.Add(tt.progress, "expected success=true, got false with body %s", resp.Body.String())
				return
			}
			if body.Data.AccessToken == "" {
				errs.Add(tt.progress, "expected access token, got empty with body %s", resp.Body.String())
				return
			}
			if body.Data.User.EmployeeID != tt.employeeID {
				errs.Add(tt.progress, "expected employee_id %q, got %q with body %s", tt.employeeID, body.Data.User.EmployeeID, resp.Body.String())
				return
			}
			if body.Data.User.Role != tt.wantRole {
				errs.Add(tt.progress, "expected role %q, got %q", tt.wantRole, body.Data.User.Role)
				return
			}
			if body.Data.User.Region != tt.wantRegion {
				errs.Add(tt.progress, "expected region %q, got %q", tt.wantRegion, body.Data.User.Region)
				return
			}

			claims, err := pkg.ValidateToken(body.Data.AccessToken, loginTestJWTSecret)
			if err != nil {
				errs.Add(tt.progress, "expected valid access token: %v", err)
				return
			}
			if claims.EmployeeID != tt.employeeID {
				errs.Add(tt.progress, "expected token employee id %q, got %q", tt.employeeID, claims.EmployeeID)
				return
			}
			if claims.Role != tt.wantRole {
				errs.Add(tt.progress, "expected token role %q, got %q", tt.wantRole, claims.Role)
				return
			}
		})
	}
	utils.PrintTestProgress("==================================================\n\n")
}

func LoginInvalidCredentials(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("測試不合法的帳號密碼\n")
	utils.PrintTestProgress("==================================================\n")

	tx, cleanup, err := utils.BeginTestTransaction(t, paramDB)
	if err != nil {
		errs.Add("open login test database", "%v", err)
		return
	}

	// Roll back seeded users and any updates made during this test.
	t.Cleanup(cleanup)

	if _, err := utils.SeedTestRole(tx, demoAccounts, false); err != nil {
		errs.Add("seed demo users", "%v", err)
		return
	}

	repos := repository.New(tx, nil)
	handler := New(service.New(repos, loginTestJWTSecret))

	router := gin.New()
	router.POST("/login", handler.Login)

	tests := []struct {
		name       string
		employeeID string
		password   string
		progress   string
	}{
		{
			name:       "不存在的員工編號",
			employeeID: "UNKNOWN",
			password:   "password",
			progress:   "不存在的員工編號登入時，應回傳正確錯誤碼。",
		},
		{
			name:       "密碼錯誤",
			employeeID: "EMP001",
			password:   "wrong-password",
			progress:   "合法員工編號搭配錯誤密碼登入時，應回傳正確錯誤碼。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			resp := utils.PerformLogin(router, tt.employeeID, tt.password)

			if resp.Code != http.StatusUnauthorized {
				errs.Add(tt.progress, "expected status 401, got %d with body %s", resp.Code, resp.Body.String())
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
