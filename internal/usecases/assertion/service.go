// Package assertion verifies student-app JWT assertions: EdDSA-signed tokens
// carrying registered claims only, resolved against the student app's JWKS.
package assertion

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"

	"thuanle/cse-mark/internal/domain/jwks"
)

// ErrInvalid marks an assertion that failed verification: bad signature,
// wrong issuer/audience/algorithm, unknown kid, missing subject, or a
// malformed token. Callers should reject the request with 401.
var ErrInvalid = errors.New("invalid assertion")

// ErrExpired marks an assertion whose exp claim is in the past. Callers
// should also reject with 401; it is separate from ErrInvalid only so logs
// and metrics can tell expired tokens apart from tampered ones.
var ErrExpired = errors.New("expired assertion")

// Service verifies student-app JWT assertions.
type Service struct {
	jwks     jwks.Repository
	issuer   string
	audience string
}

// NewService builds a Service. issuer and audience pin the expected iss and
// aud claims of every assertion.
func NewService(repo jwks.Repository, issuer, audience string) *Service {
	return &Service{
		jwks:     repo,
		issuer:   issuer,
		audience: audience,
	}
}

// Verify checks an assertion's signature (EdDSA only, key resolved by the kid
// header from the JWKS repository), issuer, audience and expiry, then returns
// the subject — the student id (MSSV). jwks.ErrUnavailable is propagated
// intact so the caller can answer 503 instead of 401; every other failure is
// ErrInvalid or ErrExpired. The token and its claims are never logged.
func (s *Service) Verify(tokenString string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		s.signingKey,
		jwt.WithValidMethods([]string{"EdDSA"}),
		jwt.WithIssuer(s.issuer),
		jwt.WithAudience(s.audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return "", ErrExpired
		case errors.Is(err, jwks.ErrUnavailable):
			return "", err
		default:
			return "", ErrInvalid
		}
	}
	if !parsed.Valid || claims.Subject == "" {
		return "", ErrInvalid
	}
	return claims.Subject, nil
}

// signingKey is the parser keyfunc: it resolves the kid header to the Ed25519
// verification key. A missing or empty kid fails as ErrInvalid; repository
// errors are propagated unchanged so Verify can distinguish them.
func (s *Service) signingKey(t *jwt.Token) (any, error) {
	kid, _ := t.Header["kid"].(string)
	if kid == "" {
		return nil, ErrInvalid
	}
	key, err := s.jwks.SigningKey(kid)
	if err != nil {
		return nil, err
	}
	return key, nil
}
