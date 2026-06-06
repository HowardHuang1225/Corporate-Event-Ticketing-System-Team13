package ticket

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"ticketing-system/backend/model"
	totputil "ticketing-system/backend/pkg/totp"
	"ticketing-system/backend/repository"
	"ticketing-system/backend/service/apperror"
	utils "ticketing-system/backend/test_utils"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var ticketServiceTestModels = []any{
	&model.User{},
	&model.Event{},
	&model.TicketType{},
	&model.Application{},
	&model.Ticket{},
	&model.Checkin{},
}

type ticketServiceUsers struct {
	Manager  model.User
	Employee model.User
}

type ticketServiceTicketSpec struct {
	IsUsed    bool
	ExpiresAt time.Time
}

func setupTicketServiceTest(t *testing.T) (*gorm.DB, ticketServiceUsers, func(), error) {
	t.Helper()

	tx, cleanup, err := utils.BeginTestTransaction(t, ticketServiceTestModels)
	if err != nil {
		return nil, ticketServiceUsers{}, nil, err
	}

	suffix := utils.UniqueTestSuffix()
	seeded, err := utils.SeedTestRole(tx, []model.User{
		{
			EmployeeID:   fmt.Sprintf("SVCMGR%s", suffix),
			Name:         "票券服務測試管理者",
			Email:        fmt.Sprintf("ticket-service-manager-%s@example.com", suffix),
			Department:   "Events",
			Region:       "Tainan",
			Role:         "event_manager",
			PasswordHash: "",
			IsActive:     true,
		},
		{
			EmployeeID:   fmt.Sprintf("SVCEMP%s", suffix),
			Name:         "票券服務測試員工",
			Email:        fmt.Sprintf("ticket-service-employee-%s@example.com", suffix),
			Department:   "Engineering",
			Region:       "Tainan",
			Role:         "employee",
			PasswordHash: "",
			IsActive:     true,
		},
	}, true)
	if err != nil {
		_ = tx.Rollback()
		return nil, ticketServiceUsers{}, nil, err
	}

	return tx, ticketServiceUsers{Manager: seeded[0], Employee: seeded[1]}, cleanup, nil
}

func newTicketService(db *gorm.DB, redisClient *redis.Client) *Service {
	return New(repository.New(db, redisClient))
}

func seedTicketServiceEventWithTicketType(
	db *gorm.DB,
	manager model.User,
	totalQuota int,
	remaining int,
	maxTicketsPerPerson int,
) (model.Event, model.TicketType, error) {
	now := time.Now().UTC().Truncate(time.Second)
	event := model.Event{
		Title:               fmt.Sprintf("票券服務測試活動 %s", utils.UniqueTestSuffix()),
		Description:         "票券服務測試資料",
		Venue:               "測試場地",
		PublishTime:         now.Add(-time.Hour),
		StartTime:           now.Add(24 * time.Hour),
		ApplyDeadline:       now.Add(12 * time.Hour),
		EndTime:             now.Add(26 * time.Hour),
		Status:              "published",
		MaxTicketsPerPerson: maxTicketsPerPerson,
		CreatedBy:           manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, model.TicketType{}, fmt.Errorf("建立票券服務測試活動失敗：%w", err)
	}

	ticketType := model.TicketType{
		EventID:    event.ID,
		Name:       "一般票",
		TotalQuota: totalQuota,
		Remaining:  remaining,
		Version:    1,
	}
	if err := db.Create(&ticketType).Error; err != nil {
		return model.Event{}, model.TicketType{}, fmt.Errorf("建立票券服務測試票種失敗：%w", err)
	}

	return event, ticketType, nil
}

func seedTicketServiceApplicationWithTickets(
	db *gorm.DB,
	users ticketServiceUsers,
	status string,
	quantity int,
	remaining int,
	ticketSpecs []ticketServiceTicketSpec,
) (model.Event, model.TicketType, model.Application, []model.Ticket, error) {
	event, ticketType, err := seedTicketServiceEventWithTicketType(db, users.Manager, remaining+quantity, remaining, 10)
	if err != nil {
		return model.Event{}, model.TicketType{}, model.Application{}, nil, err
	}

	app := model.Application{
		UserID:         users.Employee.ID,
		EventID:        event.ID,
		TicketTypeID:   ticketType.ID,
		Quantity:       quantity,
		Status:         status,
		IdempotencyKey: fmt.Sprintf("ticket-service-%s", utils.UniqueTestSuffix()),
	}
	if err := db.Create(&app).Error; err != nil {
		return model.Event{}, model.TicketType{}, model.Application{}, nil, fmt.Errorf("建立票券服務測試申請失敗：%w", err)
	}

	tickets := make([]model.Ticket, 0, len(ticketSpecs))
	for _, spec := range ticketSpecs {
		expiresAt := spec.ExpiresAt
		if expiresAt.IsZero() {
			expiresAt = event.EndTime
		}
		ticket := model.Ticket{
			ApplicationID: app.ID,
			UserID:        users.Employee.ID,
			EventID:       event.ID,
			TicketTypeID:  ticketType.ID,
			QRToken:       uuid.New().String(),
			IsUsed:        spec.IsUsed,
			ExpiresAt:     expiresAt,
		}
		if err := db.Create(&ticket).Error; err != nil {
			return model.Event{}, model.TicketType{}, model.Application{}, nil, fmt.Errorf("建立票券服務測試票券失敗：%w", err)
		}
		tickets = append(tickets, ticket)
	}

	return event, ticketType, app, tickets, nil
}

