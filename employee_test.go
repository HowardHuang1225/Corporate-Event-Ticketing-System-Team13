
import {
	"testing"
}

func TestEmployee(t *testing.T) {
	tasks := []testTask {
		{
			description: "測試員工只有符合活動資格時才能送出訂票申請",
			target:      ApplyRequiresEligibleEmployee,
		},
		{
			description: "測試送出申請單時 quantity 必須是正整數",
			target:      ApplyRejectsInvalidQuantity,
		},
		{
			description: "測試送出申請單時 status 必須是合法的數值",
			target:      ApplyRejectsInvalidStatus,
		},
		{
			description: "測試 quantity 不能超過個人上限與剩餘票量",
			target:      ApplyRejectsQuantityBeyondLimits,
		},
	}
}