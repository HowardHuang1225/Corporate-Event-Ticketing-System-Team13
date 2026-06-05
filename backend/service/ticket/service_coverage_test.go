package ticket

import (
	"net/http"
	"testing"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"github.com/google/uuid"
)

func TicketServiceListsUserAndManagerViews(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務列表：確認 MyApplications、MyTickets、ListApplications 與 ListCheckins 回傳對應資料。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備票券服務列表測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	event, _, app, tickets, err := seedTicketServiceApplicationWithTickets(tx, users, "approved", 1, 9, []ticketServiceTicketSpec{{}})
	if err != nil {
		errs.Add("建立列表測試申請與票券", "%v", err)
		return
	}
	checkin := model.Checkin{TicketID: tickets[0].ID, CheckedBy: users.Manager.ID}
	if err := tx.Create(&checkin).Error; err != nil {
		errs.Add("建立列表測試核銷紀錄", "%v", err)
		return
	}

	apps, err := service.MyApplications(users.Employee.ID.String())
	if err != nil {
		errs.Add("查詢員工申請列表", "%v", err)
		return
	}
	if len(apps) != 1 || apps[0].ID != app.ID || apps[0].Event.ID != event.ID || apps[0].TicketType.ID == uuid.Nil {
		errs.Add("檢查員工申請列表", "預期含申請 %s 與關聯資料，實際 %+v", app.ID, apps)
		return
	}

	myTickets, err := service.MyTickets(users.Employee.ID.String())
	if err != nil {
		errs.Add("查詢員工票券列表", "%v", err)
		return
	}
	if len(myTickets) != 1 || myTickets[0].ID != tickets[0].ID || myTickets[0].Event.ID != event.ID || myTickets[0].UserID != users.Employee.ID {
		errs.Add("檢查員工票券列表", "預期含票券 %s 與關聯資料，實際 %+v", tickets[0].ID, myTickets)
		return
	}

	managerApps, err := service.ListApplications(event.ID.String(), "approved")
	if err != nil {
		errs.Add("查詢管理者申請列表", "%v", err)
		return
	}
	if len(managerApps) != 1 || managerApps[0].ID != app.ID || managerApps[0].User.ID != users.Employee.ID {
		errs.Add("檢查管理者申請列表", "預期只回傳申請 %s，實際 %+v", app.ID, managerApps)
		return
	}

	checkins, err := service.ListCheckins(event.ID.String())
	if err != nil {
		errs.Add("查詢核銷列表", "%v", err)
		return
	}
	if len(checkins) != 1 || checkins[0].TicketID != tickets[0].ID || checkins[0].Ticket.Event.ID != event.ID {
		errs.Add("檢查核銷列表", "預期只回傳 ticket %s 的核銷，實際 %+v", tickets[0].ID, checkins)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func TicketServiceReviewsApplications(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務審核：確認 ApproveApplication 與 RejectApplication 會更新狀態、票券與庫存。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備票券服務審核測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	_, approvedType, pendingApprove, _, err := seedTicketServiceApplicationWithTickets(tx, users, "pending", 2, 8, nil)
	if err != nil {
		errs.Add("建立待核准申請", "%v", err)
		return
	}

	approved, err := service.ApproveApplication(pendingApprove.ID.String(), users.Manager.ID)
	if err != nil {
		errs.Add("核准 pending 申請", "%v", err)
		return
	}
	if approved.Status != "approved" || approved.ReviewedBy == nil || *approved.ReviewedBy != users.Manager.ID || approved.ReviewedAt == nil {
		errs.Add("檢查核准申請狀態", "回傳資料不符預期：%+v", approved)
		return
	}
	if len(approved.Tickets) != 2 {
		errs.Add("檢查核准後票券", "預期 2 張票，實際 %d", len(approved.Tickets))
		return
	}
	remaining, err := ticketServiceRemaining(tx, approvedType.ID)
	if err != nil {
		errs.Add("查詢核准後庫存", "%v", err)
		return
	}
	if remaining != 8 {
		errs.Add("檢查核准後庫存", "核准不應額外調整庫存，預期 8，實際 %d", remaining)
		return
	}

	_, err = service.ApproveApplication(approved.ID.String(), users.Manager.ID)
	if assertErr := assertTicketServiceAppError(err, http.StatusBadRequest, "INVALID_STATUS"); assertErr != nil {
		errs.Add("重複核准已核准申請", "%v", assertErr)
		return
	}
	_, err = service.ApproveApplication(uuid.New().String(), users.Manager.ID)
	if assertErr := assertTicketServiceAppError(err, http.StatusNotFound, "NOT_FOUND"); assertErr != nil {
		errs.Add("核准不存在申請", "%v", assertErr)
		return
	}

	_, rejectedType, pendingReject, _, err := seedTicketServiceApplicationWithTickets(tx, users, "pending", 1, 4, nil)
	if err != nil {
		errs.Add("建立待拒絕申請", "%v", err)
		return
	}
	reason := "候補順位不足"
	_, err = service.RejectApplication(pendingReject.ID.String(), users.Manager.ID, RejectRequest{Reason: reason})
	if err != nil {
		errs.Add("拒絕 pending 申請", "%v", err)
		return
	}
	updated, err := ticketServiceApplicationByID(tx, pendingReject.ID)
	if err != nil {
		errs.Add("重查拒絕後申請", "%v", err)
		return
	}
	if updated.Status != "rejected" || updated.Reason == nil || *updated.Reason != reason || updated.ReviewedBy == nil || *updated.ReviewedBy != users.Manager.ID {
		errs.Add("檢查拒絕申請狀態", "更新後資料不符預期：%+v", updated)
		return
	}
	remaining, err = ticketServiceRemaining(tx, rejectedType.ID)
	if err != nil {
		errs.Add("查詢拒絕後庫存", "%v", err)
		return
	}
	if remaining != 5 {
		errs.Add("檢查拒絕後庫存", "預期 5，實際 %d", remaining)
		return
	}
	_, err = service.RejectApplication(updated.ID.String(), users.Manager.ID, RejectRequest{Reason: reason})
	if assertErr := assertTicketServiceAppError(err, http.StatusBadRequest, "INVALID_STATUS"); assertErr != nil {
		errs.Add("重複拒絕已拒絕申請", "%v", assertErr)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func TicketServiceCheckinSuccessAndTokenErrors(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務核銷：確認成功核銷、重複核銷與過期 OTP 分支。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備票券服務核銷測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	_, _, _, tickets, err := seedTicketServiceApplicationWithTickets(tx, users, "approved", 1, 9, []ticketServiceTicketSpec{{}})
	if err != nil {
		errs.Add("建立可核銷票券", "%v", err)
		return
	}
	ticket := tickets[0]

	result, err := service.Checkin(CheckinRequest{QRToken: ticket.QRToken}, users.Manager.ID)
	if err != nil {
		errs.Add("核銷有效票券", "%v", err)
		return
	}
	if result.Message != "Check-in successful!" || result.Ticket.ID != ticket.ID || result.Ticket.Event.ID == uuid.Nil || result.Ticket.User.ID != users.Employee.ID {
		errs.Add("檢查核銷成功回應", "回應不符預期：%+v", result)
		return
	}
	var persisted model.Ticket
	if err := tx.First(&persisted, "id = ?", ticket.ID).Error; err != nil {
		errs.Add("重查核銷票券", "%v", err)
		return
	}
	if !persisted.IsUsed {
		errs.Add("檢查核銷票券狀態", "預期票券已標記使用")
		return
	}
	checkinCount, err := ticketServiceCheckinCount(tx, ticket.ID)
	if err != nil {
		errs.Add("查詢核銷紀錄", "%v", err)
		return
	}
	if checkinCount != 1 {
		errs.Add("檢查核銷紀錄", "預期 1 筆，實際 %d", checkinCount)
		return
	}

	_, err = service.Checkin(CheckinRequest{QRToken: ticket.QRToken}, users.Manager.ID)
	if assertErr := assertTicketServiceAppError(err, http.StatusConflict, "ALREADY_CHECKED_IN"); assertErr != nil {
		errs.Add("重複核銷同一張票", "%v", assertErr)
		return
	}

	_, _, _, otpTickets, err := seedTicketServiceApplicationWithTickets(tx, users, "approved", 1, 9, []ticketServiceTicketSpec{{}})
	if err != nil {
		errs.Add("建立 OTP 測試票券", "%v", err)
		return
	}
	_, err = service.Checkin(CheckinRequest{QRToken: otpTickets[0].QRToken + "|not-otp"}, users.Manager.ID)
	if assertErr := assertTicketServiceAppError(err, http.StatusForbidden, "EXPIRED_QR"); assertErr != nil {
		errs.Add("核銷過期 OTP", "%v", assertErr)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func TicketServiceApplyRejectsInvalidInputWithoutRedis(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務申請：確認 Apply 對不合法 UUID 與無庫存系統狀態回傳正確錯誤。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備票券申請測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	_, err = service.Apply(users.Employee.ID, ApplyRequest{EventID: "not-a-uuid", TicketTypeID: uuid.New().String(), Quantity: 1, IdempotencyKey: "invalid-event"})
	if assertErr := assertTicketServiceAppError(err, http.StatusBadRequest, "VALIDATION_ERROR"); assertErr != nil {
		errs.Add("申請時 event_id 格式錯誤", "%v", assertErr)
		return
	}
	_, err = service.Apply(users.Employee.ID, ApplyRequest{EventID: uuid.New().String(), TicketTypeID: "not-a-uuid", Quantity: 1, IdempotencyKey: "invalid-ticket-type"})
	if assertErr := assertTicketServiceAppError(err, http.StatusBadRequest, "VALIDATION_ERROR"); assertErr != nil {
		errs.Add("申請時 ticket_type_id 格式錯誤", "%v", assertErr)
		return
	}
	_, err = service.Apply(users.Employee.ID, ApplyRequest{EventID: uuid.New().String(), TicketTypeID: uuid.New().String(), Quantity: 1, IdempotencyKey: "missing-redis"})
	if assertErr := assertTicketServiceAppError(err, http.StatusServiceUnavailable, "BUSY"); assertErr != nil {
		errs.Add("申請時 Redis 不可用", "%v", assertErr)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}
