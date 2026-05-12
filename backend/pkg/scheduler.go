package pkg

import (
	"context"
	"time"

	"ticketing-system/backend/model"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// StartEventStateScheduler runs a background worker to periodically update event status based on timeline.
func StartEventStateScheduler(db *gorm.DB, rdb *redis.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			ctx := context.Background()

			// Distributed lock using Redis to prevent concurrent database updates from multiple backend replicas
			lockKey := "lock:event_state_scheduler"
			// Set the lock with TTL slightly shorter than the interval to allow the next run to acquire it
			ttl := interval - (interval / 12)
			if ttl < time.Second {
				ttl = 1 * time.Second
			}

			if rdb != nil {
				ok, err := rdb.SetNX(ctx, lockKey, "1", ttl).Result()
				if err == nil && !ok {
					// Another instance is already running
					continue
				}
				// If there is an error communicating with Redis, we proceed anyway to guarantee fallback execution
			}

			now := time.Now()

			// Use a transaction to process updates sequentially
			_ = db.Transaction(func(tx *gorm.DB) error {
				// 1. Auto close: published -> closed when apply deadline has passed
				tx.Model(&model.Event{}).
					Where("status = ? AND apply_deadline <= ?", "published", now).
					Update("status", "closed")

				// 2. Auto end: published or closed -> ended when event end time has passed
				tx.Model(&model.Event{}).
					Where("status IN ? AND end_time <= ?", []string{"published", "closed"}, now).
					Update("status", "ended")

				return nil
			})
		}
	}()
}
