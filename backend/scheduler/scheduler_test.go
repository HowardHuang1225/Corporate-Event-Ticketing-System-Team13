package scheduler

import (
	"fmt"
	"testing"
	"time"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"gorm.io/gorm"
)

var publishingWorkerTestModels = []any{
	&model.User{},
	&model.Event{},
}

func TestPublishingWorker(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試 publishing worker 只會發布已到 publish_time 的 draft 活動",
			Target:      CheckAndPublishPublishesDueDraftsOnly,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func TestEventStateScheduler(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試新建立的活動會依時間表自動更新狀態",
			Target:      EventStateSchedulerTransitionsEventBySchedule,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func CheckAndPublishPublishesDueDraftsOnly(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試 publishing worker 只會發布已到 publish_time 的 draft 活動\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupPublishingWorkerTest(t)
	if err != nil {
		errs.Add("準備 publishing worker 測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	now := time.Now().UTC().Truncate(time.Second)
	dueDraft, err := seedPublishingWorkerEvent(tx, manager, "draft", now.Add(-time.Hour))
	if err != nil {
		errs.Add("建立已到發布時間的 draft 活動", "%v", err)
		return
	}
	futureDraft, err := seedPublishingWorkerEvent(tx, manager, "draft", now.Add(time.Hour))
	if err != nil {
		errs.Add("建立尚未到發布時間的 draft 活動", "%v", err)
		return
	}
	publishedEvent, err := seedPublishingWorkerEvent(tx, manager, "published", now.Add(-time.Hour))
	if err != nil {
		errs.Add("建立已發布活動", "%v", err)
		return
	}
	closedEvent, err := seedPublishingWorkerEvent(tx, manager, "closed", now.Add(-time.Hour))
	if err != nil {
		errs.Add("建立已關閉活動", "%v", err)
		return
	}

	checkAndPublish(tx)

	tests := []struct {
		name       string
		event      model.Event
		wantStatus string
		progress   string
	}{
		{
			name:       "已到發布時間的 draft",
			event:      dueDraft,
			wantStatus: "published",
			progress:   "publish_time 已早於現在且狀態為 draft 的活動，應被更新為 published。",
		},
		{
			name:       "尚未到發布時間的 draft",
			event:      futureDraft,
			wantStatus: "draft",
			progress:   "publish_time 還在未來的 draft 活動，應維持 draft。",
		},
		{
			name:       "已發布活動",
			event:      publishedEvent,
			wantStatus: "published",
			progress:   "已經是 published 的活動，即使 publish_time 已到，也不應被 publishing worker 改成其他狀態。",
		},
		{
			name:       "已關閉活動",
			event:      closedEvent,
			wantStatus: "closed",
			progress:   "非 draft 活動不應被 publishing worker 更新。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			got, err := publishingWorkerEventStatus(tx, tt.event.ID.String())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if got != tt.wantStatus {
				errs.Add(tt.progress, "預期狀態為 %q，實際為 %q", tt.wantStatus, got)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func EventStateSchedulerTransitionsEventBySchedule(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("活動狀態排程：確認新建立的活動會依 publish_time、apply_deadline、end_time 自動從 draft 更新為 published、closed、ended。\n")
	utils.PrintTestProgress("==================================================\n")

	db, err := utils.OpenTestDB(t, schedulerEventTestModels)
	if err != nil {
		errs.Add("準備活動狀態排程測試資料庫", "%v", err)
		return
	}

	manager, err := seedSchedulerEventManager(db)
	if err != nil {
		errs.Add("建立活動狀態排程測試管理者", "%v", err)
		return
	}

	now := time.Now().UTC()
	event, err := createScheduledEventThroughService(db, manager, now)
	if err != nil {
		errs.Add("透過活動服務建立排程活動", "%v", err)
		return
	}
	cleanupScheduledEventTestData(t, db, manager, event)

	if err := assertScheduledEventStatus(db, event.ID.String(), "draft"); err != nil {
		errs.Add("確認新建立活動初始狀態", "%v", err)
		return
	}

	StartEventSchedulersWithInterval(db, nil, 100*time.Millisecond)

	for _, check := range schedulerTransitionChecks() {
		check := check
		failed := false
		t.Run(check.name, func(t *testing.T) {
			t.Logf("子測試：%s", check.progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", check.progress))

			if err := waitForScheduledEventStatus(db, event.ID.String(), check.wantStatus, check.timeout); err != nil {
				errs.Add(check.progress, "%v", err)
				failed = true
				t.Fail()
			}
		})
		if failed {
			return
		}
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func setupPublishingWorkerTest(t *testing.T) (*gorm.DB, model.User, func(), error) {
	t.Helper()

	tx, cleanup, err := utils.BeginTestTransaction(t, publishingWorkerTestModels)
	if err != nil {
		return nil, model.User{}, nil, err
	}

	suffix := utils.UniqueTestSuffix()
	users, err := utils.SeedTestRole(tx, []model.User{
		{
			EmployeeID:   fmt.Sprintf("PUBMGR%s", suffix),
			Name:         "發布排程測試管理者",
			Email:        fmt.Sprintf("publishing-worker-manager-%s@example.com", suffix),
			Department:   "Events",
			Region:       "Tainan",
			Role:         "event_manager",
			PasswordHash: "",
			IsActive:     true,
		},
	}, true)
	if err != nil {
		_ = tx.Rollback()
		return nil, model.User{}, nil, err
	}

	return tx, users[0], cleanup, nil
}

func seedPublishingWorkerEvent(db *gorm.DB, manager model.User, status string, publishTime time.Time) (model.Event, error) {
	event := model.Event{
		Title:               fmt.Sprintf("發布排程測試活動 %s", utils.UniqueTestSuffix()),
		Description:         "發布排程測試資料",
		Venue:               "測試場地",
		PublishTime:         publishTime,
		StartTime:           publishTime.Add(2 * time.Hour),
		ApplyDeadline:       publishTime.Add(3 * time.Hour),
		EndTime:             publishTime.Add(4 * time.Hour),
		Status:              status,
		MaxTicketsPerPerson: 1,
		CreatedBy:           manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, fmt.Errorf("建立發布排程測試活動失敗：%w", err)
	}
	return event, nil
}

func publishingWorkerEventStatus(db *gorm.DB, eventID string) (string, error) {
	var status string
	if err := db.Model(&model.Event{}).
		Select("status").
		Where("id = ?", eventID).
		Scan(&status).Error; err != nil {
		return "", fmt.Errorf("查詢活動狀態失敗：%w", err)
	}
	return status, nil
}
