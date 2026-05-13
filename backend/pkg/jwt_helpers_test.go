package pkg

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

func assertJWTClaimsMatch(claims *Claims, userID uuid.UUID, employeeID string, role string) error {
	if claims.UserID != userID.String() {
		return fmt.Errorf("預期 user_id 為 %q，實際為 %q", userID.String(), claims.UserID)
	}
	if claims.EmployeeID != employeeID {
		return fmt.Errorf("預期 employee_id 為 %q，實際為 %q", employeeID, claims.EmployeeID)
	}
	if claims.Role != role {
		return fmt.Errorf("預期 role 為 %q，實際為 %q", role, claims.Role)
	}
	return nil
}

func assertJWTRegisteredClaims(claims *Claims, issuedLowerBound time.Time) error {
	if claims.IssuedAt == nil {
		return fmt.Errorf("預期 issued_at 不應為 nil")
	}
	if claims.ExpiresAt == nil {
		return fmt.Errorf("預期 expires_at 不應為 nil")
	}
	if claims.ID == "" {
		return fmt.Errorf("預期 JWT ID 不應為空")
	}
	if _, err := uuid.Parse(claims.ID); err != nil {
		return fmt.Errorf("預期 JWT ID 為 UUID，實際為 %q：%w", claims.ID, err)
	}

	issuedAt := claims.IssuedAt.Time
	if issuedAt.Before(issuedLowerBound) || issuedAt.After(time.Now().Add(time.Second)) {
		return fmt.Errorf("預期 issued_at 接近測試執行時間，實際為 %s", issuedAt.Format(time.RFC3339))
	}

	expiresAt := claims.ExpiresAt.Time
	if !expiresAt.After(issuedAt) {
		return fmt.Errorf("預期 expires_at 晚於 issued_at，issued_at=%s，expires_at=%s", issuedAt.Format(time.RFC3339), expiresAt.Format(time.RFC3339))
	}

	lifetime := expiresAt.Sub(issuedAt)
	if lifetime < 23*time.Hour || lifetime > 25*time.Hour {
		return fmt.Errorf("預期 JWT 有效時間約為 24 小時，實際為 %s", lifetime)
	}
	return nil
}
