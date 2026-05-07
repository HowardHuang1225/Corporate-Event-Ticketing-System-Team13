package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ticketing-system/backend/model"

	"github.com/gin-gonic/gin"
)

var specEventStates = []string{"draft", "published", "closed", "ended"}

func TestEvent(t *testing.T) {
	tasks := []testTask{
		{
			description: "測試創建合法活動",
			target:      CreateValidEvent,
		},
		{
			description: "測試建立不合法活動時是否回傳正確錯誤訊息",
			target:      CreateInvalidEvent,
		},
		{
			description: "測試列表端點是否能夠正確過濾並回傳草稿、已發布、已截止和已結束的活動",
			target:      ListAllValidStates,
		},
		{
			description: "測試查看活動時是否能透過狀態、票種和活動時間篩選出對應活動",
			target:      FilterEventsByStatusTicketTypeAndTime,
		},
		{
			description: "測試活動是否會根據時間表自動變更狀態，從草稿 -> 已發布 -> 已截止 -> 已結束",
			target:      EventStateTransitionsBySchedule,
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

	mainTestFunc(t, tasks)
}

func CreateValidEvent(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試創建合法活動\n")
	printTestProgress("==================================================\n")
	// printTestProgress("Test if creating a valid event with all required fields succeeds and returns the correct response.\n")

	db, err := openEventTestDB(t)
	if err != nil {
		errs.Add("open event test database", "%v", err)
		return
	}
	tx := db.Begin()
	if tx.Error != nil {
		errs.Add("begin transaction", "failed to begin event test transaction: %v", tx.Error)
		return
	}

	// Keep the Docker Postgres database clean after the test run.
	t.Cleanup(func() {
		tx.Rollback()
	})

	manager, err := seedEventTestManager(tx)
	if err != nil {
		errs.Add("seed event test manager", "%v", err)
		return
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.POST("/events", NewEventHandler(tx, nil).CreateEvent)

	now := time.Now().UTC().Truncate(time.Second)
	resp := performEventJSON(router, http.MethodPost, "/events", validCreateEventPayload(now))

	if resp.Code != http.StatusCreated {
		errs.Add("finish creating event", "expected status 201, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeEventResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("finish decoding response", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("finish decoding response", "expected success=true")
		return
	}
	if body.Data.Title != "Unit Test Company Day" {
		errs.Add("finish decoding response", "expected created event title, got %q", body.Data.Title)
		return
	}
	if body.Data.Status != "draft" {
		errs.Add("finish decoding response", "expected new event status draft, got %q", body.Data.Status)
		return
	}
	if body.Data.CreatedBy != manager.ID {
		errs.Add("finish decoding response", "expected creator %q, got %q", manager.ID, body.Data.CreatedBy)
		return
	}
	if body.Data.MaxTicketsPerPerson != 2 {
		errs.Add("finish decoding response", "expected max tickets per person 2, got %d", body.Data.MaxTicketsPerPerson)
		return
	}
	if len(body.Data.TicketTypes) != 2 {
		errs.Add("finish decoding response", "expected 2 ticket types, got %d", len(body.Data.TicketTypes))
		return
	}
	for _, ticketType := range body.Data.TicketTypes {
		if ticketType.Remaining != ticketType.TotalQuota {
			errs.Add("finish decoding response", "expected remaining quota to match total quota for %q", ticketType.Name)
			return
		}
	}
	printTestProgress("==================================================\n\n")

}

func CreateInvalidEvent(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試建立不合法活動時是否回傳正確錯誤訊息\n")
	printTestProgress("==================================================\n")

	db, err := openEventTestDB(t)
	if err != nil {
		errs.Add("open event test database", "%v", err)
		return
	}
	tx := db.Begin()
	if tx.Error != nil {
		errs.Add("begin transaction", "failed to begin event test transaction: %v", tx.Error)
		return
	}
	t.Cleanup(func() {
		tx.Rollback()
	})

	manager, err := seedEventTestManager(tx)
	if err != nil {
		errs.Add("seed event test manager", "%v", err)
		return
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.POST("/events", NewEventHandler(tx, nil).CreateEvent)

	now := time.Now().UTC().Truncate(time.Second)
	tests := []invalidEventTest{}
	tests = append(tests, CreateTimeInvalidEvent(t, errs, now)...)
	tests = append(tests, CreateTicketInvalidEvent(t, errs)...)
	tests = append(tests, CreateValueInvalidEvent(t, errs)...)
	tests = append(tests, CreateStatusInvalidEvent(t, errs)...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// printTestProgress(tt.progress + "\n")
			payload := validCreateEventPayload(now)
			tt.mutate(payload)

			resp := performEventJSON(router, http.MethodPost, "/events", payload)
			if resp.Code != http.StatusBadRequest {
				errs.Add(tt.progress, "expected status 400, got %d with body %s", resp.Code, resp.Body.String())
				return
			}
			if err := assertHandlerErrorResponse(resp.Body.Bytes(), "VALIDATION_ERROR", tt.wantMessageContains); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}
	printTestProgress("==================================================\n\n")
}

func ListAllValidStates(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試列表端點是否能夠正確過濾並回傳草稿、已發布、已截止和已結束的活動\n")
	printTestProgress("==================================================\n")

	db, err := openEventTestDB(t)
	if err != nil {
		errs.Add("open event test database", "%v", err)
		return
	}
	tx := db.Begin()
	if tx.Error != nil {
		errs.Add("begin transaction", "failed to begin event test transaction: %v", tx.Error)
		return
	}
	// Roll back seeded state fixtures after the test.
	t.Cleanup(func() {
		tx.Rollback()
	})

	manager, err := seedEventTestManager(tx)
	if err != nil {
		errs.Add("seed event test manager", "%v", err)
		return
	}
	titlePrefix := fmt.Sprintf("Unit Test State %d", time.Now().UnixNano())
	if err := seedEventsForStates(tx, manager, titlePrefix, specEventStates); err != nil {
		errs.Add("seed event states", "%v", err)
		return
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("role", "event_manager")
		c.Next()
	})
	router.GET("/events", NewEventHandler(tx, nil).ListEvents)

	for _, state := range specEventStates {
		t.Run(state, func(t *testing.T) {
			// printTestProgressf("Test if the list endpoint can filter and return events with status %q.\n", state)

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/events?status="+state, nil)
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				errs.Add(fmt.Sprintf("finish checking event list with state %q", state), "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
				return
			}

			body, err := decodeEventListResponse(resp.Body.Bytes())
			if err != nil {
				errs.Add(fmt.Sprintf("finish decoding event list with state %q", state), "%v", err)
				return
			}
			if !body.Success {
				errs.Add(fmt.Sprintf("finish decoding event list with state %q", state), "expected success=true")
				return
			}
			if !eventListContainsStateFixture(body.Data, titlePrefix, state) {
				errs.Add(fmt.Sprintf("finish checking event list with state %q", state), "expected list response to include seeded %q event", state)
				return
			}
			for _, event := range body.Data {
				if strings.HasPrefix(event.Title, titlePrefix) && event.Status != state {
					errs.Add(fmt.Sprintf("finish checking event list with state %q", state), "expected fixture %q to have status %q, got %q", event.Title, state, event.Status)
					return
				}
			}
		})
	}
	printTestProgress("==================================================\n\n")
}

func FilterEventsByStatusTicketTypeAndTime(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試查看活動時是否能透過狀態、票種和活動時間篩選出對應活動\n")
	printTestProgress("==================================================\n")

	db, err := openEventTestDB(t)
	if err != nil {
		errs.Add("open event test database", "%v", err)
		return
	}
	tx := db.Begin()
	if tx.Error != nil {
		errs.Add("begin transaction", "failed to begin event test transaction: %v", tx.Error)
		return
	}
	t.Cleanup(func() {
		tx.Rollback()
	})

	manager, err := seedEventTestManager(tx)
	if err != nil {
		errs.Add("seed event test manager", "%v", err)
		return
	}

	now := time.Now().UTC().Truncate(time.Second)
	titlePrefix := fmt.Sprintf("Unit Test Event Filter %d", time.Now().UnixNano())
	target, err := seedEventListFilterFixture(tx, manager, titlePrefix+" target", "published", "VIP票", now.Add(72*time.Hour))
	if err != nil {
		errs.Add("seed matching event filter fixture", "%v", err)
		return
	}
	if _, err := seedEventListFilterFixture(tx, manager, titlePrefix+" wrong status", "draft", "VIP票", now.Add(72*time.Hour)); err != nil {
		errs.Add("seed status mismatch event filter fixture", "%v", err)
		return
	}
	if _, err := seedEventListFilterFixture(tx, manager, titlePrefix+" wrong ticket type", "published", "一般票", now.Add(72*time.Hour)); err != nil {
		errs.Add("seed ticket type mismatch event filter fixture", "%v", err)
		return
	}
	if _, err := seedEventListFilterFixture(tx, manager, titlePrefix+" wrong time", "published", "VIP票", now.Add(240*time.Hour)); err != nil {
		errs.Add("seed time mismatch event filter fixture", "%v", err)
		return
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("role", "event_manager")
		c.Next()
	})
	router.GET("/events", NewEventHandler(tx, nil).ListEvents)

	startFrom := now.Add(48 * time.Hour)
	startTo := now.Add(96 * time.Hour)
	query := url.Values{}
	query.Set("status", "published")
	query.Set("ticket_type", "VIP票")
	query.Set("start_from", startFrom.Format(time.RFC3339))
	query.Set("start_to", startTo.Format(time.RFC3339))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events?"+query.Encode(), nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		errs.Add("finish filtering event list", "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeEventListResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("finish decoding filtered event list", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("finish decoding filtered event list", "expected success=true")
		return
	}
	if !eventListContainsTitle(body.Data, target.Title) {
		errs.Add("finish checking filtered event list", "expected list response to include matching event %q", target.Title)
		return
	}
	for _, event := range body.Data {
		if !strings.HasPrefix(event.Title, titlePrefix) {
			continue
		}
		if event.Title != target.Title {
			errs.Add("finish checking filtered event list", "expected fixture %q to be filtered out", event.Title)
			return
		}
		if event.Status != "published" {
			errs.Add("finish checking filtered event list", "expected matching event status %q, got %q", "published", event.Status)
			return
		}
		if !eventHasTicketType(event, "VIP票") {
			errs.Add("finish checking filtered event list", "expected matching event to include ticket type %q", "VIP票")
			return
		}
		if event.StartTime.Before(startFrom) || event.StartTime.After(startTo) {
			errs.Add("finish checking filtered event list", "expected matching event start_time between %s and %s, got %s", startFrom, startTo, event.StartTime)
			return
		}
	}

	printTestProgress("==================================================\n\n")
}

func EventStateTransitionsBySchedule(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := openEventTestDB(t)
	if err != nil {
		errs.Add("open event test database", "%v", err)
		return
	}

	publishTimeExists, err := eventPublishTimeColumnExists(db)
	if err != nil {
		errs.Add("check publish_time column", "%v", err)
		return
	}
	if !publishTimeExists {
		errs.Add("check publish_time column", "events.publish_time is not in the current schema yet")
		return
	}

	manager, err := seedEventTestManager(db)
	if err != nil {
		errs.Add("seed event test manager", "%v", err)
		return
	}
	now := time.Now().UTC()
	publishTime := now.Add(4 * time.Second)
	applyDeadline := now.Add(9 * time.Second)
	startTime := now.Add(12 * time.Second)
	endTime := now.Add(14 * time.Second)

	eventID, err := insertScheduledEventFixture(t, db, manager, publishTime, applyDeadline, startTime, endTime)
	if err != nil {
		errs.Add("seed scheduled event", "%v", err)
		return
	}
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
		// printTestProgressf("Waiting for the event to transition to %q status according to the schedule...\n", check.wantStatus)
		time.Sleep(5 * time.Second)
		if err := assertEventStatusFromDB(db, eventID.String(), check.wantStatus); err != nil {
			errs.Add(fmt.Sprintf("wait for event to transition to %q", check.wantStatus), "%v", err)
		}
	}
	printTestProgress("==================================================\n\n")
}

func PublishDraftEventManually(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試管理員是否能手動將活動狀態從草稿改成已發布\n")
	printTestProgress("==================================================\n")

	db, err := openEventTestDB(t)
	if err != nil {
		errs.Add("open event test database", "%v", err)
		return
	}
	tx := db.Begin()
	if tx.Error != nil {
		errs.Add("begin transaction", "failed to begin event test transaction: %v", tx.Error)
		return
	}
	t.Cleanup(func() {
		tx.Rollback()
	})

	manager, err := seedEventTestManager(tx)
	if err != nil {
		errs.Add("seed event test manager", "%v", err)
		return
	}
	event, err := seedManualActionEvent(tx, manager, "draft")
	if err != nil {
		errs.Add("seed draft event", "%v", err)
		return
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.PATCH("/events/:id/publish", NewEventHandler(tx, nil).PublishEvent)

	resp := performEventJSON(router, http.MethodPatch, "/events/"+event.ID.String()+"/publish", gin.H{})
	if resp.Code != http.StatusOK {
		errs.Add("finish publishing draft event", "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeEventResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("finish decoding publish response", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("finish decoding publish response", "expected success=true")
		return
	}
	if body.Data.ID != event.ID {
		errs.Add("finish checking published event response", "expected event id %q, got %q", event.ID, body.Data.ID)
		return
	}
	if body.Data.Status != "published" {
		errs.Add("finish checking published event response", "expected status %q, got %q", "published", body.Data.Status)
		return
	}
	if err := assertEventStatusFromDB(tx, event.ID.String(), "published"); err != nil {
		errs.Add("finish checking published event in database", "%v", err)
		return
	}

	printTestProgress("==================================================\n\n")
}

func CloseRegistrationEarly(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試管理員是否能手動提早截止報名\n")
	printTestProgress("==================================================\n")

	db, err := openEventTestDB(t)
	if err != nil {
		errs.Add("open event test database", "%v", err)
		return
	}
	tx := db.Begin()
	if tx.Error != nil {
		errs.Add("begin transaction", "failed to begin event test transaction: %v", tx.Error)
		return
	}
	t.Cleanup(func() {
		tx.Rollback()
	})

	manager, err := seedEventTestManager(tx)
	if err != nil {
		errs.Add("seed event test manager", "%v", err)
		return
	}
	event, err := seedManualActionEvent(tx, manager, "published")
	if err != nil {
		errs.Add("seed published event", "%v", err)
		return
	}
	if !time.Now().UTC().Before(event.ApplyDeadline) {
		errs.Add("prepare published event", "expected seeded event apply_deadline to still be in the future")
		return
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.PATCH("/events/:id/close", NewEventHandler(tx, nil).CloseEvent)

	resp := performEventJSON(router, http.MethodPatch, "/events/"+event.ID.String()+"/close", gin.H{})
	if resp.Code != http.StatusOK {
		errs.Add("finish closing published event early", "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeEventResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("finish decoding close response", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("finish decoding close response", "expected success=true")
		return
	}
	if body.Data.ID != event.ID {
		errs.Add("finish checking closed event response", "expected event id %q, got %q", event.ID, body.Data.ID)
		return
	}
	if body.Data.Status != "closed" {
		errs.Add("finish checking closed event response", "expected status %q, got %q", "closed", body.Data.Status)
		return
	}
	if err := assertEventStatusFromDB(tx, event.ID.String(), "closed"); err != nil {
		errs.Add("finish checking closed event in database", "%v", err)
		return
	}

	printTestProgress("==================================================\n\n")
}

// func CloneExistingEventForEditing(t *testing.T, errs *testErrors) {
// 	t.Helper()
// 	gin.SetMode(gin.TestMode)

// 	printTestProgress("測試管理員是否能透過 POST /events/:id/clone 複製既有活動資訊做修改\n")
// 	printTestProgress("==================================================\n")

// 	db, err := openEventTestDB(t)
// 	if err != nil {
// 		errs.Add("open event test database", "%v", err)
// 		return
// 	}
// 	tx := db.Begin()
// 	if tx.Error != nil {
// 		errs.Add("begin transaction", "failed to begin event test transaction: %v", tx.Error)
// 		return
// 	}
// 	t.Cleanup(func() {
// 		tx.Rollback()
// 	})

// 	manager, err := seedEventTestManager(tx)
// 	if err != nil {
// 		errs.Add("seed event test manager", "%v", err)
// 		return
// 	}
// 	source, err := seedManualActionEvent(tx, manager, "published")
// 	if err != nil {
// 		errs.Add("seed source event", "%v", err)
// 		return
// 	}

// 	router := gin.New()
// 	router.Use(func(c *gin.Context) {
// 		c.Set("user_id", manager.ID.String())
// 		c.Set("role", "event_manager")
// 		c.Next()
// 	})
// 	router.POST("/events/:id/clone", NewEventHandler(tx, nil).CloneEvent)

// 	resp := performEventJSON(router, http.MethodPost, "/events/"+source.ID.String()+"/clone", gin.H{})
// 	if resp.Code != http.StatusCreated {
// 		errs.Add("finish cloning event", "expected status 201, got %d with body %s", resp.Code, resp.Body.String())
// 		return
// 	}

// 	body, err := decodeEventResponse(resp.Body.Bytes())
// 	if err != nil {
// 		errs.Add("finish decoding clone response", "%v", err)
// 		return
// 	}
// 	if !body.Success {
// 		errs.Add("finish decoding clone response", "expected success=true")
// 		return
// 	}

// 	cloned := body.Data
// 	if cloned.ID == source.ID {
// 		errs.Add("finish checking cloned event response", "expected cloned event to have a new id, got original id %q", source.ID)
// 		return
// 	}
// 	if cloned.Status != "draft" {
// 		errs.Add("finish checking cloned event response", "expected cloned event status %q, got %q", "draft", cloned.Status)
// 		return
// 	}
// 	if err := assertClonedEventMatchesSource(source, cloned); err != nil {
// 		errs.Add("finish checking cloned event response", "%v", err)
// 		return
// 	}

// 	var persisted model.Event
// 	if err := tx.Preload("TicketTypes").First(&persisted, "id = ?", cloned.ID).Error; err != nil {
// 		errs.Add("finish loading cloned event from database", "expected cloned event to be persisted: %v", err)
// 		return
// 	}
// 	if persisted.Status != "draft" {
// 		errs.Add("finish checking cloned event in database", "expected persisted cloned event status %q, got %q", "draft", persisted.Status)
// 		return
// 	}
// 	if len(persisted.TicketTypes) != len(source.TicketTypes) {
// 		errs.Add("finish checking cloned event in database", "expected %d cloned ticket types, got %d", len(source.TicketTypes), len(persisted.TicketTypes))
// 		return
// 	}
// 	if err := assertEventStatusFromDB(tx, source.ID.String(), "published"); err != nil {
// 		errs.Add("finish checking source event remains unchanged", "%v", err)
// 		return
// 	}

// 	printTestProgress("==================================================\n\n")
// }
