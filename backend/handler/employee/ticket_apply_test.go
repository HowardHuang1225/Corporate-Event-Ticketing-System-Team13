package employee

import (
	"context"
	"net/http"
	"testing"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestEmployeeApplyTicket(t *testing.T) {
	// t.Skip("Legacy logic per product requirement change: direct approval and no region locks.")
	tasks := []utils.Task{
		{
			Description: "測試員工可以申請活動票券",
			Target:      ApplyTicketCreatesApprovedApplication,
		},
		{
			Description: "測試員工申請錯誤流程會回傳對應錯誤碼",
			Target:      ApplyTicketRejectsBusinessRuleErrorsThroughHandler,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func ApplyTicketCreatesApprovedApplication(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("申請票券：確認員工送出活動與票種後，系統會建立申請並自動 approve。\n")
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
		"quantity":        2,
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
	if body.Data.Quantity != 2 {
		errs.Add("檢查申請票券回應", "expected quantity 2, got %d", body.Data.Quantity)
		return
	}
	if body.Data.Status != "approved" {
		errs.Add("檢查申請票券回應", "expected status approved, got %q", body.Data.Status)
		return
	}

	ticketCount, err := employeeTicketCountForApplication(tx, body.Data.ID)
	if err != nil {
		errs.Add("查詢申請後票券數量", "%v", err)
		return
	}
	if ticketCount != 2 {
		errs.Add("檢查申請後票券數量", "預期建立 2 張票券，實際 %d", ticketCount)
		return
	}
	tickets, err := employeeTicketsForApplication(tx, body.Data.ID)
	if err != nil {
		errs.Add("查詢申請後票券內容", "%v", err)
		return
	}
	for _, ticket := range tickets {
		if _, err := uuid.Parse(ticket.QRToken); err != nil {
			errs.Add("檢查票券 QR token 格式", "預期 QR token 為 UUID，實際 %q", ticket.QRToken)
			return
		}
	}
	remaining, err := employeeTicketTypeRemaining(tx, ticketType.ID)
	if err != nil {
		errs.Add("查詢申請後 DB 庫存", "%v", err)
		return
	}
	if remaining != ticketType.Remaining-2 {
		errs.Add("檢查申請後 DB 庫存", "預期 %d，實際 %d", ticketType.Remaining-2, remaining)
		return
	}
	redisRemaining, err := redisClient.Get(context.Background(), "inventory:"+ticketType.ID.String()).Int()
	if err != nil {
		errs.Add("查詢申請後 Redis 庫存", "%v", err)
		return
	}
	if redisRemaining != ticketType.Remaining-2 {
		errs.Add("檢查申請後 Redis 庫存", "預期 %d，實際 %d", ticketType.Remaining-2, redisRemaining)
		return
	}

	duplicate := utils.PerformJSON(router, http.MethodPost, "/applications", payload)
	if duplicate.Code != http.StatusOK {
		errs.Add("重複送出相同 idempotency_key", "expected status 200, got %d with body %s", duplicate.Code, duplicate.Body.String())
		return
	}
	duplicateBody, err := decodeEmployeeApplicationResponse(duplicate.Body.Bytes())
	if err != nil {
		errs.Add("解析重複申請回應", "%v", err)
		return
	}
	if duplicateBody.Data.ID != body.Data.ID {
		errs.Add("檢查重複申請回應", "預期回傳既有 application %s，實際 %s", body.Data.ID, duplicateBody.Data.ID)
		return
	}
	afterDuplicateCount, err := employeeTicketCountForApplication(tx, body.Data.ID)
	if err != nil {
		errs.Add("查詢重複申請後票券數量", "%v", err)
		return
	}
	if afterDuplicateCount != 2 {
		errs.Add("檢查重複申請後票券數量", "預期仍為 2 張，實際 %d", afterDuplicateCount)
		return
	}
	afterDuplicateRemaining, err := employeeTicketTypeRemaining(tx, ticketType.ID)
	if err != nil {
		errs.Add("查詢重複申請後庫存", "%v", err)
		return
	}
	if afterDuplicateRemaining != remaining {
		errs.Add("檢查重複申請後庫存", "預期 %d，實際 %d", remaining, afterDuplicateRemaining)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func ApplyTicketRejectsBusinessRuleErrorsThroughHandler(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("申請票券防呆：確認 handler 會轉出活動/票種/庫存與上限相關錯誤。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備申請錯誤流程測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	redisClient, err := openEmployeeTestRedis(t)
	if err != nil {
		errs.Add("準備申請錯誤流程 Redis 測試服務", "%v", err)
		return
	}
	router := newEmployeeTicketRouter(tx, redisClient, users.Employee)

	published, err := seedEmployeeEventWithTickets(tx, users.Manager, "published", "Tainan")
	if err != nil {
		errs.Add("建立可申請活動", "%v", err)
		return
	}
	publishedType := published.TicketTypes[0]
	cleanupEmployeeInventoryKeys(t, redisClient, publishedType.ID.String())

	tests := []struct {
		name       string
		payload    gin.H
		wantStatus int
		wantCode   string
	}{
		{
			name: "活動不存在",
			payload: gin.H{
				"event_id":        uuid.New().String(),
				"ticket_type_id":  publishedType.ID.String(),
				"quantity":        1,
				"idempotency_key": "missing-event-" + utils.UniqueTestSuffix(),
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name: "票種不存在",
			payload: gin.H{
				"event_id":        published.ID.String(),
				"ticket_type_id":  uuid.New().String(),
				"quantity":        1,
				"idempotency_key": "missing-ticket-type-" + utils.UniqueTestSuffix(),
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			utils.PrintTestProgress("子測試：申請 " + tt.name + " 時應回傳對應錯誤碼。\n")
			resp := utils.PerformJSON(router, http.MethodPost, "/applications", tt.payload)
			if resp.Code != tt.wantStatus {
				errs.Add("申請錯誤流程 - "+tt.name, "expected status %d, got %d body=%s", tt.wantStatus, resp.Code, resp.Body.String())
				return
			}
			if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), tt.wantCode); err != nil {
				errs.Add("檢查申請錯誤碼 - "+tt.name, "%v", err)
				return
			}
		})
	}

	draft, err := seedEmployeeEventWithTickets(tx, users.Manager, "draft", "Tainan")
	if err != nil {
		errs.Add("建立未發布活動", "%v", err)
		return
	}
	draftType := draft.TicketTypes[0]
	cleanupEmployeeInventoryKeys(t, redisClient, draftType.ID.String())
	assertApplyErrorThroughHandler(t, errs, router, "活動未發布", gin.H{
		"event_id":        draft.ID.String(),
		"ticket_type_id":  draftType.ID.String(),
		"quantity":        1,
		"idempotency_key": "draft-event-" + utils.UniqueTestSuffix(),
	}, http.StatusBadRequest, "EVENT_NOT_AVAILABLE")

	expired, err := seedEmployeeEventWithTickets(tx, users.Manager, "published", "Tainan")
	if err != nil {
		errs.Add("建立截止活動", "%v", err)
		return
	}
	expiredType := expired.TicketTypes[0]
	cleanupEmployeeInventoryKeys(t, redisClient, expiredType.ID.String())
	if err := tx.Model(&model.Event{}).Where("id = ?", expired.ID).Update("apply_deadline", expired.PublishTime).Error; err != nil {
		errs.Add("更新活動為已截止", "%v", err)
		return
	}
	expiredRouter := newEmployeeTicketRouter(tx.Session(&gorm.Session{SkipHooks: true}), redisClient, users.Employee)
	assertApplyErrorThroughHandler(t, errs, expiredRouter, "活動已截止", gin.H{
		"event_id":        expired.ID.String(),
		"ticket_type_id":  expiredType.ID.String(),
		"quantity":        1,
		"idempotency_key": "deadline-passed-" + utils.UniqueTestSuffix(),
	}, http.StatusBadRequest, "APPLY_DEADLINE_PASSED")

	soldOut, err := seedEmployeeEventWithTickets(tx, users.Manager, "published", "Tainan")
	if err != nil {
		errs.Add("建立售罄活動", "%v", err)
		return
	}
	soldOutType := soldOut.TicketTypes[0]
	cleanupEmployeeInventoryKeys(t, redisClient, soldOutType.ID.String())
	if err := tx.Model(&model.TicketType{}).Where("id = ?", soldOutType.ID).Updates(map[string]any{"remaining": 0}).Error; err != nil {
		errs.Add("更新票種為售罄", "%v", err)
		return
	}
	assertApplyErrorThroughHandler(t, errs, router, "票種售罄", gin.H{
		"event_id":        soldOut.ID.String(),
		"ticket_type_id":  soldOutType.ID.String(),
		"quantity":        1,
		"idempotency_key": "sold-out-" + utils.UniqueTestSuffix(),
	}, http.StatusConflict, "TICKET_SOLD_OUT")

	limited, err := seedEmployeeEventWithTickets(tx, users.Manager, "published", "Tainan")
	if err != nil {
		errs.Add("建立上限活動", "%v", err)
		return
	}
	limitedType := limited.TicketTypes[0]
	cleanupEmployeeInventoryKeys(t, redisClient, limitedType.ID.String())
	if err := tx.Model(&model.Event{}).Where("id = ?", limited.ID).Update("max_tickets_per_person", 1).Error; err != nil {
		errs.Add("更新每人票數上限", "%v", err)
		return
	}
	existingApp, err := seedEmployeeApplication(tx, users.Employee, limited, limitedType, "approved", 1)
	if err != nil {
		errs.Add("建立既有申請", "%v", err)
		return
	}
	if _, err := seedEmployeeTicket(tx, users.Employee, limited, limitedType, existingApp); err != nil {
		errs.Add("建立既有票券", "%v", err)
		return
	}
	assertApplyErrorThroughHandler(t, errs, router, "超過每人上限", gin.H{
		"event_id":        limited.ID.String(),
		"ticket_type_id":  limitedType.ID.String(),
		"quantity":        1,
		"idempotency_key": "exceeds-limit-" + utils.UniqueTestSuffix(),
	}, http.StatusBadRequest, "EXCEEDS_MAX_TICKETS")

	utils.PrintTestProgress("==================================================\n\n")
}

func assertApplyErrorThroughHandler(t *testing.T, errs *utils.Errors, router *gin.Engine, name string, payload gin.H, wantStatus int, wantCode string) {
	t.Helper()

	utils.PrintTestProgress("子測試：申請 " + name + " 時應回傳 " + wantCode + "。\n")
	resp := utils.PerformJSON(router, http.MethodPost, "/applications", payload)
	if resp.Code != wantStatus {
		errs.Add("申請錯誤流程 - "+name, "expected status %d, got %d body=%s", wantStatus, resp.Code, resp.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), wantCode); err != nil {
		errs.Add("檢查申請錯誤碼 - "+name, "%v", err)
		return
	}
}

func employeeTicketCountForApplication(db *gorm.DB, applicationID uuid.UUID) (int64, error) {
	var count int64
	err := db.Model(&model.Ticket{}).Where("application_id = ?", applicationID).Count(&count).Error
	return count, err
}

func employeeTicketsForApplication(db *gorm.DB, applicationID uuid.UUID) ([]model.Ticket, error) {
	var tickets []model.Ticket
	err := db.Where("application_id = ?", applicationID).Find(&tickets).Error
	return tickets, err
}
