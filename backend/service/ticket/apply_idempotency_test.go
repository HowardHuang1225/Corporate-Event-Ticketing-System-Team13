package ticket

import (
	"context"
	"testing"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"
)

func ApplyWithDuplicateIdempotencyKeyDoesNotDeductInventoryTwice(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試重複 idempotency_key 不會重複扣庫存\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupTicketServiceTest(t)
	if err != nil {
		errs.Add("準備 idempotency 測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	redisClient, err := openTicketServiceTestRedis(t)
	if err != nil {
		errs.Add("準備 idempotency Redis 測試連線", "%v", err)
		return
	}

	_, ticketType, err := seedTicketServiceEventWithTicketType(tx, users.Manager, 5, 5, 5)
	if err != nil {
		errs.Add("建立 idempotency 測試活動與票種", "%v", err)
		return
	}
	cleanupTicketServiceInventoryKeys(t, redisClient, ticketType.ID.String())

	service := newTicketService(tx, redisClient)
	idempotencyKey := "service-apply-" + utils.UniqueTestSuffix()
	req := ApplyRequest{
		EventID:        ticketType.EventID.String(),
		TicketTypeID:   ticketType.ID.String(),
		Quantity:       2,
		IdempotencyKey: idempotencyKey,
	}

	tests := []struct {
		name     string
		progress string
	}{
		{
			name:     "同一 user 重複送出 idempotency_key",
			progress: "同一位使用者用相同 idempotency_key 重複申請時，第二次應回傳既有 application，不可再次扣 DB 或 Redis 庫存。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			first, err := service.Apply(users.Employee.ID, req)
			if err != nil {
				errs.Add(tt.progress, "第一次申請失敗：%v", err)
				return
			}
			if !first.Created {
				errs.Add(tt.progress, "預期第一次申請 Created=true")
				return
			}

			second, err := service.Apply(users.Employee.ID, req)
			if err != nil {
				errs.Add(tt.progress, "第二次申請失敗：%v", err)
				return
			}
			if second.Created {
				errs.Add(tt.progress, "預期第二次申請 Created=false")
				return
			}
			if second.Application.ID != first.Application.ID {
				errs.Add(tt.progress, "預期第二次回傳原 application %s，實際為 %s", first.Application.ID, second.Application.ID)
				return
			}

			var appCount int64
			if err := tx.Model(&model.Application{}).
				Where("idempotency_key = ? AND user_id = ?", idempotencyKey, users.Employee.ID).
				Count(&appCount).Error; err != nil {
				errs.Add(tt.progress, "查詢 application 數量失敗：%v", err)
				return
			}
			if appCount != 1 {
				errs.Add(tt.progress, "預期只建立 1 筆 application，實際為 %d 筆", appCount)
				return
			}

			ticketCount, err := ticketServiceTicketCountForApplication(tx, first.Application.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if ticketCount != 2 {
				errs.Add(tt.progress, "預期只建立 2 張票券，實際為 %d 張", ticketCount)
				return
			}

			remaining, err := ticketServiceRemaining(tx, ticketType.ID)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if remaining != 3 {
				errs.Add(tt.progress, "預期 DB 剩餘庫存只被扣一次後為 3，實際為 %d", remaining)
				return
			}

			redisStock, err := redisClient.Get(context.Background(), "inventory:"+ticketType.ID.String()).Int()
			if err != nil {
				errs.Add(tt.progress, "查詢 Redis 庫存失敗：%v", err)
				return
			}
			if redisStock != 3 {
				errs.Add(tt.progress, "預期 Redis 庫存只被扣一次後為 3，實際為 %d", redisStock)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}
