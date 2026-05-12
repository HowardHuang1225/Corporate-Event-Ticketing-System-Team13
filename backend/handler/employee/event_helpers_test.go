package employee

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	eventsvc "ticketing-system/backend/service/event"
	ticketsvc "ticketing-system/backend/service/ticket"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var employeeEventTestModels = []any{
	&model.User{},
	&model.Event{},
	&model.TicketType{},
	&model.Application{},
	&model.Ticket{},
}

type employeeEventUsers struct {
	Manager  model.User
	Employee model.User
}

type employeeEventResponse struct {
	Success bool        `json:"success"`
	Data    model.Event `json:"data"`
}

type employeeEventListResponse struct {
	Success bool          `json:"success"`
	Data    []model.Event `json:"data"`
}

type employeeEligibilityResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Eligible bool `json:"eligible"`
	} `json:"data"`
}

type employeeApplicationResponse struct {
	Success bool              `json:"success"`
	Data    model.Application `json:"data"`
}

type employeeApplicationListResponse struct {
	Success bool                `json:"success"`
	Data    []model.Application `json:"data"`
}

type employeeTicketListResponse struct {
	Success bool           `json:"success"`
	Data    []model.Ticket `json:"data"`
}

func setupEmployeeEventTest(t *testing.T) (*gorm.DB, employeeEventUsers, func(), error) {
	t.Helper()

	tx, cleanup, err := utils.BeginTestTransaction(t, employeeEventTestModels)
	if err != nil {
		return nil, employeeEventUsers{}, nil, err
	}

	suffix := utils.UniqueTestSuffix()
	seeded, err := utils.SeedTestRole(tx, []model.User{
		{
			EmployeeID:   fmt.Sprintf("EVTMGR%s", suffix),
			Name:         "活動測試管理者",
			Email:        fmt.Sprintf("event-manager-%s@example.com", suffix),
			Department:   "Events",
			Region:       "Tainan",
			Role:         "event_manager",
			PasswordHash: "",
			IsActive:     true,
		},
		{
			EmployeeID:   fmt.Sprintf("EVTEMP%s", suffix),
			Name:         "活動測試員工",
			Email:        fmt.Sprintf("event-employee-%s@example.com", suffix),
			Department:   "Engineering",
			Region:       "Tainan",
			Role:         "employee",
			PasswordHash: "",
			IsActive:     true,
		},
	}, true)
	if err != nil {
		_ = tx.Rollback()
		return nil, employeeEventUsers{}, nil, err
	}

	return tx, employeeEventUsers{Manager: seeded[0], Employee: seeded[1]}, cleanup, nil
}

func newEmployeeEventRouter(db *gorm.DB, user model.User, role string) *gin.Engine {
	repos := repository.New(db, nil)
	handler := New(eventsvc.New(repos), nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", user.ID.String())
		c.Set("role", role)
		c.Next()
	})
	router.GET("/events", handler.ListEvents)
	router.GET("/events/:id", handler.GetEvent)
	router.GET("/events/:id/eligibility", handler.CheckEligibility)
	return router
}

func newEmployeeTicketRouter(db *gorm.DB, redisClient *redis.Client, user model.User) *gin.Engine {
	repos := repository.New(db, redisClient)
	handler := New(eventsvc.New(repos), ticketsvc.New(repos))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", user.ID.String())
		c.Set("role", "employee")
		c.Next()
	})
	router.POST("/applications", handler.Apply)
	router.GET("/applications/my", handler.MyApplications)
	router.GET("/tickets/my", handler.MyTickets)
	return router
}

func seedEmployeeEventsForStates(db *gorm.DB, manager model.User, titlePrefix string, states []string) error {
	now := time.Now().UTC().Truncate(time.Second)
	for index, state := range states {
		event := model.Event{
			Title:               fmt.Sprintf("%s %s", titlePrefix, state),
			Description:         "員工活動列表狀態測試資料",
			Venue:               "主會場",
			PublishTime:         now.Add(time.Duration(index) * time.Hour),
			StartTime:           now.Add(time.Duration(index+1) * 24 * time.Hour),
			EndTime:             now.Add(time.Duration(index+1)*24*time.Hour + 2*time.Hour),
			ApplyDeadline:       now.Add(time.Duration(index+1)*24*time.Hour + time.Hour),
			Status:              state,
			MaxTicketsPerPerson: 1,
			CreatedBy:           manager.ID,
		}
		if err := db.Create(&event).Error; err != nil {
			return fmt.Errorf("建立 %s 狀態活動測試資料失敗: %w", state, err)
		}
	}
	return nil
}

