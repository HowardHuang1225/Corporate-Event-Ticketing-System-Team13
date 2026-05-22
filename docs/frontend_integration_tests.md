# 前端 Integration Test 紀錄

本文記錄目前前端 Vitest integration test 的測試邊界、已覆蓋流程，以及後續建議補測項目。

## 測試定位

Integration test 放在 `frontend/src/integration/`，命名為 `*.integration.test.tsx`。

目前 integration test 以 `App` 為進入點，讓以下部分一起運作：

- `BrowserRouter` 與前端路由
- `AuthProvider` 與 localStorage token 還原
- `ProtectedRoute` 未登入導向
- `Layout` 角色導覽
- 頁面之間的使用者互動流程
- React Query query / mutation 行為

API 邊界仍 mock 在 `frontend/src/api/client.ts`，不直接呼叫真實後端。這讓 integration test 專注驗證前端元件、狀態、路由與 API 呼叫契約是否能串起來。

員工申請活動目前會自動核准並產出票券，因此 integration test 不應再驗證 manager approve/reject 審核流程。

## 目前 Integration Tests

### Auth Flow

檔案：`frontend/src/integration/auth-flow.integration.test.tsx`

測試內容：

- 未登入使用者進入 `/events` 會被 `ProtectedRoute` 導向 `/login`
- 從 `/login` 登入成功後會儲存 token，進入 `/events`，並載入活動列表
- localStorage 已有 token 時，`AuthProvider` 會呼叫 `/auth/me` 還原使用者
- 還原 event manager 身份後，layout 會顯示管理者導覽，例如「活動管理」與「現場核銷」
- 已登入使用者按下「登出」後會清除 token，回到 `/login`，並移除原頁面內容

### Employee Ticket Flow

檔案：`frontend/src/integration/employee-ticket-flow.integration.test.tsx`

測試內容：

- 員工帶著既有 token 進入活動列表
- 從活動列表點進活動詳情
- 在活動詳情選擇票種並送出申請
- 驗證 `/applications` payload 包含 `event_id`、`ticket_type_id`、`quantity`、`idempotency_key`
- 申請成功後進入「我的票券」
- 驗證自動核准的申請記錄、未使用電子票券與 QR token 會顯示
- 員工在「我的票券」確認退票後，會呼叫 `/tickets/:id/cancel`
- 退票成功後會重新查詢票券資料，確保 React Query invalidate/refetch 流程可串起來

### Manager Operations Flow

檔案：`frontend/src/integration/manager-operations.integration.test.tsx`

測試內容：

- Manager 帶著既有 token 進入 `/manage/events`
- 建立活動草稿並送出 `/events`
- 驗證活動 payload 會帶入標題、地點與地域
- 透過側邊導覽切到「現場核銷」
- 輸入含空白的 QR token 後送出核銷
- 驗證 `/checkin` payload 會 trim QR token
- 核銷成功後顯示票券與使用者資訊
- 從活動管理頁發布草稿活動，會呼叫 `/events/:id/publish`
- 從活動管理頁截止已發布活動，會呼叫 `/events/:id/close`
- 現場核銷遇到 `ALREADY_CHECKED_IN` 時會顯示已核銷錯誤
- 現場核銷遇到 `NOT_FOUND` 時會顯示找不到票券錯誤

### HR Report Flow

檔案：`frontend/src/integration/hr-report-flow.integration.test.tsx`

測試內容：

- HR 帶著既有 token 進入 `/reports`
- 載入活動總覽與活動下拉選單
- 選擇活動後查詢 `/reports/events/:id/stats`
- 顯示申請階段、核准與退票階段、核銷與出席率
- 顯示部門分佈與票種統計
- 透過側邊導覽回到活動列表

## 目前 Coverage

最近一次執行 `npm run test:coverage` 的結果：

| 指標 | 覆蓋率 |
| --- | ---: |
| Statements | 88.13% |
| Branches | 73.08% |
| Functions | 83.15% |
| Lines | 88.21% |

`Applications.tsx` 仍存在於前端實作並會被 coverage 計入，但因為員工申請已改為自動核准，目前不再為 manager 審核流程補測試。

## 後續建議補測

### 暫不補

1. Auth 失效 token 流程

   目前先不寫。這個流程是從受保護頁面進入，localStorage 有 token，但 `/auth/me` 失敗，最後應移除 token 並回到登入頁。

### 可再補

1. 員工申請排隊流程

   `/applications` 回傳 `202` 或 `queued` 時，驗證頁面保留在活動詳情、顯示排隊成功訊息，並仍可切到「我的票券」查看目前資料。

2. HR CSV 匯出流程

   從 `/reports` 匯出總覽或活動詳情 CSV，mock `URL.createObjectURL` 與下載連結，確認 App 層級報表匯出可用。

3. 角色導覽切換流程

   分別以 employee、event manager、HR token 進入 `/events`，確認各角色側邊欄可導到自己的主要功能頁。這能補目前只在 unit test 內驗證的 role navigation。

4. 萬用路由 fallback

   從不存在的路徑進入，例如 `/unknown-route`，驗證登入狀態下會回到預設活動列表，未登入狀態下會回到登入頁。

5. 活動列表篩選到詳情流程

   Manager 在活動列表切換草稿篩選，確認 `/events` 帶上 `status=draft` 後仍可點進活動詳情。這介於 unit 與 integration 之間，重要性略低。

## 不建議補的 Integration Test

- Manager approve/reject 申請審核流程：目前業務規則已改成員工申請自動核准，不應再補。
- 純格式轉換或單一 helper 行為：例如日期格式、單一 badge 顯示，保留在 unit test 比較合適。
- 真實後端串接：這一層應交給 Playwright end-to-end test 或後端 API 測試處理。
