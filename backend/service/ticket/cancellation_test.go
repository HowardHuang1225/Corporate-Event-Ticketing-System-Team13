package ticket

import (
	"fmt"
	"testing"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"
)

func CancelApplicationReturnsInventory(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試取消 pending/approved application 的庫存歸還\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備取消申請測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	tests := []struct {
		name          string
		status        string
		quantity      int
		remaining     int
		tickets       []ticketServiceTicketSpec
		wantRemaining int
		progress      string
	}{
		{
			name:          "取消 pending application",
			status:        "pending",
			quantity:      3,
			remaining:     7,
			wantRemaining: 10,
			progress:      "取消 pending application 時，應將申請數量歸還到 ticket_types.remaining。",
		},
		{
			name:          "取消 approved application",
			status:        "approved",
			quantity:      2,
			remaining:     8,
			tickets:       []ticketServiceTicketSpec{{}, {}},
			wantRemaining: 10,
			progress:      "取消尚未使用的 approved application 時，應歸還庫存並刪除該申請底下的票券。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			_, ticketType, app, _, err := seedTicketServiceApplicationWithTickets(tx, users, tt.status, tt.quantity, tt.remaining, tt.tickets)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			if err := service.CancelApplication(app.ID.String(), users.Employee.ID); err != nil {
				errs.Add(tt.progress, "取消 application 失敗：%v", err)
				return
			}

			persisted, err := ticketServiceApplicationByID(tx, app.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if persisted.Status != "cancelled" {
				errs.Add(tt.progress, "預期 application 狀態為 cancelled，實際為 %q", persisted.Status)
				return
			}

			remaining, err := ticketServiceRemaining(tx, ticketType.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if remaining != tt.wantRemaining {
				errs.Add(tt.progress, "預期剩餘庫存為 %d，實際為 %d", tt.wantRemaining, remaining)
				return
			}

			ticketCount, err := ticketServiceTicketCountForApplication(tx, app.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if ticketCount != 0 {
				errs.Add(tt.progress, "預期取消後 application 底下沒有票券，實際仍有 %d 張", ticketCount)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func CancelApplicationRejectsUsedTickets(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試已使用票券不可整筆取消\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備已使用票券整筆取消測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	tests := []struct {
		name     string
		progress string
	}{
		{
			name:     "approved application 含已使用票券",
			progress: "approved application 只要包含已使用票券，就不可整筆取消，庫存與票券都應維持原狀。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			_, ticketType, app, _, err := seedTicketServiceApplicationWithTickets(tx, users, "approved", 2, 8, []ticketServiceTicketSpec{
				{IsUsed: true},
				{IsUsed: false},
			})
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			err = service.CancelApplication(app.ID.String(), users.Employee.ID)
			if assertErr := assertTicketServiceAppErrorCode(err, "TICKETS_ALREADY_USED"); assertErr != nil {
				errs.Add(tt.progress, "%v", assertErr)
				return
			}

			persisted, err := ticketServiceApplicationByID(tx, app.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if persisted.Status != "approved" {
				errs.Add(tt.progress, "預期 application 維持 approved，實際為 %q", persisted.Status)
				return
			}

			remaining, err := ticketServiceRemaining(tx, ticketType.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if remaining != 8 {
				errs.Add(tt.progress, "預期庫存維持 8，實際為 %d", remaining)
				return
			}

			ticketCount, err := ticketServiceTicketCountForApplication(tx, app.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if ticketCount != 2 {
				errs.Add(tt.progress, "預期票券維持 2 張，實際為 %d 張", ticketCount)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func CancelTicketCreatesCancelledAuditApplication(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試單張退票會新增 cancelled audit application\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備單張退票測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	tests := []struct {
		name     string
		progress string
	}{
		{
			name:     "退還單張未使用票券",
			progress: "退還單張未使用票券時，應刪除原票券、庫存加一，並新增一筆 cancelled application 作為 audit 紀錄。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			_, ticketType, app, tickets, err := seedTicketServiceApplicationWithTickets(tx, users, "approved", 2, 8, []ticketServiceTicketSpec{{}, {}})
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			returnedTicket := tickets[0]

			if err := service.CancelTicket(returnedTicket.ID.String(), users.Employee.ID); err != nil {
				errs.Add(tt.progress, "退還單張票券失敗：%v", err)
				return
			}

			exists, err := ticketServiceTicketExists(tx, returnedTicket.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if exists {
				errs.Add(tt.progress, "預期退票後原票券 %s 已被刪除", returnedTicket.ID)
				return
			}

			remaining, err := ticketServiceRemaining(tx, ticketType.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if remaining != 9 {
				errs.Add(tt.progress, "預期退票後庫存為 9，實際為 %d", remaining)
				return
			}

			var auditApp model.Application
			auditKey := "refund-" + returnedTicket.ID.String()
			if err := tx.First(&auditApp, "idempotency_key = ?", auditKey).Error; err != nil {
				errs.Add(tt.progress, "預期找到退票 audit application %q：%v", auditKey, err)
				return
			}
			if auditApp.Status != "cancelled" {
				errs.Add(tt.progress, "預期 audit application 狀態為 cancelled，實際為 %q", auditApp.Status)
				return
			}
			if auditApp.Quantity != 1 {
				errs.Add(tt.progress, "預期 audit application quantity 為 1，實際為 %d", auditApp.Quantity)
				return
			}
			if auditApp.UserID != users.Employee.ID || auditApp.EventID != returnedTicket.EventID || auditApp.TicketTypeID != ticketType.ID {
				errs.Add(tt.progress, "audit application 的 user/event/ticket_type 應對應原退票票券")
				return
			}
			if auditApp.Reason == nil || *auditApp.Reason != fmt.Sprintf("Returned ticket %s", returnedTicket.ID.String()[:8]) {
				errs.Add(tt.progress, "預期 audit application reason 記錄退票票券，實際為 %v", auditApp.Reason)
				return
			}

			originalApp, err := ticketServiceApplicationByID(tx, app.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if originalApp.Status != "approved" {
				errs.Add(tt.progress, "預期原 application 維持 approved，實際為 %q", originalApp.Status)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func CancelTicketRejectsUsedTicket(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試已使用單張票券不可退票\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備已使用單張票券退票測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	tests := []struct {
		name     string
		progress string
	}{
		{
			name:     "退還已使用票券",
			progress: "退還已使用單張票券時，應回傳 ALREADY_USED，且不歸還庫存、不新增 audit application。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			_, ticketType, _, tickets, err := seedTicketServiceApplicationWithTickets(tx, users, "approved", 1, 9, []ticketServiceTicketSpec{{IsUsed: true}})
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			usedTicket := tickets[0]

			err = service.CancelTicket(usedTicket.ID.String(), users.Employee.ID)
			if assertErr := assertTicketServiceAppErrorCode(err, "ALREADY_USED"); assertErr != nil {
				errs.Add(tt.progress, "%v", assertErr)
				return
			}

			exists, err := ticketServiceTicketExists(tx, usedTicket.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if !exists {
				errs.Add(tt.progress, "預期已使用票券仍保留在資料庫")
				return
			}

			remaining, err := ticketServiceRemaining(tx, ticketType.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if remaining != 9 {
				errs.Add(tt.progress, "預期庫存維持 9，實際為 %d", remaining)
				return
			}

			var auditCount int64
			if err := tx.Model(&model.Application{}).Where("idempotency_key = ?", "refund-"+usedTicket.ID.String()).Count(&auditCount).Error; err != nil {
				errs.Add(tt.progress, "查詢退票 audit application 失敗：%v", err)
				return
			}
			if auditCount != 0 {
				errs.Add(tt.progress, "預期不新增 audit application，實際新增 %d 筆", auditCount)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}
