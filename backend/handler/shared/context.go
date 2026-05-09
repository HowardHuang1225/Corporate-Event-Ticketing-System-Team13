package shared

import (
	"ticketing-system/backend/service/apperror"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func UserID(c *gin.Context) (uuid.UUID, error) {
	id, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		return uuid.Nil, apperror.Unauthorized("Invalid user context")
	}
	return id, nil
}

func Role(c *gin.Context) string {
	return c.GetString("role")
}
