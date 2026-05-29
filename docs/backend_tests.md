# 後端 Test 紀錄

本文記錄目前 backend 既有 Go 測試的架構、覆蓋範圍、執行方式與後續可補測方向。

## 測試定位

後端測試目前以 Go 內建 `testing` 為主，搭配：

- Gin `httptest` 測 handler / route 行為
- GORM + PostgreSQL 測 service / handler 與資料狀態
- Redis 測票券申請庫存與 idempotency 相關流程
- 專案自訂 `backend/test_utils` 測試工具

目前共有 45 個 `*_test.go` 檔案，包含實際測試檔與 helper / fixture 測試檔。

## 執行方式

依專案規則，後端測試需在 WSL 環境執行，Windows 環境沒有安裝 Go。

常用指令：

```bash
cd backend
go test ./...
```

可針對單一 package 執行：

```bash
go test ./handler/employee
go test ./handler/manager
go test ./service/ticket
go test ./scheduler
```

資料庫與 Redis：

- 測試資料庫會讀 `TEST_DATABASE_URL`，沒有時 fallback 到 `DATABASE_URL` 或 DB_* 環境變數
- Redis 測試會讀 `TEST_REDIS_URL`，沒有時 fallback 到 `REDIS_URL` 或 `redis://localhost:6379`
- 多數 DB 測試使用 transaction，結束時 rollback

## 共用測試工具

檔案：

- `backend/test_utils/setup.go`
- `backend/test_utils/main_func.go`
- `backend/test_utils/https.go`
- `backend/test_utils/error_handler.go`

用途：

- `BeginTestTransaction`：開啟測試 DB transaction，測試結束 rollback
- `OpenTestDB`：連線測試 DB 並 AutoMigrate 測試需要的 models
- `SeedTestRole`：建立測試使用者並套用 bcrypt password
- `RunTestTasks`：用中文描述包裝測試任務，集中收集錯誤
- `PrintTestProgress`：讓測試過程輸出中文進度
- `PerformJSON` / `PerformLogin`：簡化 Gin JSON request
- `AssertHandlerErrorCode`：驗證統一錯誤格式中的 error code

## 目前測試範圍

### Config

檔案：`backend/config/config_test.go`

測試內容：

- `Load` 會優先使用 `DATABASE_URL`
- 未設定 `DATABASE_URL` 時，會用 `DB_HOST`、`DB_USER`、`DB_PASSWORD`、`DB_NAME`、`DB_PORT` 組 PostgreSQL DSN
- `REDIS_URL`、`JWT_SECRET`、`PORT`、`ALLOWED_ORIGINS`、DB connection pool 設定可使用預設值或環境變數覆寫

### JWT / Middleware / Route Guard

檔案：

- `backend/pkg/jwt_test.go`
- `backend/pkg/jwt_cases_test.go`
- `backend/middleware/auth_test.go`
- `backend/routes/routes_test.go`

測試內容：

- JWT 產生後可用同 secret 驗證 claims
- claims 包含 `user_id`、`employee_id`、`role` 與時間欄位
- 錯誤 secret、malformed token、不支援簽章都會驗證失敗
- 缺少 Authorization header、格式錯誤、簽章錯誤會回傳 `UNAUTHORIZED`
- 合法 token 會把 `user_id`、`employee_id`、`role` 寫入 Gin context
- 角色不符會回傳 `FORBIDDEN`
- employee 不可存取 manager 建立活動路由 `/v1/events`

### Shared Handler Helpers

檔案：`backend/handler/shared/context_response_test.go`

測試內容：

- `UserID` 可從 Gin context 讀出合法 UUID
- 缺少或格式錯誤的 `user_id` 會回傳 `UNAUTHORIZED`
- `Role` 可從 Gin context 讀取 role，缺少時回空字串
- `WriteError` 會把 `apperror.Error` 轉成統一錯誤格式
- 未知錯誤會轉成 `INTERNAL_ERROR`

### Auth

檔案：

- `backend/handler/auth/handler_test.go`
- `backend/handler/auth/data_test.go`
- `backend/service/auth/service_test.go`
- `backend/service/auth/service_helpers_test.go`

測試內容：

- demo 帳號可登入並取得正確角色、廠區與 JWT claims
- 不存在的員工編號與錯誤密碼會回傳 `UNAUTHORIZED`
- `Me` 會依 `user_id` 回傳 user DTO
- `Me` 遇到非法 UUID 回傳 `UNAUTHORIZED`
- `Me` 遇到不存在 user 回傳 `NOT_FOUND`
- 停用帳號即使密碼正確也不可登入

### Employee Handler

檔案：

