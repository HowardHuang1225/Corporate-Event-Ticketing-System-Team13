package handler

import "github.com/gin-gonic/gin"

func errResp(code, message string) gin.H {
	return gin.H{
		"success": false,
		"error":   gin.H{"code": code, "message": message},
	}
}

func okResp(data interface{}) gin.H {
	return gin.H{"success": true, "data": data}
}
