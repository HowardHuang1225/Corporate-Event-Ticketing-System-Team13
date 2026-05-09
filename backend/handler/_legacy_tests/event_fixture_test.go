package handler

import (
	"fmt"
	"testing"
	"time"

	"ticketing-system/backend/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func seedEventsForStates(db *gorm.DB, manager model.User, titlePrefix string, states []string) error {
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
			return fmt.Errorf("failed to seed %s event: %w", state, err)
		}
	}
	return nil
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

func eventListContainsTitle(events []model.Event, title string) bool {
	for _, event := range events {
		if event.Title == title {
			return true
		}
	}
	return false
}

func eventHasTicketType(event model.Event, ticketTypeName string) bool {
	for _, ticketType := range event.TicketTypes {
		if ticketType.Name == ticketTypeName {
			return true
		}
	}
	return false
}

func eventPublishTimeColumnExists(db *gorm.DB) (bool, error) {
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
		return false, fmt.Errorf("failed to check publish_time column: %w", err)
	}
	return exists, nil
}

func insertScheduledEventFixture(
	t *testing.T,
	db *gorm.DB,
	manager model.User,
	publishTime time.Time,
	applyDeadline time.Time,
	startTime time.Time,
	endTime time.Time,
) (uuid.UUID, error) {
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
		return uuid.Nil, fmt.Errorf("failed to seed scheduled event: %w", err)
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM events WHERE id = ?", eventID.String())
	})

	return eventID, nil
}

func seedEventListFilterFixture(
	db *gorm.DB,
	manager model.User,
	title string,
	status string,
	ticketTypeName string,
	startTime time.Time,
) (model.Event, error) {
	event := model.Event{
		Title:               title,
		Description:         "Event list filter fixture",
		Venue:               "Main Hall",
		StartTime:           startTime,
		ApplyDeadline:       startTime.Add(12 * time.Hour),
		EndTime:             startTime.Add(24 * time.Hour),
		Status:              status,
		MaxTicketsPerPerson: 1,
		CreatedBy:           manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, fmt.Errorf("failed to seed event list filter fixture %q: %w", title, err)
	}

	ticketType := model.TicketType{
		EventID:    event.ID,
		Name:       ticketTypeName,
		TotalQuota: 100,
		Remaining:  100,
	}
	if err := db.Create(&ticketType).Error; err != nil {
		return model.Event{}, fmt.Errorf("failed to seed event list filter ticket type %q: %w", ticketTypeName, err)
	}

	if err := db.Preload("TicketTypes").First(&event, "id = ?", event.ID).Error; err != nil {
		return model.Event{}, fmt.Errorf("failed to reload event list filter fixture %q: %w", title, err)
	}
	return event, nil
}

func assertEventStatusFromDB(db *gorm.DB, eventID string, want string) error {
	var got string
	if err := db.Raw("SELECT status FROM events WHERE id = ?", eventID).Scan(&got).Error; err != nil {
		return fmt.Errorf("failed to query event %s: %w", eventID, err)
	}
	if got != want {
		return fmt.Errorf("expected event status %q, got %q", want, got)
	}
	return nil
}

func seedManualActionEvent(db *gorm.DB, manager model.User, status string) (model.Event, error) {
	now := time.Now().UTC().Truncate(time.Second)
	region := "台南廠"
	event := model.Event{
		Title:               fmt.Sprintf("Unit Test Manual Event %d", time.Now().UnixNano()),
		Description:         "Manual event action fixture",
		Venue:               "Main Hall",
		ImageURL:            "https://example.com/manual-event.png",
		StartTime:           now.Add(72 * time.Hour),
		ApplyDeadline:       now.Add(96 * time.Hour),
		EndTime:             now.Add(120 * time.Hour),
		Status:              status,
		RegionRestriction:   &region,
		MaxTicketsPerPerson: 3,
		CreatedBy:           manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, fmt.Errorf("failed to seed manual action event: %w", err)
	}

	ticketTypes := []model.TicketType{
		{
			EventID:    event.ID,
			Name:       "一般票",
			TotalQuota: 80,
			Remaining:  65,
		},
		{
			EventID:    event.ID,
			Name:       "眷屬票",
			TotalQuota: 20,
			Remaining:  12,
		},
	}
	for _, ticketType := range ticketTypes {
		if err := db.Create(&ticketType).Error; err != nil {
			return model.Event{}, fmt.Errorf("failed to seed ticket type %q: %w", ticketType.Name, err)
		}
	}

	if err := db.Preload("TicketTypes").First(&event, "id = ?", event.ID).Error; err != nil {
		return model.Event{}, fmt.Errorf("failed to reload manual action event: %w", err)
	}
	return event, nil
}

