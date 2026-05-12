package testutils

import (
	"encoding/json"
	"errors"
	"fmt"
)

type Errors struct {
	errs []error
}

func (e *Errors) Add(progress string, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if progress != "" {
		message = fmt.Sprintf("\tWhen %s\n\t%s\n", progress, message)
	} else {
		message = "\t" + message
	}
	e.errs = append(e.errs, errors.New(message))
}

func (e *Errors) Err() error {
	return errors.Join(e.errs...)
}

func (e *Errors) HasErrors() bool {
	return len(e.errs) > 0
}

func AssertHandlerErrorCode(body []byte, want string) error {
	code, err := DecodeHandlerError(body)
	if err != nil {
		return err
	}
	if code != want {
		return fmt.Errorf("expected error code %q, got %q", want, code)
	}
	return nil
}

func DecodeHandlerError(body []byte) (string, error) {
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to decode error response: %w", err)
	}
	return resp.Error.Code, nil
}
