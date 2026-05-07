
import {
	"testing"
}

func TestShare(t *tseting.T) {
	tasks := []testTask {
		{
			description: "測試列表端點是否能夠正確過濾並回傳草稿、已發布、已截止和已結束的活動",
			target:      ListAllValidStates,
		},
		{
			description: "測試查看活動時是否能透過狀態、票種和活動時間篩選出對應活動",
			target:      FilterEventsByStatusTicketTypeAndTime,
		},
	}
}

func TestAuth(t *testing.T) {
	tasks := []testTask{
		{
			description: "測試 demo 帳號是否能成功登入並且拿到正確的角色、廠區和 JWT claims",
			target:      LoginDemoAccounts,
		},
		{
			description: "測試不合法的帳號密碼",
			target:      LoginInvalidCredentials,
		},
	}
}

func TestOther(t *testing.T) {
	tasks := []testTask {
		{
			description: "測試活動是否會根據時間表自動變更狀態，從草稿 -> 已發布 -> 已截止 -> 已結束",
			target:      EventStateTransitionsBySchedule,
		},
	}
}