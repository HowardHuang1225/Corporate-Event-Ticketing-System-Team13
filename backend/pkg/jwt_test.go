package pkg

import (
	"testing"
	"time"

	utils "ticketing-system/backend/test_utils"

	"github.com/google/uuid"
)

const jwtTestSecret = "pkg-jwt-test-secret"

func TestJWT(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試 JWT 產生與驗證 claims",
			Target:      GenerateAndValidateJWTClaims,
		},
		{
			Description: "測試錯誤 secret、malformed token、不支援簽章會驗證失敗",
			Target:      RejectInvalidJWTTokens,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func GenerateAndValidateJWTClaims(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試 JWT 產生與驗證 claims\n")
	utils.PrintTestProgress("==================================================\n")

	tests := []struct {
		name       string
		userID     uuid.UUID
		employeeID string
		role       string
		progress   string
	}{
		{
			name:       "合法 JWT claims",
			userID:     uuid.New(),
			employeeID: "EMP123",
			role:       "event_manager",
			progress:   "產生合法 JWT 後，使用相同 secret 驗證並確認 user_id、employee_id、role 與時間 claims。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			issuedLowerBound := time.Now().Add(-1 * time.Second)
			token, err := GenerateToken(tt.userID, tt.employeeID, tt.role, jwtTestSecret)
			if err != nil {
				errs.Add(tt.progress, "產生 JWT 失敗：%v", err)
				return
			}
			if token == "" {
				errs.Add(tt.progress, "預期 token 不應為空字串")
				return
			}

			claims, err := ValidateToken(token, jwtTestSecret)
			if err != nil {
				errs.Add(tt.progress, "預期 token 驗證成功，實際錯誤：%v", err)
				return
			}
			if claims == nil {
				errs.Add(tt.progress, "預期 claims 不應為 nil")
				return
			}
			if err := assertJWTClaimsMatch(claims, tt.userID, tt.employeeID, tt.role); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if err := assertJWTRegisteredClaims(claims, issuedLowerBound); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func RejectInvalidJWTTokens(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("測試錯誤 secret、malformed token、不支援簽章會驗證失敗\n")
	utils.PrintTestProgress("==================================================\n")

	tests, err := invalidJWTValidationCases(jwtTestSecret)
	if err != nil {
		errs.Add("準備 JWT 驗證失敗案例", "%v", err)
		return
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress("子測試：" + tt.progress + "\n")

			claims, err := ValidateToken(tt.token, jwtTestSecret)
			if err == nil {
				errs.Add(tt.progress, "預期驗證失敗，實際成功，claims：%+v", claims)
				return
			}
			if claims != nil {
				errs.Add(tt.progress, "驗證失敗時預期 claims 為 nil，實際為：%+v", claims)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}
