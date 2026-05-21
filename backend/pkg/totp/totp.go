package totp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
)

// Generate creates a 6-digit TOTP based on the secret and counter.
func Generate(secret string, counter int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))
	mac.Write(buf)
	hash := mac.Sum(nil)
	
	offset := hash[len(hash)-1] & 0xf
	binaryCode := int(hash[offset]&0x7f)<<24 |
		int(hash[offset+1]&0xff)<<16 |
		int(hash[offset+2]&0xff)<<8 |
		int(hash[offset+3]&0xff)

	otp := binaryCode % 1000000
	return fmt.Sprintf("%06d", otp)
}

// Verify checks if the provided OTP is valid within a window (usually 15s or 30s)
// allowing +/- 1 window for clock drift.
func Verify(secret string, providedOTP string, windowSeconds int64) bool {
	currentCounter := time.Now().Unix() / windowSeconds
	
	// Check current, previous, and next window
	for i := int64(-1); i <= 1; i++ {
		if Generate(secret, currentCounter+i) == providedOTP {
			return true
		}
	}
	return false
}
