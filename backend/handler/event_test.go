package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"ticketing-system/backend/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var specEventStates = []string{"draft", "published", "closed", "ended"}

// TestCreateEventSuccess verifies EVT-01 from the spec: a manager can create
// a new event with ticket types, and new events are saved as draft by default.
func TestCreateEventSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := openEventTestDB(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("failed to begin event test transaction: %v", tx.Error)
	}
	// Keep the Docker Postgres database clean after the test run.
	t.Cleanup(func() {
		tx.Rollback()
	})

	manager := seedEventTestManager(t, tx)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.POST("/events", NewEventHandler(tx, nil).CreateEvent)

	now := time.Now().UTC().Truncate(time.Second)
	resp := performEventJSON(router, http.MethodPost, "/events", gin.H{
		"title":                  "Unit Test Company Day",
		"description":            "Created from event handler test",
		"venue":                  "Main Hall",
		"start_time":             now.Add(72 * time.Hour).Format(time.RFC3339),
		"end_time":               now.Add(76 * time.Hour).Format(time.RFC3339),
		"apply_deadline":         now.Add(48 * time.Hour).Format(time.RFC3339),
		"max_tickets_per_person": 2,
		"ticket_types": []gin.H{
			{"name": "一般票", "total_quota": 100},
			{"name": "眷屬票", "total_quota": 50},
		},
	})

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d with body %s", resp.Code, resp.Body.String())
	}

	body := decodeEventResponse(t, resp.Body.Bytes())
	if !body.Success {
		t.Fatal("expected success=true")
	}
	if body.Data.Title != "Unit Test Company Day" {
		t.Fatalf("expected created event title, got %q", body.Data.Title)
	}
	if body.Data.Status != "draft" {
		t.Fatalf("expected new event status draft, got %q", body.Data.Status)
	}
	if body.Data.CreatedBy != manager.ID {
		t.Fatalf("expected creator %q, got %q", manager.ID, body.Data.CreatedBy)
	}
	if body.Data.MaxTicketsPerPerson != 2 {
		t.Fatalf("expected max tickets per person 2, got %d", body.Data.MaxTicketsPerPerson)
	}
	if len(body.Data.TicketTypes) != 2 {
		t.Fatalf("expected 2 ticket types, got %d", len(body.Data.TicketTypes))
	}
	for _, ticketType := range body.Data.TicketTypes {
		if ticketType.Remaining != ticketType.TotalQuota {
			t.Fatalf("expected remaining quota to match total quota for %q", ticketType.Name)
		}
	}
}

// TestListEventsSupportsSpecStates documents the four event states in the spec:
// draft, published, closed, and ended. The current backend has no ended
// transition endpoint yet, so this test seeds ended directly and verifies the
// list filter can still represent every spec state.
func TestListEventsSupportsSpecStates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := openEventTestDB(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("failed to begin event test transaction: %v", tx.Error)
	}
	// Roll back seeded state fixtures after the test.
	t.Cleanup(func() {
		tx.Rollback()
	})

	manager := seedEventTestManager(t, tx)
	titlePrefix := fmt.Sprintf("Unit Test State %d", time.Now().UnixNano())
	seedEventsForStates(t, tx, manager, titlePrefix, specEventStates)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("role", "event_manager")
		c.Next()
	})
	router.GET("/events", NewEventHandler(tx, nil).ListEvents)

	printTestProgress("測試列表端點是否能夠正確過濾並回傳草稿、已發布、已截止和已結束的活動\n")
	printTestProgress("==================================================\n")
	for _, state := range specEventStates {
		t.Run(state, func(t *testing.T) {
			printTestProgressf("Test if the list endpoint can filter and return events with status %q.\n", state)

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/events?status="+state, nil)
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d with body %s", resp.Code, resp.Body.String())
			}

			body := decodeEventListResponse(t, resp.Body.Bytes())
			if !body.Success {
				t.Fatal("expected success=true")
			}
			if !eventListContainsStateFixture(body.Data, titlePrefix, state) {
				t.Fatalf("expected list response to include seeded %q event", state)
			}
			for _, event := range body.Data {
				if strings.HasPrefix(event.Title, titlePrefix) && event.Status != state {
					t.Fatalf("expected fixture %q to have status %q, got %q", event.Title, state, event.Status)
				}
			}
		})
	}
	printTestProgress("==================================================\n\n")
}

// TestEventStateTransitionsBySchedule verifies the time-driven state machine:
// publish_time moves draft -> published, apply_deadline moves published ->
// closed, and end_time moves closed -> ended. The test does not update status
// directly or start any worker itself; it only inserts the initial draft event
// and queries the DB every five seconds to observe backend-owned updates.
func TestEventStateTransitionsBySchedule(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := openEventTestDB(t)
	if !eventPublishTimeColumnExists(t, db) {
		t.Skip("events.publish_time is not in the current schema yet")
	}

	manager := seedEventTestManager(t, db)
	now := time.Now().UTC()
	publishTime := now.Add(4 * time.Second)
	applyDeadline := now.Add(9 * time.Second)
	startTime := now.Add(12 * time.Second)
	endTime := now.Add(14 * time.Second)

	eventID := insertScheduledEventFixture(t, db, manager, publishTime, applyDeadline, startTime, endTime)
	t.Cleanup(func() {
		db.Delete(&model.User{}, "id = ?", manager.ID)
	})

	checks := []struct {
		wantStatus string
	}{
		{wantStatus: "published"},
		{wantStatus: "closed"},
		{wantStatus: "ended"},
	}

	printTestProgress("測試活動是否會根據時間表自動從草稿轉為已發布、再轉為已截止、最後轉為已結束\n")
	printTestProgress("==================================================\n")
	for _, check := range checks {
		printTestProgressf("Waiting for the event to transition to %q status according to the schedule...\n", check.wantStatus)
		time.Sleep(5 * time.Second)
		assertEventStatusFromDB(t, db, eventID.String(), check.wantStatus)
	}
	printTestProgress("==================================================\n\n")
}

