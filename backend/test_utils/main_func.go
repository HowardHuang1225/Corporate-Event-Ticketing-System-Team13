package testutils

import (
	"fmt"
	"os"
	"reflect"
	"runtime"
	"testing"
)

type Task struct {
	Description string
	Target      func(t *testing.T, errs *Errors)
}

func RunTestTasks(t *testing.T, tasks []Task) {
	t.Helper()

	allErrMsg := []string{}
	result := []string{}

	for _, task := range tasks {
		taskErrs := &Errors{}
		pass := t.Run(task.Description, func(t *testing.T) {
			task.Target(t, taskErrs)
			if taskErrs.HasErrors() {
				funcName := runtime.FuncForPC(reflect.ValueOf(task.Target).Pointer()).Name()
				allErrMsg = append(allErrMsg, fmt.Sprintf("Task [%s] failed:\n%v\n", funcName, taskErrs.Err()))
				t.Fail()
			}
		})

		if !pass {
			result = append(result, fmt.Sprintf("\033[31mFAILED: %s\n\033[0m", task.Description))
		} else {
			result = append(result, fmt.Sprintf("\033[32mPASSED: %s\n\033[0m", task.Description))
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

func PrintTestProgress(message string) {
	writeTestProgress(message)
}

// func printTestProgressf(format string, args ...any) {
// 	writeTestProgress(fmt.Sprintf(format, args...))
// }

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