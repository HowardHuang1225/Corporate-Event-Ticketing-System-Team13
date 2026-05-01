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
│   ├── application.go  # 報名申請流程
│   ├── ticket.go       # 票券產生、核銷 (Check-in)
│   └── report.go       # HR 統計報表邏輯
├── middleware/         # 中間件
│   ├── auth.go         # JWT 權限驗證與角色檢查 (Role-based Access Control)
│   └── cors.go         # 跨來源資源共享設定
├── model/              # 資料模型 (GORM Models)
│   ├── user.go         # 員工、管理員、HR 帳號
│   ├── event.go        # 活動資訊
│   ├── application.go  # 報名狀態
│   └── ticket.go       # 票券與核銷記錄
├── pkg/                # 通用工具
│   └── jwt.go          # JWT 生成與解析工具
└── main.go             # 程式進入點 (路由配置、啟動、Graceful Shutdown)
```

## 3. 核心功能說明

### 身份驗證 (Auth)
- 檔案：`handler/auth.go`, `middleware/auth.go`
- 說明：處理員工編號登入，驗證通過後發放 JWT。Middleware 會解析 Token 並將使用者資訊存入 Context。

### 活動管理 (Event)
- 檔案：`handler/event.go`
- 說明：管理員可建立活動（支援 `datetime-local` 格式轉換為 ISO），一般員工可查詢活動。

### 報名與核銷 (Registration & Check-in)
- 檔案：`handler/application.go`, `handler/ticket.go`
- 說明：
  - 員工申請活動 -> 管理員審核 -> 自動生成票券。
  - 現場核銷時，透過 `Checkin` 函數驗證 Token，並回傳持票人的詳細資訊（姓名、廠區、部門）。

### 數據統計 (Report)
- 檔案：`handler/report.go`
- 說明：HR 可調閱活動報表，計算各部門、各廠區的參與人數與比例。

## 4. 部署特性
- **Graceful Shutdown**: 監聽 `SIGTERM` 訊號，確保關閉時不會中斷進行中的 Request。
- **Non-root**: Docker 內部以 `appuser` 身份執行，提升安全性。
- **Environment Driven**: 所有的敏感資訊與埠號皆透過環境變數控制。
