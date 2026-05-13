package config

import (
	"strings"
	"testing"

	utils "ticketing-system/backend/test_utils"
)

func TestConfig(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試 Load 會優先使用 DATABASE_URL",
			Target:      LoadUsesDatabaseURLWhenProvided,
		},
		{
			Description: "測試 Load 會用資料庫環境變數組出預設 DSN",
			Target:      LoadBuildsDatabaseURLFromComponents,
		},
		{
			Description: "測試 Load 會套用非資料庫設定的預設值與覆寫值",
			Target:      LoadAppliesDefaultsAndOverrides,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func LoadUsesDatabaseURLWhenProvided(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試 Load 會優先使用 DATABASE_URL\n")
	utils.PrintTestProgress("==================================================\n")

	clearConfigTestEnv(t)
	const databaseURL = "postgres://user:pass@example.com:5432/app?sslmode=disable"
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("DB_HOST", "ignored-host")

	tests := []struct {
		name     string
		progress string
	}{
		{
			name:     "DATABASE_URL 優先",
			progress: "DATABASE_URL 有值時，Load 應直接採用它，不應再用 DB_HOST 等欄位組 DSN。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			cfg := Load()
			if cfg.DatabaseURL != databaseURL {
				errs.Add(tt.progress, "預期 DatabaseURL 為 %q，實際為 %q", databaseURL, cfg.DatabaseURL)
				return
			}
			if strings.Contains(cfg.DatabaseURL, "ignored-host") {
				errs.Add(tt.progress, "DatabaseURL 不應包含 DB_HOST 的值：%q", cfg.DatabaseURL)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func LoadBuildsDatabaseURLFromComponents(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試 Load 會用資料庫環境變數組出預設 DSN\n")
	utils.PrintTestProgress("==================================================\n")

	clearConfigTestEnv(t)
	t.Setenv("DB_HOST", "db.example.local")
	t.Setenv("DB_USER", "ticket_user")
	t.Setenv("DB_PASSWORD", "ticket_pass")
	t.Setenv("DB_NAME", "ticket_db")
	t.Setenv("DB_PORT", "15432")

	tests := []struct {
		name     string
		progress string
	}{
		{
			name:     "資料庫欄位組 DSN",
			progress: "DATABASE_URL 未設定時，Load 應使用 DB_HOST、DB_USER、DB_PASSWORD、DB_NAME、DB_PORT 組出 Postgres DSN。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			cfg := Load()
			want := "host=db.example.local user=ticket_user password=ticket_pass dbname=ticket_db port=15432 sslmode=disable"
			if cfg.DatabaseURL != want {
				errs.Add(tt.progress, "預期 DatabaseURL 為 %q，實際為 %q", want, cfg.DatabaseURL)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func LoadAppliesDefaultsAndOverrides(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試 Load 會套用非資料庫設定的預設值與覆寫值\n")
	utils.PrintTestProgress("==================================================\n")

	tests := []struct {
		name               string
		progress           string
		setEnv             func(t *testing.T)
		wantRedisURL       string
		wantJWTSecret      string
		wantPort           string
		wantAllowedOrigins string
	}{
		{
			name:     "預設值",
			progress: "REDIS_URL、JWT_SECRET、PORT、ALLOWED_ORIGINS 未設定時，Load 應使用預設值。",
			setEnv: func(t *testing.T) {
				clearConfigTestEnv(t)
			},
			wantRedisURL:       "redis://localhost:6379",
			wantJWTSecret:      "dev-jwt-secret-change-in-prod-32chars!!",
			wantPort:           "8001",
			wantAllowedOrigins: "http://localhost:5173,http://localhost:3000,http://localhost:8080",
		},
		{
			name:     "覆寫值",
			progress: "REDIS_URL、JWT_SECRET、PORT、ALLOWED_ORIGINS 有設定時，Load 應使用環境變數值。",
			setEnv: func(t *testing.T) {
				clearConfigTestEnv(t)
				t.Setenv("REDIS_URL", "redis://redis.example.local:6380")
				t.Setenv("JWT_SECRET", "unit-test-secret")
				t.Setenv("PORT", "19000")
				t.Setenv("ALLOWED_ORIGINS", "https://example.com")
			},
			wantRedisURL:       "redis://redis.example.local:6380",
			wantJWTSecret:      "unit-test-secret",
			wantPort:           "19000",
			wantAllowedOrigins: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")
			tt.setEnv(t)

			cfg := Load()
			if cfg.RedisURL != tt.wantRedisURL {
				errs.Add(tt.progress, "預期 RedisURL 為 %q，實際為 %q", tt.wantRedisURL, cfg.RedisURL)
			}
			if cfg.JWTSecret != tt.wantJWTSecret {
				errs.Add(tt.progress, "預期 JWTSecret 為 %q，實際為 %q", tt.wantJWTSecret, cfg.JWTSecret)
			}
			if cfg.Port != tt.wantPort {
				errs.Add(tt.progress, "預期 Port 為 %q，實際為 %q", tt.wantPort, cfg.Port)
			}
			if cfg.AllowedOrigins != tt.wantAllowedOrigins {
				errs.Add(tt.progress, "預期 AllowedOrigins 為 %q，實際為 %q", tt.wantAllowedOrigins, cfg.AllowedOrigins)
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func clearConfigTestEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"DATABASE_URL",
		"DB_HOST",
		"DB_USER",
		"DB_PASSWORD",
		"DB_NAME",
		"DB_PORT",
		"REDIS_URL",
		"JWT_SECRET",
		"PORT",
		"ALLOWED_ORIGINS",
	} {
		t.Setenv(key, "")
	}
}
