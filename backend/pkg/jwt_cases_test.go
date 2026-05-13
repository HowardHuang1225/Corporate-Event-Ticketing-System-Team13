package pkg

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type jwtInvalidValidationCase struct {
	name     string
	token    string
	progress string
}

func invalidJWTValidationCases(secret string) ([]jwtInvalidValidationCase, error) {
	wrongSecretToken, err := GenerateToken(uuid.New(), "EMP404", "employee", secret+"-wrong")
	if err != nil {
		return nil, err
	}

	unsupportedSigningToken, err := generateUnsupportedSigningToken()
	if err != nil {
		return nil, err
	}

	return []jwtInvalidValidationCase{
		{
			name:     "錯誤 secret",
			token:    wrongSecretToken,
			progress: "使用不同 secret 簽出的 JWT，應在驗證時失敗。",
		},
		{
			name:     "malformed token",
			token:    "not-a-valid-jwt-token",
			progress: "不是 JWT 結構的 token，應在驗證時失敗。",
		},
		{
			name:     "不支援簽章",
			token:    unsupportedSigningToken,
			progress: "使用非 HMAC 簽章方法的 JWT，應在驗證時失敗。",
		},
	}, nil
}

func generateUnsupportedSigningToken() (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:     uuid.New().String(),
		EmployeeID: "EMP999",
		Role:       "employee",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.New().String(),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
}
