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
			Description: "測試票券服務列表查詢會回傳員工申請、票券與核銷資料",
			Target:      TicketServiceListsUserAndCheckinViews,
		},
		{
			Description: "測試票券服務核銷成功、重複核銷與 OTP 錯誤",
			Target:      TicketServiceCheckinSuccessAndTokenErrors,
		},
		{
			Description: "測試票券服務會拒絕格式異常的動態 QR token",
			Target:      TicketServiceCheckinRejectsMalformedDynamicQRToken,
		},
		{
			Description: "測試票券服務可接受上一個 60 秒時間窗的動態 QR Code",
			Target:      TicketServiceCheckinAcceptsPreviousDynamicQRCode,
		},
		{
			Description: "測試票券服務會拒絕超過允許時間窗的舊動態 QR Code",
			Target:      TicketServiceCheckinRejectsExpiredDynamicQRCode,
		},
		{
			Description: "測試票券服務申請會拒絕不合法輸入與無 Redis 狀態",
			Target:      TicketServiceApplyRejectsInvalidInputWithoutRedis,
		},
		{
			Description: "測試票券服務 Redis 申請流程會拒絕各種 business rule",
			Target:      TicketServiceApplyRejectsBusinessRulesWithRedis,
		},
		{
			Description: "測試票券服務會優先使用 MyApplications 與 MyTickets 快取",
			Target:      TicketServiceUsesRedisCacheForMyLists,
		},
		{
			Description: "測試票券服務 queue env 與 retry helper 分支",
			Target:      TicketServiceCoversQueueEnvAndRetryHelpers,
		},
		{
			Description: "測試票券服務核銷會拒絕不合法與不存在 QR token",
			Target:      TicketServiceCheckinRejectsMissingAndInvalidTokens,
		},
	}

	utils.RunTestTasks(t, tasks)
}
