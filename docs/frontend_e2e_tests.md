# 前端 End-to-End Test 紀錄

本文記錄目前 Playwright end-to-end test 的測試邊界、已覆蓋流程，以及後續建議補測項目。

## 測試定位

End-to-end test 放在 `frontend/e2e/`，命名為 `*.spec.ts`，並由 `frontend/playwright.config.ts` 指定測試目錄與 dev server。

目前 E2E 仍以 API mock 方式隔離真實後端，mock helper 集中在 `frontend/e2e/apiMock.ts`。這一層專注驗證瀏覽器層級的路由、登入狀態、表單操作、下載、跨角色互動與主要使用者流程。

員工申請活動目前會自動核准並產出票券，因此 E2E 不驗證 manager approve/reject 審核流程。

## 測試指令

在 `frontend/` 目錄下執行：

```bash
npm run test:e2e
```

## 目前 End-to-End Tests

### Auth Flow

檔案：`frontend/e2e/auth.spec.ts`

測試內容：

- 未登入訪問受保護頁面會導向 `/login`
- 登入失敗時會顯示後端錯誤訊息
- 不同角色登入後會看到對應導覽項目

### Employee Ticket Flow

檔案：`frontend/e2e/employee-ticket-flow.spec.ts`

測試內容：

- 員工登入後可從活動列表進入活動詳情
- 送出申請時會呼叫 `/applications`，payload 包含活動、票種、數量與 idempotency key
- 申請成功後前端顯示自動核准與自動發票訊息
- 「我的票券」會顯示已核准申請與未使用票券
- QR 展開後會顯示 `qr_token|otp` 格式，測試以固定 mock TOTP 驗證完整動態 QR 與倒數提示

### Manager Event Flow

檔案：`frontend/e2e/manager-event-flow.spec.ts`

測試內容：

- Manager 可登入並進入活動管理
- 建立活動草稿時，表單測試排除圖片/PDF 的 `input[type="file"]`
- 建立活動 payload 會帶入標題、描述、地點、地域、每人限額與票種
- Manager 可發布草稿活動與截止已發布活動
- Manager 可輸入 `qr_token|otp` 動態 QR 完成核銷
- 已核銷與找不到票券會顯示對應錯誤訊息

### HR Report Flow

檔案：`frontend/e2e/hr-report-flow.spec.ts`

測試內容：

- HR 可登入並進入統計報表
- 總覽表會顯示活動申請、票券與核銷率資料
- 可匯出活動總覽 CSV
- 選擇活動後會顯示申請階段、核准與退票階段、核銷與出席率
- 可匯出活動詳情 CSV

### Cross-Role Flow

檔案：`frontend/e2e/cross-role-flow.spec.ts`

測試內容：

- Manager 發布草稿活動後，employee 重新整理即可看到活動、申請並取得自動核准票券
- Employee 報名與 manager 核銷後，HR 重新整理報表會看到統計同步更新
- Manager 截止活動後，employee 重新整理詳情頁不再看到申請入口
- Employee 退票後，HR 重新整理報表會看到有效票下降與退票數增加

## 共用測試工具

檔案：`frontend/e2e/apiMock.ts`

內容：

- 角色 fixture：employee、manager、HR
- 活動、申請、票券 fixture
- `mockApi`：攔截 `/v1/**` API
- `mockTotp`：攔截 `frontend/src/utils/totp.ts` 並固定 `generateTOTP`
- `fulfillData` / `fulfillError`：回傳一致的 API response 格式
- `loginAs`：共用登入操作

## 後續可補方向

- 失效 token 重新導向登入流程
- `/applications` 回傳 `queued` 的排隊申請流程
- CSV 匯出失敗時的錯誤提示
- 核銷動態碼過期時顯示後端 `EXPIRED_QR` 錯誤
