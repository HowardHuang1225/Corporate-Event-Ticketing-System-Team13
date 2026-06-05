package event

import (
	"net/http"
	"testing"
	"time"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestEventServiceOperations(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試建立活動會寫入 draft event 與 ticket types，且預設每人票券上限為 1",
			Target:      CreateEventPersistsDraftAndTicketTypes,
		},
		{
			Description: "測試活動查詢會回傳清單與單筆資料，找不到時回傳 NOT_FOUND",
			Target:      ListAndGetEventsReturnPersistedRecords,
		},
		{
			Description: "測試發布與關閉活動會更新狀態並拒絕非法發布",
			Target:      PublishAndCloseEventMutations,
		},
		{
			Description: "測試活動申請資格會依狀態、截止時間與不存在活動回傳結果",
			Target:      CheckEligibilityHandlesStateDeadlineAndMissingEvent,
		},
		{
			Description: "測試 event cache TTL 與 cache key helper 的 fallback 與穩定性",
			Target:      EventCacheHelpersReturnExpectedValues,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func CreateEventPersistsDraftAndTicketTypes(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("建立活動：確認 Create 會建立 draft 活動、票種 remaining 等於 total quota，且 max_tickets_per_person 預設為 1。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupEventServiceDraftMutationTest(t)
	if err != nil {
		errs.Add("準備建立活動測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newEventServiceForTest(tx)
	now := time.Now().UTC().Truncate(time.Second)
	req := CreateRequest{
		Title:         "服務層建立活動",
		Description:   "建立活動測試",
		Venue:         "台北總部",
		PublishTime:   now.Add(time.Hour),
		StartTime:     now.Add(24 * time.Hour),
		ApplyDeadline: now.Add(12 * time.Hour),
		EndTime:       now.Add(36 * time.Hour),
		TicketTypes: []struct {
			Name       string `json:"name" binding:"required"`
			TotalQuota int    `json:"total_quota" binding:"required,min=1"`
		}{
			{Name: "一般票", TotalQuota: 40},
			{Name: "眷屬票", TotalQuota: 10},
		},
	}

	created, err := service.Create(req, manager.ID)
	if err != nil {
		errs.Add("建立活動", "%v", err)
		return
	}
	if created.ID == uuid.Nil || created.Status != "draft" || created.CreatedBy != manager.ID {
		errs.Add("建立活動基本欄位", "建立結果不符預期：%+v", created)
		return
	}
	if created.MaxTicketsPerPerson != 1 {
		errs.Add("預設每人票券上限", "預期 1，實際為 %d", created.MaxTicketsPerPerson)
		return
	}
	if err := assertEventServiceTicketTypesMatch(created.TicketTypes, eventServiceExpectedTicketTypes(req)); err != nil {
		errs.Add("建立活動票種", "%v", err)
		return
	}

	persisted, err := eventServiceEventByID(tx, created.ID.String())
	if err != nil {
		errs.Add("查詢建立後活動", "%v", err)
		return
	}
	if persisted.Title != req.Title || persisted.Status != "draft" {
		errs.Add("建立後活動資料", "persisted=%+v", persisted)
		return
	}

	_, err = service.Create(invalidTimelineDraftEventRequest(now), manager.ID)
	if assertErr := assertEventServiceAppError(err, http.StatusBadRequest, "VALIDATION_ERROR"); assertErr != nil {
		errs.Add("拒絕非法時間建立活動", "%v", assertErr)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func ListAndGetEventsReturnPersistedRecords(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("查詢活動：確認 List 會套用 repository 查詢並回傳資料，Get 找不到時回傳 NOT_FOUND。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupEventServiceDraftMutationTest(t)
	if err != nil {
		errs.Add("準備查詢活動測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newEventServiceForTest(tx)
	event, _, err := seedEventServiceEventWithTicketTypes(tx, manager, "published")
	if err != nil {
		errs.Add("建立查詢活動資料", "%v", err)
		return
	}

	events, err := service.List("published", "event_manager", "", "", "")
	if err != nil {
		errs.Add("List published 活動", "%v", err)
		return
	}
	if !eventServiceContainsEvent(events, event.ID) {
		errs.Add("List 查詢結果", "預期包含活動 %s，實際為 %+v", event.ID, events)
		return
	}

	got, err := service.Get(event.ID.String())
	if err != nil {
		errs.Add("Get 活動", "%v", err)
		return
	}
	if got.ID != event.ID || got.Title != event.Title || len(got.TicketTypes) == 0 {
		errs.Add("Get 活動結果", "結果不符預期：%+v", got)
		return
	}

	_, err = service.Get(uuid.New().String())
	if assertErr := assertEventServiceAppError(err, http.StatusNotFound, "NOT_FOUND"); assertErr != nil {
		errs.Add("Get 不存在活動", "%v", assertErr)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func PublishAndCloseEventMutations(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("發布與關閉活動：確認 draft 可發布、已發布可關閉，且非 draft 不可發布。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupEventServiceDraftMutationTest(t)
	if err != nil {
		errs.Add("準備發布關閉測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newEventServiceForTest(tx)
	draftEvent, _, err := seedEventServiceEventWithTicketTypes(tx, manager, "draft")
	if err != nil {
		errs.Add("建立 draft 活動", "%v", err)
		return
	}

	published, err := service.Publish(draftEvent.ID.String())
	if err != nil {
		errs.Add("發布 draft 活動", "%v", err)
		return
	}
	if published.Status != "published" {
		errs.Add("發布活動狀態", "預期 published，實際為 %q", published.Status)
		return
	}
	persisted, err := eventServiceEventByID(tx, draftEvent.ID.String())
	if err != nil {
		errs.Add("查詢發布後活動", "%v", err)
		return
	}
	if persisted.Status != "published" {
		errs.Add("發布後 DB 狀態", "預期 published，實際為 %q", persisted.Status)
		return
	}

	closed, err := service.Close(draftEvent.ID.String())
	if err != nil {
		errs.Add("關閉活動", "%v", err)
		return
	}
	if closed.Status != "closed" {
		errs.Add("關閉活動狀態", "預期 closed，實際為 %q", closed.Status)
		return
	}

	_, err = service.Publish(draftEvent.ID.String())
	if assertErr := assertEventServiceAppError(err, http.StatusBadRequest, "INVALID_STATUS"); assertErr != nil {
		errs.Add("拒絕非 draft 發布", "%v", assertErr)
		return
	}
	_, err = service.Publish(uuid.New().String())
	if assertErr := assertEventServiceAppError(err, http.StatusNotFound, "NOT_FOUND"); assertErr != nil {
		errs.Add("發布不存在活動", "%v", assertErr)
		return
	}
	_, err = service.Close(uuid.New().String())
	if assertErr := assertEventServiceAppError(err, http.StatusNotFound, "NOT_FOUND"); assertErr != nil {
		errs.Add("關閉不存在活動", "%v", assertErr)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func CheckEligibilityHandlesStateDeadlineAndMissingEvent(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("申請資格：確認 published 且未截止才 eligible，closed、逾期與不存在活動會被拒絕。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupEventServiceDraftMutationTest(t)
	if err != nil {
		errs.Add("準備資格檢查測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newEventServiceForTest(tx)
	publishedEvent, _, err := seedEventServiceEventWithTicketTypes(tx, manager, "published")
	if err != nil {
		errs.Add("建立 published 活動", "%v", err)
		return
	}
	eligible, err := service.CheckEligibility(publishedEvent.ID.String(), uuid.New())
	if err != nil {
		errs.Add("查詢可申請活動資格", "%v", err)
		return
	}
	if !eligible.Eligible || eligible.Reason != "" {
		errs.Add("可申請活動資格", "預期 eligible，實際為 %+v", eligible)
		return
	}

	closedEvent, _, err := seedEventServiceEventWithTicketTypes(tx, manager, "closed")
	if err != nil {
		errs.Add("建立 closed 活動", "%v", err)
		return
	}
	notAccepting, err := service.CheckEligibility(closedEvent.ID.String(), uuid.New())
	if err != nil {
		errs.Add("查詢 closed 活動資格", "%v", err)
		return
	}
	if notAccepting.Eligible || notAccepting.Reason != "Event is not accepting applications" {
		errs.Add("closed 活動資格", "結果不符預期：%+v", notAccepting)
		return
	}

	expiredEvent, _, err := seedEventServiceEventWithTicketTypes(tx, manager, "published")
	if err != nil {
		errs.Add("建立逾期活動", "%v", err)
		return
	}
	expiredTimeline := map[string]any{
		"publish_time":   time.Now().Add(-2 * time.Hour),
		"apply_deadline": time.Now().Add(-time.Hour),
		"start_time":     time.Now().Add(time.Hour),
		"end_time":       time.Now().Add(2 * time.Hour),
	}
	if err := tx.Session(&gorm.Session{SkipHooks: true}).
		Model(&model.Event{}).
		Where("id = ?", expiredEvent.ID).
		Updates(expiredTimeline).Error; err != nil {
		errs.Add("更新逾期活動 deadline", "%v", err)
		return
	}
	expiredService := newEventServiceForTest(tx.Session(&gorm.Session{SkipHooks: true}))
	expired, err := expiredService.CheckEligibility(expiredEvent.ID.String(), uuid.New())
	if err != nil {
		errs.Add("查詢逾期活動資格", "%v", err)
		return
	}
	if expired.Eligible || expired.Reason != "Application deadline has passed" {
		errs.Add("逾期活動資格", "結果不符預期：%+v", expired)
		return
	}

	_, err = service.CheckEligibility(uuid.New().String(), uuid.New())
	if assertErr := assertEventServiceAppError(err, http.StatusNotFound, "NOT_FOUND"); assertErr != nil {
		errs.Add("查詢不存在活動資格", "%v", assertErr)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func EventCacheHelpersReturnExpectedValues(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("cache helper：確認 TTL fallback、合法秒數與 cache key 穩定/區分查詢參數。\n")
	utils.PrintTestProgress("==================================================\n")

	t.Setenv("EVENT_LIST_CACHE_TTL_SECONDS", "")
	if got := eventListCacheTTL(); got != 0 {
		errs.Add("空 TTL", "預期 0，實際為 %s", got)
		return
	}
	t.Setenv("EVENT_LIST_CACHE_TTL_SECONDS", "bad")
	if got := eventListCacheTTL(); got != 0 {
		errs.Add("非法 TTL", "預期 0，實際為 %s", got)
		return
	}
	t.Setenv("EVENT_LIST_CACHE_TTL_SECONDS", "-1")
	if got := eventListCacheTTL(); got != 0 {
		errs.Add("負數 TTL", "預期 0，實際為 %s", got)
		return
	}
	t.Setenv("EVENT_LIST_CACHE_TTL_SECONDS", "15")
	if got := eventListCacheTTL(); got != 15*time.Second {
		errs.Add("合法 TTL", "預期 15s，實際為 %s", got)
		return
	}

	keyA := eventListCacheKey("published", "employee", "VIP", "2026-01-01", "2026-12-31")
	keyB := eventListCacheKey("published", "employee", "VIP", "2026-01-01", "2026-12-31")
	keyC := eventListCacheKey("closed", "employee", "VIP", "2026-01-01", "2026-12-31")
	if keyA != keyB || keyA == keyC {
		errs.Add("cache key 穩定性", "keyA=%q keyB=%q keyC=%q", keyA, keyB, keyC)
		return
	}
	if len(keyA) <= len("event:list:") || keyA[:len("event:list:")] != "event:list:" {
		errs.Add("cache key prefix", "key=%q", keyA)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func eventServiceContainsEvent(events []model.Event, id uuid.UUID) bool {
	for _, event := range events {
		if event.ID == id {
			return true
		}
	}
	return false
}
