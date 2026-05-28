package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ticketing-system/backend/bootstrap"
	"ticketing-system/backend/config"
	"ticketing-system/backend/database"
	metrics "ticketing-system/backend/metrics"
	"ticketing-system/backend/pkg"
	"ticketing-system/backend/pkg/storage"
	"ticketing-system/backend/routes"
	"ticketing-system/backend/scheduler"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/redis/go-redis/extra/redisotel/v9"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	metrics.Register()
}

func initTracer() {
	log.Println("Tracing disabled (metrics only mode)")
}

func metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		start := time.Now()

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		status := fmt.Sprintf("%d", c.Writer.Status())

		httpRequestsTotal.WithLabelValues(
			c.Request.Method,
			path,
			status,
		).Inc()

		httpRequestDuration.WithLabelValues(
			c.Request.Method,
			path,
		).Observe(time.Since(start).Seconds())
	}
}

func main() {
	_ = godotenv.Load(".env", "../.env")

	cfg := config.Load()

	// tracer
	initTracer()
	db := database.Connect(cfg)
	// _ = db.Use(tracing.NewPlugin())

	redisClient := pkg.NewRedisClient(cfg.RedisURL)
	if redisClient != nil {
		redisotel.InstrumentTracing(redisClient)
	}

	minioService := storage.NewMinioService(
		cfg.MinioEndpoint,
		cfg.MinioAccessKey,
		cfg.MinioSecretKey,
		cfg.MinioBucketName,
		cfg.MinioPublicEndpoint,
	)

	bootstrap.SeedDemoData(db)
	scheduler.StartEventSchedulers(db, redisClient)

	router := gin.Default()

	router.Use(cors.New(cors.Config{
		AllowOrigins:     strings.Split(cfg.AllowedOrigins, ","),
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "traceparent", "tracestate"},
		AllowCredentials: true,
	}))


	// metrics middleware
	router.Use(metricsMiddleware())

	// routes
	routes.Register(router, routes.Dependencies{
		DB:        db,
		Redis:     redisClient,
		Minio:     minioService,
		JWTSecret: cfg.JWTSecret,
	})

	// Prometheus endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	fmt.Printf("Server running on :%s\n", cfg.Port)

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	log.Printf("Server started on port %s", cfg.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}
