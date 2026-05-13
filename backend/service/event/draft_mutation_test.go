package event

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	utils "ticketing-system/backend/test_utils"
)

func TestEventDraftMutations(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試更新 draft 活動與票種",
			Target:      UpdateDraftEventAndTicketTypes,
		},
		{
			Description: "測試非 draft 活動不可更新或刪除",
			Target:      RejectNonDraftUpdateOrDelete,
		},
		{
			Description: "測試刪除 draft 活動會刪除對應票種",
			Target:      DeleteDraftEventDeletesTicketTypes,
		},
		{
			Description: "測試更新 draft 活動失敗時不會改動既有資料",
			Target:      RejectInvalidDraftUpdateWithoutMutation,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func UpdateDraftEventAndTicketTypes(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試更新 draft 活動與票種\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupEventServiceDraftMutationTest(t)
	if err != nil {
		errs.Add("準備更新 draft 活動測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newEventServiceForTest(tx)
	now := time.Now().UTC().Truncate(time.Second)
	tests := []struct {
		name     string
		progress string
		req      CreateRequest
	}{
		{
			name:     "更新活動欄位並重建票種",
			progress: "draft 活動更新時，應更新活動欄位、刪除舊票種並以新票種重建，且新票種 remaining 應等於 total_quota。",
			req:      updatedDraftEventRequest(now),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			event, originalTicketTypes, err := seedEventServiceEventWithTicketTypes(tx, manager, "draft")
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			originalTicketTypeIDs := eventServiceTicketTypeIDs(originalTicketTypes)

			updated, err := service.UpdateDraft(event.ID.String(), tt.req)
			if err != nil {
				errs.Add(tt.progress, "更新 draft 活動失敗：%v", err)
				return
			}

			if updated.ID != event.ID {
				errs.Add(tt.progress, "預期更新後活動 ID 維持 %s，實際為 %s", event.ID, updated.ID)
				return
			}
			if updated.Status != "draft" {
				errs.Add(tt.progress, "預期更新後仍為 draft，實際為 %q", updated.Status)
				return
			}
			if err := assertEventServiceEventMatchesRequest(updated, tt.req); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if err := assertEventServiceTicketTypesMatch(updated.TicketTypes, eventServiceExpectedTicketTypes(tt.req)); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if err := assertEventServiceTicketTypesDeleted(tx, originalTicketTypeIDs); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			persisted, err := eventServiceEventByID(tx, event.ID.String())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if err := assertEventServiceEventMatchesRequest(persisted, tt.req); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if err := assertEventServiceTicketTypesMatch(persisted.TicketTypes, eventServiceExpectedTicketTypes(tt.req)); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func RejectNonDraftUpdateOrDelete(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試非 draft 活動不可更新或刪除\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupEventServiceDraftMutationTest(t)
	if err != nil {
		errs.Add("準備非 draft 活動測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newEventServiceForTest(tx)
	now := time.Now().UTC().Truncate(time.Second)
	statuses := []string{"published", "closed", "ended"}

	for _, status := range statuses {
		status := status
		t.Run("不可更新 "+status+" 活動", func(t *testing.T) {
			progress := fmt.Sprintf("%s 活動不可透過 UpdateDraft 更新，資料與票種都應維持原狀。", status)
			t.Logf("子測試：%s", progress)
			utils.PrintTestProgress("子測試：" + progress + "\n")

			event, originalTicketTypes, err := seedEventServiceEventWithTicketTypes(tx, manager, status)
			if err != nil {
				errs.Add(progress, "%v", err)
				return
			}

			_, err = service.UpdateDraft(event.ID.String(), updatedDraftEventRequest(now))
			if assertErr := assertEventServiceAppError(err, http.StatusBadRequest, "INVALID_STATUS"); assertErr != nil {
				errs.Add(progress, "%v", assertErr)
				return
			}
			if err := assertEventServiceEventUnchanged(tx, event, originalTicketTypes); err != nil {
				errs.Add(progress, "%v", err)
				return
			}
		})

		t.Run("不可刪除 "+status+" 活動", func(t *testing.T) {
			progress := fmt.Sprintf("%s 活動不可透過 DeleteDraft 刪除，資料與票種都應維持原狀。", status)
			t.Logf("子測試：%s", progress)
			utils.PrintTestProgress("子測試：" + progress + "\n")

			event, originalTicketTypes, err := seedEventServiceEventWithTicketTypes(tx, manager, status)
			if err != nil {
				errs.Add(progress, "%v", err)
				return
			}

			err = service.DeleteDraft(event.ID.String())
			if assertErr := assertEventServiceAppError(err, http.StatusBadRequest, "INVALID_STATUS"); assertErr != nil {
				errs.Add(progress, "%v", assertErr)
				return
			}
			if err := assertEventServiceEventUnchanged(tx, event, originalTicketTypes); err != nil {
				errs.Add(progress, "%v", err)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func DeleteDraftEventDeletesTicketTypes(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試刪除 draft 活動會刪除對應票種\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupEventServiceDraftMutationTest(t)
	if err != nil {
		errs.Add("準備刪除 draft 活動測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newEventServiceForTest(tx)
	tests := []struct {
		name     string
		progress string
	}{
		{
			name:     "刪除 draft 活動",
			progress: "刪除 draft 活動時，應同時刪除該活動底下所有 ticket_types。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			event, ticketTypes, err := seedEventServiceEventWithTicketTypes(tx, manager, "draft")
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			if err := service.DeleteDraft(event.ID.String()); err != nil {
				errs.Add(tt.progress, "刪除 draft 活動失敗：%v", err)
				return
			}
			if exists, err := eventServiceEventExists(tx, event.ID.String()); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			} else if exists {
				errs.Add(tt.progress, "預期活動 %s 已被刪除", event.ID)
				return
			}
			if err := assertEventServiceTicketTypesDeleted(tx, eventServiceTicketTypeIDs(ticketTypes)); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if count, err := eventServiceTicketTypeCountByEvent(tx, event.ID); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			} else if count != 0 {
				errs.Add(tt.progress, "預期活動底下票種已全數刪除，實際仍有 %d 筆", count)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func RejectInvalidDraftUpdateWithoutMutation(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試更新 draft 活動失敗時不會改動既有資料\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupEventServiceDraftMutationTest(t)
	if err != nil {
		errs.Add("準備更新失敗測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newEventServiceForTest(tx)
	now := time.Now().UTC().Truncate(time.Second)
	tests := []struct {
		name     string
		progress string
		req      CreateRequest
	}{
		{
			name:     "時間順序錯誤不更新",
			progress: "UpdateDraft 收到不合法時間順序時，應回傳 VALIDATION_ERROR，且活動與原票種都不可被改動。",
			req:      invalidTimelineDraftEventRequest(now),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			event, ticketTypes, err := seedEventServiceEventWithTicketTypes(tx, manager, "draft")
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			_, err = service.UpdateDraft(event.ID.String(), tt.req)
			if assertErr := assertEventServiceAppError(err, http.StatusBadRequest, "VALIDATION_ERROR"); assertErr != nil {
				errs.Add(tt.progress, "%v", assertErr)
				return
			}
			if err := assertEventServiceEventUnchanged(tx, event, ticketTypes); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}
