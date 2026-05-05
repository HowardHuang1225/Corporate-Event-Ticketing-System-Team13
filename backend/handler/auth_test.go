package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/pkg"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const loginTestJWTSecret = "login-test-secret"

func TestAuth(t *testing.T) {
	tasks := []testTask{
		{
			description: "測試 demo 帳號是否能成功登入並且拿到正確的角色、廠區和 JWT claims",
			target:      LoginDemoAccounts,
		},
		{
			description: "測試不合法的帳號密碼",
			target:      LoginInvalidCredentials,
		},
	}

	mainTestFunc(t, tasks)
}

func LoginDemoAccounts(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試 demo 帳號是否能成功登入並且拿到正確的角色、廠區和 JWT claims\n")
	printTestProgress("==================================================\n")

	db, err := openLoginTestDB(t)
	if err != nil {
		errs.Add("open login test database", "%v", err)
		return
	}
	tx := db.Begin()
	if tx.Error != nil {
		errs.Add("begin transaction", "failed to begin login test transaction: %v", tx.Error)
		return
	}

	// Keep the Docker Postgres database clean after the test run.
	t.Cleanup(func() {
		tx.Rollback()
	})

	if err := seedLoginDemoUsers(tx); err != nil {
		errs.Add("seed demo users", "%v", err)
		return
	}

	router := gin.New()
	router.POST("/login", NewAuthHandler(tx, loginTestJWTSecret).Login)

	tests := []struct {
		name       string
		employeeID string
		password   string
		wantRole   string
		wantRegion string
		progress   string
	}{
		{
			name:       "manager",
			employeeID: "MGR001",
			password:   "password",
			wantRole:   "event_manager",
			wantRegion: "台南廠",
			progress:   "test if the manager account can log in and receives the correct role and region.",
		},
		{
			name:       "employee tainan",
			employeeID: "EMP001",
			password:   "password",
			wantRole:   "employee",
			wantRegion: "台南廠",
			progress:   "test if the Tainan employee account can log in and receives the correct role and region.",
		},
		{
			name:       "employee hsinchu",
			employeeID: "EMP002",
			password:   "password",
			wantRole:   "employee",
			wantRegion: "新竹廠",
			progress:   "test if the Hsinchu employee account can log in and receives the correct role and region.",
		},
		{
			name:       "hr",
			employeeID: "HR001",
			password:   "password",
			wantRole:   "hr",
			wantRegion: "台南廠",
			progress:   "test if the HR account can log in and receives the correct role and region.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// printTestProgress(tt.progress + "\n")

			resp := performLogin(router, tt.employeeID, tt.password)

			if resp.Code != http.StatusOK {
				errs.Add(tt.progress, "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
				return
			}

			body, err := decodeLoginResponse(resp.Body.Bytes())
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
	printTestProgress("==================================================\n\n")
}

func LoginInvalidCredentials(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試不合法的帳號密碼\n")
	printTestProgress("==================================================\n")

	db, err := openLoginTestDB(t)
	if err != nil {
		errs.Add("open login test database", "%v", err)
		return
	}
	tx := db.Begin()
	if tx.Error != nil {
		errs.Add("begin transaction", "failed to begin login test transaction: %v", tx.Error)
		return
	}
	// Roll back seeded users and any updates made during this test.
	t.Cleanup(func() {
		tx.Rollback()
	})

	if err := seedLoginDemoUsers(tx); err != nil {
		errs.Add("seed demo users", "%v", err)
		return
	}

	router := gin.New()
	router.POST("/login", NewAuthHandler(tx, loginTestJWTSecret).Login)

	tests := []struct {
		name       string
		employeeID string
		password   string
		progress   string
	}{
		{
			name:       "wrong employee id",
			employeeID: "UNKNOWN",
			password:   "password",
			progress:   "test if an unknown employee ID is rejected with the correct error code.",
		},
		{
			name:       "wrong password",
			employeeID: "EMP001",
			password:   "wrong-password",
			progress:   "test if a valid employee ID with an incorrect password is rejected with the correct error code.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// printTestProgress(tt.progress + "\n")

			resp := performLogin(router, tt.employeeID, tt.password)

			if resp.Code != http.StatusUnauthorized {
				errs.Add(tt.progress, "expected status 401, got %d with body %s", resp.Code, resp.Body.String())
				return
			}
			if err := assertHandlerErrorCode(resp.Body.Bytes(), "UNAUTHORIZED"); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}
	printTestProgress("==================================================\n\n")
}

// openLoginTestDB connects to the local Docker Postgres used by integration-like
// login tests and returns connection or migration failures to the shared test
// error collector.
func openLoginTestDB(t *testing.T) (*gorm.DB, error) {
	t.Helper()

	db, err := gorm.Open(postgres.Open(loginTestDSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("Postgres is not available for login tests: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to access sql db: %w", err)
	}
	t.Cleanup(func() {
		sqlDB.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("Postgres is not reachable for login tests: %w", err)
	}

	if err := db.AutoMigrate(&model.User{}); err != nil {
		return nil, fmt.Errorf("failed to migrate users table: %w", err)
	}

	return db, nil
}

// loginTestDSN lets CI or a developer override the DB with TEST_DATABASE_URL,
// then falls back to the app DATABASE_URL or the repo's default .env values.
func loginTestDSN() string {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}

	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		envOrDefault("DB_HOST", "localhost"),
		envOrDefault("DB_USER", "ts_user"),
		envOrDefault("DB_PASSWORD", "ts_password"),
		envOrDefault("DB_NAME", "ticketing_system"),
		envOrDefault("DB_PORT", "5432"),
	)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// seedLoginDemoUsers upserts the demo users required by the login tests into
// the current transaction, making the test repeatable even if the DB already has
// demo data from main.seedData.
func seedLoginDemoUsers(db *gorm.DB) error {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash demo password: %w", err)
	}

	users := []model.User{
		{
			EmployeeID:   "MGR001",
			Name:         "活動管理員",
			Email:        "manager@company.com",
			Department:   "活動管理部",
			Region:       "台南廠",
			Role:         "event_manager",
			PasswordHash: string(passwordHash),
			IsActive:     true,
		},
		{
			EmployeeID:   "EMP001",
			Name:         "台南員工",
			Email:        "emp001@company.com",
			Department:   "營運部",
			Region:       "台南廠",
			Role:         "employee",
			PasswordHash: string(passwordHash),
			IsActive:     true,
		},
		{
			EmployeeID:   "EMP002",
			Name:         "新竹員工",
			Email:        "emp002@company.com",
			Department:   "營運部",
			Region:       "新竹廠",
			Role:         "employee",
			PasswordHash: string(passwordHash),
			IsActive:     true,
		},
		{
			EmployeeID:   "HR001",
			Name:         "人資使用者",
			Email:        "hr@company.com",
			Department:   "人資部門",
			Region:       "台南廠",
			Role:         "hr",
			PasswordHash: string(passwordHash),
			IsActive:     true,
		},
	}

	for _, user := range users {
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "employee_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name",
				"email",
				"department",
				"region",
				"role",
				"password_hash",
				"is_active",
			}),
		}).Create(&user).Error; err != nil {
			return fmt.Errorf("failed to seed demo user %s: %w", user.EmployeeID, err)
		}
	}
	return nil
}

// performLogin sends the same JSON payload shape used by the frontend login
// page to the Gin test router.
func performLogin(router *gin.Engine, employeeID, password string) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(gin.H{
		"employee_id": employeeID,
		"password":    password,
	})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
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
func decodeLoginResponse(body []byte) (loginResponse, error) {
	var resp loginResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return loginResponse{}, fmt.Errorf("failed to decode login response: %w", err)
	}
	return resp, nil
}
