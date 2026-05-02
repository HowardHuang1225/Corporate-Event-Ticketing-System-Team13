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

// TestLoginDemoAccounts verifies that every demo account shown in the README/UI
// can log in and receives the expected role, region, and JWT claims.
func TestLoginDemoAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := openLoginTestDB(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("failed to begin login test transaction: %v", tx.Error)
	}
	// Keep the Docker Postgres database clean after the test run.
	t.Cleanup(func() {
		tx.Rollback()
	})

	seedLoginDemoUsers(t, tx)

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
			progress:   "Test if the manager account can log in and receives the correct role and region.\n",
		},
		{
			name:       "employee tainan",
			employeeID: "EMP001",
			password:   "password",
			wantRole:   "employee",
			wantRegion: "台南廠",
			progress:   "Test if the Tainan employee account can log in and receives the correct role and region.\n",
		},
		{
			name:       "employee hsinchu",
			employeeID: "EMP002",
			password:   "password",
			wantRole:   "employee",
			wantRegion: "新竹廠",
			progress:   "Test if the Hsinchu employee account can log in and receives the correct role and region.\n",
		},
		{
			name:       "hr",
			employeeID: "HR001",
			password:   "password",
			wantRole:   "hr",
			wantRegion: "台南廠",
			progress:   "Test if the HR account can log in and receives the correct role and region.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)

			resp := performLogin(router, tt.employeeID, tt.password)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d with body %s", resp.Code, resp.Body.String())
			}

			body := decodeLoginResponse(t, resp.Body.Bytes())
			if !body.Success {
				t.Fatal("expected success=true")
			}
			if body.Data.AccessToken == "" {
				t.Fatal("expected access token")
			}
			if body.Data.User.EmployeeID != tt.employeeID {
				t.Fatalf("expected employee_id %q, got %q", tt.employeeID, body.Data.User.EmployeeID)
			}
			if body.Data.User.Role != tt.wantRole {
				t.Fatalf("expected role %q, got %q", tt.wantRole, body.Data.User.Role)
			}
			if body.Data.User.Region != tt.wantRegion {
				t.Fatalf("expected region %q, got %q", tt.wantRegion, body.Data.User.Region)
			}

			claims, err := pkg.ValidateToken(body.Data.AccessToken, loginTestJWTSecret)
			if err != nil {
				t.Fatalf("expected valid access token: %v", err)
			}
			if claims.EmployeeID != tt.employeeID {
				t.Fatalf("expected token employee id %q, got %q", tt.employeeID, claims.EmployeeID)
			}
			if claims.Role != tt.wantRole {
				t.Fatalf("expected token role %q, got %q", tt.wantRole, claims.Role)
			}
		})
	}
}

// TestLoginRejectsInvalidCredentials covers both possible invalid credential
// paths: an unknown employee id and a valid employee id with a wrong password.
func TestLoginRejectsInvalidCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := openLoginTestDB(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("failed to begin login test transaction: %v", tx.Error)
	}
	// Roll back seeded users and any updates made during this test.
	t.Cleanup(func() {
		tx.Rollback()
	})

	seedLoginDemoUsers(t, tx)

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
			progress:   "Test if an unknown employee ID is rejected with the correct error code.\n",
		},
		{
			name:       "wrong password",
			employeeID: "EMP001",
			password:   "wrong-password",
			progress:   "Test if a valid employee ID with an incorrect password is rejected with the correct error code.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)

			resp := performLogin(router, tt.employeeID, tt.password)

			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("expected status 401, got %d with body %s", resp.Code, resp.Body.String())
			}
			assertHandlerErrorCode(t, resp.Body.Bytes(), "UNAUTHORIZED")
		})
	}
}

// openLoginTestDB connects to the local Docker Postgres used by integration-like
// login tests. If Postgres is not running, the login tests are skipped so pure
// unit test runs still work without Docker.
func openLoginTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(postgres.Open(loginTestDSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("Postgres is not available for login tests: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to access sql db: %v", err)
	}
	t.Cleanup(func() {
		sqlDB.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Skipf("Postgres is not reachable for login tests: %v", err)
	}

	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("failed to migrate users table: %v", err)
	}

	return db
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
func seedLoginDemoUsers(t *testing.T, db *gorm.DB) {
	t.Helper()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash demo password: %v", err)
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
			t.Fatalf("failed to seed demo user %s: %v", user.EmployeeID, err)
		}
	}
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
func decodeLoginResponse(t *testing.T, body []byte) loginResponse {
	t.Helper()

	var resp loginResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	return resp
}
