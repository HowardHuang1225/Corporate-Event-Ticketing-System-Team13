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
	"ticketing-system/backend/routes"
	"ticketing-system/backend/scheduler"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env", "../.env")

	cfg := config.Load()
	db := database.Connect(cfg)
	redisClient := pkg.NewRedisClient(cfg.RedisURL)
	bootstrap.SeedDemoData(db)
	scheduler.StartEventSchedulers(db, redisClient)

	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     strings.Split(cfg.AllowedOrigins, ","),
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	routes.Register(router, routes.Dependencies{
		DB:        db,
		Redis:     redisClient,
		JWTSecret: cfg.JWTSecret,
	})

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
