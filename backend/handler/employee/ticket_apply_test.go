package employee

import (
	"net/http"
	"testing"

	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

func TestEmployeeApplyTicket(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試員工可以申請活動票券",
			Target:      ApplyTicketCreatesPendingApplication,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func ApplyTicketCreatesPendingApplication(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("申請票券：確認員工送出活動與票種後，系統會建立 pending 申請。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備申請票券測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	redisClient, err := openEmployeeTestRedis(t)
	if err != nil {
		errs.Add("準備申請票券 Redis 測試服務", "%v", err)
		return
	}

	event, err := seedEmployeeEventWithTickets(tx, users.Manager, "published", "Tainan")
	if err != nil {
		errs.Add("建立可申請票券的活動測試資料", "%v", err)
		return
	}
	ticketType := event.TicketTypes[0]
	cleanupEmployeeInventoryKeys(t, redisClient, ticketType.ID.String())

	router := newEmployeeTicketRouter(tx, redisClient, users.Employee)
	payload := gin.H{
		"event_id":        event.ID.String(),
		"ticket_type_id":  ticketType.ID.String(),
		"quantity":        1,
		"idempotency_key": "apply-" + utils.UniqueTestSuffix(),
	}

	resp := utils.PerformJSON(router, http.MethodPost, "/applications", payload)
	if resp.Code != http.StatusCreated {
		errs.Add("送出申請票券請求", "expected status 201, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeEmployeeApplicationResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析申請票券回應", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("解析申請票券回應", "expected success=true")
		return
	}
	if body.Data.UserID != users.Employee.ID {
		errs.Add("檢查申請票券回應", "expected user_id %q, got %q", users.Employee.ID, body.Data.UserID)
		return
	}
	if body.Data.EventID != event.ID {
		errs.Add("檢查申請票券回應", "expected event_id %q, got %q", event.ID, body.Data.EventID)
		return
	}
	if body.Data.TicketTypeID != ticketType.ID {
		errs.Add("檢查申請票券回應", "expected ticket_type_id %q, got %q", ticketType.ID, body.Data.TicketTypeID)
		return
	}
	if body.Data.Quantity != 1 {
		errs.Add("檢查申請票券回應", "expected quantity 1, got %d", body.Data.Quantity)
		return
	}
	if body.Data.Status != "pending" {
		errs.Add("檢查申請票券回應", "expected status pending, got %q", body.Data.Status)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}
