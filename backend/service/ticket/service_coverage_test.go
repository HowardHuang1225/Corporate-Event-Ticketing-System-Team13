package ticket

import (
	"net/http"
	"testing"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"github.com/google/uuid"
)

func TicketServiceListsUserAndCheckinViews(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務列表：確認 MyApplications、MyTickets 與 ListCheckins 回傳對應資料。\n")
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

	dynamicQRToken := ticketServiceCurrentWindowDynamicQRToken(ticket.QRToken)
	result, err := service.Checkin(CheckinRequest{QRToken: dynamicQRToken}, users.Manager.ID)
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

	_, err = service.Checkin(CheckinRequest{QRToken: dynamicQRToken}, users.Manager.ID)
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

func TicketServiceCheckinRejectsMalformedDynamicQRToken(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務核銷：確認動態 QR token 只能接受 qr_token|otp 兩段格式。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備 malformed QR 測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	_, _, _, tickets, err := seedTicketServiceApplicationWithTickets(tx, users, "approved", 1, 9, []ticketServiceTicketSpec{{}})
	if err != nil {
		errs.Add("建立 malformed QR 測試票券", "%v", err)
		return
	}
	ticket := tickets[0]

	cases := []struct {
		name  string
		token string
	}{
		{
			name:  "缺少 OTP",
			token: ticket.QRToken + "|",
		},
		{
			name:  "多出第三段內容",
			token: ticket.QRToken + "|123456|extra",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			utils.PrintTestProgress("子測試：核銷 " + tc.name + " 的動態 QR 應被拒絕。\n")
			_, err := service.Checkin(CheckinRequest{QRToken: tc.token}, users.Manager.ID)
			if assertErr := assertTicketServiceAppError(err, http.StatusBadRequest, "VALIDATION_ERROR"); assertErr != nil {
				errs.Add("核銷 malformed dynamic QR - "+tc.name, "%v", assertErr)
				return
			}

			var persisted model.Ticket
			if err := tx.First(&persisted, "id = ?", ticket.ID).Error; err != nil {
				errs.Add("重查 malformed QR 票券 - "+tc.name, "%v", err)
				return
			}
			if persisted.IsUsed {
				errs.Add("檢查 malformed QR 票券狀態 - "+tc.name, "預期 is_used=false")
				return
			}
			count, err := ticketServiceCheckinCount(tx, ticket.ID)
			if err != nil {
				errs.Add("查詢 malformed QR 核銷紀錄 - "+tc.name, "%v", err)
				return
			}
			if count != 0 {
				errs.Add("檢查 malformed QR 核銷紀錄 - "+tc.name, "預期 0 筆，實際 %d", count)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func TicketServiceCheckinAcceptsPreviousDynamicQRCode(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務核銷：確認上一個 60 秒時間窗的動態 QR Code 仍可核銷。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("建立票券服務動態 QR 測試資料", "%v", err)
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
	dynamicQRToken := ticketServicePreviousWindowDynamicQRToken(ticket.QRToken)

	result, err := service.Checkin(CheckinRequest{QRToken: dynamicQRToken}, users.Manager.ID)
	if err != nil {
		errs.Add("使用上一個時間窗動態 QR 核銷", "%v", err)
		return
	}
	if result.Ticket.ID != ticket.ID {
		errs.Add("檢查動態 QR 核銷票券", "預期 ticket %s，實際 %+v", ticket.ID, result.Ticket)
		return
	}

	var persisted model.Ticket
	if err := tx.First(&persisted, "id = ?", ticket.ID).Error; err != nil {
		errs.Add("讀取動態 QR 核銷後票券", "%v", err)
		return
	}
	if !persisted.IsUsed {
		errs.Add("檢查動態 QR 核銷後票券狀態", "預期 is_used=true")
		return
	}

	checkinCount, err := ticketServiceCheckinCount(tx, ticket.ID)
	if err != nil {
		errs.Add("讀取動態 QR 核銷紀錄", "%v", err)
		return
	}
	if checkinCount != 1 {
		errs.Add("檢查動態 QR 核銷紀錄", "預期 1 筆，實際 %d", checkinCount)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func TicketServiceCheckinRejectsExpiredDynamicQRCode(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("票券服務核銷：確認超過允許時間窗的舊動態 QR Code 會失敗。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("建立票券服務過期動態 QR 測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	_, _, _, tickets, err := seedTicketServiceApplicationWithTickets(tx, users, "approved", 1, 9, []ticketServiceTicketSpec{{}})
	if err != nil {
		errs.Add("建立可測試過期 QR 的票券", "%v", err)
		return
	}
	ticket := tickets[0]
	expiredQRToken := ticketServiceExpiredWindowDynamicQRToken(ticket.QRToken)

	_, err = service.Checkin(CheckinRequest{QRToken: expiredQRToken}, users.Manager.ID)
	if assertErr := assertTicketServiceAppError(err, http.StatusForbidden, "EXPIRED_QR"); assertErr != nil {
		errs.Add("使用超過允許時間窗的舊動態 QR 核銷", "%v", assertErr)
		return
	}

	var persisted model.Ticket
	if err := tx.First(&persisted, "id = ?", ticket.ID).Error; err != nil {
		errs.Add("讀取過期動態 QR 核銷後票券", "%v", err)
		return
	}
	if persisted.IsUsed {
		errs.Add("檢查過期動態 QR 核銷後票券狀態", "預期 is_used=false")
		return
	}

	checkinCount, err := ticketServiceCheckinCount(tx, ticket.ID)
	if err != nil {
		errs.Add("讀取過期動態 QR 核銷紀錄", "%v", err)
		return
	}
	if checkinCount != 0 {
		errs.Add("檢查過期動態 QR 核銷紀錄", "預期 0 筆，實際 %d", checkinCount)
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
