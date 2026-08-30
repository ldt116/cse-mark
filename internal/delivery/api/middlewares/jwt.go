package middlewares

import (
	goerrors "errors"
	"strings"

	"github.com/gin-gonic/gin"

	"thuanle/cse-mark/internal/delivery/api/errors"
	"thuanle/cse-mark/internal/domain/jwks"
	"thuanle/cse-mark/internal/usecases/assertion"
)

// bearerPrefix is the exact Authorization scheme the student app sends.
const bearerPrefix = "Bearer "

// Jwt authenticates student-app JWT assertions on GET /marks. On success the
// verified student id (MSSV) is stored in the context under "sub".
type Jwt struct {
	verify *assertion.Service
}

func NewJwtMiddleware(verify *assertion.Service) *Jwt {
	return &Jwt{
		verify: verify,
	}
}

// Handle requires an "Authorization: Bearer <token>" header and verifies the
// token. Failures are distinguished by error code: a missing or malformed
// header and an invalid token answer 401 jwt_invalid; an expired token
// answers 401 jwt_expired; a JWKS endpoint outage answers 503
// jwks_unavailable. The token itself is never logged or echoed.
func (m Jwt) Handle(c *gin.Context) {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, bearerPrefix) {
		errors.UnauthorizedWithCode(c, "jwt_invalid")
		return
	}

	sub, err := m.verify.Verify(strings.TrimPrefix(header, bearerPrefix))
	switch {
	case goerrors.Is(err, assertion.ErrExpired):
		errors.UnauthorizedWithCode(c, "jwt_expired")
		return
	case goerrors.Is(err, jwks.ErrUnavailable):
		errors.ServiceUnavailable(c, "jwks_unavailable")
		return
	case err != nil:
		errors.UnauthorizedWithCode(c, "jwt_invalid")
		return
	}

	c.Set("sub", sub)
	c.Next()
}
