package manager

import (
	"fmt"
	"net/http"
	"testing"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestManagerApplicationBatchReview(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試活動管理者可以批次核准或批次拒絕 pending 申請",
			Target:      BatchReviewApplicationRoutesProcessPendingApplications,
		},
		{
			Description: "測試批次審核包含已處理申請時不會部分更新",
			Target:      BatchReviewApplicationRoutesRejectMixedStatusApplications,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func BatchReviewApplicationRoutesProcessPendingApplications(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("批次審核申請：確認管理者可以一次核准多筆 pending 申請，或一次拒絕多筆 pending 申請。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupManagerApplicationTest(t)
	if err != nil {
		errs.Add("準備批次審核申請測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerBatchApplicationRouter(tx, users.Manager)
	rejectReason := "批次審核不符合資格"
	reviewTests := []struct {
		name          string
		progress      string
		path          string
		payload       func([]model.Application) gin.H
		wantStatus    string
		wantReason    *string
		wantTickets   int64
		wantRemaining int
	}{
		{
			name:     "批次核准 pending 申請",
			progress: "批次核准兩筆待審核申請時，兩筆申請都應變成 approved，並依各自張數產生票券",
			path:     "/applications/batch/approve",
			payload: func(apps []model.Application) gin.H {
				return gin.H{"application_ids": managerApplicationIDs(apps)}
			},
			wantStatus:    "approved",
			wantTickets:   3,
			wantRemaining: 7,
		},
		{
			name:     "批次拒絕 pending 申請",
			progress: "批次拒絕兩筆待審核申請時，兩筆申請都應變成 rejected，寫入同一個拒絕原因並歸還庫存",
			path:     "/applications/batch/reject",
			payload: func(apps []model.Application) gin.H {
				return gin.H{
					"application_ids": managerApplicationIDs(apps),
					"reason":          rejectReason,
				}
			},
			wantStatus:    "rejected",
			wantReason:    &rejectReason,
			wantTickets:   0,
			wantRemaining: 10,
		},
	}

	for _, tt := range reviewTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", tt.progress))

			_, ticketType, apps, err := seedManagerBatchApplicationsFixture(
				tx,
				users,
				[]string{"pending", "pending"},
				[]int{1, 2},
				7,
			)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			resp := utils.PerformJSON(router, http.MethodPost, tt.path, tt.payload(apps))
			if resp.Code != http.StatusOK {
				errs.Add(tt.progress, "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
				return
			}

			body, err := decodeManagerApplicationListResponse(resp.Body.Bytes())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if !body.Success {
				errs.Add(tt.progress, "expected success=true")
			}
			if len(body.Data) != len(apps) {
				errs.Add(tt.progress, "expected %d applications in response, got %d", len(apps), len(body.Data))
			}

			for _, app := range apps {
				got, ok := managerApplicationByID(body.Data, app.ID.String())
				if !ok {
					errs.Add(tt.progress, "response should contain application %q", app.ID)
					continue
				}
				if got.Status != tt.wantStatus {
					errs.Add(tt.progress, "expected response status %q for application %q, got %q", tt.wantStatus, app.ID, got.Status)
				}
				if got.ReviewedBy == nil || *got.ReviewedBy != users.Manager.ID {
					errs.Add(tt.progress, "expected response reviewed_by %q for application %q, got %v", users.Manager.ID, app.ID, got.ReviewedBy)
				}
				if got.ReviewedAt == nil {
					errs.Add(tt.progress, "expected response reviewed_at to be set for application %q", app.ID)
				}
				if tt.wantReason != nil {
					if got.Reason == nil || *got.Reason != *tt.wantReason {
						errs.Add(tt.progress, "expected response reason %q for application %q, got %v", *tt.wantReason, app.ID, got.Reason)
					}
				}
			}

			var persisted []model.Application
			if err := tx.Preload("Tickets").Where("id IN ?", managerApplicationIDs(apps)).Find(&persisted).Error; err != nil {
				errs.Add(tt.progress, "讀取批次審核後申請失敗: %v", err)
				return
			}
			if len(persisted) != len(apps) {
				errs.Add(tt.progress, "expected %d persisted applications, got %d", len(apps), len(persisted))
			}
			for _, app := range persisted {
				if app.Status != tt.wantStatus {
					errs.Add(tt.progress, "expected persisted status %q for application %q, got %q", tt.wantStatus, app.ID, app.Status)
				}
				if app.ReviewedBy == nil || *app.ReviewedBy != users.Manager.ID {
					errs.Add(tt.progress, "expected persisted reviewed_by %q for application %q, got %v", users.Manager.ID, app.ID, app.ReviewedBy)
				}
				if app.ReviewedAt == nil {
					errs.Add(tt.progress, "expected persisted reviewed_at to be set for application %q", app.ID)
				}
				if tt.wantReason != nil {
					if app.Reason == nil || *app.Reason != *tt.wantReason {
						errs.Add(tt.progress, "expected persisted reason %q for application %q, got %v", *tt.wantReason, app.ID, app.Reason)
					}
				}
			}

			totalTickets, err := managerReviewTotalTicketCount(tx, managerApplicationIDs(apps))
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if totalTickets != tt.wantTickets {
				errs.Add(tt.progress, "expected %d tickets, got %d", tt.wantTickets, totalTickets)
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

func BatchReviewApplicationRoutesRejectMixedStatusApplications(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("批次審核防護：確認同一批次只要包含非 pending 申請，就不會部分更新。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupManagerApplicationTest(t)
	if err != nil {
		errs.Add("準備批次審核防護測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerBatchApplicationRouter(tx, users.Manager)
	reviewTests := []struct {
		name       string
		progress   string
		path       string
		payload    func([]model.Application) gin.H
		statuses   []string
		wantStatus []string
	}{
		{
			name:     "批次核准不可混入已核准申請",
			progress: "批次核准同時包含 pending 與 approved 申請時，應回傳 INVALID_STATUS，pending 申請也不可被部分核准",
			path:     "/applications/batch/approve",
			payload: func(apps []model.Application) gin.H {
				return gin.H{"application_ids": managerApplicationIDs(apps)}
			},
			statuses:   []string{"pending", "approved"},
			wantStatus: []string{"pending", "approved"},
		},
		{
			name:     "批次拒絕不可混入已拒絕申請",
			progress: "批次拒絕同時包含 pending 與 rejected 申請時，應回傳 INVALID_STATUS，pending 申請也不可被部分拒絕",
			path:     "/applications/batch/reject",
			payload: func(apps []model.Application) gin.H {
				return gin.H{
					"application_ids": managerApplicationIDs(apps),
					"reason":          "批次拒絕",
				}
			},
			statuses:   []string{"pending", "rejected"},
			wantStatus: []string{"pending", "rejected"},
		},
	}

	for _, tt := range reviewTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", tt.progress))

			_, ticketType, apps, err := seedManagerBatchApplicationsFixture(
				tx,
				users,
				tt.statuses,
				[]int{1, 2},
				7,
			)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			resp := utils.PerformJSON(router, http.MethodPost, tt.path, tt.payload(apps))
			if resp.Code != http.StatusBadRequest {
				errs.Add(tt.progress, "expected status 400, got %d with body %s", resp.Code, resp.Body.String())
				return
			}
			if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), "INVALID_STATUS"); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			var persisted []model.Application
			if err := tx.Where("id IN ?", managerApplicationIDs(apps)).Find(&persisted).Error; err != nil {
				errs.Add(tt.progress, "讀取批次審核防護後申請失敗: %v", err)
				return
			}
			if len(persisted) != len(apps) {
				errs.Add(tt.progress, "expected %d persisted applications, got %d", len(apps), len(persisted))
			}
			for index, app := range apps {
				got, ok := managerApplicationByID(persisted, app.ID.String())
				if !ok {
					errs.Add(tt.progress, "database should contain application %q", app.ID)
					continue
				}
				if got.Status != tt.wantStatus[index] {
					errs.Add(tt.progress, "expected application %q to stay %q, got %q", app.ID, tt.wantStatus[index], got.Status)
				}
				if got.ReviewedBy != nil || got.ReviewedAt != nil {
					errs.Add(tt.progress, "application %q should not be marked reviewed after rejected batch", app.ID)
				}
			}

			totalTickets, err := managerReviewTotalTicketCount(tx, managerApplicationIDs(apps))
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if totalTickets != 0 {
				errs.Add(tt.progress, "expected no tickets after rejected batch, got %d", totalTickets)
			}

			remaining, err := managerReviewTicketTypeRemaining(tx, ticketType.ID.String())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if remaining != 7 {
				errs.Add(tt.progress, "expected remaining to stay 7, got %d", remaining)
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func managerApplicationIDs(apps []model.Application) []string {
	ids := make([]string, 0, len(apps))
	for _, app := range apps {
		ids = append(ids, app.ID.String())
	}
	return ids
}

func managerApplicationByID(apps []model.Application, appID string) (model.Application, bool) {
	for _, app := range apps {
		if app.ID.String() == appID {
			return app, true
		}
	}
	return model.Application{}, false
}

func managerReviewTotalTicketCount(db *gorm.DB, appIDs []string) (int64, error) {
	var count int64
	if err := db.Model(&model.Ticket{}).Where("application_id IN ?", appIDs).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("計算批次審核後票券數量失敗: %w", err)
	}
	return count, nil
}
