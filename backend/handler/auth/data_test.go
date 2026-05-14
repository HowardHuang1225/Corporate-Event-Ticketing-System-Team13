package auth

import (
	"ticketing-system/backend/model"
)

var demoAccounts = []model.User{
	{
		EmployeeID:   "MGR001",
		Name:         "活動管理員",
		Email:        "manager@company.com",
		Department:   "活動管理部",
		Region:       "台南廠",
		Role:         "event_manager",
		PasswordHash: "",
		IsActive:     true,
	},
	{
		EmployeeID:   "EMP001",
		Name:         "台南員工",
		Email:        "emp001@company.com",
		Department:   "營運部",
		Region:       "台南廠",
		Role:         "employee",
		PasswordHash: "",
		IsActive:     true,
	},
	{
		EmployeeID:   "EMP002",
		Name:         "新竹員工",
		Email:        "emp002@company.com",
		Department:   "營運部",
		Region:       "新竹廠",
		Role:         "employee",
		PasswordHash: "",
		IsActive:     true,
	},
	{
		EmployeeID:   "HR001",
		Name:         "人資使用者",
		Email:        "hr@company.com",
		Department:   "人資部門",
		Region:       "台南廠",
		Role:         "hr",
		PasswordHash: "",
		IsActive:     true,
	},
}

var paramDB = []any{&model.User{}}