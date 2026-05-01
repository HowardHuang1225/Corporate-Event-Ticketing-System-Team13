# Frontend 系統架構說明

本前端專案使用 **React + TypeScript** 開發，採用現代化的單頁式應用 (SPA) 設計，並針對移動端核銷與報表呈現進行了優化。

## 1. 技術棧 (Tech Stack)
- **Framework**: [React 19](https://react.dev/)
- **Build Tool**: [Vite](https://vitejs.dev/)
- **State Management**: [React Query (TanStack)](https://tanstack.com/query)
- **Icons**: [Lucide React](https://lucide.dev/)
- **Styling**: Vanilla CSS (CSS Variables)

## 2. 目錄架構 (Directory Structure)

```text
frontend/
├── src/
│   ├── api/            # API 客戶端
│   │   └── client.ts   # Axios 配置 (攔截器、自動注入 Token、相對路徑處理)
│   ├── components/     # 共用組件
│   │   └── Layout.tsx  # 側邊欄導航、角色顯示邏輯、廠區資訊顯示
│   ├── contexts/       # 全域狀態
│   │   └── AuthContext.tsx # 登入狀態管理、自動轉向、權限判斷
│   ├── pages/          # 頁面組件
│   │   ├── Login.tsx   # 登入頁面 (區分四種 Demo 角色)
│   │   ├── Events.tsx  # 員工瀏覽活動列表
│   │   ├── hr/         # HR 專用
│   │   │   └── Reports.tsx # 視覺化統計報表 (部門、廠區分佈、CSV 匯出)
│   │   ├── manager/    # 管理員專用
│   │   │   ├── EventManage.tsx # 活動維護 (含時間格式 ISO 轉換)
│   │   │   ├── Applications.tsx # 審核報名申請
│   │   │   └── CheckIn.tsx      # 現場掃描核銷 (顯示詳細員工廠區資訊)
│   │   └── employee/   # 一般員工專用
│   │       └── MyTickets.tsx    # 我的票券 (QR Code 生成)
│   ├── App.tsx         # 路由定義 (Role-based Routing)
│   └── main.tsx        # 程式入口
├── nginx.conf          # Docker 部署用的 Nginx 配置 (含反向代理)
└── Dockerfile          # 多階段構建 (Multi-stage build)
```

## 3. 核心功能說明

### 角色導航 (Role-based Navigation)
- 檔案：`components/Layout.tsx`
- 說明：根據 `AuthContext` 提供的角色資訊，動態切換側邊欄的功能清單，並在左下角顯示使用者的 **職稱** 與 **所屬廠區**。

### 核銷流程 (Check-in System)
- 檔案：`pages/manager/CheckIn.tsx`
- 說明：管理員輸入/掃描 Token 後，系統會串接後端 API 進行核銷，並同步顯示該名員工的「姓名、工號、部門、廠區」以供核對。

### 時間處理 (Time Management)
- 檔案：`pages/manager/EventManage.tsx`
- 說明：由於 HTML `datetime-local` 格式與後端 ISO 格式不符，前端在傳送前會自動透過 `toISOString()` 進行標準化處理。

### 數據報表 (HR Insights)
- 檔案：`pages/hr/Reports.tsx`
- 說明：整合了各部門與各廠區的報名數據，並提供 **CSV 匯出** 功能供 HR 下載存檔。

## 4. 部署特性
- **Dynamic Port**: Nginx 配置支援透過 `${PORT}` 環境變數動態綁定埠號。
- **CORS-free**: 在 Docker 部署中，所有請求皆透過 Nginx 內部的 `/v1` 轉發，避免跨域連線問題。
- **SPA Routing**: Nginx 已配置 `try_files` 以確保 React Router 的路由刷新後不會失效。
