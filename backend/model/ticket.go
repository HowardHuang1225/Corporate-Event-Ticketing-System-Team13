package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Status: queued | processing | pending | approved | rejected | cancelled
type Application struct {
	ID             uuid.UUID  `gorm:"type:varchar(36);primaryKey"      json:"id"`
	UserID         uuid.UUID  `gorm:"type:varchar(36);not null;index"  json:"user_id"`
	EventID        uuid.UUID  `gorm:"type:varchar(36);not null;index:idx_app_event_status"  json:"event_id"`
	TicketTypeID   uuid.UUID  `gorm:"type:varchar(36);not null"        json:"ticket_type_id"`
	Quantity       int        `gorm:"not null;default:1"               json:"quantity"`
	Status         string     `gorm:"not null;default:'pending';index:idx_app_event_status"       json:"status"`
	IdempotencyKey string     `gorm:"uniqueIndex;not null"             json:"idempotency_key"`
	Reason         *string    `                                        json:"reason"`
	ReviewedBy     *uuid.UUID `gorm:"type:varchar(36)"                 json:"reviewed_by"`
	ReviewedAt     *time.Time `                                        json:"reviewed_at"`
	AppliedAt      time.Time  `gorm:"not null;autoCreateTime"          json:"applied_at"`
	UpdatedAt      time.Time  `                                        json:"updated_at"`
	User           User       `gorm:"foreignKey:UserID"                json:"user,omitempty"`
	Event          Event      `gorm:"foreignKey:EventID"               json:"event,omitempty"`
	TicketType     TicketType `gorm:"foreignKey:TicketTypeID"          json:"ticket_type,omitempty"`
	Tickets        []Ticket   `gorm:"foreignKey:ApplicationID"         json:"tickets,omitempty"`
}

func (a *Application) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

type Ticket struct {
	ID            uuid.UUID   `gorm:"type:varchar(36);primaryKey"     json:"id"`
	ApplicationID uuid.UUID   `gorm:"type:varchar(36);not null;index" json:"application_id"`
	UserID        uuid.UUID   `gorm:"type:varchar(36);not null;index" json:"user_id"`
	EventID       uuid.UUID   `gorm:"type:varchar(36);not null;index" json:"event_id"`
	TicketTypeID  uuid.UUID   `gorm:"type:varchar(36);not null;index" json:"ticket_type_id"`
	QRToken       string      `gorm:"uniqueIndex;not null"            json:"qr_token"`
	IsUsed        bool        `gorm:"default:false"                   json:"is_used"`
	IssuedAt      time.Time   `gorm:"not null;autoCreateTime"         json:"issued_at"`
	ExpiresAt     time.Time   `gorm:"not null"                        json:"expires_at"`
	Application   Application `gorm:"foreignKey:ApplicationID"         json:"application,omitempty"`
	Event         Event       `gorm:"foreignKey:EventID"              json:"event,omitempty"`
	TicketType    TicketType  `gorm:"foreignKey:TicketTypeID"         json:"ticket_type,omitempty"`
	User          User        `gorm:"foreignKey:UserID"               json:"user,omitempty"`
}

func (t *Ticket) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

type Checkin struct {
	ID        uuid.UUID `gorm:"type:varchar(36);primaryKey"    json:"id"`
	TicketID  uuid.UUID `gorm:"type:varchar(36);uniqueIndex;not null" json:"ticket_id"`
	CheckedBy uuid.UUID `gorm:"type:varchar(36);not null"      json:"checked_by"`
	CheckedAt time.Time `gorm:"not null;autoCreateTime"        json:"checked_at"`
	Ticket    Ticket    `gorm:"foreignKey:TicketID"            json:"ticket,omitempty"`
	Checker   User      `gorm:"foreignKey:CheckedBy"           json:"checker,omitempty"`
}

func (c *Checkin) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}
