package integration

import (
	"net/http"
	"testing"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestIntegrationEmployeeApplicationFlow(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試員工經由真實 API 申請活動會自動核准並產生票券",
			Target:      EmployeeApplicationAutoApprovedIntegration,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func EmployeeApplicationAutoApprovedIntegration(t *testing.T, errs *utils.Errors) {
	t.Helper()

	logIntegrationStep(t, "準備整合測試資料庫、Redis、真實 /v1 router 與測試使用者")
	ctx, err := setupIntegrationTest(t)
	if err != nil {
		errs.Add("準備整合測試環境", "%v", err)
		return
	}

	event, ticketTypes, err := seedIntegrationEventWithTicketTypes(ctx.DB, ctx.Users.Manager, "published", 3, 5)
	if err != nil {
		errs.Add("建立可申請活動與票種", "%v", err)
		return
	}
	ticketType := ticketTypes[0]
	cleanupIntegrationRedisKeys(t, ctx.Redis, ticketType.ID.String())

	logIntegrationStep(t, "員工透過 /v1/auth/login 取得 JWT")
	employeeToken, err := loginIntegrationUser(ctx.Router, ctx.Users.Employee.EmployeeID)
	if err != nil {
		errs.Add("員工登入取得 JWT", "%v", err)
		return
	}

	idempotencyKey := "integration-apply-" + utils.UniqueTestSuffix()
	payload := gin.H{
		"event_id":        event.ID.String(),
		"ticket_type_id":  ticketType.ID.String(),
		"quantity":        2,
		"idempotency_key": idempotencyKey,
	}

	logIntegrationStep(t, "員工送出申請後應立即核准並回傳 201")
	resp := performIntegrationJSON(ctx.Router, http.MethodPost, "/v1/applications", employeeToken, payload)
	if resp.Code != http.StatusCreated {
		errs.Add("員工送出申請", "expected status 201, got %d with body %s", resp.Code, resp.Body.String())
		return
	}
	body, err := decodeIntegrationOK[model.Application](resp.Body.Bytes())
	if err != nil {
		errs.Add("解析員工申請成功回應", "%v", err)
		return
	}
	app := body.Data
	if app.Status != "approved" {
		errs.Add("檢查自動核准狀態", "expected approved, got %q", app.Status)
		return
	}
	if app.UserID != ctx.Users.Employee.ID || app.EventID != event.ID || app.TicketTypeID != ticketType.ID {
		errs.Add("檢查申請關聯資料", "application relation mismatch: %+v", app)
		return
	}
	if app.Quantity != 2 {
		errs.Add("檢查申請張數", "expected quantity 2, got %d", app.Quantity)
		return
	}

	logIntegrationStep(t, "檢查 DB 已建立一筆 application、兩張 ticket，且庫存同步扣除")
	var savedApp model.Application
	if err := ctx.DB.Preload("Tickets").First(&savedApp, "id = ?", app.ID).Error; err != nil {
		errs.Add("讀取 DB application 與 tickets", "%v", err)
		return
	}
	if len(savedApp.Tickets) != 2 {
		errs.Add("檢查自動產生票券數量", "expected 2 tickets, got %d", len(savedApp.Tickets))
		return
	}
	for _, ticket := range savedApp.Tickets {
		if _, err := uuid.Parse(ticket.QRToken); err != nil {
			errs.Add("檢查 QR token 格式", "ticket %s has invalid QR token %q: %v", ticket.ID, ticket.QRToken, err)
			return
		}
		if ticket.UserID != ctx.Users.Employee.ID || ticket.EventID != event.ID || ticket.TicketTypeID != ticketType.ID {
			errs.Add("檢查票券關聯資料", "ticket relation mismatch: %+v", ticket)
			return
		}
	}

	var dbRemaining int
	if err := ctx.DB.Model(&model.TicketType{}).
		Where("id = ?", ticketType.ID).
		Select("remaining").
		Scan(&dbRemaining).Error; err != nil {
		errs.Add("讀取 DB 庫存", "%v", err)
		return
	}
	if dbRemaining != 3 {
		errs.Add("檢查 DB 庫存扣除", "expected remaining 3, got %d", dbRemaining)
		return
	}
	redisRemaining, err := integrationRedisInt(ctx.Redis, "inventory:"+ticketType.ID.String())
	if err != nil {
		errs.Add("讀取 Redis 庫存", "%v", err)
		return
	}
	if redisRemaining != 3 {
		errs.Add("檢查 Redis 庫存扣除", "expected remaining 3, got %d", redisRemaining)
		return
	}

	logIntegrationStep(t, "使用相同 idempotency_key 重送時應回 200 且不重複建票或扣庫存")
	duplicateResp := performIntegrationJSON(ctx.Router, http.MethodPost, "/v1/applications", employeeToken, payload)
	if duplicateResp.Code != http.StatusOK {
		errs.Add("重送相同 idempotency_key", "expected status 200, got %d with body %s", duplicateResp.Code, duplicateResp.Body.String())
		return
	}
	duplicateBody, err := decodeIntegrationOK[model.Application](duplicateResp.Body.Bytes())
	if err != nil {
		errs.Add("解析重送申請回應", "%v", err)
		return
	}
	if duplicateBody.Data.ID != app.ID {
		errs.Add("檢查 idempotent 回應 application", "expected application %s, got %s", app.ID, duplicateBody.Data.ID)
		return
	}

	var appCount int64
	if err := ctx.DB.Model(&model.Application{}).
		Where("idempotency_key = ? AND user_id = ?", idempotencyKey, ctx.Users.Employee.ID).
		Count(&appCount).Error; err != nil {
		errs.Add("統計 idempotency application 數量", "%v", err)
		return
	}
	if appCount != 1 {
		errs.Add("檢查 idempotency application 數量", "expected 1, got %d", appCount)
		return
	}

	var ticketCount int64
	if err := ctx.DB.Model(&model.Ticket{}).
		Where("application_id = ?", app.ID).
		Count(&ticketCount).Error; err != nil {
		errs.Add("統計 application ticket 數量", "%v", err)
		return
	}
	if ticketCount != 2 {
		errs.Add("檢查 idempotency ticket 數量", "expected 2, got %d", ticketCount)
		return
	}

	if err := ctx.DB.Model(&model.TicketType{}).
		Where("id = ?", ticketType.ID).
		Select("remaining").
		Scan(&dbRemaining).Error; err != nil {
		errs.Add("重讀 DB 庫存", "%v", err)
		return
	}
	if dbRemaining != 3 {
		errs.Add("檢查重送後 DB 庫存", "expected remaining 3, got %d", dbRemaining)
		return
	}

	logIntegrationStep(t, "員工查詢自己的票券與申請紀錄時應看得到自動核准結果")
	ticketsResp := performIntegrationRequest(ctx.Router, http.MethodGet, "/v1/tickets/my", employeeToken)
	if ticketsResp.Code != http.StatusOK {
		errs.Add("查詢我的票券", "expected status 200, got %d with body %s", ticketsResp.Code, ticketsResp.Body.String())
		return
	}
	ticketsBody, err := decodeIntegrationOK[[]model.Ticket](ticketsResp.Body.Bytes())
	if err != nil {
		errs.Add("解析我的票券回應", "%v", err)
		return
	}
	if len(ticketsBody.Data) != 2 {
		errs.Add("檢查我的票券數量", "expected 2, got %d", len(ticketsBody.Data))
		return
	}

	appsResp := performIntegrationRequest(ctx.Router, http.MethodGet, "/v1/applications/my", employeeToken)
	if appsResp.Code != http.StatusOK {
		errs.Add("查詢我的申請", "expected status 200, got %d with body %s", appsResp.Code, appsResp.Body.String())
		return
	}
	appsBody, err := decodeIntegrationOK[[]model.Application](appsResp.Body.Bytes())
	if err != nil {
		errs.Add("解析我的申請回應", "%v", err)
		return
	}
	if len(appsBody.Data) != 1 || appsBody.Data[0].Status != "approved" {
		errs.Add("檢查我的申請自動核准結果", "expected one approved application, got %+v", appsBody.Data)
		return
	}
}
