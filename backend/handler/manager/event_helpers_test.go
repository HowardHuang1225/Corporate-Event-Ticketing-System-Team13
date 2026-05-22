package manager

import (
	"fmt"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	eventsvc "ticketing-system/backend/service/event"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var managerEventTestModels = []any{&model.User{}, &model.Event{}, &model.TicketType{}}

type managerEventResponse struct {
	Success bool        `json:"success"`
	Data    model.Event `json:"data"`
}

type managerCreateInvalidCase struct {
	name     string
	progress string
	mutate   func(gin.H)
}

func setupManagerEventTest(t *testing.T) (*gorm.DB, model.User, func(), error) {
	t.Helper()

	tx, cleanup, err := utils.BeginTestTransaction(t, managerEventTestModels)
	if err != nil {
		return nil, model.User{}, nil, err
	}

	suffix := utils.UniqueTestSuffix()
	manager := model.User{
		EmployeeID:   fmt.Sprintf("EVTMGR%s", suffix),
		Name:         "活動測試管理者",
		Email:        fmt.Sprintf("event-manager-%s@example.com", suffix),
		Department:   "Events",
		Region:       "Tainan",
		Role:         "event_manager",
		PasswordHash: "",
		IsActive:     true,
	}
	users, err := utils.SeedTestRole(tx, []model.User{manager}, true)
	if err != nil {
		_ = tx.Rollback()
		return nil, model.User{}, nil, err
	}

	return tx, users[0], cleanup, nil
}

func newManagerEventRouter(db *gorm.DB, manager model.User) *gin.Engine {
	repos := repository.New(db, nil)
	handler := New(eventsvc.New(repos), nil, nil, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.POST("/events", handler.CreateEvent)
	router.PATCH("/events/:id/publish", handler.PublishEvent)
	router.PATCH("/events/:id/close", handler.CloseEvent)
	return router
}

func validManagerCreateEventPayload(now time.Time) gin.H {
	return gin.H{
		"title":                  "單元測試公司日活動",
		"description":            "由管理者活動 handler 測試建立",
		"venue":                  "主會場",
		"publish_time":           now.Add(48 * time.Hour).Format(time.RFC3339),
		"start_time":             now.Add(72 * time.Hour).Format(time.RFC3339),
		"apply_deadline":         now.Add(74 * time.Hour).Format(time.RFC3339),
		"end_time":               now.Add(76 * time.Hour).Format(time.RFC3339),
		"max_tickets_per_person": 2,
		"ticket_types": []gin.H{
			{"name": "一般票", "total_quota": 100},
			{"name": "VIP票", "total_quota": 50},
		},
	}
}

func managerCreateInvalidCases(now time.Time) []managerCreateInvalidCase {
	cases := []managerCreateInvalidCase{
		{
			name:     "缺少發布時間",
			progress: "測試建立活動時 publish_time 為必填欄位。",
			mutate: func(payload gin.H) {
				delete(payload, "publish_time")
			},
		},
	}
	cases = append(cases, managerCreateInvalidTimeCases(now)...)
	cases = append(cases, managerCreateInvalidTicketCases()...)
	cases = append(cases, managerCreateInvalidValueCases()...)
	cases = append(cases, managerCreateInvalidStatusCases()...)
	return cases
}

func managerCreateInvalidTimeCases(now time.Time) []managerCreateInvalidCase {
	return []managerCreateInvalidCase{
		{
			name:     "發布時間晚於開始時間",
			progress: "測試 publish_time 晚於 start_time 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["publish_time"] = now.Add(73 * time.Hour).Format(time.RFC3339)
			},
		},
		{
			name:     "申請截止時間晚於活動結束時間",
			progress: "測試 apply_deadline 晚於 end_time 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["apply_deadline"] = now.Add(77 * time.Hour).Format(time.RFC3339)
			},
		},
	}
}

