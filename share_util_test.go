
import {
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
}

type testTask struct {
	description string
	target      func(t *testing.T, errs *testErrors)
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
		} else {
			result = append(result, fmt.Sprintf("\033[32mPASSED: %s\n\033[0m", task.description))
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