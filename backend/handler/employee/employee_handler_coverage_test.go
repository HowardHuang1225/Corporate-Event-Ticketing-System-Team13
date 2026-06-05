package employee

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	eventsvc "ticketing-system/backend/service/event"
	ticketsvc "ticketing-system/backend/service/ticket"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type employeeQueueStatusResponse struct {
	Success bool                   `json:"success"`
	Data    ticketsvc.QueueStatus  `json:"data"`
	Error   map[string]interface{} `json:"error"`
}

type employeeMessageResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Message string `json:"message"`
	} `json:"data"`
}

func TestEmployeeHandlerCoverageRoutes(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試員工查詢 queue status 會從既有 application 回傳狀態並處理不存在資料",
			Target:      QueueStatusReturnsApplicationStateAndMissingError,
		},
		{
			Description: "測試員工可以取消自己的 application 並歸還庫存",
			Target:      CancelApplicationReturnsInventoryThroughHandler,
		},
		{
			Description: "測試員工可以退還單張未使用票券並拒絕不存在票券",
			Target:      CancelTicketReturnsSingleTicketThroughHandler,
		},
		{
			Description: "測試員工送出不合法申請 payload 會回傳 VALIDATION_ERROR",
			Target:      ApplyRejectsInvalidPayloadThroughHandler,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func QueueStatusReturnsApplicationStateAndMissingError(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("員工 queue status：確認 handler 會用 user_id 查詢 application 狀態並處理不存在資料。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備 queue status 測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	event, err := seedEmployeeEventWithTickets(tx, users.Manager, "published", "Tainan")
	if err != nil {
		errs.Add("建立 queue status 活動", "%v", err)
		return
	}
	app, err := seedEmployeeApplication(tx, users.Employee, event, event.TicketTypes[0], "approved", 2)
	if err != nil {
		errs.Add("建立 queue status 申請", "%v", err)
		return
	}

	router := newEmployeeCoverageRouter(tx, nil, users.Employee, true)
	resp := performEmployeeGet(router, "/applications/queue/"+app.IdempotencyKey)
	if resp.Code != http.StatusOK {
		errs.Add("查詢既有 queue status", "expected 200, got %d body=%s", resp.Code, resp.Body.String())
		return
	}
	body, err := decodeEmployeeQueueStatusResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析 queue status 回應", "%v", err)
		return
	}
	if !body.Success || body.Data.Status != "approved" || body.Data.ApplicationID != app.ID.String() || body.Data.Quantity != "2" {
		errs.Add("檢查 queue status 回應", "回應不符預期：%+v", body)
		return
	}

	missing := performEmployeeGet(router, "/applications/queue/missing-"+utils.UniqueTestSuffix())
	if missing.Code != http.StatusNotFound {
		errs.Add("查詢不存在 queue status", "expected 404, got %d body=%s", missing.Code, missing.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(missing.Body.Bytes(), "NOT_FOUND"); err != nil {
		errs.Add("檢查不存在 queue status 錯誤", "%v", err)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func CancelApplicationReturnsInventoryThroughHandler(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("員工取消申請：確認 handler 會取消自己的 application、刪除票券並歸還庫存。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備取消申請測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	event, err := seedEmployeeEventWithTickets(tx, users.Manager, "published", "Tainan")
	if err != nil {
		errs.Add("建立取消申請活動", "%v", err)
		return
	}
	ticketType := event.TicketTypes[0]
	app, err := seedEmployeeApplication(tx, users.Employee, event, ticketType, "approved", 1)
	if err != nil {
		errs.Add("建立待取消申請", "%v", err)
		return
	}
	ticket, err := seedEmployeeTicket(tx, users.Employee, event, ticketType, app)
	if err != nil {
		errs.Add("建立待取消申請票券", "%v", err)
		return
	}

	router := newEmployeeCoverageRouter(tx, nil, users.Employee, true)
	resp := performEmployeeDelete(router, "/applications/"+app.ID.String())
	if resp.Code != http.StatusOK {
		errs.Add("取消自己的申請", "expected 200, got %d body=%s", resp.Code, resp.Body.String())
		return
	}
	body, err := decodeEmployeeMessageResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析取消申請回應", "%v", err)
		return
	}
	if !body.Success || body.Data.Message != "Application cancelled and tickets returned to pool" {
		errs.Add("檢查取消申請回應", "回應不符預期：%+v", body)
		return
	}

	var updated model.Application
	if err := tx.First(&updated, "id = ?", app.ID).Error; err != nil {
		errs.Add("重查取消後申請", "%v", err)
		return
	}
	if updated.Status != "cancelled" {
		errs.Add("檢查取消後申請狀態", "預期 cancelled，實際 %q", updated.Status)
		return
	}
	remaining, err := employeeTicketTypeRemaining(tx, ticketType.ID)
	if err != nil {
		errs.Add("查詢取消後庫存", "%v", err)
		return
	}
	if remaining != ticketType.Remaining+app.Quantity {
		errs.Add("檢查取消後庫存", "預期 %d，實際 %d", ticketType.Remaining+app.Quantity, remaining)
		return
	}
	if exists, err := employeeTicketExists(tx, ticket.ID); err != nil {
		errs.Add("查詢取消後票券", "%v", err)
		return
	} else if exists {
		errs.Add("檢查取消後票券", "預期票券已刪除")
		return
	}

	missing := performEmployeeDelete(router, "/applications/"+uuid.New().String())
	if missing.Code != http.StatusNotFound {
		errs.Add("取消不存在申請", "expected 404, got %d body=%s", missing.Code, missing.Body.String())
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func CancelTicketReturnsSingleTicketThroughHandler(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("員工單張退票：確認 handler 會退還未使用票券、歸還庫存並建立 cancelled audit application。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備單張退票測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	event, err := seedEmployeeEventWithTickets(tx, users.Manager, "published", "Tainan")
	if err != nil {
		errs.Add("建立單張退票活動", "%v", err)
		return
	}
	ticketType := event.TicketTypes[0]
	app, err := seedEmployeeApplication(tx, users.Employee, event, ticketType, "approved", 1)
	if err != nil {
		errs.Add("建立單張退票申請", "%v", err)
		return
	}
	ticket, err := seedEmployeeTicket(tx, users.Employee, event, ticketType, app)
	if err != nil {
		errs.Add("建立待退票券", "%v", err)
		return
	}

	router := newEmployeeCoverageRouter(tx, nil, users.Employee, true)
	resp := performEmployeeDelete(router, "/tickets/"+ticket.ID.String())
	if resp.Code != http.StatusOK {
		errs.Add("退還單張票券", "expected 200, got %d body=%s", resp.Code, resp.Body.String())
		return
	}
	body, err := decodeEmployeeMessageResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析單張退票回應", "%v", err)
		return
	}
	if !body.Success || body.Data.Message != "Ticket returned successfully" {
		errs.Add("檢查單張退票回應", "回應不符預期：%+v", body)
		return
	}
	if exists, err := employeeTicketExists(tx, ticket.ID); err != nil {
		errs.Add("查詢退票後票券", "%v", err)
		return
	} else if exists {
		errs.Add("檢查退票後票券", "預期票券已刪除")
		return
	}
	remaining, err := employeeTicketTypeRemaining(tx, ticketType.ID)
	if err != nil {
		errs.Add("查詢退票後庫存", "%v", err)
		return
	}
	if remaining != ticketType.Remaining+1 {
		errs.Add("檢查退票後庫存", "預期 %d，實際 %d", ticketType.Remaining+1, remaining)
		return
	}
	if count, err := employeeCancelledAuditCount(tx, ticket.ID.String()); err != nil {
		errs.Add("查詢退票 audit application", "%v", err)
		return
	} else if count != 1 {
		errs.Add("檢查退票 audit application", "預期 1 筆，實際 %d", count)
		return
	}

	missing := performEmployeeDelete(router, "/tickets/"+uuid.New().String())
	if missing.Code != http.StatusNotFound {
		errs.Add("退還不存在票券", "expected 404, got %d body=%s", missing.Code, missing.Body.String())
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func ApplyRejectsInvalidPayloadThroughHandler(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("員工申請票券防呆：確認 handler 會拒絕缺少必要欄位的 payload。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備申請防呆測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newEmployeeCoverageRouter(tx, nil, users.Employee, true)
	resp := utils.PerformJSON(router, http.MethodPost, "/applications", gin.H{"quantity": 1})
	if resp.Code != http.StatusBadRequest {
		errs.Add("送出缺少欄位的申請 payload", "expected 400, got %d body=%s", resp.Code, resp.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), "VALIDATION_ERROR"); err != nil {
		errs.Add("檢查申請防呆錯誤", "%v", err)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func newEmployeeCoverageRouter(db *gorm.DB, redisClient *redis.Client, user model.User, withUser bool) *gin.Engine {
	repos := repository.New(db, redisClient)
	handler := New(eventsvc.New(repos), ticketsvc.New(repos))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		if withUser {
			c.Set("user_id", user.ID.String())
			c.Set("role", "employee")
		}
		c.Next()
	})
	router.POST("/applications", handler.Apply)
	router.GET("/applications/queue/:idempotency_key", handler.QueueStatus)
	router.DELETE("/applications/:id", handler.CancelApplication)
	router.DELETE("/tickets/:id", handler.CancelTicket)
	return router
}

func performEmployeeGet(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func performEmployeeDelete(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func decodeEmployeeQueueStatusResponse(body []byte) (employeeQueueStatusResponse, error) {
	return utils.DecodeJSON[employeeQueueStatusResponse](body)
}

func decodeEmployeeMessageResponse(body []byte) (employeeMessageResponse, error) {
	return utils.DecodeJSON[employeeMessageResponse](body)
}

func employeeTicketTypeRemaining(db *gorm.DB, ticketTypeID uuid.UUID) (int, error) {
	var remaining int
	if err := db.Model(&model.TicketType{}).Select("remaining").Where("id = ?", ticketTypeID).Scan(&remaining).Error; err != nil {
		return 0, err
	}
	return remaining, nil
}

func employeeTicketExists(db *gorm.DB, ticketID uuid.UUID) (bool, error) {
	var count int64
	if err := db.Model(&model.Ticket{}).Where("id = ?", ticketID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func employeeCancelledAuditCount(db *gorm.DB, ticketIDPrefix string) (int64, error) {
	var count int64
	if len(ticketIDPrefix) > 8 {
		ticketIDPrefix = ticketIDPrefix[:8]
	}
	if err := db.Model(&model.Application{}).
		Where("status = ? AND idempotency_key LIKE ?", "cancelled", "refund-%").
		Where("reason = ?", "Returned ticket "+ticketIDPrefix).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
