package ticket

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func TicketServiceApplyRejectsBusinessRulesWithRedis(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務申請防呆：確認 Redis flow 會拒絕不存在票種、活動不存在、活動不可申請、超過上限與售完。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備 Apply business rule 測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	redisClient, err := openTicketServiceTestRedis(t)
	if err != nil {
		errs.Add("準備 Apply business rule Redis", "%v", err)
		return
	}
	service := newTicketService(tx, redisClient)

	missingTicketTypeID := uuid.New().String()
	cleanupTicketServiceInventoryKeys(t, redisClient, missingTicketTypeID)
	_, err = service.Apply(users.Employee.ID, ApplyRequest{
		EventID:        uuid.New().String(),
		TicketTypeID:   missingTicketTypeID,
		Quantity:       1,
		IdempotencyKey: "missing-ticket-type-" + utils.UniqueTestSuffix(),
	})
	if assertErr := assertTicketServiceAppError(err, http.StatusNotFound, "NOT_FOUND"); assertErr != nil {
		errs.Add("申請不存在票種", "%v", assertErr)
		return
	}

	unavailableEvent, unavailableType, err := seedTicketServiceEventWithTicketType(tx, users.Manager, 3, 3, 3)
	if err != nil {
		errs.Add("建立不可申請活動", "%v", err)
		return
	}
	if err := tx.Model(&model.Event{}).Where("id = ?", unavailableEvent.ID).Update("status", "draft").Error; err != nil {
		errs.Add("更新活動為 draft", "%v", err)
		return
	}
	cleanupTicketServiceInventoryKeys(t, redisClient, unavailableType.ID.String())
	_, err = service.Apply(users.Employee.ID, ApplyRequest{
		EventID:        unavailableEvent.ID.String(),
		TicketTypeID:   unavailableType.ID.String(),
		Quantity:       1,
		IdempotencyKey: "event-unavailable-" + utils.UniqueTestSuffix(),
	})
	if assertErr := assertTicketServiceAppError(err, http.StatusBadRequest, "EVENT_NOT_AVAILABLE"); assertErr != nil {
		errs.Add("申請 draft 活動", "%v", assertErr)
		return
	}

	limitedEvent, limitedType, err := seedTicketServiceEventWithTicketType(tx, users.Manager, 3, 3, 1)
	if err != nil {
		errs.Add("建立上限測試活動", "%v", err)
		return
	}
	_, _, _, _, err = seedTicketServiceApplicationWithTickets(tx, users, "approved", 1, 2, []ticketServiceTicketSpec{{}})
	if err != nil {
		errs.Add("建立既有票券資料", "%v", err)
		return
	}
	// The helper creates its own event, so create the issued ticket for the limited event explicitly.
	existingApp := model.Application{UserID: users.Employee.ID, EventID: limitedEvent.ID, TicketTypeID: limitedType.ID, Quantity: 1, Status: "approved", IdempotencyKey: "limit-existing-" + utils.UniqueTestSuffix()}
	if err := tx.Create(&existingApp).Error; err != nil {
		errs.Add("建立上限測試既有申請", "%v", err)
		return
	}
	existingTicket := model.Ticket{ApplicationID: existingApp.ID, UserID: users.Employee.ID, EventID: limitedEvent.ID, TicketTypeID: limitedType.ID, QRToken: uuid.New().String(), ExpiresAt: limitedEvent.EndTime}
	if err := tx.Create(&existingTicket).Error; err != nil {
		errs.Add("建立上限測試既有票券", "%v", err)
		return
	}
	cleanupTicketServiceInventoryKeys(t, redisClient, limitedType.ID.String())
	_, err = service.Apply(users.Employee.ID, ApplyRequest{
		EventID:        limitedEvent.ID.String(),
		TicketTypeID:   limitedType.ID.String(),
		Quantity:       1,
		IdempotencyKey: "exceeds-limit-" + utils.UniqueTestSuffix(),
	})
	if assertErr := assertTicketServiceAppError(err, http.StatusBadRequest, "EXCEEDS_MAX_TICKETS"); assertErr != nil {
		errs.Add("申請超過每人上限", "%v", assertErr)
		return
	}

	soldOutEvent, soldOutType, err := seedTicketServiceEventWithTicketType(tx, users.Manager, 1, 0, 3)
	if err != nil {
		errs.Add("建立售完活動", "%v", err)
		return
	}
	cleanupTicketServiceInventoryKeys(t, redisClient, soldOutType.ID.String())
	_, err = service.Apply(users.Employee.ID, ApplyRequest{
		EventID:        soldOutEvent.ID.String(),
		TicketTypeID:   soldOutType.ID.String(),
		Quantity:       1,
		IdempotencyKey: "sold-out-" + utils.UniqueTestSuffix(),
	})
	if assertErr := assertTicketServiceAppError(err, http.StatusConflict, "TICKET_SOLD_OUT"); assertErr != nil {
		errs.Add("申請售完票種", "%v", assertErr)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func TicketServiceUsesRedisCacheForMyLists(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務快取：確認 MyApplications 與 MyTickets 會優先回傳 Redis 快取資料。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備快取測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	redisClient, err := openTicketServiceTestRedis(t)
	if err != nil {
		errs.Add("準備快取 Redis", "%v", err)
		return
	}
	ctx := context.Background()
	appsKey := "user:applications:" + users.Employee.ID.String()
	ticketsKey := "user:tickets:" + users.Employee.ID.String()
	redisClient.Del(ctx, appsKey, ticketsKey)
	t.Cleanup(func() { redisClient.Del(context.Background(), appsKey, ticketsKey) })

	cachedAppID := uuid.New()
	cachedApps := []model.Application{{ID: cachedAppID, UserID: users.Employee.ID, Status: "cached", Quantity: 7}}
	appsPayload, _ := json.Marshal(cachedApps)
	if err := redisClient.Set(ctx, appsKey, appsPayload, time.Minute).Err(); err != nil {
		errs.Add("寫入申請快取", "%v", err)
		return
	}

	cachedTicketID := uuid.New()
	cachedTickets := []model.Ticket{{ID: cachedTicketID, UserID: users.Employee.ID, QRToken: "cached-ticket"}}
	ticketsPayload, _ := json.Marshal(cachedTickets)
	if err := redisClient.Set(ctx, ticketsKey, ticketsPayload, time.Minute).Err(); err != nil {
		errs.Add("寫入票券快取", "%v", err)
		return
	}

	service := newTicketService(tx, redisClient)
	apps, err := service.MyApplications(users.Employee.ID.String())
	if err != nil {
		errs.Add("讀取申請快取", "%v", err)
		return
	}
	if len(apps) != 1 || apps[0].ID != cachedAppID || apps[0].Status != "cached" || apps[0].Quantity != 7 {
		errs.Add("檢查申請快取", "回傳不符預期：%+v", apps)
		return
	}

	tickets, err := service.MyTickets(users.Employee.ID.String())
	if err != nil {
		errs.Add("讀取票券快取", "%v", err)
		return
	}
	if len(tickets) != 1 || tickets[0].ID != cachedTicketID || tickets[0].QRToken != "cached-ticket" {
		errs.Add("檢查票券快取", "回傳不符預期：%+v", tickets)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func TicketServiceCoversQueueEnvAndRetryHelpers(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務 helper：確認 EnableQueueFromEnv 與 retry error 判斷分支。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, _, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備 helper 測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	serviceWithoutRedis := newTicketService(tx, nil)
	t.Setenv("TICKET_QUEUE_ENABLED", "true")
	serviceWithoutRedis.EnableQueueFromEnv()
	if serviceWithoutRedis.queue.Enabled {
		errs.Add("無 Redis 啟用 queue", "預期 redis nil 時 queue 仍為 disabled，實際 %+v", serviceWithoutRedis.queue)
		return
	}

	redisClient, err := openTicketServiceTestRedis(t)
	if err != nil {
		errs.Add("準備 helper Redis", "%v", err)
		return
	}
	serviceWithRedis := newTicketService(tx, redisClient)
	t.Setenv("TICKET_QUEUE_STREAM", "edge-stream")
	serviceWithRedis.EnableQueueFromEnv()
	if !serviceWithRedis.queue.Enabled || serviceWithRedis.queue.Stream != "edge-stream" {
		errs.Add("有 Redis 啟用 queue", "queue 設定不符預期：%+v", serviceWithRedis.queue)
		return
	}

	if !isRetryableApplyError(gorm.ErrInvalidTransaction) {
		errs.Add("retryable gorm error", "預期 ErrInvalidTransaction 是 retryable")
		return
	}
	if !isRetryableApplyError(&pgconn.PgError{Code: "40001"}) {
		errs.Add("retryable pg error", "預期 40001 是 retryable")
		return
	}
	if isRetryableApplyError(&pgconn.PgError{Code: "23505"}) {
		errs.Add("non-retryable pg error", "預期 23505 不是 retryable")
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func TicketServiceCheckinRejectsMissingAndInvalidTokens(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務核銷防呆：確認不合法 UUID 與不存在 QR token 會回傳正確錯誤。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備核銷防呆測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	_, err = service.Checkin(CheckinRequest{QRToken: "not-a-uuid"}, users.Manager.ID)
	if assertErr := assertTicketServiceAppError(err, http.StatusBadRequest, "VALIDATION_ERROR"); assertErr != nil {
		errs.Add("核銷不合法 UUID", "%v", assertErr)
		return
	}

	_, err = service.Checkin(CheckinRequest{QRToken: uuid.New().String()}, users.Manager.ID)
	if assertErr := assertTicketServiceAppError(err, http.StatusNotFound, "NOT_FOUND"); assertErr != nil {
		errs.Add("核銷不存在 QR token", "%v", assertErr)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}