func seedEmployeeListFilterEvent(
	db *gorm.DB,
	manager model.User,
	title string,
	status string,
	ticketTypeName string,
	startTime time.Time,
) (model.Event, error) {
	event := model.Event{
		Title:               title,
		Description:         "員工活動條件篩選測試資料",
		Venue:               "主會場",
		PublishTime:         startTime.Add(-24 * time.Hour),
		StartTime:           startTime,
		ApplyDeadline:       startTime.Add(12 * time.Hour),
		EndTime:             startTime.Add(24 * time.Hour),
		Status:              status,
		MaxTicketsPerPerson: 1,
		CreatedBy:           manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, fmt.Errorf("建立活動篩選測試資料失敗: %w", err)
	}

	ticketType := model.TicketType{
		EventID:    event.ID,
		Name:       ticketTypeName,
		TotalQuota: 100,
		Remaining:  100,
	}
	if err := db.Create(&ticketType).Error; err != nil {
		return model.Event{}, fmt.Errorf("建立活動篩選票種測試資料失敗: %w", err)
	}

	if err := db.Preload("TicketTypes").First(&event, "id = ?", event.ID).Error; err != nil {
		return model.Event{}, fmt.Errorf("重新讀取活動篩選測試資料失敗: %w", err)
	}
	return event, nil
}

func seedEmployeeEventWithTickets(db *gorm.DB, manager model.User, status string, region string) (model.Event, error) {
	now := time.Now().UTC().Truncate(time.Second)
	event := model.Event{
		Title:               fmt.Sprintf("員工活動詳細資料測試 %s", utils.UniqueTestSuffix()),
		Description:         "活動詳細資料測試資料",
		Venue:               "主會場",
		PublishTime:         now.Add(-time.Hour),
		StartTime:           now.Add(12 * time.Hour),
		ApplyDeadline:       now.Add(24 * time.Hour),
		EndTime:             now.Add(26 * time.Hour),
		Status:              status,
		RegionRestriction:   &region,
		MaxTicketsPerPerson: 2,
		CreatedBy:           manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, fmt.Errorf("建立活動詳細資料測試資料失敗: %w", err)
	}

	ticketTypes := []model.TicketType{
		{EventID: event.ID, Name: "一般票", TotalQuota: 100, Remaining: 100},
		{EventID: event.ID, Name: "VIP票", TotalQuota: 20, Remaining: 20},
	}
	for _, ticketType := range ticketTypes {
		if err := db.Create(&ticketType).Error; err != nil {
			return model.Event{}, fmt.Errorf("建立票種測試資料 %q 失敗: %w", ticketType.Name, err)
		}
	}

	if err := db.Preload("TicketTypes").First(&event, "id = ?", event.ID).Error; err != nil {
		return model.Event{}, fmt.Errorf("重新讀取活動詳細資料測試資料失敗: %w", err)
	}
	return event, nil
}

func seedEmployeeApplication(
	db *gorm.DB,
	user model.User,
	event model.Event,
	ticketType model.TicketType,
	status string,
	quantity int,
) (model.Application, error) {
	app := model.Application{
		UserID:         user.ID,
		EventID:        event.ID,
		TicketTypeID:   ticketType.ID,
		Quantity:       quantity,
		Status:         status,
		IdempotencyKey: fmt.Sprintf("app-%s", utils.UniqueTestSuffix()),
	}
	if err := db.Create(&app).Error; err != nil {
		return model.Application{}, fmt.Errorf("建立申請狀態測試資料失敗: %w", err)
	}
	return app, nil
}

func seedEmployeeTicket(
	db *gorm.DB,
	user model.User,
	event model.Event,
	ticketType model.TicketType,
	app model.Application,
) (model.Ticket, error) {
	ticket := model.Ticket{
		ApplicationID: app.ID,
		UserID:        user.ID,
		EventID:       event.ID,
		TicketTypeID:  ticketType.ID,
		QRToken:       fmt.Sprintf("qr-%s", utils.UniqueTestSuffix()),
		ExpiresAt:     event.EndTime,
	}
	if err := db.Create(&ticket).Error; err != nil {
		return model.Ticket{}, fmt.Errorf("建立已核准票券測試資料失敗: %w", err)
	}
	return ticket, nil
}

