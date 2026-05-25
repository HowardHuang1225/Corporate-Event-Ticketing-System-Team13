# 前端 Unit Test 紀錄

本文記錄目前前端 Vitest unit test 涵蓋的範圍、測試指令與覆蓋率狀態。Integration test 詳細紀錄請見 `docs/frontend_integration_tests.md`。

## 測試指令

在 `frontend/` 目錄下執行：

```bash
npm test
npm run test:coverage
npm run lint
npm run build
```

## 測試架構

目前前端測試主要分成兩類：

- Unit test：放在被測檔案旁邊，命名為 `*.test.ts` 或 `*.test.tsx`
- Integration test：放在 `frontend/src/integration/`，命名為 `*.integration.test.tsx`

共用測試工具放在：

- `frontend/src/test/setup.ts`
- `frontend/src/test/test-utils.tsx`

## 目前覆蓋率

最近一次執行 `npm run test:coverage` 的結果：

| 指標 | 覆蓋率 |
| --- | ---: |
| Statements | 88.13% |
| Branches | 73.08% |
| Functions | 83.15% |
| Lines | 88.21% |

目前員工申請不需要 manager 核准，因此已刪除 `Applications.test.tsx` 中的 approve/reject 測試。`Applications.tsx` 仍存在於前端實作並會被 coverage 計入，但測試不再驗證 manager 審核流程。

## Unit Tests

### API Client

檔案：`frontend/src/api/client.test.ts`

測試內容：

- 預設 API baseURL 會使用 `/v1`
- localStorage 有 token 時會加上 `Authorization: Bearer ...`
- 沒有 token 時不會加入授權 header
- 成功 response 會原樣回傳
- 401 response 會清除 token 並導向 `/login`
- 非 401 response error 不會清 token，也不會導向登入頁

### Auth Context / Provider

檔案：

- `frontend/src/contexts/AuthContext.test.tsx`
- `frontend/src/contexts/AuthProvider.test.tsx`

測試內容：

- `useAuth` 不在 `AuthProvider` 內使用時會拋錯
- 沒有 token 時不會呼叫 `/auth/me`
- 有 token 時會呼叫 `/auth/me` 並還原使用者
- `/auth/me` 失敗時會移除失效 token
- login 成功會儲存 access token 並更新 user
- logout 會清除 token、user 與 React Query cache

### Route / Layout

檔案：

- `frontend/src/components/ProtectedRoute.test.tsx`
- `frontend/src/components/Layout.test.tsx`

測試內容：

- 驗證 loading 狀態會顯示載入畫面
- 未登入使用者會被導向 `/login`
- 已登入使用者可看到受保護內容
- employee / event_manager / hr 會看到各自角色的導覽項目
- 登出會呼叫 logout 並導向登入頁

### Login

檔案：`frontend/src/pages/Login.test.tsx`

測試內容：

- 輸入員工編號與密碼後會呼叫 login
- login 成功會導向 `/events`
- login 失敗會顯示後端錯誤訊息
- login 失敗且沒有後端 message 時會顯示預設錯誤訊息

### Employee Pages

檔案：

- `frontend/src/pages/employee/EventList.test.tsx`
- `frontend/src/pages/employee/EventDetail.test.tsx`
- `frontend/src/pages/employee/MyTickets.test.tsx`

測試內容：

- 活動列表會載入活動資料並可點進詳情頁
- event_manager 可以看到草稿篩選，employee 不會看到草稿篩選
- 活動列表會依篩選狀態送出 API query params
- 活動列表空資料會顯示空狀態
- 未知活動狀態會顯示原始狀態文字
- 活動詳情會依可申請條件顯示申請按鈕
- 地域不一致時會顯示地域提醒，同地域時不顯示
- 未發布、已截止、售罄活動不提供申請
- 申請票券會送出 `event_id`、`ticket_type_id`、`quantity`、`idempotency_key`
- 申請成功包含一般成功與排隊成功兩種訊息
- 申請失敗會顯示後端錯誤
- 我的票券會顯示申請記錄與電子票券
- QR 可以展開/收合
- 退票取消時不呼叫 API
- 退票成功會刷新票券與申請資料
- 退票失敗會顯示錯誤
- 已核銷票券不顯示 QR 與退票操作
- 沒有申請記錄時會顯示空狀態

### Manager Pages

檔案：

- `frontend/src/pages/manager/CheckIn.test.tsx`
- `frontend/src/pages/manager/EventManage.test.tsx`
- `frontend/src/pages/manager/EventManage.publish-time.test.mjs`

測試內容：

- 員工申請目前不需要 manager 核准，前端測試不再驗證 approve/reject 審核流程
- 現場核銷會 trim QR token 後送出 API
- 空白 token 不會送出核銷 API
- 核銷成功會顯示票券與使用者資訊
- 核銷錯誤涵蓋已核銷、找不到票券、未知錯誤
- 核銷紀錄活動篩選會送出 `event_id`
- 活動管理會載入活動列表並顯示不同狀態 badge
- 建立活動會把日期轉成 ISO，空日期送空字串，空地域送 `null`
- 編輯活動會預填資料並送出 update API
- 表單票種可以新增與移除
- 發布前會檢查發布時間、申請截止時間、開始時間、結束時間
- 發布、截止、刪除會呼叫對應 API
- 刪除取消時不呼叫 API
- 建立失敗會顯示預設錯誤
- `EventManage.publish-time.test.mjs` 檢查 publish_time 表單契約仍存在

### HR Reports

檔案：`frontend/src/pages/hr/Reports.test.tsx`

測試內容：

- 報表會載入活動列表與總覽資料
- 未選活動時不查詢詳細統計
- 選擇活動後會查詢 `/reports/events/:id/stats`
- 詳細統計會顯示申請階段、核准與退票階段、核銷與出席率
- 部門、廠區、票種分佈會正確顯示
- 分佈資料為空時會顯示尚無資料
- 總覽 CSV 與詳細 CSV 會建立下載連結

## Integration Tests

Integration test 已拆到獨立文件記錄：`docs/frontend_integration_tests.md`。

## 後續可補方向

- 針對 branch coverage 仍偏低的 fallback UI 再補更細案例，例如缺少巢狀資料時的 `—` 顯示
- 補 Playwright end-to-end test，涵蓋瀏覽器層級的登入、申請票券、核銷與報表流程
- 若後續產品邏輯固定，可再把目前部分 source contract test 改成真正元件互動測試
