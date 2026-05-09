package shared

import (
	"errors"
	"net/http"

	"ticketing-system/backend/service/apperror"

	"github.com/gin-gonic/gin"
)

func OK(data any) gin.H {
	return gin.H{"success": true, "data": data}
}

func Error(code string, message string) gin.H {
	return gin.H{
		"success": false,
		"error":   gin.H{"code": code, "message": message},
	}
}

func WriteError(c *gin.Context, err error) {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		c.JSON(appErr.Status, Error(appErr.Code, appErr.Message))
		return
	}
	c.JSON(http.StatusInternalServerError, Error("INTERNAL_ERROR", "Internal server error"))
}
