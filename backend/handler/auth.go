package handler

import (
	"net/http"

	"ticketing-system/backend/model"
	"ticketing-system/backend/pkg"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthHandler struct {
	db        *gorm.DB
	jwtSecret string
}

func NewAuthHandler(db *gorm.DB, jwtSecret string) *AuthHandler {
	return &AuthHandler{db: db, jwtSecret: jwtSecret}
}

type LoginRequest struct {
	EmployeeID string `json:"employee_id" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errResp("VALIDATION_ERROR", err.Error()))
		return
	}

	var user model.User
	if err := h.db.Where("employee_id = ? AND is_active = true", req.EmployeeID).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, errResp("UNAUTHORIZED", "Invalid credentials"))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, errResp("UNAUTHORIZED", "Invalid credentials"))
		return
	}

	token, err := pkg.GenerateToken(user.ID, user.EmployeeID, user.Role, h.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL_ERROR", "Failed to generate token"))
		return
	}

	c.JSON(http.StatusOK, okResp(gin.H{
		"access_token": token,
		"user": gin.H{
			"id":          user.ID,
			"employee_id": user.EmployeeID,
			"name":        user.Name,
			"email":       user.Email,
			"department":  user.Department,
			"region":      user.Region,
			"role":        user.Role,
		},
	}))
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID := c.GetString("user_id")
	var user model.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", "User not found"))
		return
	}
	c.JSON(http.StatusOK, okResp(gin.H{
		"id":          user.ID,
		"employee_id": user.EmployeeID,
		"name":        user.Name,
		"email":       user.Email,
		"department":  user.Department,
		"region":      user.Region,
		"role":        user.Role,
	}))
}
