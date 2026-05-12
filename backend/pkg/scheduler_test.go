package pkg

import (
	"fmt"
	"testing"
	"time"

	utils "ticketing-system/backend/test_utils"
)

func TestEventStateScheduler(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試新建立的活動會依時間表自動更新狀態",
			Target:      EventStateSchedulerTransitionsEventBySchedule,
		},
	}

	utils.RunTestTasks(t, tasks)
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

	StartEventStateScheduler(db, nil, 100*time.Millisecond)

	for _, check := range schedulerTransitionChecks() {
		check := check
		failed := false
		t.Run(check.name, func(t *testing.T) {
			t.Logf("檢查活動狀態排程：%s", check.progress)
			utils.PrintTestProgress(fmt.Sprintf("- %s\n", check.progress))

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