func openTicketServiceTestRedis(t *testing.T) (*redis.Client, error) {
	t.Helper()

	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = os.Getenv("REDIS_URL")
	}
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("解析 Redis 測試 URL 失敗：%w", err)
	}
	client := redis.NewClient(opt)
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis 測試連線失敗：%w", err)
	}
	return client, nil
}

func cleanupTicketServiceInventoryKeys(t *testing.T, redisClient *redis.Client, ticketTypeID string) {
	t.Helper()

	keys := ticketServiceInventoryKeys(ticketTypeID)
	redisClient.Del(context.Background(), keys...)
	t.Cleanup(func() {
		redisClient.Del(context.Background(), keys...)
	})
}

func ticketServiceInventoryKeys(ticketTypeID string) []string {
	return []string{
		"inventory:" + ticketTypeID,
		"inventory_loaded:" + ticketTypeID,
		"lock:init_lock:" + ticketTypeID,
	}
}

func ticketServiceRemaining(db *gorm.DB, ticketTypeID uuid.UUID) (int, error) {
	var remaining int
	if err := db.Model(&model.TicketType{}).
		Select("remaining").
		Where("id = ?", ticketTypeID).
		Scan(&remaining).Error; err != nil {
		return 0, fmt.Errorf("查詢票種剩餘庫存失敗：%w", err)
	}
	return remaining, nil
}

func ticketServiceTicketCountForApplication(db *gorm.DB, applicationID uuid.UUID) (int64, error) {
	var count int64
	if err := db.Model(&model.Ticket{}).Where("application_id = ?", applicationID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("查詢申請票券數量失敗：%w", err)
	}
	return count, nil
}

func ticketServiceApplicationByID(db *gorm.DB, applicationID uuid.UUID) (model.Application, error) {
	var app model.Application
	if err := db.First(&app, "id = ?", applicationID).Error; err != nil {
		return model.Application{}, fmt.Errorf("查詢申請失敗：%w", err)
	}
	return app, nil
}

func ticketServiceTicketExists(db *gorm.DB, ticketID uuid.UUID) (bool, error) {
	var count int64
	if err := db.Model(&model.Ticket{}).Where("id = ?", ticketID).Count(&count).Error; err != nil {
		return false, fmt.Errorf("查詢票券是否存在失敗：%w", err)
	}
	return count > 0, nil
}

func ticketServiceCheckinCount(db *gorm.DB, ticketID uuid.UUID) (int64, error) {
	var count int64
	if err := db.Model(&model.Checkin{}).Where("ticket_id = ?", ticketID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("查詢核銷紀錄數量失敗：%w", err)
	}
	return count, nil
}

func ticketServicePreviousWindowDynamicQRToken(baseToken string) string {
	now := time.Now()
	if seconds := now.Unix() % 60; seconds >= 58 {
		time.Sleep(time.Duration(61-seconds) * time.Second)
		now = time.Now()
	}
	return baseToken + "|" + totputil.Generate(baseToken, now.Unix()/60-1)
}

func ticketServiceCurrentWindowDynamicQRToken(baseToken string) string {
	return baseToken + "|" + totputil.Generate(baseToken, time.Now().Unix()/60)
}

func ticketServiceExpiredWindowDynamicQRToken(baseToken string) string {
	currentCounter := time.Now().Unix() / 60
	allowed := map[string]bool{
		totputil.Generate(baseToken, currentCounter-1): true,
		totputil.Generate(baseToken, currentCounter):   true,
		totputil.Generate(baseToken, currentCounter+1): true,
	}

	for offset := int64(2); offset < 20; offset++ {
		otp := totputil.Generate(baseToken, currentCounter-offset)
		if !allowed[otp] {
			return baseToken + "|" + otp
		}
	}

	panic("could not find a non-colliding expired dynamic QR token")
}

func assertTicketServiceAppErrorCode(err error, wantCode string) error {
	if err == nil {
		return fmt.Errorf("預期錯誤代碼 %q，實際沒有錯誤", wantCode)
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		return fmt.Errorf("預期 apperror.Error，實際錯誤為 %T：%v", err, err)
	}
	if appErr.Code != wantCode {
		return fmt.Errorf("預期錯誤代碼 %q，實際為 %q", wantCode, appErr.Code)
	}
	return nil
}

func assertTicketServiceAppError(err error, wantStatus int, wantCode string) error {
	if err := assertTicketServiceAppErrorCode(err, wantCode); err != nil {
		return err
	}
	var appErr *apperror.Error
	errors.As(err, &appErr)
	if appErr.Status != wantStatus {
		return fmt.Errorf("預期 HTTP 狀態 %d，實際為 %d", wantStatus, appErr.Status)
	}
	return nil
}
