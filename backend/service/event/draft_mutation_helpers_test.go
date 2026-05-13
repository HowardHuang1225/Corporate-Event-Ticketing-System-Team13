package event

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	"ticketing-system/backend/service/apperror"
	utils "ticketing-system/backend/test_utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var eventServiceDraftMutationTestModels = []any{
	&model.User{},
	&model.Event{},
	&model.TicketType{},
}

type expectedEventServiceTicketType struct {
	Name       string
	TotalQuota int
	Remaining  int
}

func setupEventServiceDraftMutationTest(t *testing.T) (*gorm.DB, model.User, func(), error) {
	t.Helper()

	tx, cleanup, err := utils.BeginTestTransaction(t, eventServiceDraftMutationTestModels)
	if err != nil {
		return nil, model.User{}, nil, err
	}

	suffix := utils.UniqueTestSuffix()
	manager := model.User{
		EmployeeID:   fmt.Sprintf("SVCEVT%s", suffix),
		Name:         "活動服務測試管理者",
		Email:        fmt.Sprintf("event-service-manager-%s@example.com", suffix),
		Department:   "Events",
		Region:       "Tainan",
		Role:         "event_manager",
		PasswordHash: "",
		IsActive:     true,
	}
	users, err := utils.SeedTestRole(tx, []model.User{manager}, true)
	if err != nil {
		_ = tx.Rollback()
		return nil, model.User{}, nil, err
	}

	return tx, users[0], cleanup, nil
}

func newEventServiceForTest(db *gorm.DB) *Service {
	return New(repository.New(db, nil))
}

func seedEventServiceEventWithTicketTypes(db *gorm.DB, manager model.User, status string) (model.Event, []model.TicketType, error) {
	now := time.Now().UTC().Truncate(time.Second)
	region := "Tainan"
	event := model.Event{
		Title:               fmt.Sprintf("活動服務測試活動 %s", utils.UniqueTestSuffix()),
		Description:         "活動服務測試資料",
		Venue:               "測試場地",
		ImageURL:            "https://example.com/event-service.png",
		PublishTime:         now.Add(24 * time.Hour),
		StartTime:           now.Add(72 * time.Hour),
		ApplyDeadline:       now.Add(96 * time.Hour),
		EndTime:             now.Add(120 * time.Hour),
		Status:              status,
		RegionRestriction:   &region,
		MaxTicketsPerPerson: 3,
		CreatedBy:           manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, nil, fmt.Errorf("建立活動服務測試活動失敗：%w", err)
	}

	ticketTypes := []model.TicketType{
		{EventID: event.ID, Name: "一般票", TotalQuota: 80, Remaining: 65},
		{EventID: event.ID, Name: "VIP票", TotalQuota: 20, Remaining: 12},
	}
	for index := range ticketTypes {
		if err := db.Create(&ticketTypes[index]).Error; err != nil {
			return model.Event{}, nil, fmt.Errorf("建立活動服務測試票種 %q 失敗：%w", ticketTypes[index].Name, err)
		}
	}

	if err := db.Preload("TicketTypes").First(&event, "id = ?", event.ID).Error; err != nil {
		return model.Event{}, nil, fmt.Errorf("讀取活動服務測試活動失敗：%w", err)
	}
	return event, ticketTypes, nil
}

func updatedDraftEventRequest(now time.Time) CreateRequest {
	region := "Hsinchu"
	return CreateRequest{
		Title:               "更新後活動標題",
		Description:         "更新後活動說明",
		Venue:               "更新後場地",
		PublishTime:         now.Add(2 * time.Hour),
		StartTime:           now.Add(24 * time.Hour),
		ApplyDeadline:       now.Add(36 * time.Hour),
		EndTime:             now.Add(48 * time.Hour),
		RegionRestriction:   &region,
		MaxTicketsPerPerson: 4,
		TicketTypes: []struct {
			Name       string `json:"name" binding:"required"`
			TotalQuota int    `json:"total_quota" binding:"required,min=1"`
		}{
			{Name: "更新一般票", TotalQuota: 30},
			{Name: "更新眷屬票", TotalQuota: 6},
		},
	}
}

func invalidTimelineDraftEventRequest(now time.Time) CreateRequest {
	req := updatedDraftEventRequest(now)
	req.PublishTime = now.Add(72 * time.Hour)
	req.StartTime = now.Add(24 * time.Hour)
	return req
}

func eventServiceExpectedTicketTypes(req CreateRequest) []expectedEventServiceTicketType {
	expected := make([]expectedEventServiceTicketType, 0, len(req.TicketTypes))
	for _, ticketType := range req.TicketTypes {
		expected = append(expected, expectedEventServiceTicketType{
			Name:       ticketType.Name,
			TotalQuota: ticketType.TotalQuota,
			Remaining:  ticketType.TotalQuota,
		})
	}
	return expected
}

func assertEventServiceEventMatchesRequest(event model.Event, req CreateRequest) error {
	if event.Title != req.Title {
		return fmt.Errorf("預期活動標題為 %q，實際為 %q", req.Title, event.Title)
	}
	if event.Description != req.Description {
		return fmt.Errorf("預期活動說明為 %q，實際為 %q", req.Description, event.Description)
	}
	if event.Venue != req.Venue {
		return fmt.Errorf("預期活動場地為 %q，實際為 %q", req.Venue, event.Venue)
	}
	if event.MaxTicketsPerPerson != req.MaxTicketsPerPerson {
		return fmt.Errorf("預期每人上限為 %d，實際為 %d", req.MaxTicketsPerPerson, event.MaxTicketsPerPerson)
	}
	if req.RegionRestriction == nil {
		if event.RegionRestriction != nil {
			return fmt.Errorf("預期活動沒有廠區限制，實際為 %q", *event.RegionRestriction)
		}
		return nil
	}
	if event.RegionRestriction == nil || *event.RegionRestriction != *req.RegionRestriction {
		return fmt.Errorf("預期廠區限制為 %q，實際為 %v", *req.RegionRestriction, event.RegionRestriction)
	}
	return nil
}

