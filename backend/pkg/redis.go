package pkg

import (
	"context"
	"log"
	"time"

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
func AcquireLock(ctx context.Context, client *redis.Client, key string, ttl time.Duration) (bool, error) {
	return client.SetNX(ctx, "lock:"+key, 1, ttl).Result()
}

// ReleaseLock releases the distributed lock.
func ReleaseLock(ctx context.Context, client *redis.Client, key string) {
	client.Del(ctx, "lock:"+key)
}
