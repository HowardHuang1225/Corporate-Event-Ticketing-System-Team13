# Corporate Event Ticketing System

本專案包含以下目錄與服務：

- `backend/`: Go 語言撰寫的後端服務 (使用 Gin 框架)
- `frontend/`: React + Vite 撰寫的前端 SPA
- `api/`: OpenAPI Spec
- `k8s/`: Kubernetes 部署文件
- `docker-compose.yml`: 本地開發所需依賴 (Postgres, Redis, Prometheus, Grafana)

## 如何開始開發

### 1. 啟動資料庫與快取

如果你是開發前端或後端，都請先打開 Docker Desktop 並執行：

```bash
cd ticketing-system
docker-compose up -d
```

這會自動在背景啟動 Postgres, Redis, Prometheus 和 Grafana。

### 2. 後端開發者

```bash
cd backend
go mod tidy
go run main.go
```

後端服務將會開啟在 `http://localhost:8001`
測試：`curl http://localhost:8001/health`

### 3. 前端開發者

```bash
cd frontend
npm install
npm run dev
```

這會開啟 Vite 開發伺服器，預設於 `http://localhost:5173`。
請參考 `api/openapi.yaml` 進行 Mock API 開發。

---

## Git 協作規範

- 每個功能開一個新的 branch (`feature/your-feature-name`)
- Commit message 加上 prefix: (`feat:`, `fix:`, `docs:`, `chore:`)