func assertEventServiceTicketTypesMatch(got []model.TicketType, want []expectedEventServiceTicketType) error {
	if len(got) != len(want) {
		return fmt.Errorf("預期票種數量為 %d，實際為 %d", len(want), len(got))
	}

	byName := map[string]model.TicketType{}
	for _, ticketType := range got {
		byName[ticketType.Name] = ticketType
	}
	for _, expected := range want {
		gotTicketType, ok := byName[expected.Name]
		if !ok {
			return fmt.Errorf("預期票種 %q 存在", expected.Name)
		}
		if gotTicketType.TotalQuota != expected.TotalQuota {
			return fmt.Errorf("預期票種 %q total_quota 為 %d，實際為 %d", expected.Name, expected.TotalQuota, gotTicketType.TotalQuota)
		}
		if gotTicketType.Remaining != expected.Remaining {
			return fmt.Errorf("預期票種 %q remaining 為 %d，實際為 %d", expected.Name, expected.Remaining, gotTicketType.Remaining)
		}
	}
	return nil
}

func assertEventServiceTicketTypesDeleted(db *gorm.DB, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	var count int64
	if err := db.Model(&model.TicketType{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
		return fmt.Errorf("查詢舊票種是否刪除失敗：%w", err)
	}
	if count != 0 {
		return fmt.Errorf("預期舊票種都已刪除，實際仍有 %d 筆", count)
	}
	return nil
}

func assertEventServiceEventUnchanged(db *gorm.DB, original model.Event, originalTicketTypes []model.TicketType) error {
	persisted, err := eventServiceEventByID(db, original.ID.String())
	if err != nil {
		return err
	}
	if persisted.Title != original.Title || persisted.Status != original.Status || persisted.Venue != original.Venue {
		return fmt.Errorf("預期活動基本欄位維持不變，實際 title=%q status=%q venue=%q", persisted.Title, persisted.Status, persisted.Venue)
	}
	if persisted.MaxTicketsPerPerson != original.MaxTicketsPerPerson {
		return fmt.Errorf("預期每人上限維持 %d，實際為 %d", original.MaxTicketsPerPerson, persisted.MaxTicketsPerPerson)
	}
	if err := assertEventServiceTicketTypesByIDMatch(persisted.TicketTypes, originalTicketTypes); err != nil {
		return err
	}
	return nil
}

func assertEventServiceTicketTypesByIDMatch(got []model.TicketType, want []model.TicketType) error {
	if len(got) != len(want) {
		return fmt.Errorf("預期票種數量維持 %d，實際為 %d", len(want), len(got))
	}
	byID := map[uuid.UUID]model.TicketType{}
	for _, ticketType := range got {
		byID[ticketType.ID] = ticketType
	}
	for _, expected := range want {
		gotTicketType, ok := byID[expected.ID]
		if !ok {
			return fmt.Errorf("預期原票種 %s 仍存在", expected.ID)
		}
		if gotTicketType.Name != expected.Name || gotTicketType.TotalQuota != expected.TotalQuota || gotTicketType.Remaining != expected.Remaining {
			return fmt.Errorf("預期原票種 %s 維持不變", expected.ID)
		}
	}
	return nil
}

func eventServiceTicketTypeIDs(ticketTypes []model.TicketType) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(ticketTypes))
	for _, ticketType := range ticketTypes {
		ids = append(ids, ticketType.ID)
	}
	return ids
}

func eventServiceEventByID(db *gorm.DB, id string) (model.Event, error) {
	var event model.Event
	if err := db.Preload("TicketTypes").First(&event, "id = ?", id).Error; err != nil {
		return model.Event{}, fmt.Errorf("查詢活動 %s 失敗：%w", id, err)
	}
	return event, nil
}

func eventServiceEventExists(db *gorm.DB, id string) (bool, error) {
	var count int64
	if err := db.Model(&model.Event{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, fmt.Errorf("查詢活動是否存在失敗：%w", err)
	}
	return count > 0, nil
}

func eventServiceTicketTypeCountByEvent(db *gorm.DB, eventID uuid.UUID) (int64, error) {
	var count int64
	if err := db.Model(&model.TicketType{}).Where("event_id = ?", eventID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("查詢活動票種數量失敗：%w", err)
	}
	return count, nil
}

func assertEventServiceAppError(err error, wantStatus int, wantCode string) error {
	if err == nil {
		return fmt.Errorf("預期錯誤代碼 %q，實際沒有錯誤", wantCode)
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		return fmt.Errorf("預期 apperror.Error，實際錯誤為 %T：%v", err, err)
	}
	if appErr.Status != wantStatus {
		return fmt.Errorf("預期 HTTP 狀態 %d，實際為 %d", wantStatus, appErr.Status)
	}
	if appErr.Code != wantCode {
		return fmt.Errorf("預期錯誤代碼 %q，實際為 %q", wantCode, appErr.Code)
	}
	return nil
}
