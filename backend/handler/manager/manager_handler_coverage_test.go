package manager

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	reportsvc "ticketing-system/backend/service/report"
	ticketsvc "ticketing-system/backend/service/ticket"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type managerCheckinListResponse struct {
	Success bool            `json:"success"`
	Data    []model.Checkin `json:"data"`
}

type managerEventStatsResponse struct {
	Success bool  `json:"success"`
	Data    gin.H `json:"data"`
}

func TestManagerHandlerCoverageRoutes(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試管理者可列出核銷紀錄並依活動篩選",
			Target:      ManagerListCheckinsReturnsFilteredRecords,
		},
		{
			Description: "測試管理者可查詢單一活動統計並處理不存在活動",
			Target:      ManagerEventStatsRouteReturnsMetricsAndRejectsMissingEvent,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func ManagerListCheckinsReturnsFilteredRecords(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("管理者核銷紀錄：確認 ListCheckins handler 會回傳核銷紀錄並依 event_id 篩選。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupManagerTicketLifecycleTest(t)
	if err != nil {
		errs.Add("準備核銷列表測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	wantedTicket, err := seedManagerCheckinTicket(tx, users, true)
	if err != nil {
		errs.Add("建立目標核銷票券", "%v", err)
		return
	}
	otherTicket, err := seedManagerCheckinTicket(tx, users, true)
	if err != nil {
		errs.Add("建立其他核銷票券", "%v", err)
		return
	}
	if err := seedManagerHandlerCheckin(tx, users.Manager, wantedTicket); err != nil {
		errs.Add("建立目標核銷紀錄", "%v", err)
		return
	}
	if err := seedManagerHandlerCheckin(tx, users.Manager, otherTicket); err != nil {
		errs.Add("建立其他核銷紀錄", "%v", err)
		return
	}

	router := newManagerCoverageRouter(tx, users.Manager)
	resp := performManagerGet(router, "/checkins?event_id="+wantedTicket.EventID.String())
	if resp.Code != http.StatusOK {
		errs.Add("列出核銷紀錄", "expected 200, got %d body=%s", resp.Code, resp.Body.String())
		return
	}
	body, err := decodeManagerCheckinListResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析核銷紀錄", "%v", err)
		return
	}
	if len(body.Data) != 1 || body.Data[0].TicketID != wantedTicket.ID {
		errs.Add("檢查核銷紀錄篩選", "預期只回傳 ticket %s，實際 %+v", wantedTicket.ID, body.Data)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func ManagerEventStatsRouteReturnsMetricsAndRejectsMissingEvent(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("管理者活動統計：確認 EventStats handler 可回傳報表統計並處理不存在活動。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupManagerTicketLifecycleTest(t)
	if err != nil {
		errs.Add("準備活動統計測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	approvedTicket, err := seedManagerCheckinTicket(tx, users, true)
	if err != nil {
		errs.Add("建立統計票券", "%v", err)
		return
	}
	if err := seedManagerHandlerCheckin(tx, users.Manager, approvedTicket); err != nil {
		errs.Add("建立統計核銷紀錄", "%v", err)
		return
	}

	router := newManagerCoverageRouter(tx, users.Manager)
	resp := performManagerGet(router, "/reports/events/"+approvedTicket.EventID.String()+"/stats")
	if resp.Code != http.StatusOK {
		errs.Add("查詢活動統計", "expected 200, got %d body=%s", resp.Code, resp.Body.String())
		return
	}
	body, err := decodeManagerEventStatsResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析活動統計", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("解析活動統計", "expected success=true")
		return
	}
	if got := fmt.Sprint(body.Data["total_tickets"]); got != "1" {
		errs.Add("檢查活動統計票券數", "預期 total_tickets=1，實際 %s", got)
		return
	}
	if got := fmt.Sprint(body.Data["checked_in_tickets"]); got != "1" {
		errs.Add("檢查活動統計核銷數", "預期 checked_in_tickets=1，實際 %s", got)
		return
	}

	missing := performManagerGet(router, "/reports/events/"+uuid.New().String()+"/stats")
	if missing.Code != http.StatusNotFound {
		errs.Add("查詢不存在活動統計", "expected 404, got %d body=%s", missing.Code, missing.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(missing.Body.Bytes(), "NOT_FOUND"); err != nil {
		errs.Add("查詢不存在活動統計", "%v", err)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func newManagerCoverageRouter(db *gorm.DB, manager model.User) *gin.Engine {
	repos := repository.New(db, nil)
	ticketService := ticketsvc.New(repos)
	reportService := reportsvc.New(repos)
	handler := New(nil, ticketService, reportService, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.GET("/checkins", handler.ListCheckins)
	router.GET("/reports/events/:id/stats", handler.EventStats)
	return router
}

func performManagerGet(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func decodeManagerCheckinListResponse(body []byte) (managerCheckinListResponse, error) {
	return utils.DecodeJSON[managerCheckinListResponse](body)
}

func decodeManagerEventStatsResponse(body []byte) (managerEventStatsResponse, error) {
	return utils.DecodeJSON[managerEventStatsResponse](body)
}

func seedManagerHandlerCheckin(db *gorm.DB, manager model.User, ticket model.Ticket) error {
	checkin := model.Checkin{
		TicketID:  ticket.ID,
		CheckedBy: manager.ID,
	}
	if err := db.Create(&checkin).Error; err != nil {
		return fmt.Errorf("建立 manager handler 核銷紀錄失敗: %w", err)
	}
	return nil
}
