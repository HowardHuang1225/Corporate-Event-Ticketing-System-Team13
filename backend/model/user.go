package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID `gorm:"type:varchar(36);primaryKey" json:"id"`
	EmployeeID   string    `gorm:"uniqueIndex;not null"        json:"employee_id"`
	Name         string    `gorm:"not null"                    json:"name"`
	Email        string    `gorm:"uniqueIndex;not null"        json:"email"`
	Department   string    `gorm:"not null"                    json:"department"`
	Region       string    `gorm:"not null"                    json:"region"`
	Role         string    `gorm:"not null;default:'employee'" json:"role"`
	PasswordHash string    `gorm:"not null"                    json:"-"`
	IsActive     bool      `gorm:"default:true"                json:"is_active"`
	CreatedAt    time.Time `                                   json:"created_at"`
	UpdatedAt    time.Time `                                   json:"updated_at"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}
