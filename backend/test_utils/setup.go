package testutils

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"ticketing-system/backend/model"
)

func OpenTestDB(t *testing.T, paramDB []any) (*gorm.DB, error) {
	t.Helper()

	db, err := gorm.Open(postgres.Open(testDSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("Postgres is not available for tests: %w", err)
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
		return nil, fmt.Errorf("Postgres is not reachable for tests: %w", err)
	}

	if err := db.AutoMigrate(paramDB...); err != nil {
		return nil, fmt.Errorf("failed to migrate test tables: %w", err)
	}

	return db, nil
}

func BeginTestTransaction(t *testing.T, paramDB []any) (*gorm.DB, func(), error) {
	t.Helper()

	db, err := OpenTestDB(t, paramDB)
	if err != nil {
		return nil, nil, err
	}

	tx := db.Begin()
	if tx.Error != nil {
		return nil, nil, fmt.Errorf("failed to begin test transaction: %w", tx.Error)
	}

	cleanup := func() {
		_ = tx.Rollback().Error
	}
	return tx, cleanup, nil
}

func UniqueTestSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func testDSN() string {
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

func SeedTestRole(db *gorm.DB, users []model.User, needReturn bool) ([]model.User, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	if err != nil {
		return []model.User{}, fmt.Errorf("failed to hash manager password: %w", err)
	}

	for i := range users {
		users[i].PasswordHash = string(passwordHash)
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&users[i]).Error; err != nil {
			return []model.User{}, fmt.Errorf("failed to seed user %q: %w", users[i].Email, err)
		}
	}

	return users, nil
}
