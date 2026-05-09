package repository

import (
	"ticketing-system/backend/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func (r *UserRepository) FindActiveByEmployeeID(employeeID string) (model.User, error) {
	var user model.User
	err := r.db.Where("employee_id = ? AND is_active = true", employeeID).First(&user).Error
	return user, err
}

func (r *UserRepository) FindByID(id uuid.UUID) (model.User, error) {
	var user model.User
	err := r.db.First(&user, "id = ?", id).Error
	return user, err
}
