package totp

import (
	"fmt"
	"testing"
	"time"
)

func TestGenerateReturnsDeterministicSixDigitCodes(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		counter int64
		want    string
	}{
		{"counter zero", "test-secret", 0, "870290"},
		{"counter one", "test-secret", 1, "027384"},
		{"counter two", "test-secret", 2, "054004"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Generate(tt.secret, tt.counter); got != tt.want {
				t.Fatalf("Generate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerifyAcceptsCurrentAdjacentWindowAndRejectsInvalidOTP(t *testing.T) {
	secret := "test-secret"
	windowSeconds := int64(60)
	currentCounter := time.Now().Unix() / windowSeconds

	validOTP := Generate(secret, currentCounter)
	if !Verify(secret, validOTP, windowSeconds) {
		t.Fatal("Verify() should accept the current window OTP")
	}

	previousOTP := Generate(secret, currentCounter-1)
	if !Verify(secret, previousOTP, windowSeconds) {
		t.Fatal("Verify() should accept the previous window OTP")
	}

	nextOTP := Generate(secret, currentCounter+1)
	if !Verify(secret, nextOTP, windowSeconds) {
		t.Fatal("Verify() should accept the next window OTP")
	}

	if Verify(secret, "000000", windowSeconds) {
		t.Fatal("Verify() should reject an invalid OTP")
	}
}

func TestVerifyRejectsExpiredPreviousWindowOTP(t *testing.T) {
	windowSeconds := int64(60)
	currentCounter := time.Now().Unix() / windowSeconds

	for i := 0; i < 100; i++ {
		secret := fmt.Sprintf("expired-window-secret-%d", i)
		expiredOTP := Generate(secret, currentCounter-2)
		if expiredOTP == Generate(secret, currentCounter-1) ||
			expiredOTP == Generate(secret, currentCounter) ||
			expiredOTP == Generate(secret, currentCounter+1) {
			continue
		}

		if Verify(secret, expiredOTP, windowSeconds) {
			t.Fatal("Verify() should reject an OTP older than the allowed adjacent window")
		}
		return
	}

	t.Fatal("could not find a non-colliding expired OTP test secret")
}
