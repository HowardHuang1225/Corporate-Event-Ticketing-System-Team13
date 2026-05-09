package handler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ticketing-system/backend/model"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTicketTestDB(t *testing.T) (*gorm.DB, error) {
	t.Helper()

	db, err := gorm.Open(postgres.Open(eventTestDSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("Postgres is not available for ticket tests: %w", err)
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
		return nil, fmt.Errorf("Postgres is not reachable for ticket tests: %w", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.Event{},
		&model.TicketType{},
		&model.Application{},
		&model.Ticket{},
		&model.Checkin{},
	); err != nil {
		return nil, fmt.Errorf("failed to migrate ticket test tables: %w", err)
	}

	return db, nil
}

func openTicketTestRedis(t *testing.T) (*redis.Client, error) {
	t.Helper()

	opt, err := redis.ParseURL(eventEnvOrDefault("REDIS_URL", "redis://localhost:6379"))
	if err != nil {
		return nil, fmt.Errorf("invalid Redis URL for ticket tests: %w", err)
	}
	client := redis.NewClient(opt)
	t.Cleanup(func() {
		client.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis is not reachable for ticket tests: %w", err)
	}
	return client, nil
}

func seedTicketTestUser(db *gorm.DB, employeeIDPrefix string, region string, role string) (model.User, error) {
	suffix := time.Now().UnixNano()
	user := model.User{
		EmployeeID:   fmt.Sprintf("%s%d", employeeIDPrefix, suffix),
		Name:         fmt.Sprintf("%s User", role),
		Email:        fmt.Sprintf("%s-%d@example.com", employeeIDPrefix, suffix),
		Department:   "QA",
		Region:       region,
		Role:         role,
		PasswordHash: "not-used",
		IsActive:     true,
	}
	if err := db.Create(&user).Error; err != nil {
		return model.User{}, fmt.Errorf("failed to seed %s user: %w", role, err)
	}
	return user, nil
}

func seedTicketApplyEvent(
	db *gorm.DB,
	manager model.User,
	regionRestriction *string,
	maxTicketsPerPerson int,
	remaining int,
) (model.Event, model.TicketType, error) {
	now := time.Now().UTC().Truncate(time.Second)
	event := model.Event{
		Title:               fmt.Sprintf("Ticket Apply Event %d", time.Now().UnixNano()),
		Description:         "Ticket application fixture",
		Venue:               "Main Hall",
		StartTime:           now.Add(24 * time.Hour),
		ApplyDeadline:       now.Add(48 * time.Hour),
		EndTime:             now.Add(72 * time.Hour),
		Status:              "published",
		RegionRestriction:   regionRestriction,
		MaxTicketsPerPerson: maxTicketsPerPerson,
		CreatedBy:           manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, model.TicketType{}, fmt.Errorf("failed to seed ticket application event: %w", err)
	}

	ticketType := model.TicketType{
		EventID:    event.ID,
		Name:       "Employee Ticket",
		TotalQuota: remaining,
		Remaining:  remaining,
	}
	if err := db.Create(&ticketType).Error; err != nil {
		return model.Event{}, model.TicketType{}, fmt.Errorf("failed to seed ticket type: %w", err)
	}

	return event, ticketType, nil
}

func resetTicketRedisKeys(t *testing.T, redisClient *redis.Client, ticketTypeID string) {
	t.Helper()

	keys := []string{
		"inventory:" + ticketTypeID,
		"inventory_loaded:" + ticketTypeID,
		"init_lock:" + ticketTypeID,
		"lock:init_lock:" + ticketTypeID,
	}
	if err := redisClient.Del(context.Background(), keys...).Err(); err != nil {
		t.Fatalf("failed to reset Redis inventory keys: %v", err)
	}
	t.Cleanup(func() {
		redisClient.Del(context.Background(), keys...)
	})
}

func assertTicketApplicationCount(db *gorm.DB, eventID uuid.UUID, want int64) error {
	var got int64
	if err := db.Model(&model.Application{}).Where("event_id = ?", eventID.String()).Count(&got).Error; err != nil {
		return fmt.Errorf("failed to count applications: %w", err)
	}
	if got != want {
		return fmt.Errorf("expected %d applications, got %d", want, got)
	}
	return nil
}

func assertTicketTypeRemaining(db *gorm.DB, ticketTypeID uuid.UUID, want int) error {
	var got int
	if err := db.Model(&model.TicketType{}).Where("id = ?", ticketTypeID.String()).Select("remaining").Scan(&got).Error; err != nil {
		return fmt.Errorf("failed to query ticket type remaining: %w", err)
	}
	if got != want {
		return fmt.Errorf("expected remaining %d, got %d", want, got)
	}
	return nil
}

func newTicketTestUUID() string {
	return uuid.New().String()
}
