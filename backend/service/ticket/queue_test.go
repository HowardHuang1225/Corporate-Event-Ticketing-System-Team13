package ticket

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/service/apperror"
	utils "ticketing-system/backend/test_utils"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func TestTicketQueue(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試 queue 設定會從環境變數載入並套用 fallback",
			Target:      QueueOptionsLoadFromEnvironment,
		},
		{
			Description: "測試 queue 純函式會正確轉換 key、狀態、錯誤與 stream message",
			Target:      QueueHelpersMapStatusMessagesAndErrors,
		},
		{
			Description: "測試 queue 申請會建立 Redis waiting-room 狀態且重複送出維持同一筆 queue",
			Target:      ApplyQueuedCreatesRedisReservationAndIsIdempotent,
		},
		{
			Description: "測試 queue 狀態查詢會先讀 Redis，沒有熱資料時回退到 DB",
			Target:      QueueStatusReadsRedisThenDatabase,
		},
		{
			Description: "測試 queue worker 會把有效訊息轉成 approved application 與票券",
			Target:      QueueWorkerProcessesMessageIntoApprovedApplication,
		},
		{
			Description: "測試 queue 申請會拒絕無效輸入、滿載 waiting room 與不可申請活動",
			Target:      ApplyQueuedRejectsInvalidAndUnavailableRequests,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func QueueOptionsLoadFromEnvironment(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("queue 環境設定：確認合法值會覆蓋預設值，非法數字與未知布林會使用 fallback。\n")
	utils.PrintTestProgress("==================================================\n")

	t.Setenv("TICKET_QUEUE_ENABLED", "yes")
	t.Setenv("TICKET_QUEUE_STREAM", "custom-stream")
	t.Setenv("TICKET_QUEUE_GROUP", "custom-group")
	t.Setenv("TICKET_QUEUE_CONSUMER_PREFIX", "consumer")
	t.Setenv("TICKET_QUEUE_WORKERS", "4")
	t.Setenv("TICKET_QUEUE_RESERVATION_TTL_SECONDS", "30")
	t.Setenv("TICKET_QUEUE_MAX_WAITING", "12")
	t.Setenv("TICKET_QUEUE_READ_BLOCK_SECONDS", "2")
	t.Setenv("TICKET_QUEUE_READ_COUNT", "3")
	t.Setenv("TICKET_QUEUE_PROCESS_ATTEMPTS", "5")
	t.Setenv("TICKET_QUEUE_STATUS_TTL_SECONDS", "90")

	options := loadQueueOptionsFromEnv()
	if !options.Enabled || options.Stream != "custom-stream" || options.Group != "custom-group" || options.ConsumerPrefix != "consumer" {
		errs.Add("載入 queue 字串與布林設定", "設定未正確載入：%+v", options)
		return
	}
	if options.Workers != 4 || options.ReservationTTL != 30*time.Second || options.MaxWaiting != 12 ||
		options.ReadBlock != 2*time.Second || options.ReadCount != 3 || options.MaxProcessAttempts != 5 || options.StatusTTL != 90*time.Second {
		errs.Add("載入 queue 數值設定", "數值設定未正確載入：%+v", options)
		return
	}

	if got := getEnvString("MISSING_QUEUE_STRING", "fallback"); got != "fallback" {
		errs.Add("字串 fallback", "預期 fallback，實際為 %q", got)
		return
	}
	t.Setenv("BAD_QUEUE_INT", "not-a-number")
	if got := getEnvInt("BAD_QUEUE_INT", 7); got != 7 {
		errs.Add("整數 fallback", "預期 7，實際為 %d", got)
		return
	}
	t.Setenv("UNKNOWN_QUEUE_BOOL", "maybe")
	if got := getEnvBool("UNKNOWN_QUEUE_BOOL", true); !got {
		errs.Add("布林 fallback", "預期未知布林值會回傳 fallback=true")
		return
	}
	t.Setenv("FALSE_QUEUE_BOOL", "off")
	if got := getEnvBool("FALSE_QUEUE_BOOL", true); got {
		errs.Add("布林 false value", "預期 off 會被解析為 false")
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func QueueHelpersMapStatusMessagesAndErrors(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("queue helper：確認 key、status mapping、message parsing 與錯誤代碼轉換。\n")
	utils.PrintTestProgress("==================================================\n")

	userID := uuid.New()
	eventID := uuid.New()
	ticketTypeID := uuid.New()
	req := ApplyRequest{
		EventID:        eventID.String(),
		TicketTypeID:   ticketTypeID.String(),
		Quantity:       2,
		IdempotencyKey: "queue-helper-" + utils.UniqueTestSuffix(),
	}
	queueID := uuid.New()
	fields := map[string]string{
		"queue_id": queueID.String(),
		"quantity": "3",
		"status":   "processing",
	}

	app := queuedApplicationFromStatus(userID, eventID, ticketTypeID, req, fields)
	if app.ID != queueID || app.Quantity != 3 || app.Status != "processing" || app.IdempotencyKey != req.IdempotencyKey {
		errs.Add("status 轉 application", "轉換結果不符預期：%+v", app)
		return
	}

	fallbackApp := queuedApplicationFromStatus(userID, eventID, ticketTypeID, req, map[string]string{
		"queue_id": "not-a-uuid",
		"quantity": "bad",
	})
	if fallbackApp.ID == uuid.Nil || fallbackApp.Quantity != req.Quantity || fallbackApp.Status != "queued" {
		errs.Add("status fallback", "fallback 結果不符預期：%+v", fallbackApp)
		return
	}

	if queueStatusKey("u", "k") != "queue:status:u:k" ||
		queueReservationKey("u", "k") != "queue:reservation:u:k" ||
		queueUserEventReservationKey("u", "e") != "queue:user_event_reserved:u:e" {
		errs.Add("queue key helper", "key helper 回傳值不符預期")
		return
	}

	job, err := queueJobFromMessage(redis.XMessage{
		ID: "1-0",
		Values: map[string]any{
			"queue_id":        "qid",
			"user_id":         userID.String(),
			"event_id":        eventID.String(),
			"ticket_type_id":  ticketTypeID.String(),
			"quantity":        "2",
			"idempotency_key": req.IdempotencyKey,
		},
	})
	if err != nil || job.UserID != userID || job.EventID != eventID || job.TicketTypeID != ticketTypeID ||
		job.Quantity != 2 || job.IdempotencyKey != req.IdempotencyKey || job.QueueID != "qid" {
		errs.Add("stream message 轉 job", "job=%+v err=%v", job, err)
		return
	}

	invalidMessages := []redis.XMessage{
		{ID: "bad-user", Values: map[string]any{"user_id": "bad"}},
		{ID: "bad-event", Values: map[string]any{"user_id": userID.String(), "event_id": "bad"}},
		{ID: "bad-ticket-type", Values: map[string]any{"user_id": userID.String(), "event_id": eventID.String(), "ticket_type_id": "bad"}},
		{ID: "bad-quantity", Values: map[string]any{"user_id": userID.String(), "event_id": eventID.String(), "ticket_type_id": ticketTypeID.String(), "quantity": "0"}},
	}
	for _, msg := range invalidMessages {
		if _, err := queueJobFromMessage(msg); err == nil {
			errs.Add("拒絕無效 stream message", "message %s 預期失敗，實際成功", msg.ID)
			return
		}
	}

	code, message := errorCodeAndMessage(apperror.Conflict("TICKET_SOLD_OUT", "sold out"))
	if code != "TICKET_SOLD_OUT" || message != "sold out" {
		errs.Add("apperror 代碼轉換", "code=%q message=%q", code, message)
		return
	}
	code, message = errorCodeAndMessage(errors.New("plain error"))
	if code != "INTERNAL_ERROR" || message != "plain error" {
		errs.Add("一般錯誤代碼轉換", "code=%q message=%q", code, message)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func ApplyQueuedCreatesRedisReservationAndIsIdempotent(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("queue 申請：確認成功排隊會寫入 Redis status，重複送出會回傳既有 queue。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備 queue 申請測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	redisClient, err := openTicketServiceTestRedis(t)
	if err != nil {
		errs.Add("準備 queue 申請 Redis 連線", "%v", err)
		return
	}

	_, ticketType, err := seedTicketServiceEventWithTicketType(tx, users.Manager, 5, 5, 5)
	if err != nil {
		errs.Add("建立 queue 申請活動與票種", "%v", err)
		return
	}
	req := ApplyRequest{
		EventID:        ticketType.EventID.String(),
		TicketTypeID:   ticketType.ID.String(),
		Quantity:       2,
		IdempotencyKey: "queue-apply-" + utils.UniqueTestSuffix(),
	}
	stream := "ticket:test:queue:" + utils.UniqueTestSuffix()
	cleanupTicketQueueKeys(t, redisClient, users.Employee.ID.String(), req.EventID, req.TicketTypeID, req.IdempotencyKey, stream)

	service := newTicketQueueService(tx, redisClient, stream)
	result, err := service.ApplyQueued(users.Employee.ID, req)
	if err != nil {
		errs.Add("送出 queue 申請", "%v", err)
		return
	}
	if !result.Created || !result.Queued || result.QueueID == "" || result.Application.Status != "queued" {
		errs.Add("queue 申請結果", "結果不符預期：%+v", result)
		return
	}

	status, err := service.QueueStatus(users.Employee.ID, req.IdempotencyKey)
	if err != nil {
		errs.Add("查詢 Redis queue status", "%v", err)
		return
	}
	if status.Status != "queued" || status.QueueID != result.QueueID || status.Quantity != "2" || status.EventID != req.EventID {
		errs.Add("Redis queue status", "狀態不符預期：%+v", status)
		return
	}

	duplicate, err := service.ApplyQueued(users.Employee.ID, req)
	if err != nil {
		errs.Add("重複送出 queue 申請", "%v", err)
		return
	}
	if duplicate.Created || !duplicate.Queued || duplicate.QueueID != result.QueueID || duplicate.Application.ID != result.Application.ID {
		errs.Add("重複 queue 申請結果", "結果不符預期：%+v original=%+v", duplicate, result)
		return
	}

	streamLen, err := redisClient.XLen(context.Background(), stream).Result()
	if err != nil {
		errs.Add("查詢 stream 長度", "%v", err)
		return
	}
	if streamLen != 1 {
		errs.Add("重複申請不可新增 stream 訊息", "預期 stream 長度 1，實際為 %d", streamLen)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func QueueStatusReadsRedisThenDatabase(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("queue status：確認 Redis 熱資料優先，沒有熱資料時讀取 DB application。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備 queue status 測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	redisClient, err := openTicketServiceTestRedis(t)
	if err != nil {
		errs.Add("準備 queue status Redis 連線", "%v", err)
		return
	}

	event, ticketType, err := seedTicketServiceEventWithTicketType(tx, users.Manager, 3, 3, 3)
	if err != nil {
		errs.Add("建立 queue status 活動與票種", "%v", err)
		return
	}
	req := ApplyRequest{
		EventID:        event.ID.String(),
		TicketTypeID:   ticketType.ID.String(),
		Quantity:       1,
		IdempotencyKey: "queue-status-" + utils.UniqueTestSuffix(),
	}
	stream := "ticket:test:status:" + utils.UniqueTestSuffix()
	cleanupTicketQueueKeys(t, redisClient, users.Employee.ID.String(), req.EventID, req.TicketTypeID, req.IdempotencyKey, stream)

	service := newTicketQueueService(tx, redisClient, stream)
	statusKey := queueStatusKey(users.Employee.ID.String(), req.IdempotencyKey)
	if err := redisClient.HSet(context.Background(), statusKey, map[string]any{
		"status":          "processing",
		"queue_id":        uuid.New().String(),
		"application_id":  uuid.New().String(),
		"event_id":        req.EventID,
		"ticket_type_id":  req.TicketTypeID,
		"quantity":        "1",
		"reason":          "testing",
		"queued_at":       "queued-time",
		"updated_at":      "updated-time",
		"idempotency_key": req.IdempotencyKey,
	}).Err(); err != nil {
		errs.Add("建立 Redis queue status", "%v", err)
		return
	}

	hot, err := service.QueueStatus(users.Employee.ID, req.IdempotencyKey)
	if err != nil {
		errs.Add("查詢 Redis queue status", "%v", err)
		return
	}
	if hot.Status != "processing" || hot.Quantity != "1" || hot.Reason != "testing" || hot.IdempotencyKey != req.IdempotencyKey {
		errs.Add("Redis queue status 內容", "狀態不符預期：%+v", hot)
		return
	}

	if err := redisClient.Del(context.Background(), statusKey).Err(); err != nil {
		errs.Add("清除 Redis queue status", "%v", err)
		return
	}
	app := model.Application{
		UserID:         users.Employee.ID,
		EventID:        event.ID,
		TicketTypeID:   ticketType.ID,
		Quantity:       1,
		Status:         "approved",
		IdempotencyKey: req.IdempotencyKey,
	}
	if err := tx.Create(&app).Error; err != nil {
		errs.Add("建立 DB application status", "%v", err)
		return
	}

	cold, err := service.QueueStatus(users.Employee.ID, req.IdempotencyKey)
	if err != nil {
		errs.Add("查詢 DB queue status", "%v", err)
		return
	}
	if cold.Status != "approved" || cold.ApplicationID != app.ID.String() || cold.Quantity != "1" {
		errs.Add("DB queue status 內容", "狀態不符預期：%+v", cold)
		return
	}

	_, err = service.QueueStatus(users.Employee.ID, "missing-"+utils.UniqueTestSuffix())
	if assertErr := assertTicketServiceAppError(err, http.StatusNotFound, "NOT_FOUND"); assertErr != nil {
		errs.Add("查無 queue status", "%v", assertErr)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func QueueWorkerProcessesMessageIntoApprovedApplication(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("queue worker：確認有效 stream message 會建立 approved application、票券並更新 status。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備 queue worker 測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	redisClient, err := openTicketServiceTestRedis(t)
	if err != nil {
		errs.Add("準備 queue worker Redis 連線", "%v", err)
		return
	}

	event, ticketType, err := seedTicketServiceEventWithTicketType(tx, users.Manager, 4, 4, 4)
	if err != nil {
		errs.Add("建立 queue worker 活動與票種", "%v", err)
		return
	}
	idempotencyKey := "queue-worker-" + utils.UniqueTestSuffix()
	stream := "ticket:test:worker:" + utils.UniqueTestSuffix()
	cleanupTicketQueueKeys(t, redisClient, users.Employee.ID.String(), event.ID.String(), ticketType.ID.String(), idempotencyKey, stream)

	service := newTicketQueueService(tx, redisClient, stream)
	statusKey := queueStatusKey(users.Employee.ID.String(), idempotencyKey)
	if err := redisClient.HSet(context.Background(), statusKey, map[string]any{
		"status":         "queued",
		"queue_id":       uuid.New().String(),
		"user_id":        users.Employee.ID.String(),
		"event_id":       event.ID.String(),
		"ticket_type_id": ticketType.ID.String(),
		"quantity":       "2",
	}).Err(); err != nil {
		errs.Add("建立 queue worker status", "%v", err)
		return
	}
	if err := redisClient.Set(context.Background(), queueUserEventReservationKey(users.Employee.ID.String(), event.ID.String()), 2, time.Hour).Err(); err != nil {
		errs.Add("建立 queue worker user reservation", "%v", err)
		return
	}

	msg := redis.XMessage{
		ID: "1-0",
		Values: map[string]any{
			"queue_id":        uuid.New().String(),
			"user_id":         users.Employee.ID.String(),
			"event_id":        event.ID.String(),
			"ticket_type_id":  ticketType.ID.String(),
			"quantity":        "2",
			"idempotency_key": idempotencyKey,
		},
	}
	service.processQueueMessage(context.Background(), msg)

	status, err := service.QueueStatus(users.Employee.ID, idempotencyKey)
	if err != nil {
		errs.Add("查詢 queue worker status", "%v", err)
		return
	}
	if status.Status != "approved" || status.ApplicationID == "" {
		errs.Add("queue worker status", "狀態不符預期：%+v", status)
		return
	}

	app, err := ticketServiceApplicationByID(tx, uuid.MustParse(status.ApplicationID))
	if err != nil {
		errs.Add("查詢 queue worker application", "%v", err)
		return
	}
	if app.Status != "approved" || app.Quantity != 2 {
		errs.Add("queue worker application", "application 不符預期：%+v", app)
		return
	}
	ticketCount, err := ticketServiceTicketCountForApplication(tx, app.ID)
	if err != nil {
		errs.Add("查詢 queue worker 票券數量", "%v", err)
		return
	}
	if ticketCount != 2 {
		errs.Add("queue worker 票券數量", "預期 2 張，實際為 %d", ticketCount)
		return
	}
	remaining, err := ticketServiceRemaining(tx, ticketType.ID)
	if err != nil {
		errs.Add("查詢 queue worker 庫存", "%v", err)
		return
	}
	if remaining != 2 {
		errs.Add("queue worker 庫存", "預期剩餘 2，實際為 %d", remaining)
		return
	}

	invalid := redis.XMessage{ID: "bad-1", Values: map[string]any{"user_id": "bad"}}
	service.processQueueMessage(context.Background(), invalid)

	utils.PrintTestProgress("==================================================\n\n")
}

func ApplyQueuedRejectsInvalidAndUnavailableRequests(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("queue 申請防呆：確認無效輸入、無 Redis、滿載 waiting room、活動不可申請與超過上限會被拒絕。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備 queue 防呆測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	redisClient, err := openTicketServiceTestRedis(t)
	if err != nil {
		errs.Add("準備 queue 防呆 Redis 連線", "%v", err)
		return
	}

	event, ticketType, err := seedTicketServiceEventWithTicketType(tx, users.Manager, 1, 1, 1)
	if err != nil {
		errs.Add("建立 queue 防呆活動與票種", "%v", err)
		return
	}
	stream := "ticket:test:reject:" + utils.UniqueTestSuffix()
	validReq := ApplyRequest{
		EventID:        event.ID.String(),
		TicketTypeID:   ticketType.ID.String(),
		Quantity:       1,
		IdempotencyKey: "queue-reject-" + utils.UniqueTestSuffix(),
	}
	cleanupTicketQueueKeys(t, redisClient, users.Employee.ID.String(), validReq.EventID, validReq.TicketTypeID, validReq.IdempotencyKey, stream)

	service := newTicketQueueService(tx, redisClient, stream)
	tests := []struct {
		name     string
		req      ApplyRequest
		service  *Service
		wantCode string
	}{
		{
			name:     "invalid event id",
			req:      ApplyRequest{EventID: "bad", TicketTypeID: validReq.TicketTypeID, Quantity: 1, IdempotencyKey: "k"},
			service:  service,
			wantCode: "VALIDATION_ERROR",
		},
		{
			name:     "invalid ticket type id",
			req:      ApplyRequest{EventID: validReq.EventID, TicketTypeID: "bad", Quantity: 1, IdempotencyKey: "k"},
			service:  service,
			wantCode: "VALIDATION_ERROR",
		},
		{
			name:     "missing redis",
			req:      validReq,
			service:  newTicketQueueService(tx, nil, stream),
			wantCode: "BUSY",
		},
		{
			name:     "missing idempotency key",
			req:      ApplyRequest{EventID: validReq.EventID, TicketTypeID: validReq.TicketTypeID, Quantity: 1},
			service:  service,
			wantCode: "VALIDATION_ERROR",
		},
	}
	for _, tt := range tests {
		if _, err := tt.service.ApplyQueued(users.Employee.ID, tt.req); assertTicketServiceAppErrorCode(err, tt.wantCode) != nil {
			errs.Add("拒絕 "+tt.name, "預期錯誤代碼 %s，實際錯誤 %v", tt.wantCode, err)
			return
		}
	}

	if err := redisClient.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"seed": "1"},
	}).Err(); err != nil {
		errs.Add("建立滿載 waiting room 訊息", "%v", err)
		return
	}
	fullService := newTicketQueueService(tx, redisClient, stream)
	fullService.queue.MaxWaiting = 1
	if _, err := fullService.ApplyQueued(users.Employee.ID, validReq); assertTicketServiceAppError(err, http.StatusTooManyRequests, "WAITING_ROOM_FULL") != nil {
		errs.Add("拒絕滿載 waiting room", "實際錯誤 %v", err)
		return
	}

	closedEvent, closedTicketType, err := seedTicketServiceEventWithTicketType(tx, users.Manager, 1, 1, 1)
	if err != nil {
		errs.Add("建立 closed 活動資料", "%v", err)
		return
	}
	if err := tx.Model(&model.Event{}).Where("id = ?", closedEvent.ID).Update("status", "closed").Error; err != nil {
		errs.Add("更新 closed 活動狀態", "%v", err)
		return
	}
	closedReq := ApplyRequest{
		EventID:        closedEvent.ID.String(),
		TicketTypeID:   closedTicketType.ID.String(),
		Quantity:       1,
		IdempotencyKey: "queue-closed-" + utils.UniqueTestSuffix(),
	}
	cleanupTicketQueueKeys(t, redisClient, users.Employee.ID.String(), closedReq.EventID, closedReq.TicketTypeID, closedReq.IdempotencyKey, stream)
	if _, err := service.ApplyQueued(users.Employee.ID, closedReq); assertTicketServiceAppError(err, http.StatusBadRequest, "EVENT_NOT_AVAILABLE") != nil {
		errs.Add("拒絕 closed 活動", "實際錯誤 %v", err)
		return
	}

	expiredEvent, expiredTicketType, err := seedTicketServiceEventWithTicketType(tx, users.Manager, 1, 1, 1)
	if err != nil {
		errs.Add("建立過期活動資料", "%v", err)
		return
	}
	if err := tx.Model(&model.Event{}).Where("id = ?", expiredEvent.ID).Update("apply_deadline", time.Now().Add(-time.Hour)).Error; err != nil {
		errs.Add("更新過期活動 deadline", "%v", err)
		return
	}
	expiredReq := ApplyRequest{
		EventID:        expiredEvent.ID.String(),
		TicketTypeID:   expiredTicketType.ID.String(),
		Quantity:       1,
		IdempotencyKey: "queue-expired-" + utils.UniqueTestSuffix(),
	}
	cleanupTicketQueueKeys(t, redisClient, users.Employee.ID.String(), expiredReq.EventID, expiredReq.TicketTypeID, expiredReq.IdempotencyKey, stream)
	expiredService := newTicketQueueService(tx.Session(&gorm.Session{SkipHooks: true}), redisClient, stream)
	if _, err := expiredService.ApplyQueued(users.Employee.ID, expiredReq); assertTicketServiceAppError(err, http.StatusBadRequest, "APPLY_DEADLINE_PASSED") != nil {
		errs.Add("拒絕過期活動", "實際錯誤 %v", err)
		return
	}

	limitedEvent, limitedTicketType, err := seedTicketServiceEventWithTicketType(tx, users.Manager, 2, 2, 1)
	if err != nil {
		errs.Add("建立超過上限活動資料", "%v", err)
		return
	}
	limitedReq := ApplyRequest{
		EventID:        limitedEvent.ID.String(),
		TicketTypeID:   limitedTicketType.ID.String(),
		Quantity:       2,
		IdempotencyKey: "queue-limit-" + utils.UniqueTestSuffix(),
	}
	cleanupTicketQueueKeys(t, redisClient, users.Employee.ID.String(), limitedReq.EventID, limitedReq.TicketTypeID, limitedReq.IdempotencyKey, stream)
	if _, err := service.ApplyQueued(users.Employee.ID, limitedReq); assertTicketServiceAppError(err, http.StatusBadRequest, "EXCEEDS_MAX_TICKETS") != nil {
		errs.Add("拒絕超過票券上限", "實際錯誤 %v", err)
		return
	}

	missingEventReq := ApplyRequest{
		EventID:        uuid.New().String(),
		TicketTypeID:   uuid.New().String(),
		Quantity:       1,
		IdempotencyKey: "queue-missing-event-" + utils.UniqueTestSuffix(),
	}
	if _, err := service.ApplyQueued(users.Employee.ID, missingEventReq); assertTicketServiceAppError(err, http.StatusNotFound, "NOT_FOUND") != nil {
		errs.Add("拒絕不存在活動", "實際錯誤 %v", err)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func newTicketQueueService(db *gorm.DB, redisClient *redis.Client, stream string) *Service {
	service := newTicketService(db, redisClient)
	service.queue = QueueOptions{
		Enabled:            true,
		Stream:             stream,
		Group:              "ticket-test-group",
		ConsumerPrefix:     "ticket-test-worker",
		Workers:            1,
		ReservationTTL:     time.Minute,
		MaxWaiting:         100,
		ReadBlock:          time.Millisecond,
		ReadCount:          10,
		MaxProcessAttempts: 1,
		StatusTTL:          time.Minute,
	}
	return service
}

func cleanupTicketQueueKeys(t *testing.T, redisClient *redis.Client, userID string, eventID string, ticketTypeID string, idempotencyKey string, stream string) {
	t.Helper()

	keys := []string{
		queueStatusKey(userID, idempotencyKey),
		queueReservationKey(userID, idempotencyKey),
		queueUserEventReservationKey(userID, eventID),
		"inventory:" + ticketTypeID,
		"inventory_loaded:" + ticketTypeID,
		queueTicketTypeMetaKey(ticketTypeID),
		"lock:init_lock:" + ticketTypeID,
		stream,
	}
	redisClient.Del(context.Background(), keys...)
	t.Cleanup(func() {
		redisClient.Del(context.Background(), keys...)
	})
}
