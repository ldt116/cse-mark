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

// UnauthorizedWithCode rejects with 401 and a machine-readable error code
// (e.g. jwt_invalid, jwt_expired) so callers can tell failure causes apart.
func UnauthorizedWithCode(c *gin.Context, code string) {
	c.JSON(401, gin.H{"error": code})
	c.Abort()
}

// ServiceUnavailable rejects with 503 when an upstream dependency (e.g. the
// JWKS endpoint) is down and the request may succeed on retry.
func ServiceUnavailable(c *gin.Context, code string) {
	c.JSON(503, gin.H{"error": code})
	c.Abort()
}

// InternalError rejects with 500 for unexpected failures whose details must
// not leak to the client.
func InternalError(c *gin.Context) {
	c.JSON(500, gin.H{"error": "internal_error"})
	c.Abort()
}
