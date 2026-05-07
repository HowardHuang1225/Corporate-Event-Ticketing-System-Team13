impor {
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
}

type testErrors struct {
	errs []error
}

func (e *testErrors) Add(progress string, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if progress != "" {
		message = fmt.Sprintf("\tWhen %s\n\t%s\n", progress, message)
	} else {
		message = "\t" + message
	}
	e.errs = append(e.errs, errors.New(message))
}

func (e *testErrors) Err() error {
	return errors.Join(e.errs...)
}

func (e *testErrors) HasErrors() bool {
	return len(e.errs) > 0
}

func assertHandlerErrorCode(body []byte, want string) error {
	code, _, err := decodeHandlerError(body)
	if err != nil {
		return err
	}
	if code != want {
		return fmt.Errorf("expected error code %q, got %q", want, code)
	}
	return nil
}

func assertHandlerErrorResponse(body []byte, wantCode string, wantMessageContains string) error {
	code, message, err := decodeHandlerError(body)
	if err != nil {
		return err
	}
	if code != wantCode {
		return fmt.Errorf("expected error code %q, got %q", wantCode, code)
	}
	if !strings.Contains(message, wantMessageContains) {
		return fmt.Errorf("expected error message to contain %q, got %q", wantMessageContains, message)
	}
	return nil
}

func decodeHandlerError(body []byte) (string, string, error) {
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("failed to decode error response: %w", err)
	}
	return resp.Error.Code, resp.Error.Message, nil
}