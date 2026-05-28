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
GO 的所有測試檔案都是XXX_test.go，並且在執行go test的時候會跑路徑底下XXX_test.go中所有Test開頭的function <BR>
[詳細文件](https://hackmd.io/@gmyhp/B1BzocPR-g)
```bash
docker-compose up --build

cd backend
go test ${path}  // 跑某個資料夾底下的所有測試
go test -run Test...  // 跑某個function name為Test...的測試
```

## Load Test

Load Test 包含以下步驟：
- generate 1個虛假 event 和2000個虛假 users
- 把虛假資料注入資料庫
- 跑k6測試（可在 book-ticket.js scenarios 中設定用戶數量和每人購買張數）
- 刪除虛假測試資料

```bash
chmod +x run_load_test.sh
./load-test/run_load_test.sh
```
## High-Concurrency Booking Mode: Waiting Room + Redis Stream

For flash-sale style traffic, the backend supports an optional queue-based ticket application mode. Enable it with:

```env
TICKET_QUEUE_ENABLED=true
```

In this mode, `POST /v1/applications` quickly reserves inventory in Redis, appends the request to a Redis Stream, and returns `202 Accepted` with status `queued`. Background workers then create the final application and tickets in PostgreSQL. This reduces burst pressure on the database and is useful for cloud-native high-concurrency experiments.

See `docs/queue_waiting_room.md` for the full design and testing steps.

---

## CI/CD 與 Kubernetes 監控設計說明

本專案採用「GitHub Actions + GHCR + AKS + Prometheus/Grafana」的 Cloud-Native 交付模式。  
核心目標是：**只重建有變更的服務、維持可回滾的映像版本、部署後立即可觀測**。

### 1) CI/CD 設計邏輯（`.github/workflows/deployment.yml`）

- **觸發條件**
  - `push` 到 `main` 或 `feat/*` 分支。
  - 且變更路徑包含 `frontend/**`、`backend/**`、`api/**`、`k8s/**`、workflow 檔等。
- **變更偵測（paths-filter）**
  - `check-changes` job 先判斷是 frontend、backend 或兩者皆有變更。
  - 只對有變更的服務建置與推送，降低 CD 成本與時間。
- **映像建置與版本策略**
  - 使用 `docker/build-push-action` + Buildx。
  - 推送到 GHCR，tag 使用 `github.sha`，確保每次部署可追溯到特定 commit。
- **部署到 AKS**
  - 透過 `KUBE_CONFIG_DATA` 設定 kube context。
  - 建立/更新 `ghcr-auth` image pull secret，讓叢集可拉取私有 GHCR 映像。
  - `kubectl apply -f k8s/ -R` 先套用所有基礎資源。
  - 針對有變更的服務執行 `kubectl set image` 與 `rollout status`（滾動更新 + 健康檢查等待）。

### 2) CD 流程（從 commit 到上線）

1. 開發者 push 程式碼到 `main` 或 `feat/*`。  
2. Actions 判斷變更範圍（frontend/backend）。  
3. 建置並推送對應 Docker image 到 GHCR（tag = commit SHA）。  
4. 連線 AKS、更新 `ghcr-auth`、apply `k8s/` manifests。  
5. 對有變更的 deployment 設定新 image 並等待 rollout 完成。  
6. 新版本 Pod 就緒後對外服務；失敗時可透過 deployment revision 進行回滾。  

### 3) `k8s/` 目錄部署設計（職責分層）

- **應用層**
  - `frontend.yaml`: 前端 Deployment + Service（目前為 NodePort，便於開發/展示）。
  - `backend.yaml`: 後端 Deployment + Service，透過 ConfigMap/Secret 注入環境變數。
- **配置與敏感資料**
  - `configmap.yaml`: 非敏感設定（DB host、Redis URL、Queue 參數、MinIO endpoint 等）。
  - `secret.yaml`: 敏感資訊（DB password、JWT secret、MinIO root credentials）。
- **資料服務層**
  - `postgres.yaml`: PostgreSQL + PVC（10Gi）。
  - `redis.yaml`: Redis + AOF + PVC（5Gi）。
  - `minio.yaml`: MinIO + PVC（10Gi）+ bucket 初始化 Job。
- **流量入口與憑證**
  - `ingress.yaml`: 主站域名入口與 TLS。
  - `minio-ingress.yaml`: `/minio-api` 路徑轉發到 MinIO API。
  - `clusterissuer.yaml`: cert-manager + Let's Encrypt ACME 簽發憑證。
  - `nginx-config.yaml`: 前端 Nginx 反向代理 `/v1/` 到 backend service。
- **監控層**
  - `monitoring/namespace.yaml`: 獨立 `monitoring` namespace。
  - `monitoring/prometheus-cm.yaml` + `prometheus-setup.yaml`: Prometheus 設定與部署。
  - `monitoring/kube-state-metrics.yaml`: 叢集物件狀態指標來源。
  - `monitoring/grafana-setup.yaml`: Grafana + PVC + Service。
  - `monitoring/dashboard.json`: 匯入用 dashboard（K8s 資源觀測面板）。

### 4) 監控設計邏輯（Prometheus + Grafana）

#### A. 指標暴露來源

- backend 在 `main.go` 註冊 `/metrics`（Prometheus handler）。
- middleware 持續記錄：
  - `http_requests_total`（method/path/status）
  - `http_request_duration_seconds`（method/path）
- 票務業務指標（`backend/metrics/ticket_metrics.go`）：
  - `event_ticket_remaining`
  - `ticket_apply_total`
  - `ticket_redeem_total`
- `backend.yaml` 的 pod annotation 啟用 scrape：
  - `prometheus.io/scrape: "true"`
  - `prometheus.io/path: "/metrics"`
  - `prometheus.io/port: "8001"`

#### B. Prometheus 抓取策略

- `scrape_interval: 15s`，每 15 秒抓取一次。
- 除了監控 Prometheus 自身，也透過 `kubernetes_sd_configs` 自動發現 Pod。
- relabel 會將 namespace/pod 名稱等 metadata 帶入 labels，便於 Grafana 查詢與分群。

#### C. Grafana 呈現策略

- Grafana 使用 PVC 保留 dashboard 與設定。
- 可匯入 `k8s/monitoring/dashboard.json` 做叢集級監控（node/pod/resource/network）。
- 建議搭配自訂 panel 針對本專案業務指標（票券申請、核銷、剩餘票量）做告警門檻。

### 5) 建議套用順序（首次部署）

```bash
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/secret.yaml
kubectl apply -f k8s/postgres.yaml
kubectl apply -f k8s/redis.yaml
kubectl apply -f k8s/minio.yaml
kubectl apply -f k8s/nginx-config.yaml
kubectl apply -f k8s/backend.yaml
kubectl apply -f k8s/frontend.yaml
kubectl apply -f k8s/clusterissuer.yaml
kubectl apply -f k8s/ingress.yaml
kubectl apply -f k8s/minio-ingress.yaml

kubectl apply -f k8s/monitoring/namespace.yaml
kubectl apply -f k8s/monitoring/prometheus-cm.yaml
kubectl apply -f k8s/monitoring/prometheus-setup.yaml
kubectl apply -f k8s/monitoring/kube-state-metrics.yaml
kubectl apply -f k8s/monitoring/grafana-setup.yaml
```

### 6) 目前架構重點（你報告可以直接講）

- **效率**：paths-filter 避免每次都全量 build/deploy。  
- **可追溯**：image tag 綁 commit SHA，問題版本可快速定位。  
- **可維運**：ConfigMap/Secret 分離，參數與密碼解耦。  
- **可觀測**：系統指標（HTTP）+ 業務指標（票務）+ 叢集指標（kube-state-metrics）三層監控。  
- **可擴展**：backend 與 frontend 皆為 Deployment，可直接水平擴展 replicas。  
- **高併發友善**：Queue 模式（Redis Stream）與監控結合，可觀察尖峰流量下延遲與申請量。  
