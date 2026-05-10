package handler

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"ticketing-system/backend/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openEventTestDB connects to local Docker Postgres for event handler tests.
func openEventTestDB(t *testing.T) (*gorm.DB, error) {
	t.Helper()

	db, err := gorm.Open(postgres.Open(eventTestDSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("Postgres is not available for event tests: %w", err)
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
		return nil, fmt.Errorf("Postgres is not reachable for event tests: %w", err)
	}

	if err := db.AutoMigrate(&model.User{}, &model.Event{}, &model.TicketType{}); err != nil {
		return nil, fmt.Errorf("failed to migrate event test tables: %w", err)
	}

	return db, nil
}

func eventTestDSN() string {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}

	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		eventEnvOrDefault("DB_HOST", "localhost"),
		eventEnvOrDefault("DB_USER", "ts_user"),
		eventEnvOrDefault("DB_PASSWORD", "ts_password"),
		eventEnvOrDefault("DB_NAME", "ticketing_system"),
		eventEnvOrDefault("DB_PORT", "5432"),
	)
}

func eventEnvOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func seedEventTestManager(db *gorm.DB) (model.User, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, fmt.Errorf("failed to hash manager password: %w", err)
	}

	manager := model.User{
		EmployeeID:   fmt.Sprintf("EVTMGR%d", time.Now().UnixNano()),
		Name:         "Event Test Manager",
		Email:        fmt.Sprintf("event-manager-%d@example.com", time.Now().UnixNano()),
		Department:   "Events",
		Region:       "台南廠",
		Role:         "event_manager",
		PasswordHash: string(passwordHash),
		IsActive:     true,
	}
	if err := db.Create(&manager).Error; err != nil {
		return model.User{}, fmt.Errorf("failed to seed event test manager: %w", err)
	}

	return manager, nil
}
