# 後端 Test 紀錄

本文記錄目前 backend 既有 Go 測試的架構、覆蓋範圍、執行方式與後續可補測方向。

## 測試定位

後端測試目前以 Go 內建 `testing` 為主，搭配：

- Gin `httptest` 測 handler / route 行為
- GORM + PostgreSQL 測 service / handler 與資料狀態
- Redis 測票券申請庫存與 idempotency 相關流程
- 專案自訂 `backend/test_utils` 測試工具

目前共有 50 個 `*_test.go` 檔案，包含實際測試檔與 helper / fixture 測試檔。

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
- `backend/handler/employee/employee_handler_coverage_test.go`

測試內容：

- manager 情境可依 `draft`、`published`、`closed`、`ended` 篩選活動
- employee 情境看不到 draft 活動
- 活動列表可依 `status`、`ticket_type`、`start_from`、`start_to` 篩選
- 查詢單一活動會回傳活動詳細資料與票種
- 查詢不存在活動會回傳 `NOT_FOUND`
- 活動資格會依狀態與截止時間判斷可否申請
- 員工送出申請後，目前測試期待狀態為 `approved`
- 員工送出申請成功後，handler 測試會驗證實際建立對應數量票券、DB / Redis 庫存扣減、票券 QR token 為 UUID，以及重複 `idempotency_key` 不會重複出票或扣庫存
- 員工只能看到自己的申請紀錄
- 員工只能看到自己的已核准票券，且票券帶活動與票種資訊
- queue status handler 可從既有 application 回傳狀態，查無資料時回傳 `NOT_FOUND`
- queue mode handler 會回傳 `202 Accepted`、寫入 Redis queue status，且重複送出同一組 `idempotency_key` 會維持同一筆 queue application
- 取消自己的 application 會歸還庫存並刪除票券
- 單張退票 handler 會歸還庫存、建立 cancelled audit application，且不存在票券會回傳 `NOT_FOUND`
- 不合法申請 payload 會回傳 `VALIDATION_ERROR`
- 申請錯誤流程已補 handler 層：活動不存在、票種不存在、活動未發布、報名截止、售罄與超過每人上限

注意：`ticket_apply_test.go` 的 function 名稱與進度文字仍有「pending」字樣，但 assertion 已經驗證目前自動核准流程，也就是回應 status 需為 `approved`。

### Event Service

檔案：

- `backend/service/event/lifecycle_test.go`
- `backend/service/event/draft_mutation_test.go`
- `backend/service/event/draft_mutation_helpers_test.go`
- `backend/service/event/service_operations_test.go`

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
- 建立活動會寫入 draft event 與 ticket types，未指定時每人票券上限預設為 1
- 活動查詢會回傳清單與單筆資料，不存在活動回傳 `NOT_FOUND`
- 發布與關閉活動會更新狀態，非法發布會被拒絕
- 活動申請資格會依活動狀態、申請截止時間與不存在活動回傳結果
- event cache TTL 與 cache key helper 會使用穩定 fallback

### Ticket Service

檔案：

- `backend/service/ticket/service_test.go`
- `backend/service/ticket/apply_idempotency_test.go`
- `backend/service/ticket/cancellation_test.go`
- `backend/service/ticket/checkin_test.go`
- `backend/service/ticket/service_helpers_test.go`
- `backend/service/ticket/service_coverage_test.go`
- `backend/service/ticket/service_edge_coverage_test.go`
- `backend/service/ticket/queue_test.go`

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
- 員工票券列表會回傳申請、票券與核銷資料，manager 可查核銷資料
- 核銷流程新增成功、重複核銷、缺少/格式錯誤/不存在 QR token、錯誤 OTP、上一個 60 秒時間窗動態 QR 可核銷，以及超過允許時間窗的舊動態 QR 回 `EXPIRED_QR` 分支
- 核銷 API 只接受 `qr_token|otp` 兩段格式；裸 UUID、缺少 OTP 或 `qr_token|otp|extra` 這類 malformed dynamic QR 會回 `VALIDATION_ERROR`，不得核銷成功
- 無 Redis 與不合法申請輸入會被拒絕；有 Redis 時會覆蓋售罄、未發布、截止、超過上限等 business rule
- `MyApplications` / `MyTickets` 會優先使用 Redis cache，並有 cache miss fallback
- queue 設定可由環境變數載入，helper 會處理 key、status mapping、stream message 與錯誤代碼轉換
- queue 申請會建立 Redis waiting-room reservation，重複 idempotency key 會回傳同一筆 queue
- queue status 會先讀 Redis，沒有熱資料時回退到 DB application
- queue worker 會把有效 stream message 轉成 approved application 與票券，並更新 queue status
- queue 會拒絕無效輸入、無 Redis、滿載 waiting room、不可申請活動、截止活動、超過上限與不存在活動

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
- manager 使用超過允許時間窗的舊動態 QR token 核銷時會回 `EXPIRED_QR`，且不標記 used、不建立 checkin
- manager 使用上一個 60 秒時間窗的動態 QR token 核銷票券後，票券會標記 used 並建立 checkin
- 重複核銷會回傳 `ALREADY_CHECKED_IN`
- 已核銷票券不可退票，會回傳 `ALREADY_USED`
- 真實 route 權限矩陣會驗證未登入、employee、event_manager、HR 對主要 `/v1` routes 的允許與拒絕狀態
- route 權限矩陣已擴充到 employee 申請/票券/queue、manager checkin/checkins/upload、manager/HR report stats、HR CSV export 等主要路由
- `shared_utils_test.go` 提供 integration 測試共用的 setup、登入、HTTP request、Redis cleanup 與 response decode helper

