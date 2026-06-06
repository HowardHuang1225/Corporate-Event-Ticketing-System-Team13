# Backend 系統架構說明

本後端服務使用 **Go 語言** 開發，採用 RESTful API 架構，並遵循 **12-Factor App** 原則進行容器化設計。

## 1. 技術棧 (Tech Stack)
- **Framework**: [Gin Web Framework](https://github.com/gin-gonic/gin)
- **ORM**: [GORM](https://gorm.io/)
- **Database**: PostgreSQL (主要存儲), Redis (快取)
- **Authentication**: JWT (JSON Web Token)

## 2. 目錄架構 (Directory Structure)

```text
backend/
├── config/             # 配置管理
│   └── config.go       # 讀取環境變數 (DB URL, Redis, JWT Secret 等)
├── database/           # 資料庫連接
│   └── database.go     # 初始化 PostgreSQL 連線與 GORM 設定
├── handler/            # API 控制器 (邏輯核心)
│   ├── auth.go         # 登入、身份驗證
│   ├── event.go        # 活動管理 (增刪查改)
│   ├── ticket.go       # 票務核心：申請、取消、退票、產生、核銷
│   └── report.go       # HR 統計報表邏輯
├── middleware/         # 中間件
│   ├── auth.go         # JWT 權限驗證與角色檢查 (Role-based Access Control)
│   └── cors.go         # 跨來源資源共享設定
├── model/              # 資料模型 (GORM Models)
│   ├── user.go         # 使用者與帳號
│   ├── event.go        # 活動與票種 (TicketType) 資訊
│   └── ticket.go       # 申請 (Application)、票券 (Ticket) 與核銷記錄
├── pkg/                # 通用工具
│   ├── jwt.go          # JWT 生成工具
│   ├── redis.go        # 分散式鎖實作
│   └── utils.go        # 通用 Helper (如 Ptr)
└── main.go             # 程式進入點 (路由配置、Graceful Shutdown)
```

## 3. 核心功能說明

### 身份驗證 (Auth)
- 檔案：`handler/auth.go`, `middleware/auth.go`
- 說明：處理員工編號登入，發放 JWT。Middleware 會解析 Token 並將使用者資訊存入 Context 進行 RBAC 檢查。

### 活動管理 (Event)
- 檔案：`handler/event.go`, `model/event.go`
- 說明：管理員可建立活動（支援 `MaxTicketsPerPerson` 每人限額）。一般員工可查詢已發佈的活動。

### 票務與防超賣 (Ticketing & Anti-Oversell)
- 檔案：`handler/ticket.go`
- 說明：
  - **防超賣機制**：採用 **Redis 分散式鎖 (pkg.AcquireLock)** 結合 **資料庫樂觀鎖 (Version/CAS)**，確保高併發下庫存準確。
  - **動態限額**：申請時計算「(核准+待審核) - 退票紀錄」之淨票數，確保不超過每人上限。
  - **單張退票與審計 (Audit Trail)**：
    - 支持電子票券「一張一張退」。
    - **邏輯**：不修改原始申請單數量，而是新增一筆狀態為 `cancelled` 且原因為「退票」的 Application 紀錄。
    - 退票後自動回撥 `TicketType.Remaining` 並作廢該實體票券。

### 核銷流程 (Check-in)
- 檔案：`handler/manager/handler.go`, `service/ticket/service.go`, `pkg/totp/totp.go`
- 說明：管理員掃描 QR Code 後，系統會解析 `qr_token`。核銷 API 目前只接受動態 QR 格式 `<qr_token>|<otp>`，其中 `otp` 是以 `qr_token` 與時間窗產生的 6 位數 TOTP。
- 裸 UUID、缺少 OTP 或超過兩段的 token 會被視為 `VALIDATION_ERROR`。收到動態 QR 後，後端會先驗證 OTP；目前核銷驗證會接受目前、上一個與下一個 60 秒時間窗，以容忍現場裝置時間差與剛跨秒的舊 QR。錯誤或過期時回傳 `EXPIRED_QR`。驗證通過後再用 base `qr_token` 查票券，確保票券未核銷且未過期，成功後標記 `is_used = true` 並建立 `Checkin` 紀錄。

### 數據統計 (Report)
- 檔案：`handler/report.go`
- 說明：HR 可調閱活動報表，計算各部門、各廠區的參與人數、佔比，並支援匯出。

## 4. 部署特性
- **Graceful Shutdown**: 監聽 `SIGTERM`，確保關閉時不會中斷請求。
- **Non-root**: Docker 內部以 `appuser` 身份執行。
- **Environment Driven**: 12-Factor 原則，配置全由環境變數控制。
