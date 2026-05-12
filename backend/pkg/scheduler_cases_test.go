package pkg

import "time"

type schedulerTransitionCheck struct {
	name       string
	wantStatus string
	timeout    time.Duration
	progress   string
}

func schedulerTransitionChecks() []schedulerTransitionCheck {
	return []schedulerTransitionCheck{
		{
			name:       "自動發布",
			wantStatus: "published",
			timeout:    3 * time.Second,
			progress:   "測試活動到達 publish_time 後會從 draft 自動更新為 published。",
		},
		{
			name:       "自動截止",
			wantStatus: "closed",
			timeout:    4 * time.Second,
			progress:   "測試活動到達 apply_deadline 後會從 published 自動更新為 closed。",
		},
		{
			name:       "自動結束",
			wantStatus: "ended",
			timeout:    4 * time.Second,
			progress:   "測試活動到達 end_time 後會自動更新為 ended。",
		},
	}
}