func managerCreateInvalidTicketCases() []managerCreateInvalidCase {
	validTicket := gin.H{"name": "一般票", "total_quota": 10}
	return []managerCreateInvalidCase{
		{
			name:     "未提供任何票種",
			progress: "測試 ticket_types 是空陣列時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{}
			},
		},
		{
			name:     "票種欄位為空值",
			progress: "測試 ticket_types 為 nil 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = nil
			},
		},
		{
			name:     "票種缺少名稱",
			progress: "測試其中一個票種缺少 name 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"total_quota": 10},
				}
			},
		},
		{
			name:     "票種缺少總名額",
			progress: "測試其中一個票種缺少 total_quota 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": "無效票"},
				}
			},
		},
		{
			name:     "票種缺少名稱與總名額",
			progress: "測試其中一個票種同時缺少 name 與 total_quota 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{},
				}
			},
		},
		{
			name:     "票種項目為空值",
			progress: "測試 ticket_types 內含 nil 票種項目時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []any{
					validTicket,
					nil,
				}
			},
		},
		{
			name:     "票種名稱為空值",
			progress: "測試票種 name 為 nil 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": nil, "total_quota": 10},
				}
			},
		},
		{
			name:     "票種名稱為空字串",
			progress: "測試票種 name 是空字串時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": "", "total_quota": 10},
				}
			},
		},
		{
			name:     "票種名稱只有空白",
			progress: "測試票種 name 只有空白字元時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": "   ", "total_quota": 10},
				}
			},
		},
		{
			name:     "票種名稱重複",
			progress: "測試同一個活動內有重複票種名稱時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": "重複票", "total_quota": 10},
					{"name": "重複票", "total_quota": 20},
				}
			},
		},
		{
			name:     "票種名稱不是字串",
			progress: "測試票種 name 不是字串型別時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": 123, "total_quota": 10},
				}
			},
		},
		{
			name:     "票種總名額為空值",
			progress: "測試票種 total_quota 為 nil 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": "無效票", "total_quota": nil},
				}
			},
		},
		{
			name:     "票種總名額為零",
			progress: "測試票種 total_quota 為 0 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": "無效票", "total_quota": 0},
				}
			},
		},
		{
			name:     "票種總名額為負數",
			progress: "測試票種 total_quota 是負數時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": "無效票", "total_quota": -5},
				}
			},
		},
		{
			name:     "票種總名額為小數",
			progress: "測試票種 total_quota 是小數時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": "無效票", "total_quota": 10.5},
				}
			},
		},
		{
			name:     "票種總名額不是數字",
			progress: "測試票種 total_quota 不是整數型別時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": "無效票", "total_quota": "ten"},
				}
			},
		},
		{
			name:     "票種包含額外欄位",
			progress: "測試票種帶入未定義的 extra_field 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					validTicket,
					{"name": "無效票", "total_quota": 10, "extra_field": "not allowed"},
				}
			},
		},
	}
}

func managerCreateInvalidValueCases() []managerCreateInvalidCase {
	return []managerCreateInvalidCase{
		{
			name:     "每人可申請張數為空值",
			progress: "測試 max_tickets_per_person 為 nil 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["max_tickets_per_person"] = nil
			},
		},
		{
			name:     "每人可申請張數為零",
			progress: "測試 max_tickets_per_person 為 0 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["max_tickets_per_person"] = 0
			},
		},
		{
			name:     "每人可申請張數為負數",
			progress: "測試 max_tickets_per_person 是負數時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["max_tickets_per_person"] = -1
			},
		},
		{
			name:     "每人可申請張數為小數",
			progress: "測試 max_tickets_per_person 是小數時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["max_tickets_per_person"] = 2.5
			},
		},
		{
			name:     "每人可申請張數不是數字",
			progress: "測試 max_tickets_per_person 不是整數型別時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["max_tickets_per_person"] = "two"
			},
		},
		{
			name:     "活動名稱為空字串",
			progress: "測試 title 為空字串時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["title"] = ""
			},
		},
		{
			name:     "活動名稱只有空白",
			progress: "測試 title 只有空白字元時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["title"] = "   "
			},
		},
		{
			name:     "地點為空字串",
			progress: "測試 venue 為空字串時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["venue"] = ""
			},
		},
		{
			name:     "地點只有空白",
			progress: "測試 venue 只有空白字元時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["venue"] = "   "
			},
		},
		{
			name:     "每人票數大於總票數限制",
			progress: "測試 max_tickets_per_person 大於所有票種總 quota 時會被拒絕。",
			mutate: func(payload gin.H) {
				payload["max_tickets_per_person"] = 151
			},
		},
	}
}

