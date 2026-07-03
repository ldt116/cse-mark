package verification

import "errors"

// ErrNotFound is returned when no verification matches the query.
var ErrNotFound = errors.New("no verification in result")

// Repository stores pending OTP verifications. Records self-delete via the TTL
// index on `expiry`, so there is no explicit Delete method in the foundation.
type Repository interface {
	// Upsert stores (or overwrites) the OTP for a platform user id.
	Upsert(m Model) error

	// Find returns the pending verification for a platform user id.
	Find(platformUserID string) (Model, error)
}