### Manager Handler

檔案：

- `backend/handler/manager/event_create_test.go`
- `backend/handler/manager/event_action_test.go`
- `backend/handler/manager/ticket_checkin_test.go`
- `backend/handler/manager/event_mutation_upload_test.go`
- `backend/handler/manager/manager_handler_coverage_test.go`
- `backend/handler/manager/*_helpers_test.go`

目前啟用的測試內容：

- manager 建立活動會儲存 draft 活動與票種
- 建立活動時不合法 payload 會回傳 `VALIDATION_ERROR`
- 發布 draft 活動會更新狀態為 `published`
- 關閉 published 活動會更新狀態為 `closed`
- 發布非 draft 活動會回傳 `INVALID_STATUS`
- manager 可用 `qr_token|otp` 動態 QR token 核銷票券
- 核銷成功會標記 ticket used 並新增 checkin 紀錄
- 核銷缺少 token、空白 token、格式錯誤會回傳 `VALIDATION_ERROR`
- 核銷不存在 token 會回傳 `NOT_FOUND`
- 核銷已使用票券會回傳 `ALREADY_CHECKED_IN`
- 更新 draft event 會替換票種，不存在、非 draft 與不合法 payload 會被拒絕
- 刪除 draft event 會連同票種刪除，不存在或非 draft event 會被拒絕
- 上傳檔案 validation 會拒絕缺少檔案、錯誤副檔名、內容類型不符與無效圖片
- manager 可列出核銷紀錄並依活動篩選，也可查詢單一活動統計與不存在活動錯誤

因產品邏輯已改為員工申請自動核准，不再需要 manager approve/reject 審核流程；legacy manager 申請審核測試與核准後產票測試已移除。

### Report Service

檔案：`backend/service/report/service_test.go`

測試內容：

- 單一活動統計會計算報名、核准、取消、有效票券、核銷數、核銷率、部門分佈與票種統計
- 單一活動 CSV 匯出會包含對應 metrics
- 活動總覽會列出各活動報名與核銷統計，沒有票券的活動核銷率維持 0
- 查詢不存在活動會回傳 `NOT_FOUND`

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
- manager 單筆/批次審核 route、handler 與測試已移除；目前申請成功時由員工申請流程直接自動核准並產出票券
- `ticket_apply_test.go` 仍有舊命名與舊進度文字，但實際 assertion 已是自動核准的 `approved`
- 動態 QR 核銷目前後端實作只接受 `qr_token|otp` 兩段格式，不再接受裸 UUID token；service 測試已覆蓋錯誤 OTP 回傳 `EXPIRED_QR`、上一個 60 秒時間窗合法 OTP 成功核銷、超過允許時間窗的舊 OTP 核銷失敗，integration 測試也已用真實 `/v1/checkin` 驗證允許窗內舊碼可通過、超過允許窗舊碼會失敗
- malformed dynamic QR 測試已轉綠：`qr_token|otp|extra` 不會再被當成裸 `qr_token` 核銷成功
- 若之後要清理測試命名，可以把 `ApplyTicketCreatesPendingApplication` 改成描述自動核准的名稱，但這會是測試檔維護工作，不影響目前行為

## 後續可補方向

1. 員工申請錯誤流程

   handler 層已補活動不存在、票種不存在、活動未發布、報名截止、售罄、超過每人票數上限與重複 idempotency。後續若 queue worker 與真實 route 要一起測，可再補「排隊後 worker drain 成 approved 並出票」的 integration。

2. Employee 退票 handler

   handler 層已補成功退票與票券不存在。仍可再補 `/tickets/:id/cancel` 權限、票券屬於別人、已核銷不可退票等情境。

3. 動態 QR 核銷

   service 層已補錯誤 OTP 回傳 `EXPIRED_QR`、格式錯誤、裸 UUID 拒絕、上一個 60 秒時間窗的合法 `qr_token|otp` 成功核銷，以及超過允許時間窗的舊動態 QR 失敗案例；integration 層也已補允許窗內舊碼成功與超過允許窗舊碼失敗案例。

5. Route 權限矩陣

   目前已有 `backend/integration/role_matrix_test.go` 覆蓋一組基礎 `/v1` route 權限矩陣。可再擴充 manager / hr / employee 對更多主要 routes 的允許與拒絕案例。

6. HR 匯出錯誤流程

   可補匯出不存在活動、沒有統計資料、權限不足時的 response。

7. Scheduler 邊界

   可補同時間邊界、重複執行 idempotency，以及 scheduler 不應覆蓋手動 closed/ended 狀態的更多案例。
