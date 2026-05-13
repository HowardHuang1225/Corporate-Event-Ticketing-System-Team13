package ticket

import (
	"net/http"
	"testing"
	"time"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"
)

func CheckinExpiredTicketReturnsTicketExpired(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試過期票券核銷回傳 TICKET_EXPIRED\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備過期票券核銷測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newTicketService(tx, nil)
	tests := []struct {
		name     string
		progress string
	}{
		{
			name:     "核銷過期票券",
			progress: "核銷 expires_at 已經早於現在的票券時，應回傳 TICKET_EXPIRED，且不可標記為已使用。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			_, _, _, tickets, err := seedTicketServiceApplicationWithTickets(tx, users, "approved", 1, 9, []ticketServiceTicketSpec{
				{ExpiresAt: time.Now().Add(-time.Hour)},
			})
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			expiredTicket := tickets[0]

			_, err = service.Checkin(CheckinRequest{QRToken: expiredTicket.QRToken}, users.Manager.ID)
			if assertErr := assertTicketServiceAppError(err, http.StatusGone, "TICKET_EXPIRED"); assertErr != nil {
				errs.Add(tt.progress, "%v", assertErr)
				return
			}

			var persisted model.Ticket
			if err := tx.First(&persisted, "id = ?", expiredTicket.ID).Error; err != nil {
				errs.Add(tt.progress, "查詢過期票券失敗：%v", err)
				return
			}
			if persisted.IsUsed {
				errs.Add(tt.progress, "預期過期票券不可被標記為已使用")
				return
			}

			checkinCount, err := ticketServiceCheckinCount(tx, expiredTicket.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if checkinCount != 0 {
				errs.Add(tt.progress, "預期過期票券不新增核銷紀錄，實際為 %d 筆", checkinCount)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}
