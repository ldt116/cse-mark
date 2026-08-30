package jwks

import (
	"crypto/ed25519"
	"errors"
)

// ErrUnavailable is returned when the JWKS endpoint cannot be fetched or its
// document cannot be parsed. The previously cached key set, if any, is kept.
var ErrUnavailable = errors.New("jwks endpoint unavailable")

// ErrUnknownKid is returned when the JWKS key set contains no key for the
// requested key id, even after a refresh.
var ErrUnknownKid = errors.New("unknown signing key id")

// Repository resolves JWT signing keys by key id (kid) from the student app's
// JWKS endpoint. The api server runs handlers on many goroutines, so
// implementations must be safe for concurrent use.
type Repository interface {
	// SigningKey returns the Ed25519 public key for a key id.
	SigningKey(kid string) (ed25519.PublicKey, error)
}
