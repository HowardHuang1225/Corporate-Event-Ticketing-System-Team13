package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/routes"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var integrationTestModels = []any{
	&model.User{},
	&model.Event{},
	&model.TicketType{},
	&model.Application{},
	&model.Ticket{},
	&model.Checkin{},
}

type integrationContext struct {
	DB        *gorm.DB
	Redis     *redis.Client
	Router    *gin.Engine
	Users     integrationUsers
	JWTSecret string
}

type integrationUsers struct {
	Manager       model.User
	Employee      model.User
	OtherEmployee model.User
	HR            model.User
}

type integrationLoginData struct {
	AccessToken string `json:"access_token"`
	User        struct {
		ID         string `json:"id"`
		EmployeeID string `json:"employee_id"`
		Role       string `json:"role"`
	} `json:"user"`
}

type integrationOKResponse[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
}

func setupIntegrationTest(t *testing.T) (*integrationContext, error) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	t.Setenv("TICKET_QUEUE_ENABLED", "false")

	tx, cleanup, err := utils.BeginTestTransaction(t, integrationTestModels)
	if err != nil {
		return nil, err
	}
	t.Cleanup(cleanup)

	redisClient, err := openIntegrationTestRedis(t)
	if err != nil {
		return nil, err
	}

	users, err := seedIntegrationUsers(tx)
	if err != nil {
		return nil, err
	}

	secret := "integration-test-secret-" + utils.UniqueTestSuffix()
	router := gin.New()
	routes.Register(router, routes.Dependencies{
		DB:        tx,
		Redis:     redisClient,
		JWTSecret: secret,
	})

	return &integrationContext{
		DB:        tx,
		Redis:     redisClient,
		Router:    router,
		Users:     users,
		JWTSecret: secret,
	}, nil
}

func seedIntegrationUsers(db *gorm.DB) (integrationUsers, error) {
	suffix := utils.UniqueTestSuffix()
	users, err := utils.SeedTestRole(db, []model.User{
		{
			EmployeeID: fmt.Sprintf("ITMGR%s", suffix),
			Name:       "整合測試活動管理者",
			Email:      fmt.Sprintf("integration-manager-%s@example.com", suffix),
			Department: "Events",
			Region:     "Taipei",
			Role:       "event_manager",
			IsActive:   true,
		},
		{
			EmployeeID: fmt.Sprintf("ITEMP%s", suffix),
			Name:       "整合測試員工",
			Email:      fmt.Sprintf("integration-employee-%s@example.com", suffix),
			Department: "Engineering",
			Region:     "Taipei",
			Role:       "employee",
			IsActive:   true,
		},
		{
			EmployeeID: fmt.Sprintf("ITOTH%s", suffix),
			Name:       "整合測試其他員工",
			Email:      fmt.Sprintf("integration-other-%s@example.com", suffix),
			Department: "Sales",
			Region:     "Tainan",
			Role:       "employee",
			IsActive:   true,
		},
		{
			EmployeeID: fmt.Sprintf("ITHR%s", suffix),
			Name:       "整合測試 HR",
			Email:      fmt.Sprintf("integration-hr-%s@example.com", suffix),
			Department: "People",
			Region:     "Taipei",
			Role:       "hr",
			IsActive:   true,
		},
	}, true)
	if err != nil {
		return integrationUsers{}, err
	}

	return integrationUsers{
		Manager:       users[0],
		Employee:      users[1],
		OtherEmployee: users[2],
		HR:            users[3],
	}, nil
}

func openIntegrationTestRedis(t *testing.T) (*redis.Client, error) {
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
		return nil, fmt.Errorf("Redis 測試服務不可用: %w", err)
	}
	return client, nil
}

