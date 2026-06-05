package manager

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	eventsvc "ticketing-system/backend/service/event"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestManagerEventMutationAndUploadCoverage(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試管理者可更新 draft event 並替換票種",
			Target:      UpdateDraftEventRouteUpdatesEventAndTicketTypes,
		},
		{
			Description: "測試管理者更新 event 會拒絕不存在、非 draft 與不合法 payload",
			Target:      UpdateDraftEventRouteRejectsInvalidCases,
		},
		{
			Description: "測試管理者可刪除 draft event 並拒絕不存在或非 draft event",
			Target:      DeleteDraftEventRouteDeletesOnlyDraftEvents,
		},
		{
			Description: "測試管理者上傳檔案 validation 會拒絕缺少檔案、錯誤副檔名、內容類型不符與無效圖片",
			Target:      UploadFileRouteRejectsInvalidFilesBeforeStorage,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func UpdateDraftEventRouteUpdatesEventAndTicketTypes(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("更新活動：確認 manager handler 可更新 draft event 並替換票種。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupManagerEventTest(t)
	if err != nil {
		errs.Add("準備更新活動測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	event, err := seedManagerActionEvent(tx, manager, "draft")
	if err != nil {
		errs.Add("建立 draft 活動", "%v", err)
		return
	}

	now := time.Now().UTC().Truncate(time.Second)
	payload := validManagerCreateEventPayload(now)
	payload["title"] = "更新後公司日"
	payload["venue"] = "更新後大會堂"
	payload["max_tickets_per_person"] = 1
	payload["ticket_types"] = []gin.H{{"name": "更新票", "total_quota": 12}}

	router := newManagerEventMutationRouter(tx, manager)
	resp := utils.PerformJSON(router, http.MethodPut, "/events/"+event.ID.String(), payload)
	if resp.Code != http.StatusOK {
		errs.Add("更新 draft 活動", "expected 200, got %d body=%s", resp.Code, resp.Body.String())
		return
	}
	body, err := decodeManagerEventResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析更新活動回應", "%v", err)
		return
	}
	if !body.Success || body.Data.ID != event.ID || body.Data.Title != "更新後公司日" || body.Data.Venue != "更新後大會堂" {
		errs.Add("檢查更新活動回應", "回應不符預期：%+v", body)
		return
	}
	if len(body.Data.TicketTypes) != 1 || body.Data.TicketTypes[0].Name != "更新票" || body.Data.TicketTypes[0].TotalQuota != 12 || body.Data.TicketTypes[0].Remaining != 12 {
		errs.Add("檢查更新後票種", "票種不符預期：%+v", body.Data.TicketTypes)
		return
	}

	var persisted model.Event
	if err := tx.Preload("TicketTypes").First(&persisted, "id = ?", event.ID).Error; err != nil {
		errs.Add("重查更新後活動", "%v", err)
		return
	}
	if persisted.Title != "更新後公司日" || len(persisted.TicketTypes) != 1 || persisted.TicketTypes[0].Name != "更新票" {
		errs.Add("檢查更新後 DB 資料", "資料不符預期：%+v", persisted)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func UpdateDraftEventRouteRejectsInvalidCases(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("更新活動防呆：確認 handler 會拒絕不合法 payload、不存在 event 與非 draft event。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupManagerEventTest(t)
	if err != nil {
		errs.Add("準備更新活動防呆測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	published, err := seedManagerActionEvent(tx, manager, "published")
	if err != nil {
		errs.Add("建立 published 活動", "%v", err)
		return
	}

	now := time.Now().UTC().Truncate(time.Second)
	valid := validManagerCreateEventPayload(now)
	router := newManagerEventMutationRouter(tx, manager)

	missing := utils.PerformJSON(router, http.MethodPut, "/events/"+uuid.New().String(), valid)
	if missing.Code != http.StatusNotFound {
		errs.Add("更新不存在活動", "expected 404, got %d body=%s", missing.Code, missing.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(missing.Body.Bytes(), "NOT_FOUND"); err != nil {
		errs.Add("檢查不存在活動錯誤", "%v", err)
		return
	}

	nonDraft := utils.PerformJSON(router, http.MethodPut, "/events/"+published.ID.String(), valid)
	if nonDraft.Code != http.StatusBadRequest {
		errs.Add("更新非 draft 活動", "expected 400, got %d body=%s", nonDraft.Code, nonDraft.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(nonDraft.Body.Bytes(), "INVALID_STATUS"); err != nil {
		errs.Add("檢查非 draft 更新錯誤", "%v", err)
		return
	}

	invalid := validManagerCreateEventPayload(now)
	invalid["ticket_types"] = []gin.H{}
	invalidResp := utils.PerformJSON(router, http.MethodPut, "/events/"+published.ID.String(), invalid)
	if invalidResp.Code != http.StatusBadRequest {
		errs.Add("更新活動不合法 payload", "expected 400, got %d body=%s", invalidResp.Code, invalidResp.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(invalidResp.Body.Bytes(), "VALIDATION_ERROR"); err != nil {
		errs.Add("檢查更新活動 payload 錯誤", "%v", err)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func DeleteDraftEventRouteDeletesOnlyDraftEvents(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("刪除活動：確認 handler 只允許刪除 draft event，並會連同票種刪除。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupManagerEventTest(t)
	if err != nil {
		errs.Add("準備刪除活動測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	draft, err := seedManagerActionEvent(tx, manager, "draft")
	if err != nil {
		errs.Add("建立 draft 活動", "%v", err)
		return
	}
	published, err := seedManagerActionEvent(tx, manager, "published")
	if err != nil {
		errs.Add("建立 published 活動", "%v", err)
		return
	}

	router := newManagerEventMutationRouter(tx, manager)
	resp := performManagerDelete(router, "/events/"+draft.ID.String())
	if resp.Code != http.StatusOK {
		errs.Add("刪除 draft 活動", "expected 200, got %d body=%s", resp.Code, resp.Body.String())
		return
	}
	if count, err := managerEventCount(tx, draft.ID); err != nil {
		errs.Add("查詢刪除後活動", "%v", err)
		return
	} else if count != 0 {
		errs.Add("檢查刪除後活動", "預期活動已刪除，實際 %d 筆", count)
		return
	}
	if count, err := managerTicketTypeCountForEvent(tx, draft.ID); err != nil {
		errs.Add("查詢刪除後票種", "%v", err)
		return
	} else if count != 0 {
		errs.Add("檢查刪除後票種", "預期票種已刪除，實際 %d 筆", count)
		return
	}

	nonDraft := performManagerDelete(router, "/events/"+published.ID.String())
	if nonDraft.Code != http.StatusBadRequest {
		errs.Add("刪除非 draft 活動", "expected 400, got %d body=%s", nonDraft.Code, nonDraft.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(nonDraft.Body.Bytes(), "INVALID_STATUS"); err != nil {
		errs.Add("檢查刪除非 draft 錯誤", "%v", err)
		return
	}

	missing := performManagerDelete(router, "/events/"+uuid.New().String())
	if missing.Code != http.StatusNotFound {
		errs.Add("刪除不存在活動", "expected 404, got %d body=%s", missing.Code, missing.Body.String())
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func UploadFileRouteRejectsInvalidFilesBeforeStorage(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("上傳檔案防呆：確認 handler 在呼叫 storage 前拒絕不合法檔案。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, manager, cleanup, err := setupManagerEventTest(t)
	if err != nil {
		errs.Add("準備上傳檔案測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerEventMutationRouter(tx, manager)

	noFile := performManagerMultipartUpload(router, "/upload", "", nil)
	if noFile.Code != http.StatusBadRequest {
		errs.Add("未提供檔案", "expected 400, got %d body=%s", noFile.Code, noFile.Body.String())
		return
	}

	badExt := performManagerMultipartUpload(router, "/upload", "notes.txt", []byte("plain text"))
	if badExt.Code != http.StatusBadRequest {
		errs.Add("副檔名不合法", "expected 400, got %d body=%s", badExt.Code, badExt.Body.String())
		return
	}

	mismatch := performManagerMultipartUpload(router, "/upload", "fake.png", []byte("%PDF-1.4\n%fake pdf\n"))
	if mismatch.Code != http.StatusBadRequest {
		errs.Add("副檔名與內容不符", "expected 400, got %d body=%s", mismatch.Code, mismatch.Body.String())
		return
	}

	invalidContent := performManagerMultipartUpload(router, "/upload", "fake.png", []byte("not a real image"))
	if invalidContent.Code != http.StatusBadRequest {
		errs.Add("檔案內容不合法", "expected 400, got %d body=%s", invalidContent.Code, invalidContent.Body.String())
		return
	}

	pngHeaderButInvalidImage := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	invalidImage := performManagerMultipartUpload(router, "/upload", "broken.png", pngHeaderButInvalidImage)
	if invalidImage.Code != http.StatusBadRequest {
		errs.Add("圖片格式無效", "expected 400, got %d body=%s", invalidImage.Code, invalidImage.Body.String())
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func newManagerEventMutationRouter(db *gorm.DB, manager model.User) *gin.Engine {
	repos := repository.New(db, nil)
	handler := New(eventsvc.New(repos), nil, nil, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.PUT("/events/:id", handler.UpdateEvent)
	router.DELETE("/events/:id", handler.DeleteEvent)
	router.POST("/upload", handler.UploadFile)
	return router
}

func performManagerDelete(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func performManagerMultipartUpload(router *gin.Engine, path string, filename string, content []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if filename != "" {
		part, _ := writer.CreateFormFile("file", filename)
		_, _ = part.Write(content)
	}
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func managerEventCount(db *gorm.DB, eventID uuid.UUID) (int64, error) {
	var count int64
	if err := db.Model(&model.Event{}).Where("id = ?", eventID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func managerTicketTypeCountForEvent(db *gorm.DB, eventID uuid.UUID) (int64, error) {
	var count int64
	if err := db.Model(&model.TicketType{}).Where("event_id = ?", eventID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
