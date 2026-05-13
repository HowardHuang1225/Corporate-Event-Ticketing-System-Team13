package scheduler

import (
	"fmt"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	eventsvc "ticketing-system/backend/service/event"
	utils "ticketing-system/backend/test_utils"

	"gorm.io/gorm"
)

var schedulerEventTestModels = []any{&model.User{}, &model.Event{}, &model.TicketType{}}

func seedSchedulerEventManager(db *gorm.DB) (model.User, error) {
	suffix := utils.UniqueTestSuffix()
	manager := model.User{
		EmployeeID:   fmt.Sprintf("SCHMGR%s", suffix),
		Name:         "活動排程測試管理者",
		Email:        fmt.Sprintf("scheduler-manager-%s@example.com", suffix),
		Department:   "Events",
		Region:       "Tainan",
		Role:         "event_manager",
		PasswordHash: "",
		IsActive:     true,
	}
	users, err := utils.SeedTestRole(db, []model.User{manager}, true)
	if err != nil {
		return model.User{}, err
	}
	return users[0], nil
}

func createScheduledEventThroughService(db *gorm.DB, manager model.User, now time.Time) (model.Event, error) {
	repos := repository.New(db, nil)
	service := eventsvc.New(repos)

	return service.Create(eventsvc.CreateRequest{
		Title:               fmt.Sprintf("活動狀態排程測試 %s", utils.UniqueTestSuffix()),
		Description:         "確認新建立活動會依時間自動更新狀態",
		Venue:               "主會場",
		PublishTime:         now.Add(1 * time.Second),
		StartTime:           now.Add(2 * time.Second),
		ApplyDeadline:       now.Add(4 * time.Second),
		EndTime:             now.Add(6 * time.Second),
		MaxTicketsPerPerson: 1,
		TicketTypes: []struct {
			Name       string `json:"name" binding:"required"`
			TotalQuota int    `json:"total_quota" binding:"required,min=1"`
		}{
			{Name: "一般票", TotalQuota: 10},
		},
	}, manager.ID)
}

func waitForScheduledEventStatus(db *gorm.DB, eventID string, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastStatus := ""

	for time.Now().Before(deadline) {
		if err := queryScheduledEventStatus(db, eventID, &lastStatus); err != nil {
			return err
		}
		if lastStatus == want {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("等待活動狀態變成 %q 超時，最後查到 %q", want, lastStatus)
}

func assertScheduledEventStatus(db *gorm.DB, eventID string, want string) error {
	got := ""
	if err := queryScheduledEventStatus(db, eventID, &got); err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("expected event status %q, got %q", want, got)
	}
	return nil
}

func queryScheduledEventStatus(db *gorm.DB, eventID string, dest *string) error {
	return db.Raw("SELECT status FROM events WHERE id = ?", eventID).Scan(dest).Error
}

func cleanupScheduledEventTestData(t *testing.T, db *gorm.DB, manager model.User, event model.Event) {
	t.Helper()

	t.Cleanup(func() {
		_ = db.Delete(&model.TicketType{}, "event_id = ?", event.ID).Error
		_ = db.Delete(&model.Event{}, "id = ?", event.ID).Error
		_ = db.Delete(&model.User{}, "id = ?", manager.ID).Error
	})
}
