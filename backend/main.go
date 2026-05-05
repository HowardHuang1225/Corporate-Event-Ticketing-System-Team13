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

	"ticketing-system/backend/config"
	"ticketing-system/backend/database"
	"ticketing-system/backend/handler"
	"ticketing-system/backend/middleware"
	"ticketing-system/backend/model"
	"ticketing-system/backend/pkg"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func main() {
	// Load .env file from root or current directory
	// It's okay if it fails (e.g. in production/docker where env vars are already set)
	godotenv.Load(".env", "../.env")

	cfg := config.Load()
	db := database.Connect(cfg.DatabaseURL)
	redisClient := pkg.NewRedisClient(cfg.RedisURL)
	seedData(db)

	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     strings.Split(cfg.AllowedOrigins, ","),
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/ready", func(c *gin.Context) {
		sqlDB, _ := db.DB()
		if err := sqlDB.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	authH := handler.NewAuthHandler(db, cfg.JWTSecret)
	eventH := handler.NewEventHandler(db, redisClient)
	ticketH := handler.NewTicketHandler(db, redisClient)
	reportH := handler.NewReportHandler(db)

	v1 := r.Group("/v1")
	v1.POST("/auth/login", authH.Login)

	api := v1.Group("")
	api.Use(middleware.AuthMiddleware(cfg.JWTSecret))
	api.GET("/auth/me", authH.Me)

	// Events
	api.GET("/events", eventH.ListEvents)
	api.GET("/events/:id", eventH.GetEvent)
	api.POST("/events", middleware.RequireRole("event_manager"), eventH.CreateEvent)
	api.PUT("/events/:id", middleware.RequireRole("event_manager"), eventH.UpdateEvent)
	api.PATCH("/events/:id/publish", middleware.RequireRole("event_manager"), eventH.PublishEvent)
	api.PATCH("/events/:id/close", middleware.RequireRole("event_manager"), eventH.CloseEvent)

	// Applications & Tickets
	api.POST("/applications", middleware.RequireRole("employee"), ticketH.Apply)
	api.GET("/applications/my", middleware.RequireRole("employee"), ticketH.MyApplications)
	api.GET("/applications", middleware.RequireRole("event_manager"), ticketH.ListApplications)
	api.POST("/applications/:id/approve", middleware.RequireRole("event_manager"), ticketH.ApproveApplication)
	api.POST("/applications/:id/reject", middleware.RequireRole("event_manager"), ticketH.RejectApplication)
	api.POST("/applications/:id/cancel", middleware.RequireRole("employee"), ticketH.CancelApplication)
	api.POST("/tickets/:id/cancel", middleware.RequireRole("employee"), ticketH.CancelTicket)
	api.GET("/tickets/my", middleware.RequireRole("employee"), ticketH.MyTickets)
	api.POST("/checkin", middleware.RequireRole("event_manager"), ticketH.Checkin)
	api.GET("/checkins", middleware.RequireRole("event_manager"), ticketH.ListCheckins)

	// Reports
	api.GET("/reports/events/:id/stats", middleware.RequireRole("event_manager", "hr"), reportH.EventStats)
	api.GET("/reports/overview", middleware.RequireRole("hr"), reportH.AllEventsOverview)

	fmt.Printf("🚀 Server running on :%s\n", cfg.Port)
	// Create HTTP Server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Start server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	log.Printf("Server started on port %s", cfg.Port)

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

func seedData(db *gorm.DB) {
	var count int64
	db.Model(&model.User{}).Count(&count)
	if count > 0 {
		log.Println("Seed data already exists")
		return
	}
	log.Println("Seeding demo data...")

	hash := func(pw string) string {
		b, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		return string(b)
	}
	sp := func(s string) *string { return &s }

	users := []model.User{
		{EmployeeID: "MGR001", Name: "王大明", Email: "manager@company.com", Department: "福委會", Region: "台南廠", Role: "event_manager", PasswordHash: hash("password")},
		{EmployeeID: "EMP001", Name: "李小花", Email: "emp001@company.com", Department: "工程部", Region: "台南廠", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "EMP002", Name: "張三", Email: "emp002@company.com", Department: "人事部", Region: "新竹廠", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "EMP003", Name: "陳小明", Email: "emp003@company.com", Department: "製程部", Region: "台南廠", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "EMP004", Name: "林美玲", Email: "emp004@company.com", Department: "設計部", Region: "台南廠", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "HR001", Name: "陳副理", Email: "hr@company.com", Department: "人資部", Region: "台南廠", Role: "hr", PasswordHash: hash("password")},
	}
	db.Create(&users)
	managerID := users[0].ID
	now := time.Now()

	type eventSeed struct {
		event model.Event
		types []model.TicketType
	}
	seeds := []eventSeed{
		{
			event: model.Event{
				Title: "2024 藝文展覽 — 當代水墨特展", Venue: "台南市立美術館",
				Description:         "精選 20 位知名藝術家的水墨作品，帶您感受東方藝術的魅力。",
				StartTime:           now.Add(30 * 24 * time.Hour), EndTime: now.Add(31 * 24 * time.Hour),
				ApplyDeadline: now.Add(20 * 24 * time.Hour), Status: "published",
				RegionRestriction: sp("台南廠"), MaxTicketsPerPerson: 2, CreatedBy: managerID,
			},
			types: []model.TicketType{{Name: "一般票", TotalQuota: 100, Remaining: 100}, {Name: "眷屬票", TotalQuota: 50, Remaining: 50}},
		},
		{
			event: model.Event{
				Title: "員工家庭日 — 六福村主題樂園", Venue: "六福村主題樂園",
				Description:         "一年一度的員工家庭日！費用全額補助。",
				StartTime:           now.Add(45 * 24 * time.Hour), EndTime: now.Add(45*24*time.Hour + 8*time.Hour),
				ApplyDeadline: now.Add(30 * 24 * time.Hour), Status: "published",
				RegionRestriction: nil, MaxTicketsPerPerson: 4, CreatedBy: managerID,
			},
			types: []model.TicketType{{Name: "員工票（含眷屬 3 人）", TotalQuota: 200, Remaining: 200}},
		},
		{
			event: model.Event{
				Title: "AI 技能提升講座", Venue: "台積電研發大樓 B1 大講堂",
				Description:         "業界專家分享 AI 工具應用。",
				StartTime:           now.Add(15 * 24 * time.Hour), EndTime: now.Add(15*24*time.Hour + 4*time.Hour),
				ApplyDeadline: now.Add(7 * 24 * time.Hour), Status: "draft",
				RegionRestriction: nil, MaxTicketsPerPerson: 1, CreatedBy: managerID,
			},
			types: []model.TicketType{{Name: "入場票", TotalQuota: 300, Remaining: 300}},
		},
	}

	for _, s := range seeds {
		db.Create(&s.event)
		for i := range s.types {
			s.types[i].EventID = s.event.ID
			db.Create(&s.types[i])
		}
	}

	fmt.Println("Seed complete!")
	fmt.Println("   MGR001 / password  → event_manager")
	fmt.Println("   EMP001 / password  → employee (台南廠)")
	fmt.Println("   EMP002 / password  → employee (新竹廠)")
	fmt.Println("   HR001  / password  → hr")
}
