package handler

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func assertHandlerErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != want {
		t.Fatalf("expected error code %q, got %q", want, resp.Error.Code)
	}
}

func printTestProgress(message string) {
	writeTestProgress(message)
}

func printTestProgressf(format string, args ...any) {
	writeTestProgress(fmt.Sprintf(format, args...))
}

func writeTestProgress(message string) {
	if message == "" {
		return
	}

	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err == nil {
		if _, writeErr := tty.WriteString(message); writeErr == nil {
			_ = tty.Close()
			return
		}
		_ = tty.Close()
	}

	fmt.Print(message)
}