func assertClonedEventMatchesSource(source model.Event, cloned model.Event) error {
	if cloned.Title != source.Title {
		return fmt.Errorf("expected title %q, got %q", source.Title, cloned.Title)
	}
	if cloned.Description != source.Description {
		return fmt.Errorf("expected description %q, got %q", source.Description, cloned.Description)
	}
	if cloned.Venue != source.Venue {
		return fmt.Errorf("expected venue %q, got %q", source.Venue, cloned.Venue)
	}
	if cloned.ImageURL != source.ImageURL {
		return fmt.Errorf("expected image_url %q, got %q", source.ImageURL, cloned.ImageURL)
	}
	if !cloned.StartTime.Equal(source.StartTime) {
		return fmt.Errorf("expected start_time %s, got %s", source.StartTime, cloned.StartTime)
	}
	if !cloned.ApplyDeadline.Equal(source.ApplyDeadline) {
		return fmt.Errorf("expected apply_deadline %s, got %s", source.ApplyDeadline, cloned.ApplyDeadline)
	}
	if !cloned.EndTime.Equal(source.EndTime) {
		return fmt.Errorf("expected end_time %s, got %s", source.EndTime, cloned.EndTime)
	}
	if cloned.MaxTicketsPerPerson != source.MaxTicketsPerPerson {
		return fmt.Errorf("expected max_tickets_per_person %d, got %d", source.MaxTicketsPerPerson, cloned.MaxTicketsPerPerson)
	}
	if !sameOptionalString(cloned.RegionRestriction, source.RegionRestriction) {
		return fmt.Errorf("expected region_restriction %v, got %v", optionalStringValue(source.RegionRestriction), optionalStringValue(cloned.RegionRestriction))
	}
	if len(cloned.TicketTypes) != len(source.TicketTypes) {
		return fmt.Errorf("expected %d cloned ticket types, got %d", len(source.TicketTypes), len(cloned.TicketTypes))
	}

	sourceTicketTypes := make(map[string]model.TicketType, len(source.TicketTypes))
	for _, ticketType := range source.TicketTypes {
		sourceTicketTypes[ticketType.Name] = ticketType
	}
	for _, clonedTicketType := range cloned.TicketTypes {
		sourceTicketType, exists := sourceTicketTypes[clonedTicketType.Name]
		if !exists {
			return fmt.Errorf("unexpected cloned ticket type %q", clonedTicketType.Name)
		}
		if clonedTicketType.ID == sourceTicketType.ID {
			return fmt.Errorf("expected cloned ticket type %q to have a new id", clonedTicketType.Name)
		}
		if clonedTicketType.EventID != cloned.ID {
			return fmt.Errorf("expected cloned ticket type %q event_id %q, got %q", clonedTicketType.Name, cloned.ID, clonedTicketType.EventID)
		}
		if clonedTicketType.TotalQuota != sourceTicketType.TotalQuota {
			return fmt.Errorf("expected cloned ticket type %q total_quota %d, got %d", clonedTicketType.Name, sourceTicketType.TotalQuota, clonedTicketType.TotalQuota)
		}
		if clonedTicketType.Remaining != sourceTicketType.TotalQuota {
			return fmt.Errorf("expected cloned ticket type %q remaining quota to reset to %d, got %d", clonedTicketType.Name, sourceTicketType.TotalQuota, clonedTicketType.Remaining)
		}
	}
	return nil
}

func sameOptionalString(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func optionalStringValue(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}
