# Ticketing System (員工票務系統)

本專案是一個基於 Cloud Native 原則設計的企業活動票務系統。支援員工報名、管理員審核、現場 QR Code 核銷以及 HR 統計報表功能。

## Quick Start

這是最推薦的啟動方式，會自動建立並運行所有服務（前端、後端、資料庫、快取、監控）。

```bash
# 1. 準備環境變數
cp .env.example .env

# 2. 啟動服務
docker-compose up --build
```

啟動完成後：

- **前端網頁**: [http://localhost:8080](http://localhost:8080)
- **後端 API**: [http://localhost:8001](http://localhost:8001)
- **Grafana 監控**: [http://localhost:3001](http://localhost:3001) (預設帳密: `admin` / `admin`)

---

## 環境設定 (Configuration)

本專案採用 12-Factor App 原則管理配置：

- **.env**: 儲存開發環境的變數（如 `JWT_SECRET`、資料庫帳密）。
- **Docker Compose**: 自動讀取根目錄的 `.env` 並注入容器。
- **Go Backend**: 啟動時會透過 `godotenv` 載入 `.env`，支援本地直接執行與 Docker 環境。

**設定步驟：**
1. 複製範本：`cp .env.example .env`
2. 根據需求修改 `.env` 內容（例如修改 `JWT_SECRET`）。

---

## 專案架構與技術文件

詳細的設計細節與檔案分配請參考：

- [Backend 技術說明 (Go/Gin/GORM)](docs/backend_spec.md)
- [Frontend 技術說明 (React/TS/Vite)](docs/frontend_spec.md)

### 目錄職責

```text
.
├── api/                  # OpenAPI 規格與前後端 API contract
├── backend/              # Go 後端服務，負責 API、資料存取、商業邏輯與背景初始化
├── docs/                 # 系統設計、前後端規格與開發文件
├── frontend/             # React + TypeScript 前端應用
├── docker-compose.yml    # 本地整合啟動 Postgres、Redis、後端與前端
├── .env.example          # 本地環境變數範本
└── *_test.go             # 專案層級測試檔與共用測試輔助
```

### Backend 架構

```text
backend/
├── bootstrap/            # Demo seed data 與啟動時初始化流程
├── config/               # 環境變數與服務設定讀取
├── database/             # GORM 連線、AutoMigrate 與資料庫 schema 補遷移
├── handler/              # Gin HTTP handler，負責 request binding、身份 context 與 response
│   ├── auth/             # 登入與身份驗證 API
│   ├── employee/         # 員工活動瀏覽、報名、票券 API
│   ├── hr/               # HR 報表 API
│   ├── manager/          # 管理員活動、申請審核、核銷 API
│   └── shared/           # handler 共用 response/context 工具
├── middleware/           # JWT 驗證、角色權限等 HTTP middleware
├── model/                # GORM model 與 JSON 資料結構
├── pkg/                  # JWT、Redis、通用工具等基礎元件
├── repository/           # 資料存取層，封裝 DB query 與 persistence
├── routes/               # API route 註冊與角色權限掛載
├── service/              # 核心商業邏輯，例如活動時間規則、報名 eligibility、票券流程
└── main.go               # 後端服務入口，初始化設定、DB、Redis、router 與 graceful shutdown
```

### Frontend 架構

```text
frontend/
├── public/               # 靜態公開資源，例如 favicon、sprite icon
├── src/
│   ├── api/              # Axios client 與 API baseURL/JWT interceptor
│   ├── assets/           # 前端使用的圖片與靜態素材
│   ├── components/       # 共用 UI 元件，例如 Layout、ProtectedRoute
│   ├── contexts/         # React context，例如登入狀態與使用者資訊
│   ├── pages/            # 依角色與功能切分的頁面
│   │   ├── employee/     # 員工活動列表、活動詳情、個人票券
│   │   ├── hr/           # HR 報表頁
│   │   └── manager/      # 管理員活動管理、申請審核、現場核銷
│   ├── App.tsx           # 前端路由與主要應用組裝
│   ├── App.css           # 應用層樣式
│   ├── index.css         # 全域樣式與 CSS variables
│   └── main.tsx          # React entry point
├── nginx.conf            # Docker production image 使用的 Nginx 設定
├── vite.config.ts        # Vite dev server、React plugin 與 API proxy
└── package.json          # 前端 scripts 與 npm dependency
```

---

## 開發環境手動啟動

如果你需要進行程式碼開發，可以分別啟動前後端：

### 1. 啟動基礎設施 (DB & Redis)

```bash
docker-compose up -d postgres redis
```

### 2. 啟動後端 (Backend)

```bash
cd backend
go mod tidy
go run main.go
```

後端服務將開啟在 `http://localhost:8001`

### 3. 啟動前端 (Frontend)

```bash
cd frontend
npm install
npm run dev
```

前端開發伺服器將開啟在 `http://localhost:5173` (具備 Hot Reload 與 API Proxy)

---

## Demo 帳號

| 角色   | 員工編號 | 密碼     | 說明                                  |
| :----- | :------- | :------- | :------------------------------------ |
| 管理員 | MGR001   | password | 建立活動、審核申請、現場核銷          |
| 員工   | EMP001   | password | 瀏覽活動、報名、查看個人票券 (台南廠) |
| 員工   | EMP002   | password | 瀏覽活動、報名、查看個人票券 (新竹廠) |
| HR     | HR001    | password | 查閱各廠區/部門統計報表               |

---

## Git 協作規範

- Commit message 必須加上 prefix:
  - `feat:` (新功能)
  - `fix:` (修復 Bug)
  - `docs:` (文件更新)
  - `style:` (程式碼格式、UI 調整)
  - `chore:` (建置工具、依賴項更新)

---

## Unit Test
GO 的所有測試檔案都是XXX_test.go，並且在執行go test的時候會跑路徑底下XXX_test.go中所有Test開頭的function

```bash
docker-compose up --build

cd backend
go test ${path}  // 跑某個資料夾底下的所有測試
go test -run Test...  // 跑某個function name為Test...的測試
```
