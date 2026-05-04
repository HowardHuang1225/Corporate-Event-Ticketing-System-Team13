package database

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ticketing-system/backend/model"
)

func Connect(dsn string) *gorm.DB {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.Event{},
		&model.TicketType{},
		&model.Application{},
		&model.Ticket{},
		&model.Checkin{},
	); err != nil {
		log.Fatalf("❌ AutoMigrate failed: %v", err)
	}

	sqlDB, _ := db.DB()
	sqlDB.SetMaxIdleConns(50)
	sqlDB.SetMaxOpenConns(200)

	log.Println("✅ Database connected and migrated with optimized pool")
	return db
}
