package employee

import (
	"net/http"
	"net/http/httptest"
	"testing"

	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

func TestEmployeeMyTickets(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試員工可以查看自己所有已批准的票券",
			Target:      MyTicketsReturnsOnlyCurrentEmployeeTickets,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func MyTicketsReturnsOnlyCurrentEmployeeTickets(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("查看我的票券：確認員工只能看到自己已核准後產生的票券。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備查看我的票券測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	event, err := seedEmployeeEventWithTickets(tx, users.Manager, "published", "Tainan")
	if err != nil {
		errs.Add("建立我的票券活動測試資料", "%v", err)
		return
	}
	ticketType := event.TicketTypes[0]
	app, err := seedEmployeeApplication(tx, users.Employee, event, ticketType, "approved", 1)
	if err != nil {
		errs.Add("建立已批准申請測試資料", "%v", err)
		return
	}
	ticket, err := seedEmployeeTicket(tx, users.Employee, event, ticketType, app)
	if err != nil {
		errs.Add("建立自己的票券測試資料", "%v", err)
		return
	}

	otherUser, err := seedExtraEmployee(tx, "Tainan")
	if err != nil {
		errs.Add("建立其他員工測試資料", "%v", err)
		return
	}
	otherApp, err := seedEmployeeApplication(tx, otherUser, event, ticketType, "approved", 1)
	if err != nil {
		errs.Add("建立其他員工已批准申請測試資料", "%v", err)
		return
	}
	otherTicket, err := seedEmployeeTicket(tx, otherUser, event, ticketType, otherApp)
	if err != nil {
		errs.Add("建立其他員工票券測試資料", "%v", err)
		return
	}

	router := newEmployeeTicketRouter(tx, nil, users.Employee)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tickets/my", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		errs.Add("查詢我的票券", "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeEmployeeTicketListResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析我的票券回應", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("解析我的票券回應", "expected success=true")
		return
	}
	if !employeeTicketListContains(body.Data, ticket.ID.String()) {
		errs.Add("檢查我的票券", "回應應包含自己的票券")
		return
	}
	if employeeTicketListContains(body.Data, otherTicket.ID.String()) {
		errs.Add("檢查我的票券", "回應不應包含其他員工的票券")
		return
	}
	for _, gotTicket := range body.Data {
		if gotTicket.UserID != users.Employee.ID {
			errs.Add("檢查我的票券", "expected user_id %q, got %q", users.Employee.ID, gotTicket.UserID)
			return
		}
		if gotTicket.Event.ID == event.ID && gotTicket.Event.Title == "" {
			errs.Add("檢查我的票券", "票券應包含活動資訊")
			return
		}
		if gotTicket.TicketType.ID == ticketType.ID && gotTicket.TicketType.Name == "" {
			errs.Add("檢查我的票券", "票券應包含票種資訊")
			return
		}
	}

	utils.PrintTestProgress("==================================================\n\n")
}
