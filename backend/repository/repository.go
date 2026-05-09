package repository

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Repositories struct {
	DB           *gorm.DB
	Redis        *redis.Client
	Users        *UserRepository
	Events       *EventRepository
	Applications *ApplicationRepository
	Tickets      *TicketRepository
	Reports      *ReportRepository
}

func New(db *gorm.DB, redis *redis.Client) *Repositories {
	return &Repositories{
		DB:           db,
		Redis:        redis,
		Users:        &UserRepository{db: db},
		Events:       &EventRepository{db: db},
		Applications: &ApplicationRepository{db: db},
		Tickets:      &TicketRepository{db: db},
		Reports:      &ReportRepository{db: db},
	}
}
