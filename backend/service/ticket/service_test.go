package ticket

import (
	"testing"

	utils "ticketing-system/backend/test_utils"
)

func TestTicketService(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試取消 pending/approved application 的庫存歸還",
			Target:      CancelApplicationReturnsInventory,
		},
		{
			Description: "測試已使用票券不可整筆取消",
			Target:      CancelApplicationRejectsUsedTickets,
		},
		{
			Description: "測試單張退票會新增 cancelled audit application",
			Target:      CancelTicketCreatesCancelledAuditApplication,
		},
		{
			Description: "測試已使用單張票券不可退票",
			Target:      CancelTicketRejectsUsedTicket,
		},
		{
			Description: "測試重複 idempotency_key 不會重複扣庫存",
			Target:      ApplyWithDuplicateIdempotencyKeyDoesNotDeductInventoryTwice,
		},
		{
			Description: "測試過期票券核銷回傳 TICKET_EXPIRED",
			Target:      CheckinExpiredTicketReturnsTicketExpired,
		},
	}

	utils.RunTestTasks(t, tasks)
}
