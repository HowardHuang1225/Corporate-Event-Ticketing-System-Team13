package scheduler

import (
	"log"
	"time"

	"ticketing-system/backend/model"

	"gorm.io/gorm"
)

// StartPublishingWorker starts a background goroutine that checks for events
// in 'draft' status whose publish_time has passed and updates them to 'published'.
func StartPublishingWorker(db *gorm.DB) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		log.Println("Publishing worker started")
		for range ticker.C {
			checkAndPublish(db)
		}
	}()
}

func checkAndPublish(db *gorm.DB) {
	now := time.Now()
	
	// Find drafts that should be published
	var events []model.Event
	err := db.Where("status = ? AND publish_time <= ?", "draft", now).Find(&events).Error
	if err != nil {
		log.Printf("Scheduler error: failed to query drafts: %v", err)
		return
	}

	if len(events) == 0 {
		return
	}

	log.Printf("Scheduler: found %d events to publish", len(events))

	for _, event := range events {
		err := db.Model(&event).Update("status", "published").Error
		if err != nil {
			log.Printf("Scheduler error: failed to publish event %s: %v", event.ID, err)
			continue
		}
		log.Printf("Scheduler: event %s (%s) published successfully", event.ID, event.Title)
	}
}
