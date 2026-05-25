package integration

import (
	"net/http"
	"testing"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

func TestIntegrationTicketLifecycleFlow(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試員工申請後的退票、核銷與已核銷票券不可退票流程",
			Target:      TicketLifecycleIntegration,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func TicketLifecycleIntegration(t *testing.T, errs *utils.Errors) {
	t.Helper()

	logIntegrationStep(t, "準備員工、活動管理者、可申請活動與真實 /v1 router")
	ctx, err := setupIntegrationTest(t)
	if err != nil {
		errs.Add("準備整合測試環境", "%v", err)
		return
	}

	event, ticketTypes, err := seedIntegrationEventWithTicketTypes(ctx.DB, ctx.Users.Manager, "published", 3, 4)
	if err != nil {
		errs.Add("建立票券生命週期活動", "%v", err)
		return
	}
	ticketType := ticketTypes[0]
	cleanupIntegrationRedisKeys(t, ctx.Redis, ticketType.ID.String())

	employeeToken, err := loginIntegrationUser(ctx.Router, ctx.Users.Employee.EmployeeID)
	if err != nil {
		errs.Add("員工登入取得 JWT", "%v", err)
		return
	}
	managerToken, err := loginIntegrationUser(ctx.Router, ctx.Users.Manager.EmployeeID)
	if err != nil {
		errs.Add("活動管理者登入取得 JWT", "%v", err)
		return
	}

	logIntegrationStep(t, "員工先申請兩張票，讓後續流程分別測退票與核銷")
	applyResp := performIntegrationJSON(ctx.Router, http.MethodPost, "/v1/applications", employeeToken, gin.H{
		"event_id":        event.ID.String(),
		"ticket_type_id":  ticketType.ID.String(),
		"quantity":        2,
		"idempotency_key": "integration-lifecycle-" + utils.UniqueTestSuffix(),
	})
	if applyResp.Code != http.StatusCreated {
		errs.Add("建立票券生命週期申請", "expected status 201, got %d with body %s", applyResp.Code, applyResp.Body.String())
		return
	}
	applyBody, err := decodeIntegrationOK[model.Application](applyResp.Body.Bytes())
	if err != nil {
		errs.Add("解析票券生命週期申請回應", "%v", err)
		return
	}

	var tickets []model.Ticket
	if err := ctx.DB.Where("application_id = ?", applyBody.Data.ID).
		Order("issued_at asc").
		Find(&tickets).Error; err != nil {
		errs.Add("讀取申請產生的票券", "%v", err)
		return
	}
	if len(tickets) != 2 {
		errs.Add("檢查申請產生票券數量", "expected 2 tickets, got %d", len(tickets))
		return
	}
	ticketToCancel := tickets[0]
	ticketToCheckin := tickets[1]

	logIntegrationStep(t, "員工退還一張未使用票券時，應刪除原票券、回補庫存並建立 cancelled audit application")
	cancelResp := performIntegrationJSON(ctx.Router, http.MethodPost, "/v1/tickets/"+ticketToCancel.ID.String()+"/cancel", employeeToken, gin.H{})
	if cancelResp.Code != http.StatusOK {
		errs.Add("退還未使用票券", "expected status 200, got %d with body %s", cancelResp.Code, cancelResp.Body.String())
		return
	}

	var cancelledTicketCount int64
	if err := ctx.DB.Model(&model.Ticket{}).
		Where("id = ?", ticketToCancel.ID).
		Count(&cancelledTicketCount).Error; err != nil {
		errs.Add("統計被退還票券是否刪除", "%v", err)
		return
	}
	if cancelledTicketCount != 0 {
		errs.Add("檢查被退還票券已刪除", "expected 0, got %d", cancelledTicketCount)
		return
	}

	var refundApp model.Application
	if err := ctx.DB.First(&refundApp, "idempotency_key = ?", "refund-"+ticketToCancel.ID.String()).Error; err != nil {
		errs.Add("讀取退票 audit application", "%v", err)
		return
	}
	if refundApp.Status != "cancelled" || refundApp.Quantity != 1 {
		errs.Add("檢查退票 audit application", "expected cancelled quantity 1, got %+v", refundApp)
		return
	}

	var dbRemaining int
	if err := ctx.DB.Model(&model.TicketType{}).
		Where("id = ?", ticketType.ID).
		Select("remaining").
		Scan(&dbRemaining).Error; err != nil {
		errs.Add("讀取退票後 DB 庫存", "%v", err)
		return
	}
	if dbRemaining != 3 {
		errs.Add("檢查退票後 DB 庫存", "expected remaining 3, got %d", dbRemaining)
		return
	}

	logIntegrationStep(t, "活動管理者使用另一張票的 QR token 核銷，應標記 used 並建立 checkin")
	checkinResp := performIntegrationJSON(ctx.Router, http.MethodPost, "/v1/checkin", managerToken, gin.H{
		"qr_token": ticketToCheckin.QRToken,
	})
	if checkinResp.Code != http.StatusOK {
		errs.Add("活動管理者核銷票券", "expected status 200, got %d with body %s", checkinResp.Code, checkinResp.Body.String())
		return
	}

	var checkedTicket model.Ticket
	if err := ctx.DB.First(&checkedTicket, "id = ?", ticketToCheckin.ID).Error; err != nil {
		errs.Add("讀取核銷後票券", "%v", err)
		return
	}
	if !checkedTicket.IsUsed {
		errs.Add("檢查票券已核銷", "expected ticket is_used=true")
		return
	}

	var checkinCount int64
	if err := ctx.DB.Model(&model.Checkin{}).
		Where("ticket_id = ?", ticketToCheckin.ID).
		Count(&checkinCount).Error; err != nil {
		errs.Add("統計 checkin 紀錄", "%v", err)
		return
	}
	if checkinCount != 1 {
		errs.Add("檢查 checkin 紀錄數量", "expected 1, got %d", checkinCount)
		return
	}

	logIntegrationStep(t, "同一張票重複核銷時應回 ALREADY_CHECKED_IN")
	duplicateCheckinResp := performIntegrationJSON(ctx.Router, http.MethodPost, "/v1/checkin", managerToken, gin.H{
		"qr_token": ticketToCheckin.QRToken,
	})
	if duplicateCheckinResp.Code != http.StatusConflict {
		errs.Add("重複核銷票券", "expected status 409, got %d with body %s", duplicateCheckinResp.Code, duplicateCheckinResp.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(duplicateCheckinResp.Body.Bytes(), "ALREADY_CHECKED_IN"); err != nil {
		errs.Add("檢查重複核銷錯誤碼", "%v", err)
		return
	}

	logIntegrationStep(t, "員工嘗試退還已核銷票券時應回 ALREADY_USED")
	cancelUsedResp := performIntegrationJSON(ctx.Router, http.MethodPost, "/v1/tickets/"+ticketToCheckin.ID.String()+"/cancel", employeeToken, gin.H{})
	if cancelUsedResp.Code != http.StatusConflict {
		errs.Add("退還已核銷票券", "expected status 409, got %d with body %s", cancelUsedResp.Code, cancelUsedResp.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(cancelUsedResp.Body.Bytes(), "ALREADY_USED"); err != nil {
		errs.Add("檢查已核銷票券不可退票錯誤碼", "%v", err)
		return
	}
}
