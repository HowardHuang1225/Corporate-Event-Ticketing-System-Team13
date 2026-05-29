- 前端中我們一樣不許動任何實作相關的code，除非有我明確的指示或允許
- 使用 vitest 以及 playwright 兩種框架去撰寫unit test, integration test以及end-to-end test
- 前端 unit test 使用 Vitest，測試檔案盡量與被測檔案放在同一個目錄，命名為 `*.test.ts` 或 `*.test.tsx`
- 前端 integration test 也使用 Vitest，測試檔案放在 `frontend/src/integration/`，命名為 `*.integration.test.tsx`
- 前端 integration test 應以 `App`、`AuthProvider`、路由、共用 layout 與頁面互動串接為主，API 仍 mock 在 `frontend/src/api/client.ts` 邊界，不直接呼叫真實後端
- 員工申請活動後會自動核准並產出票券，因此前端測試不應再驗證 manager approve/reject 審核流程
- 我的票券 QR Code 目前會顯示 `qr_token|6位數動態碼`，測試時應 mock `frontend/src/utils/totp.ts` 的 `generateTOTP`，用固定動態碼驗證完整格式與倒數提示，不應只斷言裸 `qr_token`
- 活動管理表單已有圖片與 PDF 上傳欄位，測試若用 DOM selector 取文字/日期欄位，應排除 `input[type="file"]`，避免欄位索引被上傳欄位影響
- 前端共用測試工具放在 `frontend/src/test/`，例如 `setup.ts`、`test-utils.tsx`、`mocks/`
- 前端 end-to-end test 使用 Playwright，測試檔案統一放在 `frontend/e2e/`
- 建議的前端測試架構如下：
  ```text
  frontend/
    src/
      test/
        setup.ts
        test-utils.tsx
        mocks/
          api.ts
      integration/
        auth-flow.integration.test.tsx
        employee-ticket-flow.integration.test.tsx
        manager-operations.integration.test.tsx
        hr-report-flow.integration.test.tsx
      api/
        client.test.ts
      contexts/
        AuthProvider.test.tsx
      components/
        ProtectedRoute.test.tsx
        Layout.test.tsx
      pages/
        Login.test.tsx
        employee/
          EventList.test.tsx
          EventDetail.test.tsx
          MyTickets.test.tsx
        manager/
          EventManage.test.tsx
          CheckIn.test.tsx
        hr/
          Reports.test.tsx
    e2e/
      apiMock.ts
      auth.spec.ts
      employee-ticket-flow.spec.ts
      manager-event-flow.spec.ts
      hr-report-flow.spec.ts
      cross-role-flow.spec.ts
  ```
