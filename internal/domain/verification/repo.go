package verification

import "errors"

// ErrNotFound is returned when no verification matches the query.
var ErrNotFound = errors.New("no verification in result")

// Repository stores pending OTP verifications. Records self-delete via the TTL
// index on `expiry`, so there is no explicit Delete method in the foundation.
type Repository interface {
	// Upsert stores (or overwrites) the OTP for a platform user id, resetting
	// the failed-attempt counter to 0.
	Upsert(m Model) error

	// Find returns the pending verification for a platform user id.
	Find(platformUserID string) (Model, error)

	// IncrementAttempts atomically increments the failed-attempt counter for a
	// pending verification and returns the new count. The identity use case
	// compares it to Config.OtpMaxAttempts to decide invalidation.
	IncrementAttempts(platformUserID string) (int, error)

	// FindByEmail returns pending verifications matching an email, to enforce a
	// per-email resend cooldown (defends against Sybil email bombing).
	FindByEmail(email string) ([]Model, error)
}
