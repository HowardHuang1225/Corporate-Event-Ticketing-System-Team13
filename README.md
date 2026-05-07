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

```bash
docker-compose up --build

cd backedn
go test ./handler // 跑handler底下所有測試
go test ./handler -run TestEvent // 跑event_test.go
go test ./handler -run TestAuth // 跑auth_test.go
go test ./handler -run TestTicket // 跑ticket_test.go
```