func seedIntegrationEventWithTicketTypes(
	db *gorm.DB,
	manager model.User,
	status string,
	maxTicketsPerPerson int,
	quotas ...int,
) (model.Event, []model.TicketType, error) {
	now := time.Now().UTC().Truncate(time.Second)
	event := model.Event{
		Title:               fmt.Sprintf("整合測試活動 %s", utils.UniqueTestSuffix()),
		Description:         "整合測試使用真實路由與服務層建立的活動資料",
		Venue:               "整合測試會議室",
		PublishTime:         now.Add(-2 * time.Hour),
		StartTime:           now.Add(24 * time.Hour),
		ApplyDeadline:       now.Add(36 * time.Hour),
		EndTime:             now.Add(48 * time.Hour),
		Status:              status,
		MaxTicketsPerPerson: maxTicketsPerPerson,
		CreatedBy:           manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, nil, fmt.Errorf("建立整合測試活動失敗: %w", err)
	}

	names := []string{"一般票", "VIP票", "候補票"}
	ticketTypes := make([]model.TicketType, 0, len(quotas))
	for index, quota := range quotas {
		name := fmt.Sprintf("票種%d", index+1)
		if index < len(names) {
			name = names[index]
		}
		ticketType := model.TicketType{
			EventID:    event.ID,
			Name:       name,
			TotalQuota: quota,
			Remaining:  quota,
		}
		if err := db.Create(&ticketType).Error; err != nil {
			return model.Event{}, nil, fmt.Errorf("建立整合測試票種失敗: %w", err)
		}
		ticketTypes = append(ticketTypes, ticketType)
	}

	if err := db.Preload("TicketTypes").First(&event, "id = ?", event.ID).Error; err != nil {
		return model.Event{}, nil, fmt.Errorf("讀回整合測試活動失敗: %w", err)
	}
	return event, ticketTypes, nil
}

func cleanupIntegrationRedisKeys(t *testing.T, redisClient *redis.Client, ticketTypeIDs ...string) {
	t.Helper()
	if redisClient == nil {
		return
	}

	keys := make([]string, 0, len(ticketTypeIDs)*4)
	for _, ticketTypeID := range ticketTypeIDs {
		keys = append(keys,
			"inventory:"+ticketTypeID,
			"inventory_loaded:"+ticketTypeID,
			"init_lock:"+ticketTypeID,
			"lock:init_lock:"+ticketTypeID,
		)
	}
	if len(keys) == 0 {
		return
	}

	ctx := context.Background()
	redisClient.Del(ctx, keys...)
	t.Cleanup(func() {
		redisClient.Del(context.Background(), keys...)
	})
}

func loginIntegrationUser(router *gin.Engine, employeeID string) (string, error) {
	resp := performIntegrationJSON(router, http.MethodPost, "/v1/auth/login", "", gin.H{
		"employee_id": employeeID,
		"password":    "password",
	})
	if resp.Code != http.StatusOK {
		return "", fmt.Errorf("登入 %s 預期狀態 200，實際 %d，body=%s", employeeID, resp.Code, resp.Body.String())
	}

	body, err := decodeIntegrationOK[integrationLoginData](resp.Body.Bytes())
	if err != nil {
		return "", err
	}
	if body.Data.AccessToken == "" {
		return "", fmt.Errorf("登入 %s 後沒有取得 access token", employeeID)
	}
	return body.Data.AccessToken, nil
}

func performIntegrationJSON(router *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func performIntegrationRequest(router *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func decodeIntegrationOK[T any](body []byte) (integrationOKResponse[T], error) {
	var resp integrationOKResponse[T]
	if err := json.Unmarshal(body, &resp); err != nil {
		return resp, fmt.Errorf("解析成功回應失敗: %w", err)
	}
	if !resp.Success {
		return resp, fmt.Errorf("預期 success=true，實際 body=%s", string(body))
	}
	return resp, nil
}

func integrationRedisInt(redisClient *redis.Client, key string) (int64, error) {
	value, err := redisClient.Get(context.Background(), key).Int64()
	if err != nil {
		return 0, fmt.Errorf("讀取 Redis key %s 失敗: %w", key, err)
	}
	return value, nil
}

func logIntegrationStep(t *testing.T, message string) {
	t.Helper()
	t.Log(message)
	fmt.Println("整合測試：" + message)
}
