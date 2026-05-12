package repository

import "gorm.io/gorm"

type ReportRepository struct {
	db *gorm.DB
}
