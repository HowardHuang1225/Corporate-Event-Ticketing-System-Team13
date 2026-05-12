package auth

import (
	"ticketing-system/backend/model"
	"ticketing-system/backend/pkg"
	"ticketing-system/backend/repository"
	"ticketing-system/backend/service/apperror"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	users     *repository.UserRepository
	jwtSecret string
}

func New(repos *repository.Repositories, jwtSecret string) *Service {
	return &Service{users: repos.Users, jwtSecret: jwtSecret}
}

type LoginRequest struct {
	EmployeeID string `json:"employee_id" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

type UserDTO struct {
	ID         uuid.UUID `json:"id"`
	EmployeeID string    `json:"employee_id"`
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	Department string    `json:"department"`
	Region     string    `json:"region"`
	Role       string    `json:"role"`
}

type LoginResult struct {
	AccessToken string  `json:"access_token"`
	User        UserDTO `json:"user"`
}

func (s *Service) Login(req LoginRequest) (LoginResult, error) {
	user, err := s.users.FindActiveByEmployeeID(req.EmployeeID)
	if err != nil {
		return LoginResult{}, apperror.Unauthorized("Invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return LoginResult{}, apperror.Unauthorized("Invalid credentials")
	}

	token, err := pkg.GenerateToken(user.ID, user.EmployeeID, user.Role, s.jwtSecret)
	if err != nil {
		return LoginResult{}, apperror.Internal("Failed to generate token")
	}

	return LoginResult{
		AccessToken: token,
		User:        ToUserDTO(user),
	}, nil
}

func (s *Service) Me(userID string) (UserDTO, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return UserDTO{}, apperror.Unauthorized("Invalid user context")
	}
	user, err := s.users.FindByID(id)
	if err != nil {
		return UserDTO{}, apperror.NotFound("User not found")
	}
	return ToUserDTO(user), nil
}

func ToUserDTO(user model.User) UserDTO {
	return UserDTO{
		ID:         user.ID,
		EmployeeID: user.EmployeeID,
		Name:       user.Name,
		Email:      user.Email,
		Department: user.Department,
		Region:     user.Region,
		Role:       user.Role,
	}
}