// openEventTestDB connects to local Docker Postgres for event handler tests.
// If Postgres is unavailable, these DB-backed tests are skipped.
func openEventTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(postgres.Open(eventTestDSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("Postgres is not available for event tests: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to access sql db: %v", err)
	}
	t.Cleanup(func() {
		sqlDB.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Skipf("Postgres is not reachable for event tests: %v", err)
	}

	if err := db.AutoMigrate(&model.User{}, &model.Event{}, &model.TicketType{}); err != nil {
		t.Fatalf("failed to migrate event test tables: %v", err)
	}

	return db
}

func eventTestDSN() string {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}

	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		eventEnvOrDefault("DB_HOST", "localhost"),
		eventEnvOrDefault("DB_USER", "ts_user"),
		eventEnvOrDefault("DB_PASSWORD", "ts_password"),
		eventEnvOrDefault("DB_NAME", "ticketing_system"),
		eventEnvOrDefault("DB_PORT", "5432"),
	)
}

func eventEnvOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func seedEventTestManager(t *testing.T, db *gorm.DB) model.User {
	t.Helper()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash manager password: %v", err)
	}

	manager := model.User{
		EmployeeID:   fmt.Sprintf("EVTMGR%d", time.Now().UnixNano()),
		Name:         "Event Test Manager",
		Email:        fmt.Sprintf("event-manager-%d@example.com", time.Now().UnixNano()),
		Department:   "Events",
		Region:       "台南廠",
		Role:         "event_manager",
		PasswordHash: string(passwordHash),
		IsActive:     true,
	}
	if err := db.Create(&manager).Error; err != nil {
		t.Fatalf("failed to seed event test manager: %v", err)
	}

	return manager
}

func seedEventsForStates(t *testing.T, db *gorm.DB, manager model.User, titlePrefix string, states []string) {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Second)
	for index, state := range states {
		event := model.Event{
			Title:               fmt.Sprintf("%s %s", titlePrefix, state),
			Description:         "State fixture for event list tests",
			Venue:               "Main Hall",
			StartTime:           now.Add(time.Duration(index+1) * 24 * time.Hour),
			EndTime:             now.Add(time.Duration(index+1)*24*time.Hour + 2*time.Hour),
			ApplyDeadline:       now.Add(time.Duration(index+1) * 12 * time.Hour),
			Status:              state,
			MaxTicketsPerPerson: 1,
			CreatedBy:           manager.ID,
		}
		if err := db.Create(&event).Error; err != nil {
			t.Fatalf("failed to seed %s event: %v", state, err)
		}
	}
}

func performEventJSON(router *gin.Engine, method, path string, body gin.H) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

type eventResponse struct {
	Success bool        `json:"success"`
	Data    model.Event `json:"data"`
}

func decodeEventResponse(t *testing.T, body []byte) eventResponse {
	t.Helper()

	var resp eventResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to decode event response: %v", err)
	}
	return resp
}

type eventListResponse struct {
	Success bool          `json:"success"`
	Data    []model.Event `json:"data"`
}

func decodeEventListResponse(t *testing.T, body []byte) eventListResponse {
	t.Helper()

	var resp eventListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to decode event list response: %v", err)
	}
	return resp
}

func eventListContainsStateFixture(events []model.Event, titlePrefix, state string) bool {
	wantTitle := fmt.Sprintf("%s %s", titlePrefix, state)
	for _, event := range events {
		if event.Title == wantTitle && event.Status == state {
			return true
		}
	}
	return false
}

func eventPublishTimeColumnExists(t *testing.T, db *gorm.DB) bool {
	t.Helper()

	var exists bool
	if err := db.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'events'
			  AND column_name = 'publish_time'
		)
	`).Scan(&exists).Error; err != nil {
		t.Fatalf("failed to check publish_time column: %v", err)
	}
	return exists
}

func insertScheduledEventFixture(
	t *testing.T,
	db *gorm.DB,
	manager model.User,
	publishTime time.Time,
	applyDeadline time.Time,
	startTime time.Time,
	endTime time.Time,
) uuid.UUID {
	t.Helper()

	eventID := uuid.New()
	if err := db.Exec(`
		INSERT INTO events (
			id,
			title,
			description,
			venue,
			publish_time,
			start_time,
			end_time,
			apply_deadline,
			status,
			max_tickets_per_person,
			created_by,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		eventID.String(),
		fmt.Sprintf("Unit Test Scheduled State %d", time.Now().UnixNano()),
		"Scheduled state transition fixture",
		"Main Hall",
		publishTime,
		startTime,
		endTime,
		applyDeadline,
		"draft",
		1,
		manager.ID.String(),
		time.Now().UTC(),
		time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("failed to seed scheduled event: %v", err)
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM events WHERE id = ?", eventID.String())
	})

	return eventID
}

func assertEventStatusFromDB(t *testing.T, db *gorm.DB, eventID string, want string) {
	t.Helper()

	var got string
	if err := db.Raw("SELECT status FROM events WHERE id = ?", eventID).Scan(&got).Error; err != nil {
		t.Fatalf("failed to query event %s: %v\n", eventID, err)
	}
	if got != want {
		t.Fatalf("expected event status %q, got %q\n", want, got)
	}
}
