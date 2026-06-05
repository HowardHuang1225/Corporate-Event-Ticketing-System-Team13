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
		{
			Description: "測試票券服務列表查詢會回傳申請、票券與核銷資料",
			Target:      TicketServiceListsUserAndManagerViews,
		},
		{
			Description: "測試票券服務審核會更新申請、票券與庫存",
			Target:      TicketServiceReviewsApplications,
		},
		{
			Description: "測試票券服務核銷成功、重複核銷與 OTP 錯誤",
			Target:      TicketServiceCheckinSuccessAndTokenErrors,
		},
		{
			Description: "測試票券服務申請會拒絕不合法輸入與無 Redis 狀態",
			Target:      TicketServiceApplyRejectsInvalidInputWithoutRedis,
		},
	}

	utils.RunTestTasks(t, tasks)
}
