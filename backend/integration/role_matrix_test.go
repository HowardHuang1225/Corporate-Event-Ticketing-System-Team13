package integration

import (
	"net/http"
	"testing"

	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

func TestIntegrationRoleMatrix(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試真實 /v1 router 的 JWT 與角色權限矩陣",
			Target:      RoleMatrixIntegration,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func RoleMatrixIntegration(t *testing.T, errs *utils.Errors) {
	t.Helper()

	logIntegrationStep(t, "準備真實 router 並分別登入 employee、event_manager、hr")
	ctx, err := setupIntegrationTest(t)
	if err != nil {
		errs.Add("準備角色權限整合測試環境", "%v", err)
		return
	}

	employeeToken, err := loginIntegrationUser(ctx.Router, ctx.Users.Employee.EmployeeID)
	if err != nil {
		errs.Add("employee 登入", "%v", err)
		return
	}
	managerToken, err := loginIntegrationUser(ctx.Router, ctx.Users.Manager.EmployeeID)
	if err != nil {
		errs.Add("event_manager 登入", "%v", err)
		return
	}
	hrToken, err := loginIntegrationUser(ctx.Router, ctx.Users.HR.EmployeeID)
	if err != nil {
		errs.Add("hr 登入", "%v", err)
		return
	}

	event, _, err := seedIntegrationEventWithTicketTypes(ctx.DB, ctx.Users.Manager, "published", 2, 10)
	if err != nil {
		errs.Add("建立角色矩陣活動資料", "%v", err)
		return
	}

	tests := []struct {
		name       string
		method     string
		path       string
		token      string
		body       any
		wantStatus int
		wantCode   string
	}{
		{
			name:       "未登入不可讀取目前使用者",
			method:     http.MethodGet,
			path:       "/v1/auth/me",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "employee 不可建立活動",
			method:     http.MethodPost,
			path:       "/v1/events",
			token:      employeeToken,
			body:       gin.H{},
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "event_manager 不可送出員工申請",
			method:     http.MethodPost,
			path:       "/v1/applications",
			token:      managerToken,
			body:       gin.H{},
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "employee 不可讀取 HR overview",
			method:     http.MethodGet,
			path:       "/v1/reports/overview",
			token:      employeeToken,
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "event_manager 不可讀取 HR overview",
			method:     http.MethodGet,
			path:       "/v1/reports/overview",
			token:      managerToken,
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "hr 可以讀取 overview",
			method:     http.MethodGet,
			path:       "/v1/reports/overview",
			token:      hrToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "employee 可以讀取活動列表",
			method:     http.MethodGet,
			path:       "/v1/events",
			token:      employeeToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "employee 可以查詢自己的申請列表",
			method:     http.MethodGet,
			path:       "/v1/applications/my",
			token:      employeeToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "event_manager 不可查詢員工申請列表",
			method:     http.MethodGet,
			path:       "/v1/applications/my",
			token:      managerToken,
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "employee 可以查詢自己的票券列表",
			method:     http.MethodGet,
			path:       "/v1/tickets/my",
			token:      employeeToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "hr 不可查詢員工票券列表",
			method:     http.MethodGet,
			path:       "/v1/tickets/my",
			token:      hrToken,
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "employee 可以查詢活動報名資格",
			method:     http.MethodGet,
			path:       "/v1/events/" + event.ID.String() + "/eligibility",
			token:      employeeToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "event_manager 不可查詢員工報名資格",
			method:     http.MethodGet,
			path:       "/v1/events/" + event.ID.String() + "/eligibility",
			token:      managerToken,
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "employee 查詢不存在 queue status 會進入 handler 並回傳 NOT_FOUND",
			method:     http.MethodGet,
			path:       "/v1/applications/queue/missing-" + utils.UniqueTestSuffix(),
			token:      employeeToken,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "event_manager 不可查詢 employee queue status",
			method:     http.MethodGet,
			path:       "/v1/applications/queue/any-key",
			token:      managerToken,
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "event_manager 缺少核銷 payload 時會進入 handler validation",
			method:     http.MethodPost,
			path:       "/v1/checkin",
			token:      managerToken,
			body:       gin.H{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "employee 不可核銷票券",
			method:     http.MethodPost,
			path:       "/v1/checkin",
			token:      employeeToken,
			body:       gin.H{},
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "event_manager 可以查詢核銷紀錄",
			method:     http.MethodGet,
			path:       "/v1/checkins",
			token:      managerToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "hr 不可查詢核銷紀錄",
			method:     http.MethodGet,
			path:       "/v1/checkins",
			token:      hrToken,
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "event_manager 缺少上傳檔案時會進入 handler validation",
			method:     http.MethodPost,
			path:       "/v1/upload",
			token:      managerToken,
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "employee 不可上傳活動檔案",
			method:     http.MethodPost,
			path:       "/v1/upload",
			token:      employeeToken,
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "event_manager 可以查詢單一活動統計",
			method:     http.MethodGet,
			path:       "/v1/reports/events/" + event.ID.String() + "/stats",
			token:      managerToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "hr 可以查詢單一活動統計",
			method:     http.MethodGet,
			path:       "/v1/reports/events/" + event.ID.String() + "/stats",
			token:      hrToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "employee 不可查詢活動統計",
			method:     http.MethodGet,
			path:       "/v1/reports/events/" + event.ID.String() + "/stats",
			token:      employeeToken,
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "hr 可以匯出單一活動 CSV",
			method:     http.MethodGet,
			path:       "/v1/reports/events/" + event.ID.String() + "/export",
			token:      hrToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "event_manager 不可匯出 HR CSV",
			method:     http.MethodGet,
			path:       "/v1/reports/events/" + event.ID.String() + "/export",
			token:      managerToken,
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logIntegrationStep(t, tt.name)

			var respStatus int
			var respBody []byte
			if tt.body != nil {
				resp := performIntegrationJSON(ctx.Router, tt.method, tt.path, tt.token, tt.body)
				respStatus = resp.Code
				respBody = resp.Body.Bytes()
			} else {
				resp := performIntegrationRequest(ctx.Router, tt.method, tt.path, tt.token)
				respStatus = resp.Code
				respBody = resp.Body.Bytes()
			}

			if respStatus != tt.wantStatus {
				errs.Add(tt.name, "expected status %d, got %d with body %s", tt.wantStatus, respStatus, string(respBody))
				return
			}
			if tt.wantCode != "" {
				if err := utils.AssertHandlerErrorCode(respBody, tt.wantCode); err != nil {
					errs.Add(tt.name, "%v", err)
					return
				}
			}
		})
	}
}
