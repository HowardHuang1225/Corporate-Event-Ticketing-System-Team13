package auth

import (
	"testing"

	utils "ticketing-system/backend/test_utils"

	"github.com/google/uuid"
)

const authServiceTestJWTSecret = "auth-service-test-secret"

func TestAuthService(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試 Me 會依 user_id 回傳使用者資料或正確錯誤",
			Target:      MeReturnsUserDTOAndRejectsInvalidContext,
		},
		{
			Description: "測試停用帳號不可登入",
			Target:      LoginRejectsInactiveUser,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func MeReturnsUserDTOAndRejectsInvalidContext(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試 Me 會依 user_id 回傳使用者資料或正確錯誤\n")
	utils.PrintTestProgress("==================================================\n")

	tx, user, cleanup, err := setupAuthServiceTest(t)
	if err != nil {
		errs.Add("準備 Me 測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := newAuthServiceForTest(tx)
	tests := []struct {
		name     string
		userID   string
		wantCode string
		progress string
	}{
		{
			name:     "合法 user_id",
			userID:   user.ID.String(),
			progress: "context 中有合法且存在的 user_id 時，Me 應回傳對應使用者 DTO。",
		},
		{
			name:     "user_id 格式錯誤",
			userID:   "not-a-uuid",
			wantCode: "UNAUTHORIZED",
			progress: "context 中的 user_id 不是 UUID 時，Me 應回傳 UNAUTHORIZED。",
		},
		{
			name:     "user_id 不存在",
			userID:   uuid.New().String(),
			wantCode: "NOT_FOUND",
			progress: "context 中的 user_id 格式正確但資料庫不存在時，Me 應回傳 NOT_FOUND。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			got, err := service.Me(tt.userID)
			if tt.wantCode != "" {
				if assertErr := assertAuthServiceAppErrorCode(err, tt.wantCode); assertErr != nil {
					errs.Add(tt.progress, "%v", assertErr)
				}
				return
			}

			if err != nil {
				errs.Add(tt.progress, "預期 Me 成功，實際錯誤：%v", err)
				return
			}
			if err := assertAuthServiceUserDTO(got, user); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func LoginRejectsInactiveUser(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試停用帳號不可登入\n")
	utils.PrintTestProgress("==================================================\n")

	tx, user, cleanup, err := setupAuthServiceTest(t)
	if err != nil {
		errs.Add("準備停用帳號登入測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	if err := tx.Model(&user).Update("is_active", false).Error; err != nil {
		errs.Add("停用測試帳號", "%v", err)
		return
	}

	service := newAuthServiceForTest(tx)
	tests := []struct {
		name     string
		progress string
	}{
		{
			name:     "停用帳號登入",
			progress: "帳號已停用時，即使 employee_id 與密碼正確，也應回傳 UNAUTHORIZED。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			_, err := service.Login(LoginRequest{
				EmployeeID: user.EmployeeID,
				Password:   "password",
			})
			if assertErr := assertAuthServiceAppErrorCode(err, "UNAUTHORIZED"); assertErr != nil {
				errs.Add(tt.progress, "%v", assertErr)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}