func managerCreateInvalidStatusCases() []managerCreateInvalidCase {
	return []managerCreateInvalidCase{
		{
			name:     "狀態為空值",
			progress: "測試建立活動時 status 為 nil 會被拒絕。",
			mutate: func(payload gin.H) {
				payload["status"] = nil
			},
		},
		{
			name:     "狀態為空字串",
			progress: "測試建立活動時 status 是空字串會被拒絕。",
			mutate: func(payload gin.H) {
				payload["status"] = ""
			},
		},
		{
			name:     "狀態只有空白",
			progress: "測試建立活動時 status 只有空白字元會被拒絕。",
			mutate: func(payload gin.H) {
				payload["status"] = "   "
			},
		},
		{
			name:     "狀態為大寫",
			progress: "測試建立活動時 status 使用大寫 DRAFT 會被拒絕。",
			mutate: func(payload gin.H) {
				payload["status"] = "DRAFT"
			},
		},
		{
			name:     "狀態為已關閉",
			progress: "測試建立活動時 status 是 closed 會被拒絕。",
			mutate: func(payload gin.H) {
				payload["status"] = "closed"
			},
		},
		{
			name:     "狀態為已結束",
			progress: "測試建立活動時 status 是 ended 會被拒絕。",
			mutate: func(payload gin.H) {
				payload["status"] = "ended"
			},
		},
		{
			name:     "狀態為未知值",
			progress: "測試建立活動時 status 是 invalid_status 會被拒絕。",
			mutate: func(payload gin.H) {
				payload["status"] = "invalid_status"
			},
		},
		{
			name:     "狀態前後包含空白",
			progress: "測試建立活動時 status 前後有空白會被拒絕。",
			mutate: func(payload gin.H) {
				payload["status"] = " draft "
			},
		},
	}
}

func decodeManagerEventResponse(body []byte) (managerEventResponse, error) {
	return utils.DecodeJSON[managerEventResponse](body)
}

func seedManagerActionEvent(db *gorm.DB, manager model.User, status string) (model.Event, error) {
	now := time.Now().UTC().Truncate(time.Second)
	region := "Tainan"
	event := model.Event{
		Title:               fmt.Sprintf("單元測試手動狀態活動 %s", utils.UniqueTestSuffix()),
		Description:         "手動操作活動狀態測試資料",
		Venue:               "主會場",
		ImageURL:            "https://example.com/manual-event.png",
		PublishTime:         now.Add(24 * time.Hour),
		StartTime:           now.Add(72 * time.Hour),
		ApplyDeadline:       now.Add(96 * time.Hour),
		EndTime:             now.Add(120 * time.Hour),
		Status:              status,
		RegionRestriction:   &region,
		MaxTicketsPerPerson: 3,
		CreatedBy:           manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, fmt.Errorf("建立活動狀態操作測試資料失敗: %w", err)
	}

	ticketTypes := []model.TicketType{
		{EventID: event.ID, Name: "一般票", TotalQuota: 80, Remaining: 65},
		{EventID: event.ID, Name: "VIP票", TotalQuota: 20, Remaining: 12},
	}
	for _, ticketType := range ticketTypes {
		if err := db.Create(&ticketType).Error; err != nil {
			return model.Event{}, fmt.Errorf("建立票種測試資料 %q 失敗: %w", ticketType.Name, err)
		}
	}

	if err := db.Preload("TicketTypes").First(&event, "id = ?", event.ID).Error; err != nil {
		return model.Event{}, fmt.Errorf("重新讀取活動狀態操作測試資料失敗: %w", err)
	}
	return event, nil
}

func assertManagerEventStatus(db *gorm.DB, eventID string, want string) error {
	var got string
	if err := db.Raw("SELECT status FROM events WHERE id = ?", eventID).Scan(&got).Error; err != nil {
		return fmt.Errorf("查詢活動 %s 狀態失敗: %w", eventID, err)
	}
	if got != want {
		return fmt.Errorf("expected event status %q, got %q", want, got)
	}
	return nil
}