func seedExtraEmployee(db *gorm.DB, region string) (model.User, error) {
	suffix := utils.UniqueTestSuffix()
	users, err := utils.SeedTestRole(db, []model.User{
		{
			EmployeeID:   fmt.Sprintf("OTHER%s", suffix),
			Name:         "其他測試員工",
			Email:        fmt.Sprintf("other-employee-%s@example.com", suffix),
			Department:   "Engineering",
			Region:       region,
			Role:         "employee",
			PasswordHash: "",
			IsActive:     true,
		},
	}, true)
	if err != nil {
		return model.User{}, err
	}
	return users[0], nil
}

func seedEmployeeEligibilityEvent(db *gorm.DB, manager model.User, status string, region string, deadline time.Time) (model.Event, error) {
	startTime := deadline.Add(-12 * time.Hour)
	event := model.Event{
		Title:               fmt.Sprintf("員工報名資格測試 %s", utils.UniqueTestSuffix()),
		Description:         "活動報名資格測試資料",
		Venue:               "主會場",
		PublishTime:         startTime.Add(-time.Hour),
		StartTime:           startTime,
		ApplyDeadline:       deadline,
		EndTime:             deadline.Add(2 * time.Hour),
		Status:              status,
		RegionRestriction:   &region,
		MaxTicketsPerPerson: 1,
		CreatedBy:           manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, fmt.Errorf("建立活動報名資格測試資料失敗: %w", err)
	}
	return event, nil
}

func decodeEmployeeEventResponse(body []byte) (employeeEventResponse, error) {
	return utils.DecodeJSON[employeeEventResponse](body)
}

func decodeEmployeeEventListResponse(body []byte) (employeeEventListResponse, error) {
	return utils.DecodeJSON[employeeEventListResponse](body)
}

func decodeEmployeeEligibilityResponse(body []byte) (employeeEligibilityResponse, error) {
	return utils.DecodeJSON[employeeEligibilityResponse](body)
}

func decodeEmployeeApplicationResponse(body []byte) (employeeApplicationResponse, error) {
	return utils.DecodeJSON[employeeApplicationResponse](body)
}

func decodeEmployeeApplicationListResponse(body []byte) (employeeApplicationListResponse, error) {
	return utils.DecodeJSON[employeeApplicationListResponse](body)
}

func decodeEmployeeTicketListResponse(body []byte) (employeeTicketListResponse, error) {
	return utils.DecodeJSON[employeeTicketListResponse](body)
}

func employeeEventListContainsStateFixture(events []model.Event, titlePrefix, state string) bool {
	wantTitle := fmt.Sprintf("%s %s", titlePrefix, state)
	for _, event := range events {
		if event.Title == wantTitle && event.Status == state {
			return true
		}
	}
	return false
}

func hasEmployeeEventPrefix(title string, prefix string) bool {
	return strings.HasPrefix(title, prefix)
}

func employeeEventHasTicketType(event model.Event, ticketTypeName string) bool {
	for _, ticketType := range event.TicketTypes {
		if ticketType.Name == ticketTypeName {
			return true
		}
	}
	return false
}

func employeeApplicationListContains(apps []model.Application, appID string) bool {
	for _, app := range apps {
		if app.ID.String() == appID {
			return true
		}
	}
	return false
}

func employeeTicketListContains(tickets []model.Ticket, ticketID string) bool {
	for _, ticket := range tickets {
		if ticket.ID.String() == ticketID {
			return true
		}
	}
	return false
}

func openEmployeeTestRedis(t *testing.T) (*redis.Client, error) {
	t.Helper()

	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = os.Getenv("REDIS_URL")
	}
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("解析 Redis 測試連線字串失敗: %w", err)
	}
	client := redis.NewClient(opt)
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis 測試服務無法連線: %w", err)
	}
	return client, nil
}

func cleanupEmployeeInventoryKeys(t *testing.T, redisClient *redis.Client, ticketTypeID string) {
	t.Helper()

	ctx := context.Background()
	keys := []string{
		"inventory:" + ticketTypeID,
		"inventory_loaded:" + ticketTypeID,
		"lock:init_lock:" + ticketTypeID,
	}
	redisClient.Del(ctx, keys...)
	t.Cleanup(func() {
		redisClient.Del(context.Background(), keys...)
	})
}
