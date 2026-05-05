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
