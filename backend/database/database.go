package database

import (
	"log"

	"ticketing-system/backend/config" 
	"ticketing-system/backend/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg *config.Config) *gorm.DB {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.Event{},
		&model.TicketType{},
		&model.Application{},
		&model.Ticket{},
		&model.Checkin{},
	); err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}
	if err := ensureEventTimelineSchema(db); err != nil {
		log.Fatalf("Event timeline schema migration failed: %v", err)
	}

	sqlDB, _ := db.DB()
	
	// 動態設定連線池
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	log.Printf("Database connected with optimized pool (MaxOpen: %d, MaxIdle: %d)", cfg.DBMaxOpenConns, cfg.DBMaxIdleConns)
	return db
}

func ensureEventTimelineSchema(db *gorm.DB) error {
	if !db.Migrator().HasTable(&model.Event{}) {
		return nil
	}

	if !db.Migrator().HasColumn(&model.Event{}, "PublishTime") {
		if err := db.Exec("ALTER TABLE events ADD COLUMN publish_time timestamptz").Error; err != nil {
			return err
		}
	}

	if err := db.Exec(`
		UPDATE events
		SET publish_time = COALESCE(publish_time, start_time, created_at, NOW())
		WHERE publish_time IS NULL
	`).Error; err != nil {
		return err
	}

	if err := db.Exec("ALTER TABLE events ALTER COLUMN publish_time SET NOT NULL").Error; err != nil {
		return err
	}

	// Drop old constraint to apply the updated time-proofing rule: publish_time <= (start_time, apply_deadline) <= end_time
	_ = db.Exec("ALTER TABLE events DROP CONSTRAINT IF EXISTS chk_events_time_order")

	return db.Exec(`
		ALTER TABLE events
		ADD CONSTRAINT chk_events_time_order
		CHECK (
			publish_time <= start_time AND 
			publish_time <= apply_deadline AND 
			start_time <= end_time AND 
			apply_deadline <= end_time
		) NOT VALID;
	`).Error
}
