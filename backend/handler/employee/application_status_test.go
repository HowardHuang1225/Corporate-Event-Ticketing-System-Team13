package employee

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

func TestEmployeeApplicationStatus(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試員工可以查看自己的申請狀態",
			Target:      MyApplicationsReturnsOnlyCurrentEmployeeApplications,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func MyApplicationsReturnsOnlyCurrentEmployeeApplications(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("查看申請狀態：確認員工只能看到自己的申請紀錄與狀態。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備查看申請狀態測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	event, err := seedEmployeeEventWithTickets(tx, users.Manager, "published", "Tainan")
	if err != nil {
		errs.Add("建立申請狀態活動測試資料", "%v", err)
		return
	}
	ticketType := event.TicketTypes[0]
	pendingApp, err := seedEmployeeApplication(tx, users.Employee, event, ticketType, "pending", 1)
	if err != nil {
		errs.Add("建立 pending 申請測試資料", "%v", err)
		return
	}
	approvedApp, err := seedEmployeeApplication(tx, users.Employee, event, ticketType, "approved", 1)
	if err != nil {
		errs.Add("建立 approved 申請測試資料", "%v", err)
		return
	}
	otherUser, err := seedExtraEmployee(tx, "Tainan")
	if err != nil {
		errs.Add("建立其他員工測試資料", "%v", err)
		return
	}
	otherApp, err := seedEmployeeApplication(tx, otherUser, event, ticketType, "pending", 1)
	if err != nil {
		errs.Add("建立其他員工申請測試資料", "%v", err)
		return
	}

	router := newEmployeeTicketRouter(tx, nil, users.Employee)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/applications/my", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		errs.Add("查詢自己的申請狀態", "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeEmployeeApplicationListResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析自己的申請狀態回應", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("解析自己的申請狀態回應", "expected success=true")
		return
	}
	if !employeeApplicationListContains(body.Data, pendingApp.ID.String()) {
		errs.Add("檢查自己的申請狀態", "回應應包含 pending 申請")
		return
	}
	if !employeeApplicationListContains(body.Data, approvedApp.ID.String()) {
		errs.Add("檢查自己的申請狀態", "回應應包含 approved 申請")
		return
	}
	if employeeApplicationListContains(body.Data, otherApp.ID.String()) {
		errs.Add("檢查自己的申請狀態", "回應不應包含其他員工的申請")
		return
	}
	for _, app := range body.Data {
		if app.UserID != users.Employee.ID {
			errs.Add("檢查自己的申請狀態", "expected user_id %q, got %q", users.Employee.ID, app.UserID)
			return
		}
		if app.Status == "" {
			errs.Add("檢查自己的申請狀態", "申請狀態不可為空")
			return
		}
	}

	var persisted model.Application
	if err := tx.First(&persisted, "id = ?", pendingApp.ID).Error; err != nil {
		errs.Add("確認申請狀態測試資料仍存在", "%v", err)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}
