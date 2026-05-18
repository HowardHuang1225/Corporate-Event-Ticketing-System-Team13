package pkg

import (
	"context"
	"log"
	"time"
	"errors"

	"github.com/redis/go-redis/v9"
)

func NewRedisClient(url string) *redis.Client {
	opt, err := redis.ParseURL(url)
	if err != nil {
		log.Printf("⚠️  Invalid Redis URL, using defaults: %v", err)
		opt = &redis.Options{Addr: "localhost:6379"}
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("⚠️  Redis not reachable: %v (anti-oversell lock degraded)", err)
	} else {
		log.Println("✅ Redis connected")
	}
	return client
}

// AcquireLock tries to get a distributed lock. Returns true if acquired.
func AcquireLock(ctx context.Context, client *redis.Client, key string, ttl time.Duration, waitTimeout time.Duration) (bool, error) {
	deadline := time.Now().Add(waitTimeout)
	// for time.Now().Before(deadline) {
	// 	acquired, err := client.SetNX(ctx, "lock:"+key, 1, ttl).Result()
	// 	if err != nil {
	// 		return false, err
	// 	}
	// 	if acquired {
	// 		return true, nil
	// 	}
	// 	time.Sleep(5 * time.Millisecond)
	// }
	for time.Now().Before(deadline) {
        _, err := client.SetArgs(ctx, "lock:"+key, 1, redis.SetArgs{
            Mode: "NX",
            TTL:  ttl,
        }).Result()
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, redis.Nil) {
			return false, err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false, nil
}

// ReleaseLock releases the distributed lock.
func ReleaseLock(ctx context.Context, client *redis.Client, key string) {
	client.Del(ctx, "lock:"+key)
}
