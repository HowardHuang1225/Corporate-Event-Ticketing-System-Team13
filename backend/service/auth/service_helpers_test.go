package auth

import (
	"errors"
	"fmt"
	"testing"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	"ticketing-system/backend/service/apperror"
	utils "ticketing-system/backend/test_utils"

	"gorm.io/gorm"
)

var authServiceTestModels = []any{&model.User{}}

func setupAuthServiceTest(t *testing.T) (*gorm.DB, model.User, func(), error) {
	t.Helper()

	tx, cleanup, err := utils.BeginTestTransaction(t, authServiceTestModels)
	if err != nil {
		return nil, model.User{}, nil, err
	}

	suffix := utils.UniqueTestSuffix()
	users, err := utils.SeedTestRole(tx, []model.User{
		{
			EmployeeID:   fmt.Sprintf("AUTHSVC%s", suffix),
			Name:         "認證服務測試使用者",
			Email:        fmt.Sprintf("auth-service-user-%s@example.com", suffix),
			Department:   "Engineering",
			Region:       "Tainan",
			Role:         "employee",
			PasswordHash: "",
			IsActive:     true,
		},
	}, true)
	if err != nil {
		_ = tx.Rollback()
		return nil, model.User{}, nil, err
	}

	return tx, users[0], cleanup, nil
}

func newAuthServiceForTest(db *gorm.DB) *Service {
	return New(repository.New(db, nil), authServiceTestJWTSecret)
}

func assertAuthServiceUserDTO(got UserDTO, want model.User) error {
	if got.ID != want.ID {
		return fmt.Errorf("預期使用者 ID 為 %s，實際為 %s", want.ID, got.ID)
	}
	if got.EmployeeID != want.EmployeeID {
		return fmt.Errorf("預期 employee_id 為 %q，實際為 %q", want.EmployeeID, got.EmployeeID)
	}
	if got.Name != want.Name {
		return fmt.Errorf("預期 name 為 %q，實際為 %q", want.Name, got.Name)
	}
	if got.Email != want.Email {
		return fmt.Errorf("預期 email 為 %q，實際為 %q", want.Email, got.Email)
	}
	if got.Department != want.Department {
		return fmt.Errorf("預期 department 為 %q，實際為 %q", want.Department, got.Department)
	}
	if got.Region != want.Region {
		return fmt.Errorf("預期 region 為 %q，實際為 %q", want.Region, got.Region)
	}
	if got.Role != want.Role {
		return fmt.Errorf("預期 role 為 %q，實際為 %q", want.Role, got.Role)
	}
	return nil
}

func assertAuthServiceAppErrorCode(err error, wantCode string) error {
	if err == nil {
		return fmt.Errorf("預期錯誤代碼 %q，實際沒有錯誤", wantCode)
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		return fmt.Errorf("預期 apperror.Error，實際錯誤為 %T：%v", err, err)
	}
	if appErr.Code != wantCode {
		return fmt.Errorf("預期錯誤代碼 %q，實際為 %q", wantCode, appErr.Code)
	}
	return nil
}
