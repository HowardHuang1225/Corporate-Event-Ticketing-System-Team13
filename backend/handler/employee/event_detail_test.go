package employee

import (
	"net/http"
	"net/http/httptest"
	"testing"

	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestEmployeeGetEvent(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試查詢單一活動會回傳活動詳細資料與票種",
			Target:      GetEventReturnsDetailsWithTicketTypes,
		},
		{
			Description: "測試查詢不存在的活動會回傳 NOT_FOUND",
			Target:      GetEventReturnsNotFoundForUnknownEvent,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func GetEventReturnsDetailsWithTicketTypes(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("查詢單一活動：確認回應會包含活動詳細資料與票種。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備查詢單一活動測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	event, err := seedEmployeeEventWithTickets(tx, users.Manager, "published", "Tainan")
	if err != nil {
		errs.Add("建立活動詳細資料測試資料", "%v", err)
		return
	}

	router := newEmployeeEventRouter(tx, users.Employee, "employee")
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events/"+event.ID.String(), nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		errs.Add("查詢活動詳細資料", "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeEmployeeEventResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析查詢活動回應", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("解析查詢活動回應", "expected success=true")
		return
	}
	if body.Data.ID != event.ID {
		errs.Add("檢查查詢活動回應內容", "expected event id %q, got %q", event.ID, body.Data.ID)
		return
	}
	if body.Data.Title != event.Title {
		errs.Add("檢查查詢活動回應內容", "expected title %q, got %q", event.Title, body.Data.Title)
		return
	}
	if len(body.Data.TicketTypes) != len(event.TicketTypes) {
		errs.Add("檢查查詢活動回應內容", "expected %d ticket types, got %d", len(event.TicketTypes), len(body.Data.TicketTypes))
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func GetEventReturnsNotFoundForUnknownEvent(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("查詢單一活動：確認不存在的活動 ID 會回傳 NOT_FOUND。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備查詢不存在活動測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newEmployeeEventRouter(tx, users.Employee, "employee")
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events/"+uuid.NewString(), nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		errs.Add("查詢不存在的活動", "expected status 404, got %d with body %s", resp.Code, resp.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), "NOT_FOUND"); err != nil {
		errs.Add("查詢不存在的活動", "%v", err)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}
