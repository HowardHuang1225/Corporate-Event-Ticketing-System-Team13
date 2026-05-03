package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Status values: draft | published | closed | ended
type Event struct {
	ID                  uuid.UUID    `gorm:"type:varchar(36);primaryKey"   json:"id"`
	Title               string       `gorm:"not null"                       json:"title"`
	Description         string       `                                      json:"description"`
	Venue               string       `gorm:"not null"                       json:"venue"`
	ImageURL            string       `                                      json:"image_url"`
	StartTime           time.Time    `gorm:"not null"                       json:"start_time"`
	EndTime             time.Time    `gorm:"not null"                       json:"end_time"`
	ApplyDeadline       time.Time    `gorm:"not null"                       json:"apply_deadline"`
	Status              string       `gorm:"not null;default:'draft'"       json:"status"`
	RegionRestriction   *string      `                                      json:"region_restriction"`
	MaxTicketsPerPerson int          `gorm:"not null;default:1"             json:"max_tickets_per_person"`
	CreatedBy           uuid.UUID    `gorm:"type:varchar(36);not null"      json:"created_by"`
	Creator             User         `gorm:"foreignKey:CreatedBy"           json:"creator,omitempty"`
	TicketTypes         []TicketType `gorm:"foreignKey:EventID"             json:"ticket_types,omitempty"`
	CreatedAt           time.Time    `                                      json:"created_at"`
	UpdatedAt           time.Time    `                                      json:"updated_at"`
}

func (e *Event) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// AfterFind 會在從資料庫讀取資料後執行
// 如果活動狀態為 'published' 但截止時間已過，自動將狀態視為 'closed'
func (e *Event) AfterFind(tx *gorm.DB) (err error) {
	if e.Status == "published" && !e.ApplyDeadline.IsZero() && time.Now().After(e.ApplyDeadline) {
		e.Status = "closed"
	}
	return nil
}

type TicketType struct {
	ID         uuid.UUID `gorm:"type:varchar(36);primaryKey"  json:"id"`
	EventID    uuid.UUID `gorm:"type:varchar(36);not null;index" json:"event_id"`
	Name       string    `gorm:"not null"                      json:"name"`
	TotalQuota int       `gorm:"not null"                      json:"total_quota"`
	Remaining  int       `gorm:"not null"                      json:"remaining"`
	Version    int64     `gorm:"not null;default:0"            json:"version"`
	CreatedAt  time.Time `                                     json:"created_at"`
}

func (t *TicketType) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}
