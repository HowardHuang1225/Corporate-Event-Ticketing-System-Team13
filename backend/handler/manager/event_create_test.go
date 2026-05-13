package manager

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"ticketing-system/backend/model"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

func TestManagerCreateEvent(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試建立活動成功時會儲存草稿活動與票種",
			Target:      CreateEventStoresDraftWithTicketTypes,
		},
		{
			Description: "測試建立活動時會拒絕不合法的請求內容",
			Target:      CreateEventRejectsInvalidPayloads,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func CreateEventStoresDraftWithTicketTypes(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("建立活動：確認活動管理者送出合法請求後，系統會建立草稿活動並儲存票種。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupManagerEventTest(t)
	if err != nil {
		errs.Add("準備建立活動測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerEventRouter(tx, manager)
	now := time.Now().UTC().Truncate(time.Second)
	payload := validManagerCreateEventPayload(now)

	resp := utils.PerformJSON(router, http.MethodPost, "/events", payload)
	if resp.Code != http.StatusCreated {
		errs.Add("送出合法建立活動請求", "expected status 201, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeManagerEventResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析建立活動回應", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("解析建立活動回應", "expected success=true")
		return
	}
	event := body.Data
	if event.Title != payload["title"] {
		errs.Add("檢查建立活動回應內容", "expected title %q, got %q", payload["title"], event.Title)
		return
	}
	if event.Status != "draft" {
		errs.Add("檢查建立活動回應內容", "expected status draft, got %q", event.Status)
		return
	}
	if event.CreatedBy != manager.ID {
		errs.Add("檢查建立活動回應內容", "expected created_by %q, got %q", manager.ID, event.CreatedBy)
		return
	}
	if event.MaxTicketsPerPerson != 2 {
		errs.Add("檢查建立活動回應內容", "expected max_tickets_per_person 2, got %d", event.MaxTicketsPerPerson)
		return
	}
	if len(event.TicketTypes) != 2 {
		errs.Add("檢查建立活動回應內容", "expected 2 ticket types, got %d", len(event.TicketTypes))
		return
	}
	for _, ticketType := range event.TicketTypes {
		if ticketType.Remaining != ticketType.TotalQuota {
			errs.Add("檢查建立活動回應內容", "expected remaining quota to equal total quota for %q", ticketType.Name)
			return
		}
	}

	var persisted model.Event
	if err := tx.Preload("TicketTypes").First(&persisted, "id = ?", event.ID).Error; err != nil {
		errs.Add("從資料庫讀取剛建立的活動", "expected event to be persisted: %v", err)
		return
	}
	if persisted.Status != "draft" {
		errs.Add("檢查資料庫中的活動內容", "expected status draft, got %q", persisted.Status)
		return
	}
	if len(persisted.TicketTypes) != 2 {
		errs.Add("檢查資料庫中的活動內容", "expected 2 persisted ticket types, got %d", len(persisted.TicketTypes))
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func CreateEventRejectsInvalidPayloads(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("建立活動欄位驗證：確認不合法請求會回傳 VALIDATION_ERROR。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupManagerEventTest(t)
	if err != nil {
		errs.Add("準備建立活動驗證測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerEventRouter(tx, manager)
	now := time.Now().UTC().Truncate(time.Second)
	tests := managerCreateInvalidCases(now)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", tt.progress))

			payload := validManagerCreateEventPayload(now)
			tt.mutate(payload)

			resp := utils.PerformJSON(router, http.MethodPost, "/events", payload)
			if resp.Code != http.StatusBadRequest {
				errs.Add(tt.progress, "expected status 400, got %d with body %s", resp.Code, resp.Body.String())
				return
			}
			if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), "VALIDATION_ERROR"); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}
