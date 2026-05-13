package shared

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ticketing-system/backend/service/apperror"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestSharedHandlerHelpers(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試 UserID 會從 gin context 讀出 UUID 或回傳 UNAUTHORIZED",
			Target:      UserIDReadsUUIDOrRejectsInvalidContext,
		},
		{
			Description: "測試 Role 會從 gin context 讀出角色",
			Target:      RoleReadsContextValue,
		},
		{
			Description: "測試 WriteError 會輸出統一錯誤格式",
			Target:      WriteErrorSerializesAppAndUnknownErrors,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func UserIDReadsUUIDOrRejectsInvalidContext(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("測試 UserID 會從 gin context 讀出 UUID 或回傳 UNAUTHORIZED\n")
	utils.PrintTestProgress("==================================================\n")

	validUserID := uuid.New()
	tests := []struct {
		name     string
		value    string
		want     uuid.UUID
		wantCode string
		progress string
	}{
		{
			name:     "合法 user_id",
			value:    validUserID.String(),
			want:     validUserID,
			progress: "gin context 中的 user_id 是合法 UUID 時，UserID 應回傳該 UUID。",
		},
		{
			name:     "缺少 user_id",
			wantCode: "UNAUTHORIZED",
			progress: "gin context 缺少 user_id 時，UserID 應回傳 UNAUTHORIZED。",
		},
		{
			name:     "user_id 格式錯誤",
			value:    "not-a-uuid",
			wantCode: "UNAUTHORIZED",
			progress: "gin context 中的 user_id 不是 UUID 時，UserID 應回傳 UNAUTHORIZED。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			c := newSharedTestContext()
			if tt.value != "" {
				c.Set("user_id", tt.value)
			}

			got, err := UserID(c)
			if tt.wantCode != "" {
				if assertErr := assertSharedAppErrorCode(err, tt.wantCode); assertErr != nil {
					errs.Add(tt.progress, "%v", assertErr)
				}
				return
			}
			if err != nil {
				errs.Add(tt.progress, "預期 UserID 成功，實際錯誤：%v", err)
				return
			}
			if got != tt.want {
				errs.Add(tt.progress, "預期 UUID 為 %s，實際為 %s", tt.want, got)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func RoleReadsContextValue(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("測試 Role 會從 gin context 讀出角色\n")
	utils.PrintTestProgress("==================================================\n")

	tests := []struct {
		name     string
		role     string
		progress string
	}{
		{
			name:     "讀取角色",
			role:     "event_manager",
			progress: "gin context 中有 role 時，Role 應回傳該角色字串。",
		},
		{
			name:     "缺少角色",
			role:     "",
			progress: "gin context 缺少 role 時，Role 應回傳空字串。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			c := newSharedTestContext()
			if tt.role != "" {
				c.Set("role", tt.role)
			}
			if got := Role(c); got != tt.role {
				errs.Add(tt.progress, "預期 role 為 %q，實際為 %q", tt.role, got)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func WriteErrorSerializesAppAndUnknownErrors(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("測試 WriteError 會輸出統一錯誤格式\n")
	utils.PrintTestProgress("==================================================\n")

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
		progress   string
	}{
		{
			name:       "應用程式錯誤",
			err:        apperror.Conflict("ALREADY_EXISTS", "資料已存在"),
			wantStatus: http.StatusConflict,
			wantCode:   "ALREADY_EXISTS",
			wantMsg:    "資料已存在",
			progress:   "WriteError 收到 apperror.Error 時，應使用該錯誤的 status、code、message。",
		},
		{
			name:       "未知錯誤",
			err:        errors.New("database exploded"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
			wantMsg:    "Internal server error",
			progress:   "WriteError 收到非 apperror.Error 時，應轉成 INTERNAL_ERROR。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			c, resp := newSharedResponseContext()
			WriteError(c, tt.err)

			if resp.Code != tt.wantStatus {
				errs.Add(tt.progress, "預期狀態碼 %d，實際為 %d，回應內容：%s", tt.wantStatus, resp.Code, resp.Body.String())
				return
			}
			body, err := decodeSharedErrorResponse(resp.Body.Bytes())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if body.Success {
				errs.Add(tt.progress, "預期 success=false")
				return
			}
			if body.Error.Code != tt.wantCode {
				errs.Add(tt.progress, "預期錯誤代碼 %q，實際為 %q", tt.wantCode, body.Error.Code)
				return
			}
			if body.Error.Message != tt.wantMsg {
				errs.Add(tt.progress, "預期錯誤訊息 %q，實際為 %q", tt.wantMsg, body.Error.Message)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func newSharedTestContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	return c
}

func newSharedResponseContext() (*gin.Context, *httptest.ResponseRecorder) {
	resp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(resp)
	return c, resp
}

func decodeSharedErrorResponse(body []byte) (struct {
	Success bool `json:"success"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}, error) {
	var resp struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	err := json.Unmarshal(body, &resp)
	return resp, err
}

func assertSharedAppErrorCode(err error, wantCode string) error {
	if err == nil {
		return errors.New("預期有錯誤，實際沒有錯誤")
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		return errors.New("預期錯誤型別為 apperror.Error")
	}
	if appErr.Code != wantCode {
		return errors.New("錯誤代碼不符合預期")
	}
	return nil
}
