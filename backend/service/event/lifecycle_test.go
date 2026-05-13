package event

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/service/apperror"
	utils "ticketing-system/backend/test_utils"
)

type timelineTestCase struct {
	name          string
	progress      string
	publishTime   time.Time
	startTime     time.Time
	applyDeadline time.Time
	endTime       time.Time
}

func TestStatusAt(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試活動狀態會依目前時間、報名截止時間與活動結束時間推算",
			Target:      StatusAtReturnsExpectedState,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func StatusAtReturnsExpectedState(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("活動狀態推算：確認 published 會在報名截止後變成 closed，closed 會在活動結束後變成 ended。\n")
	utils.PrintTestProgress("==================================================\n")

	now := time.Now().UTC().Truncate(time.Second)
	tests := []struct {
		name     string
		progress string
		event    model.Event
		want     string
	}{
		{
			name:     "草稿活動維持草稿",
			progress: "測試 draft 活動不會因時間推進而改變狀態。",
			event: model.Event{
				Status: "draft",
			},
			want: "draft",
		},
		{
			name:     "已發布活動尚未到報名截止時間",
			progress: "測試 published 活動在 apply_deadline 前仍維持 published。",
			event: model.Event{
				Status:        "published",
				ApplyDeadline: now.Add(time.Hour),
			},
			want: "published",
		},
		{
			name:     "已發布活動正好在報名截止時間",
			progress: "測試目前時間等於 apply_deadline 時仍維持 published。",
			event: model.Event{
				Status:        "published",
				ApplyDeadline: now,
			},
			want: "published",
		},
		{
			name:     "已發布活動超過報名截止時間",
			progress: "測試 published 活動超過 apply_deadline 後會變成 closed。",
			event: model.Event{
				Status:        "published",
				ApplyDeadline: now.Add(-time.Hour),
			},
			want: "closed",
		},
		{
			name:     "已發布活動沒有報名截止時間",
			progress: "測試 published 活動缺少 apply_deadline 時不會自動變成 closed。",
			event: model.Event{
				Status: "published",
			},
			want: "published",
		},
		{
			name:     "已關閉活動尚未到活動結束時間",
			progress: "測試 closed 活動在 end_time 前仍維持 closed。",
			event: model.Event{
				Status:  "closed",
				EndTime: now.Add(time.Hour),
			},
			want: "closed",
		},
		{
			name:     "已關閉活動正好在活動結束時間",
			progress: "測試目前時間等於 end_time 時仍維持 closed。",
			event: model.Event{
				Status:  "closed",
				EndTime: now,
			},
			want: "closed",
		},
		{
			name:     "已關閉活動超過活動結束時間",
			progress: "測試 closed 活動超過 end_time 後會變成 ended。",
			event: model.Event{
				Status:  "closed",
				EndTime: now.Add(-time.Hour),
			},
			want: "ended",
		},
		{
			name:     "已關閉活動沒有活動結束時間",
			progress: "測試 closed 活動缺少 end_time 時不會自動變成 ended。",
			event: model.Event{
				Status: "closed",
			},
			want: "closed",
		},
		{
			name:     "已結束活動維持已結束",
			progress: "測試 ended 活動不會因時間推算改變狀態。",
			event: model.Event{
				Status: "ended",
			},
			want: "ended",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", tt.progress))

			if got := StatusAt(tt.event, now); got != tt.want {
				errs.Add(tt.progress, "expected status %q, got %q", tt.want, got)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func TestValidateTimeline(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試活動時間驗證接受 publish_time <= (start_time, apply_deadline) <= end_time 的合法範圍",
			Target:      ValidateTimelineAcceptsValidTimeline,
		},
		{
			Description: "測試活動時間驗證拒絕一個或多個時間為空",
			Target:      ValidateTimelineRejectsMissingTimes,
		},
		{
			Description: "測試活動時間驗證拒絕不符合 publish_time <= (start_time, apply_deadline) <= end_time 的範圍",
			Target:      ValidateTimelineRejectsInvalidTimeline,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func ValidateTimelineAcceptsValidTimeline(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("活動時間驗證：確認 start_time 與 apply_deadline 只要都落在 publish_time 和 end_time 之間即可通過，兩者先後順序不限制。\n")
	utils.PrintTestProgress("==================================================\n")

	now := time.Now().UTC().Truncate(time.Second)
	runTimelineCases(t, errs, validTimelineCases(now), false)

	utils.PrintTestProgress("==================================================\n\n")
}

func ValidateTimelineRejectsMissingTimes(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("活動時間驗證：確認 publish_time、start_time、apply_deadline、end_time 任一或多個為空時都會被拒絕。\n")
	utils.PrintTestProgress("==================================================\n")

	now := time.Now().UTC().Truncate(time.Second)
	runTimelineCases(t, errs, missingTimelineCases(now), true)

	utils.PrintTestProgress("==================================================\n\n")
}

func ValidateTimelineRejectsInvalidTimeline(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("活動時間驗證：確認時間範圍不符合 publish_time <= (start_time, apply_deadline) <= end_time 時會被拒絕。\n")
	utils.PrintTestProgress("==================================================\n")

	now := time.Now().UTC().Truncate(time.Second)
	runTimelineCases(t, errs, invalidTimelineCases(now), true)

	utils.PrintTestProgress("==================================================\n\n")
}

func validTimelineCases(now time.Time) []timelineTestCase {
	return []timelineTestCase{
		{
			name:          "開始時間早於報名截止時間",
			progress:      "測試發布時間 <= 開始時間 <= 報名截止時間 <= 活動結束時間時可通過。",
			publishTime:   now,
			startTime:     now.Add(time.Hour),
			applyDeadline: now.Add(2 * time.Hour),
			endTime:       now.Add(3 * time.Hour),
		},
		{
			name:          "報名截止時間早於開始時間",
			progress:      "測試發布時間 <= 報名截止時間 <= 開始時間 <= 活動結束時間時可通過。",
			publishTime:   now,
			startTime:     now.Add(2 * time.Hour),
			applyDeadline: now.Add(time.Hour),
			endTime:       now.Add(3 * time.Hour),
		},
		{
			name:          "開始時間等於報名截止時間",
			progress:      "測試 start_time 與 apply_deadline 相同時可通過。",
			publishTime:   now,
			startTime:     now.Add(time.Hour),
			applyDeadline: now.Add(time.Hour),
			endTime:       now.Add(2 * time.Hour),
		},
		{
			name:          "發布時間等於開始時間",
			progress:      "測試 publish_time 等於 start_time 時可通過。",
			publishTime:   now,
			startTime:     now,
			applyDeadline: now.Add(time.Hour),
			endTime:       now.Add(2 * time.Hour),
		},
		{
			name:          "發布時間等於報名截止時間",
			progress:      "測試 publish_time 等於 apply_deadline 時可通過。",
			publishTime:   now,
			startTime:     now.Add(time.Hour),
			applyDeadline: now,
			endTime:       now.Add(2 * time.Hour),
		},
		{
			name:          "開始時間等於活動結束時間",
			progress:      "測試 start_time 等於 end_time 且 apply_deadline 不晚於 end_time 時可通過。",
			publishTime:   now,
			startTime:     now.Add(2 * time.Hour),
			applyDeadline: now.Add(time.Hour),
			endTime:       now.Add(2 * time.Hour),
		},
		{
			name:          "報名截止時間等於活動結束時間",
			progress:      "測試 apply_deadline 等於 end_time 且 start_time 不晚於 end_time 時可通過。",
			publishTime:   now,
			startTime:     now.Add(time.Hour),
			applyDeadline: now.Add(2 * time.Hour),
			endTime:       now.Add(2 * time.Hour),
		},
		{
			name:          "四個時間全部相同",
			progress:      "測試四個時間全部相同時符合小於等於邊界並可通過。",
			publishTime:   now,
			startTime:     now,
			applyDeadline: now,
			endTime:       now,
		},
	}
}

func missingTimelineCases(now time.Time) []timelineTestCase {
	base := timelineTestCase{
		publishTime:   now,
		startTime:     now.Add(time.Hour),
		applyDeadline: now.Add(2 * time.Hour),
		endTime:       now.Add(3 * time.Hour),
	}
	fields := []struct {
		label string
		clear func(*timelineTestCase)
	}{
		{
			label: "發布時間",
			clear: func(tt *timelineTestCase) {
				tt.publishTime = time.Time{}
			},
		},
		{
			label: "開始時間",
			clear: func(tt *timelineTestCase) {
				tt.startTime = time.Time{}
			},
		},
		{
			label: "報名截止時間",
			clear: func(tt *timelineTestCase) {
				tt.applyDeadline = time.Time{}
			},
		},
		{
			label: "活動結束時間",
			clear: func(tt *timelineTestCase) {
				tt.endTime = time.Time{}
			},
		},
	}

	tests := make([]timelineTestCase, 0, (1<<len(fields))-1)
	for mask := 1; mask < 1<<len(fields); mask++ {
		tt := base
		missing := []string{}
		for i, field := range fields {
			if mask&(1<<i) == 0 {
				continue
			}
			field.clear(&tt)
			missing = append(missing, field.label)
		}

		missingText := strings.Join(missing, "、")
		tt.name = missingText + "為空"
		tt.progress = fmt.Sprintf("測試 %s 為空時會回傳 VALIDATION_ERROR。", missingText)
		tests = append(tests, tt)
	}
	return tests
}

func invalidTimelineCases(now time.Time) []timelineTestCase {
	return []timelineTestCase{
		{
			name:          "發布時間晚於開始時間",
			progress:      "測試 publish_time 晚於 start_time 時會被拒絕。",
			publishTime:   now.Add(3 * time.Hour),
			startTime:     now.Add(2 * time.Hour),
			applyDeadline: now.Add(4 * time.Hour),
			endTime:       now.Add(5 * time.Hour),
		},
		{
			name:          "發布時間晚於報名截止時間",
			progress:      "測試 publish_time 晚於 apply_deadline 時會被拒絕。",
			publishTime:   now.Add(3 * time.Hour),
			startTime:     now.Add(4 * time.Hour),
			applyDeadline: now.Add(2 * time.Hour),
			endTime:       now.Add(5 * time.Hour),
		},
		{
			name:          "開始時間晚於活動結束時間",
			progress:      "測試 start_time 晚於 end_time 時會被拒絕。",
			publishTime:   now,
			startTime:     now.Add(4 * time.Hour),
			applyDeadline: now.Add(time.Hour),
			endTime:       now.Add(3 * time.Hour),
		},
		{
			name:          "報名截止時間晚於活動結束時間",
			progress:      "測試 apply_deadline 晚於 end_time 時會被拒絕。",
			publishTime:   now,
			startTime:     now.Add(time.Hour),
			applyDeadline: now.Add(4 * time.Hour),
			endTime:       now.Add(3 * time.Hour),
		},
		{
			name:          "發布時間同時晚於開始與報名截止時間",
			progress:      "測試 publish_time 同時晚於 start_time 與 apply_deadline 時會被拒絕。",
			publishTime:   now.Add(4 * time.Hour),
			startTime:     now.Add(time.Hour),
			applyDeadline: now.Add(2 * time.Hour),
			endTime:       now.Add(5 * time.Hour),
		},
		{
			name:          "開始與報名截止時間都晚於活動結束時間",
			progress:      "測試 start_time 與 apply_deadline 都晚於 end_time 時會被拒絕。",
			publishTime:   now,
			startTime:     now.Add(3 * time.Hour),
			applyDeadline: now.Add(4 * time.Hour),
			endTime:       now.Add(2 * time.Hour),
		},
	}
}

func runTimelineCases(t *testing.T, errs *utils.Errors, tests []timelineTestCase, wantErr bool) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", tt.progress))

			err := validateTimeline(tt.publishTime, tt.startTime, tt.applyDeadline, tt.endTime)
			if wantErr {
				if err == nil {
					errs.Add(tt.progress, "expected validateTimeline to return VALIDATION_ERROR")
					return
				}
				if assertErr := assertValidationError(err); assertErr != nil {
					errs.Add(tt.progress, "%v", assertErr)
					return
				}
				return
			}
			if err != nil {
				errs.Add(tt.progress, "expected validateTimeline to pass, got %v", err)
				return
			}
		})
	}
}

func assertValidationError(err error) error {
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		return fmt.Errorf("expected apperror.Error with code VALIDATION_ERROR, got %T: %v", err, err)
	}
	if appErr.Code != "VALIDATION_ERROR" {
		return fmt.Errorf("expected error code VALIDATION_ERROR, got %q", appErr.Code)
	}
	return nil
}
