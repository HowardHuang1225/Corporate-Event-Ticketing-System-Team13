package manager

import (
	"fmt"
	"net/http"
	"testing"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

func TestManagerApplicationReview(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試活動管理者可以核准或拒絕 pending 申請",
			Target:      ReviewApplicationRoutesProcessPendingApplications,
		},
		{
			Description: "測試已審核申請不可以重複審核",
			Target:      ReviewApplicationRoutesRejectAlreadyProcessedApplications,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func ReviewApplicationRoutesProcessPendingApplications(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("審核申請：確認管理者可以核准 pending 申請並發票，或拒絕 pending 申請並歸還庫存。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupManagerApplicationTest(t)
	if err != nil {
		errs.Add("準備審核申請測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerApplicationRouter(tx, users.Manager)
	rejectReason := "資格不符合本次活動"
	reviewTests := []struct {
		name          string
		progress      string
		pathSuffix    string
		payload       gin.H
		quantity      int
		remaining     int
		wantStatus    string
		wantReason    *string
		wantTickets   int64
		wantRemaining int
	}{
		{
			name:          "核准 pending 申請",
			progress:      "核准待審核申請時，申請狀態會變成 approved，並依照申請張數產生票券",
			pathSuffix:    "approve",
			payload:       gin.H{},
			quantity:      2,
			remaining:     8,
			wantStatus:    "approved",
			wantTickets:   2,
			wantRemaining: 8,
		},
		{
			name:          "拒絕 pending 申請",
			progress:      "拒絕待審核申請時，申請狀態會變成 rejected，會記錄原因並把保留票數加回庫存",
			pathSuffix:    "reject",
			payload:       gin.H{"reason": rejectReason},
			quantity:      2,
			remaining:     8,
			wantStatus:    "rejected",
			wantReason:    &rejectReason,
			wantTickets:   0,
			wantRemaining: 10,
		},
	}

	for _, tt := range reviewTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("檢查審核申請流程：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("- %s\n", tt.progress))

			_, ticketType, app, err := seedManagerApplicationFixture(tx, users, "pending", tt.quantity, tt.remaining)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			resp := utils.PerformJSON(router, http.MethodPost, "/applications/"+app.ID.String()+"/"+tt.pathSuffix, tt.payload)
			if resp.Code != http.StatusOK {
				errs.Add(tt.progress, "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
				return
			}

			body, err := decodeManagerApplicationResponse(resp.Body.Bytes())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if !body.Success {
				errs.Add(tt.progress, "expected success=true")
			}
			if body.Data.ID != app.ID {
				errs.Add(tt.progress, "expected application id %q, got %q", app.ID, body.Data.ID)
			}
			if body.Data.Status != tt.wantStatus {
				errs.Add(tt.progress, "expected response status %q, got %q", tt.wantStatus, body.Data.Status)
			}
			if body.Data.ReviewedBy == nil || *body.Data.ReviewedBy != users.Manager.ID {
				errs.Add(tt.progress, "expected response reviewed_by %q, got %v", users.Manager.ID, body.Data.ReviewedBy)
			}
			if body.Data.ReviewedAt == nil {
				errs.Add(tt.progress, "expected response reviewed_at to be set")
			}
			if tt.wantReason != nil {
				if body.Data.Reason == nil || *body.Data.Reason != *tt.wantReason {
					errs.Add(tt.progress, "expected response reason %q, got %v", *tt.wantReason, body.Data.Reason)
				}
			}

			var persisted model.Application
			if err := tx.Preload("Tickets").First(&persisted, "id = ?", app.ID).Error; err != nil {
				errs.Add(tt.progress, "讀取審核後申請失敗: %v", err)
				return
			}
			if persisted.Status != tt.wantStatus {
				errs.Add(tt.progress, "expected persisted status %q, got %q", tt.wantStatus, persisted.Status)
			}
			if persisted.ReviewedBy == nil || *persisted.ReviewedBy != users.Manager.ID {
				errs.Add(tt.progress, "expected persisted reviewed_by %q, got %v", users.Manager.ID, persisted.ReviewedBy)
			}
			if persisted.ReviewedAt == nil {
				errs.Add(tt.progress, "expected persisted reviewed_at to be set")
			}
			if tt.wantReason != nil {
				if persisted.Reason == nil || *persisted.Reason != *tt.wantReason {
					errs.Add(tt.progress, "expected persisted reason %q, got %v", *tt.wantReason, persisted.Reason)
				}
			}

			ticketCount, err := managerReviewTicketCount(tx, app.ID.String())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if ticketCount != tt.wantTickets {
				errs.Add(tt.progress, "expected %d tickets, got %d", tt.wantTickets, ticketCount)
			}

			remaining, err := managerReviewTicketTypeRemaining(tx, ticketType.ID.String())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if remaining != tt.wantRemaining {
				errs.Add(tt.progress, "expected remaining %d, got %d", tt.wantRemaining, remaining)
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func ReviewApplicationRoutesRejectAlreadyProcessedApplications(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("重複審核防護：確認 approved/rejected 申請不能再被核准或拒絕。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupManagerApplicationTest(t)
	if err != nil {
		errs.Add("準備重複審核測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerApplicationRouter(tx, users.Manager)
	reviewTests := []struct {
		name       string
		progress   string
		status     string
		pathSuffix string
		payload    gin.H
	}{
		{
			name:       "不能核准已核准申請",
			progress:   "對 approved 申請再次送出核准請求時，應回傳 INVALID_STATUS 且不得新增票券",
			status:     "approved",
			pathSuffix: "approve",
			payload:    gin.H{},
		},
		{
			name:       "不能拒絕已拒絕申請",
			progress:   "對 rejected 申請再次送出拒絕請求時，應回傳 INVALID_STATUS 且不得改變庫存",
			status:     "rejected",
			pathSuffix: "reject",
			payload:    gin.H{"reason": "再次拒絕"},
		},
	}

	for _, tt := range reviewTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("檢查重複審核防護：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("- %s\n", tt.progress))

			_, ticketType, app, err := seedManagerApplicationFixture(tx, users, tt.status, 1, 8)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			resp := utils.PerformJSON(router, http.MethodPost, "/applications/"+app.ID.String()+"/"+tt.pathSuffix, tt.payload)
			if resp.Code != http.StatusBadRequest {
				errs.Add(tt.progress, "expected status 400, got %d with body %s", resp.Code, resp.Body.String())
				return
			}
			if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), "INVALID_STATUS"); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			var persisted model.Application
			if err := tx.First(&persisted, "id = ?", app.ID).Error; err != nil {
				errs.Add(tt.progress, "讀取重複審核後申請失敗: %v", err)
				return
			}
			if persisted.Status != tt.status {
				errs.Add(tt.progress, "expected persisted status %q, got %q", tt.status, persisted.Status)
			}

			ticketCount, err := managerReviewTicketCount(tx, app.ID.String())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if ticketCount != 0 {
				errs.Add(tt.progress, "expected no new tickets, got %d", ticketCount)
			}

			remaining, err := managerReviewTicketTypeRemaining(tx, ticketType.ID.String())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if remaining != 8 {
				errs.Add(tt.progress, "expected remaining to stay 8, got %d", remaining)
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}