- `backend/handler/employee/event_list_test.go`
- `backend/handler/employee/event_detail_test.go`
- `backend/handler/employee/event_eligibility_test.go`
- `backend/handler/employee/ticket_apply_test.go`
- `backend/handler/employee/application_status_test.go`
- `backend/handler/employee/ticket_my_test.go`
- `backend/handler/employee/event_helpers_test.go`

測試內容：

- manager 情境可依 `draft`、`published`、`closed`、`ended` 篩選活動
- employee 情境看不到 draft 活動
- 活動列表可依 `status`、`ticket_type`、`start_from`、`start_to` 篩選
- 查詢單一活動會回傳活動詳細資料與票種
- 查詢不存在活動會回傳 `NOT_FOUND`
- 活動資格會依狀態與截止時間判斷可否申請
- 員工送出申請後，目前測試期待狀態為 `approved`
- 員工只能看到自己的申請紀錄
- 員工只能看到自己的已核准票券，且票券帶活動與票種資訊

注意：`ticket_apply_test.go` 的 function 名稱與進度文字仍有「pending」字樣，但 assertion 已經驗證目前自動核准流程，也就是回應 status 需為 `approved`。

### Event Service

檔案：

- `backend/service/event/lifecycle_test.go`
- `backend/service/event/draft_mutation_test.go`
- `backend/service/event/draft_mutation_helpers_test.go`

測試內容：

- `StatusAt` 依目前時間推算活動狀態
- draft 維持 draft
- published 在報名截止後變 closed
- closed 在活動結束後變 ended
- ended 維持 ended
- `validateTimeline` 接受合法時間範圍
- `validateTimeline` 拒絕缺少必要時間
- `validateTimeline` 拒絕 publish/start/apply/end 時序不合法
- draft 活動可更新活動欄位並重建票種
- 非 draft 活動不可更新或刪除
- 刪除 draft 活動會刪除對應票種
- draft 更新失敗時不會改動既有活動與票種

### Ticket Service

檔案：

- `backend/service/ticket/service_test.go`
- `backend/service/ticket/apply_idempotency_test.go`
- `backend/service/ticket/cancellation_test.go`
- `backend/service/ticket/checkin_test.go`
- `backend/service/ticket/service_helpers_test.go`

測試內容：

- 取消 pending application 會歸還庫存
- 取消 approved application 會歸還庫存並刪除票券
- approved application 只要包含已使用票券，就不可整筆取消
- 單張退票會刪除原票券、庫存加一，並新增 cancelled audit application
- 已使用單張票券不可退票，且不新增 audit application
- 重複 `idempotency_key` 不會重複建立 application、票券或扣庫存
- Redis inventory 也會維持只扣一次
- 核銷過期票券會回傳 `TICKET_EXPIRED`
- 過期票券不可被標記為已使用，也不可新增 checkin 紀錄

### Integration

檔案：

- `backend/integration/employee_application_flow_test.go`
- `backend/integration/ticket_lifecycle_flow_test.go`
- `backend/integration/role_matrix_test.go`
- `backend/integration/shared_utils_test.go`

測試內容：

- 使用真實 `/v1` router、JWT middleware、handler、service、DB transaction 與 Redis 測員工申請流程
- 員工登入後送出申請會自動核准、建立 application 與對應 ticket，且 DB / Redis 庫存同步扣除
- 相同 `idempotency_key` 重送不會重複建 application、票券或扣庫存
- 員工可查到自己的已核准申請與票券
- 員工退還未使用票券會刪除原票券、回補庫存並建立 cancelled audit application
- manager 使用 QR token 核銷票券後，票券會標記 used 並建立 checkin
- 重複核銷會回傳 `ALREADY_CHECKED_IN`
- 已核銷票券不可退票，會回傳 `ALREADY_USED`
- 真實 route 權限矩陣會驗證未登入、employee、event_manager、HR 對主要 `/v1` routes 的允許與拒絕狀態
- `shared_utils_test.go` 提供 integration 測試共用的 setup、登入、HTTP request、Redis cleanup 與 response decode helper

### Manager Handler

檔案：

- `backend/handler/manager/event_create_test.go`
- `backend/handler/manager/event_action_test.go`
- `backend/handler/manager/ticket_checkin_test.go`
- `backend/handler/manager/ticket_generation_test.go`
- `backend/handler/manager/application_review_test.go`
- `backend/handler/manager/application_batch_review_test.go`
- `backend/handler/manager/*_helpers_test.go`

目前啟用的測試內容：

