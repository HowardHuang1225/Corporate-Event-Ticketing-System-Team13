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
	"ticketing-system/backend/pkg"
	"ticketing-system/backend/pkg/storage"
	"ticketing-system/backend/routes"
	"ticketing-system/backend/scheduler"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	// 引入 Prometheus 官方的 HTTP 處理套件
	"github.com/prometheus/client_golang/prometheus/promhttp"

	// 引入 OpenTelemetry 核心與外掛套件
    "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
    "gorm.io/plugin/opentelemetry/tracing"
    "github.com/redis/go-redis/extra/redisotel/v9"
)

func initTracer() *sdktrace.TracerProvider {
	ctx := context.Background()

	// 設定把 Trace 資料送到 K8s 叢集內的 Jaeger Collector (4318 埠口)
	exporter, err := otlptracehttp.New(ctx, 
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithEndpoint("jaeger-collector.monitoring.svc.cluster.local:4318"),
	)
	if err != nil {
		log.Fatalf("Failed to create OTLP exporter: %v", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // Demo 階段 100% 採樣
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("ticket-service"), 
		)),
	)
	
	otel.SetTracerProvider(tp)
	return tp
}

func main() {
	_ = godotenv.Load(".env", "../.env")

	cfg := config.Load()
	// 啟動 OpenTelemetry 追蹤引擎
    tp := initTracer()
    defer func() {
        if err := tp.Shutdown(context.Background()); err != nil {
            log.Printf("Error shutting down tracer provider: %v", err)
        }
    }()
	db := database.Connect(cfg)
	// 注入 GORM 追蹤外掛（自動側錄所有 SQL 語句、DB 耗時）
    if err := db.Use(tracing.NewPlugin()); err != nil {
        log.Printf("Failed to plug OpenTelemetry into GORM: %v", err)
    }
	redisClient := pkg.NewRedisClient(cfg.RedisURL)
	// 注入 Redis 追蹤外掛（自動側錄 Redis Get/Set/DecrBy 耗時）
    if redisClient != nil {
        redisotel.InstrumentTracing(redisClient)
    }
	minioService := storage.NewMinioService(cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioBucketName, cfg.MinioPublicEndpoint)
	bootstrap.SeedDemoData(db)
	scheduler.StartEventSchedulers(db, redisClient)

	router := gin.Default()
	router.Use(cors.New(cors.Config{
        AllowOrigins:     strings.Split(cfg.AllowedOrigins, ","),
        AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
        AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "traceparent", "tracestate"},
        AllowCredentials: true,
    }))
	router.Use(otelgin.Middleware("ticket-service"))

	routes.Register(router, routes.Dependencies{
		DB:        db,
		Redis:     redisClient,
		Minio:     minioService,
		JWTSecret: cfg.JWTSecret,
	})

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
