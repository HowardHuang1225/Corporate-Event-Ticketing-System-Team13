package routes

import (
	"net/http"

	authhandler "ticketing-system/backend/handler/auth"
	employeehandler "ticketing-system/backend/handler/employee"
	hrhandler "ticketing-system/backend/handler/hr"
	managerhandler "ticketing-system/backend/handler/manager"
	"ticketing-system/backend/middleware"
	"ticketing-system/backend/repository"
	authsvc "ticketing-system/backend/service/auth"
	eventsvc "ticketing-system/backend/service/event"
	reportsvc "ticketing-system/backend/service/report"
	ticketsvc "ticketing-system/backend/service/ticket"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Dependencies struct {
	DB        *gorm.DB
	Redis     *redis.Client
	JWTSecret string
}

func Register(router *gin.Engine, deps Dependencies) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/ready", func(c *gin.Context) {
		sqlDB, _ := deps.DB.DB()
		if err := sqlDB.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	repos := repository.New(deps.DB, deps.Redis)
	authService := authsvc.New(repos, deps.JWTSecret)
	eventService := eventsvc.New(repos)
	ticketService := ticketsvc.New(repos)
	reportService := reportsvc.New(repos)

	authHandler := authhandler.New(authService)
	employeeHandler := employeehandler.New(eventService, ticketService)
	managerHandler := managerhandler.New(eventService, ticketService, reportService)
	hrHandler := hrhandler.New(reportService)

	v1 := router.Group("/v1")
	v1.POST("/auth/login", authHandler.Login)

	api := v1.Group("")
	api.Use(middleware.AuthMiddleware(deps.JWTSecret))
	api.GET("/auth/me", authHandler.Me)

	api.GET("/events", employeeHandler.ListEvents)
	api.GET("/events/:id", employeeHandler.GetEvent)
	api.GET("/events/:id/eligibility", middleware.RequireRole("employee"), employeeHandler.CheckEligibility)
	api.POST("/events", middleware.RequireRole("event_manager"), managerHandler.CreateEvent)
	api.PUT("/events/:id", middleware.RequireRole("event_manager"), managerHandler.UpdateEvent)
	api.PATCH("/events/:id/publish", middleware.RequireRole("event_manager"), managerHandler.PublishEvent)
	api.PATCH("/events/:id/close", middleware.RequireRole("event_manager"), managerHandler.CloseEvent)

	api.POST("/applications", middleware.RequireRole("employee"), employeeHandler.Apply)
	api.GET("/applications/my", middleware.RequireRole("employee"), employeeHandler.MyApplications)
	api.POST("/applications/:id/cancel", middleware.RequireRole("employee"), employeeHandler.CancelApplication)

	api.GET("/applications", middleware.RequireRole("event_manager"), managerHandler.ListApplications)
	api.POST("/applications/:id/approve", middleware.RequireRole("event_manager"), managerHandler.ApproveApplication)
	api.POST("/applications/:id/reject", middleware.RequireRole("event_manager"), managerHandler.RejectApplication)

	api.GET("/tickets/my", middleware.RequireRole("employee"), employeeHandler.MyTickets)
	api.POST("/tickets/:id/cancel", middleware.RequireRole("employee"), employeeHandler.CancelTicket)

	api.POST("/checkin", middleware.RequireRole("event_manager"), managerHandler.Checkin)
	api.GET("/checkins", middleware.RequireRole("event_manager"), managerHandler.ListCheckins)

	api.GET("/reports/events/:id/stats", middleware.RequireRole("event_manager", "hr"), func(c *gin.Context) {
		if c.GetString("role") == "hr" {
			hrHandler.EventStats(c)
			return
		}
		managerHandler.EventStats(c)
	})
	api.GET("/reports/events/:id/export", middleware.RequireRole("hr"), hrHandler.ExportEventCSV)
	api.GET("/reports/overview", middleware.RequireRole("hr"), hrHandler.Overview)
}
