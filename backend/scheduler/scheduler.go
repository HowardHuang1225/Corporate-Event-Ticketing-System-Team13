package scheduler

import (
	"context"
	"log"
	"time"

	"ticketing-system/backend/model"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	defaultEventSchedulerInterval = time.Minute
	eventSchedulerLockKey         = "lock:event_scheduler"
)

// StartEventSchedulers starts the unified event lifecycle scheduler.
func StartEventSchedulers(db *gorm.DB, rdb *redis.Client) {
	StartEventSchedulersWithInterval(db, rdb, defaultEventSchedulerInterval)
}

// StartEventSchedulersWithInterval starts the unified scheduler with a custom interval.
func StartEventSchedulersWithInterval(db *gorm.DB, rdb *redis.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		log.Println("Event scheduler started")
		runEventScheduler(db, rdb, interval)
		for range ticker.C {
			runEventScheduler(db, rdb, interval)
		}
	}()
}

// StartEventStateScheduler keeps the old API available while using the unified scheduler.
func StartEventStateScheduler(db *gorm.DB, rdb *redis.Client, interval time.Duration) {
	StartEventSchedulersWithInterval(db, rdb, interval)
}

// StartPublishingWorker keeps the old publishing-only API available.
func StartPublishingWorker(db *gorm.DB) {
	ticker := time.NewTicker(defaultEventSchedulerInterval)
	go func() {
		log.Println("Publishing worker started")
		checkAndPublish(db)
		for range ticker.C {
			checkAndPublish(db)
		}
	}()
}

func checkAndPublish(db *gorm.DB) {
	now := time.Now()

	updated, err := publishDueDrafts(db, now)
	if err != nil {
		log.Printf("Scheduler error: failed to publish due draft events: %v", err)
		return
	}

	if updated == 0 {
		return
	}

	log.Printf("Scheduler: published %d due draft events", updated)
}

func runEventScheduler(db *gorm.DB, rdb *redis.Client, interval time.Duration) {
	ctx := context.Background()
	if !acquireEventSchedulerLock(ctx, rdb, interval) {
		return
	}

	if err := updateEventStatuses(db, time.Now()); err != nil {
		log.Printf("Scheduler error: failed to update event statuses: %v", err)
	}
}

func acquireEventSchedulerLock(ctx context.Context, rdb *redis.Client, interval time.Duration) bool {
	if rdb == nil {
		return true
	}

	ok, err := rdb.SetNX(ctx, eventSchedulerLockKey, "1", schedulerLockTTL(interval)).Result()
	if err != nil {
		log.Printf("Scheduler warning: failed to acquire Redis lock, running without lock: %v", err)
		return true
	}
	return ok
}

func schedulerLockTTL(interval time.Duration) time.Duration {
	ttl := interval - (interval / 12)
	if ttl < time.Second {
		return time.Second
	}
	return ttl
}

func updateEventStatuses(db *gorm.DB, now time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if _, err := publishDueDrafts(tx, now); err != nil {
			return err
		}
		if err := closeExpiredApplications(tx, now); err != nil {
			return err
		}
		return endFinishedEvents(tx, now)
	})
}

func validTimelineWhere() string {
	return "publish_time <= start_time AND publish_time <= apply_deadline AND start_time <= end_time AND apply_deadline <= end_time"
}

func publishDueDrafts(db *gorm.DB, now time.Time) (int64, error) {
	var invalidDueDrafts int64
	if err := db.Model(&model.Event{}).
		Where("status = ? AND publish_time <= ?", "draft", now).
		Where("NOT (" + validTimelineWhere() + ")").
		Count(&invalidDueDrafts).Error; err == nil && invalidDueDrafts > 0 {
		log.Printf("Scheduler warning: skipped %d due draft event(s) with invalid timeline", invalidDueDrafts)
	}

	result := db.Model(&model.Event{}).
		Where("status = ? AND publish_time <= ?", "draft", now).
		Where(validTimelineWhere()).
		Update("status", "published")
	return result.RowsAffected, result.Error
}

func closeExpiredApplications(db *gorm.DB, now time.Time) error {
	return db.Model(&model.Event{}).
		Where("status = ? AND apply_deadline <= ?", "published", now).
		Update("status", "closed").Error
}

func endFinishedEvents(db *gorm.DB, now time.Time) error {
	return db.Model(&model.Event{}).
		Where("status IN ? AND end_time <= ?", []string{"published", "closed"}, now).
		Update("status", "ended").Error
}
