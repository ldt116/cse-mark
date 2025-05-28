package middlewares

import (
	"github.com/gin-gonic/gin"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/delivery/api/errors"
)

type Auth struct {
	token string
}

func NewAuthMiddleware(config *configs.Config) *Auth {
	return &Auth{
		token: config.ApiToken,
	}
}

func (m Auth) Handle(c *gin.Context) {
	if c.Query("token") != m.token {
		errors.Unauthorized(c)
		return
	}
	c.Next()
}
