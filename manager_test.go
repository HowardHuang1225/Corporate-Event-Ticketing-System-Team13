
import {
	"testing"
}

func TestManager(t *testing.T) {
	tasks := []testTask {
		{
			description: "測試創建合法活動",
			target:      CreateValidEvent,
		},
		{
			description: "測試建立不合法活動時是否回傳正確錯誤訊息",
			target:      CreateInvalidEvent,
		},
		{
			description: "測試管理員是否能手動將活動狀態從草稿改成已發布",
			target:      PublishDraftEventManually,
		},
		{
			description: "測試管理員是否能手動提早截止報名",
			target:      CloseRegistrationEarly,
		},
		// {
		// 	description: "測試管理員是否能透過既有活動複製出可修改的新活動草稿",
		// 	target:      CloneExistingEventForEditing,
		// },
	}
}