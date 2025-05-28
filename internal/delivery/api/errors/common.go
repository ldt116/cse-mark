package errors

import "github.com/gin-gonic/gin"

func BadRequest(c *gin.Context) {
	c.JSON(400, gin.H{"error": "bad request"})
	c.Abort()
}

func Unauthorized(c *gin.Context) {
	c.JSON(401, gin.H{"error": "unauthorized"})
	c.Abort()
}