- manager 建立活動會儲存 draft 活動與票種
- 建立活動時不合法 payload 會回傳 `VALIDATION_ERROR`
- 發布 draft 活動會更新狀態為 `published`
- 關閉 published 活動會更新狀態為 `closed`
- 發布非 draft 活動會回傳 `INVALID_STATUS`
- manager 可用 QR token / UUID token 核銷票券
- 核銷成功會標記 ticket used 並新增 checkin 紀錄
- 核銷缺少 token、空白 token、格式錯誤會回傳 `VALIDATION_ERROR`
- 核銷不存在 token 會回傳 `NOT_FOUND`
- 核銷已使用票券會回傳 `ALREADY_CHECKED_IN`
- 票券產生測試會驗證 approve 後產生的 ticket 與 QR token 皆為 UUID 且唯一

目前跳過的測試：

- `backend/handler/manager/application_review_test.go`
- `backend/handler/manager/application_batch_review_test.go`

跳過原因是產品邏輯已改為員工申請自動核准，不再需要 manager approve/reject 審核流程。這兩個測試內仍保留 legacy 單筆與批次審核邏輯，但 `TestManagerApplicationReview` 與 `TestManagerApplicationBatchReview` 都已 `t.Skip`。

### HR Report Handler

檔案：

- `backend/handler/hr/event_stats_test.go`
- `backend/handler/hr/overview_test.go`
- `backend/handler/hr/report_helpers_test.go`

測試內容：

- HR 可查單一活動統計
- 單一活動統計包含報名筆數、申請票數、報名人數、核准筆數、核准票數、取消票數
- 統計包含已發票券、已核銷票券、已核銷人數與核銷率
- 核准使用者會依部門與地區分組
- 每個票種會回報申請、核准、取消與有效票券數
- 查詢不存在活動統計會回傳 `NOT_FOUND`
- HR overview 會列出各活動報名與核銷統計
- 僅有待審報名、尚未發票活動的核銷率維持 0
- 單一活動 CSV 匯出會帶正確 `Content-Type` 與 `Content-Disposition`
- CSV 會包含報名、核准、取消、票券與核銷率 metrics

### Scheduler

檔案：

- `backend/scheduler/scheduler_test.go`
- `backend/scheduler/scheduler_cases_test.go`
- `backend/scheduler/scheduler_helpers_test.go`

測試內容：

- publishing worker 只會發布已到 `publish_time` 的 draft 活動
- 尚未到 publish time 的 draft 會維持 draft
- 已 published / closed 活動不會被 publishing worker 改掉
- event state scheduler 會依時間自動轉換：
  - draft -> published
  - published -> closed
  - closed -> ended
- scheduler 測試會實際啟動短 interval worker，並等待狀態更新

## 已知狀態與注意事項

- 後端測試需要 PostgreSQL；部分 ticket/employee apply 測試需要 Redis
- 測試檔案中的註解、進度與子測試描述大多已使用中文
- manager 單筆/批次審核測試目前是 legacy 且跳過，符合自動核准的新流程
- `ticket_apply_test.go` 仍有舊命名與舊進度文字，但實際 assertion 已是自動核准的 `approved`
- 動態 QR 核銷目前後端實作可解析 `qr_token|otp` 並保留裸 UUID token 相容性；既有後端測試主要仍覆蓋 UUID token 核銷流程，動態 OTP 成功/失敗案例尚未補齊
- 若之後要清理測試命名，可以把 `ApplyTicketCreatesPendingApplication` 改成描述自動核准的名稱，但這會是測試檔維護工作，不影響目前行為

## 後續可補方向

1. 員工自動核准後的 handler 細節

   目前 `ticket_apply_test.go` 驗證申請回應是 `approved`，但可再補：成功後實際建立對應數量 ticket、扣庫存、ticket QR token 格式、以及 application/ticket DB 關聯。

2. 員工申請錯誤流程

   可補活動不存在、票種不存在、活動未發布、報名截止、庫存不足、超過每人票數上限、重複 idempotency 的 handler 層測試。

3. Employee 退票 handler

   service 已測單張退票，但 handler 層可補 `/tickets/:id/cancel` 權限、成功 response、票券不存在、票券屬於別人、已核銷不可退票等情境。

4. 動態 QR 核銷

   可補合法 `qr_token|otp` 可核銷、錯誤或過期 OTP 回傳 `EXPIRED_QR`、格式錯誤維持 `VALIDATION_ERROR`，以及裸 UUID token 相容流程仍可使用。

5. Route 權限矩陣

   目前已有 `backend/integration/role_matrix_test.go` 覆蓋一組基礎 `/v1` route 權限矩陣。可再擴充 manager / hr / employee 對更多主要 routes 的允許與拒絕案例。

6. HR 匯出錯誤流程

   可補匯出不存在活動、沒有統計資料、權限不足時的 response。

7. Scheduler 邊界

   可補同時間邊界、重複執行 idempotency，以及 scheduler 不應覆蓋手動 closed/ended 狀態的更多案例。
