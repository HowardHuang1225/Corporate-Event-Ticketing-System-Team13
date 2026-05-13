package manager

import (
	"fmt"
	"net/http"
	"testing"

	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

func TestManagerEventActions(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試發布與關閉活動會更新活動狀態",
			Target:      PublishAndCloseEventRoutesUpdateStatus,
		},
		{
			Description: "測試發布活動時會拒絕非草稿活動",
			Target:      PublishEventRejectsNonDraftEvent,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func PublishAndCloseEventRoutesUpdateStatus(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("活動狀態操作：確認發布與關閉路由會正確更新活動狀態。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupManagerEventTest(t)
	if err != nil {
		errs.Add("準備活動狀態操作測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerEventRouter(tx, manager)
	event, err := seedManagerActionEvent(tx, manager, "draft")
	if err != nil {
		errs.Add("建立草稿活動測試資料", "%v", err)
		return
	}

	actionTests := []struct {
		name       string
		method     string
		path       string
		wantStatus string
		progress   string
	}{
		{
			name:       "發布草稿活動",
			method:     http.MethodPatch,
			path:       "/events/" + event.ID.String() + "/publish",
			wantStatus: "published",
			progress:   "測試草稿活動可以被活動管理者手動發布。",
		},
		{
			name:       "關閉已發布活動",
			method:     http.MethodPatch,
			path:       "/events/" + event.ID.String() + "/close",
			wantStatus: "closed",
			progress:   "測試已發布活動可以被活動管理者提前關閉。",
		},
	}

	for _, tt := range actionTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", tt.progress))

			resp := utils.PerformJSON(router, tt.method, tt.path, gin.H{})
			if resp.Code != http.StatusOK {
				errs.Add(tt.progress, "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
				return
			}

			body, err := decodeManagerEventResponse(resp.Body.Bytes())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if !body.Success {
				errs.Add(tt.progress, "expected success=true")
				return
			}
			if body.Data.ID != event.ID {
				errs.Add(tt.progress, "expected event id %q, got %q", event.ID, body.Data.ID)
				return
			}
			if body.Data.Status != tt.wantStatus {
				errs.Add(tt.progress, "expected response status %q, got %q", tt.wantStatus, body.Data.Status)
				return
			}
			if err := assertManagerEventStatus(tx, event.ID.String(), tt.wantStatus); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func PublishEventRejectsNonDraftEvent(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("活動狀態操作：確認發布路由會拒絕非草稿狀態的活動。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupManagerEventTest(t)
	if err != nil {
		errs.Add("準備發布狀態驗證測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerEventRouter(tx, manager)
	event, err := seedManagerActionEvent(tx, manager, "published")
	if err != nil {
		errs.Add("建立已發布活動測試資料", "%v", err)
		return
	}

	resp := utils.PerformJSON(router, http.MethodPatch, "/events/"+event.ID.String()+"/publish", gin.H{})
	if resp.Code != http.StatusBadRequest {
		errs.Add("發布非草稿狀態活動", "expected status 400, got %d with body %s", resp.Code, resp.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), "INVALID_STATUS"); err != nil {
		errs.Add("發布非草稿狀態活動", "%v", err)
		return
	}
	if err := assertManagerEventStatus(tx, event.ID.String(), "published"); err != nil {
		errs.Add("確認非草稿活動維持原本狀態", "%v", err)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}
