package jwks

import (
	"crypto/ed25519"
	"errors"
)

// ErrUnavailable is returned when the JWKS endpoint cannot be fetched or its
// document cannot be parsed. The previously cached key set, if any, is kept.
var ErrUnavailable = errors.New("jwks endpoint unavailable")

// ErrUnknownKid is returned when the JWKS key set contains no key for the
// requested key id, even after a refresh. Exception (Ruling 8, #44): a fresh
// cache holding an empty key set fails with this error without a refresh —
// the empty set was fetched whole, so refetching cannot surface the kid.
var ErrUnknownKid = errors.New("unknown signing key id")

// Repository resolves JWT signing keys by key id (kid) from the student app's
// JWKS endpoint. The api server runs handlers on many goroutines, so
// implementations must be safe for concurrent use.
type Repository interface {
	// SigningKey returns the Ed25519 public key for a key id.
	SigningKey(kid string) (ed25519.PublicKey, error)
}
