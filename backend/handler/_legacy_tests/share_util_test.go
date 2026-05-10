package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type testTask struct {
	description string
	target      func(t *testing.T, errs *testErrors)
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

func mainTestFunc(t *testing.T, tasks []testTask) {
	t.Helper()

	allErrMsg := []string{}
	result := []string{}

	for _, task := range tasks {
		taskErrs := &testErrors{}
		pass := t.Run(task.description, func(t *testing.T) {
			task.target(t, taskErrs)
			if taskErrs.HasErrors() {
				funcName := runtime.FuncForPC(reflect.ValueOf(task.target).Pointer()).Name()
				allErrMsg = append(allErrMsg, fmt.Sprintf("Task [%s] failed:\n%v\n", funcName, taskErrs.Err()))
				t.Fail()
			}
		})

		if !pass {
			result = append(result, fmt.Sprintf("\033[31mFAILED: %s\n\033[0m", task.description))
			// fmt.Print("\033[31m") // red
			// fmt.Printf("FAILED: %s\n", task.description)
			// fmt.Print("\033[0m") // reset
		} else {
			result = append(result, fmt.Sprintf("\033[32mPASSED: %s\n\033[0m", task.description))
			// fmt.Print("\033[32m") // green
			// fmt.Printf("PASSED: %s\n", task.description)
			// fmt.Print("\033[0m") // reset
		}
	}

	if len(allErrMsg) > 0 {
		fmt.Print("\033[31m") // red
		fmt.Printf("\nFAILED: %d tasks failed\n", len(allErrMsg))
		for _, errMsg := range allErrMsg {
			fmt.Printf("- %s\n", errMsg)
		}
		fmt.Print("\033[0m") // reset
	}

	for _, r := range result {
		fmt.Print(r)
	}
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
