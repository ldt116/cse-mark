package binding

import "errors"

// ErrNotFound is returned when no binding matches the query.
var ErrNotFound = errors.New("no binding in result")

// Repository persists platform <-> MSSV bindings.
type Repository interface {
	// Upsert inserts or updates a binding keyed by (platform, platform_user_id).
	Upsert(m Model) error

	// FindByPlatformUser resolves a chat account to its binding (/mark lookup).
	FindByPlatformUser(platform, platformUserID string) (Model, error)

	// FindByPlatformMSSV resolves an MSSV on a given platform (1:1:1 check).
	FindByPlatformMSSV(platform, mssv string) (Model, error)

	// FindByMSSV returns all bindings for an MSSV across platforms.
	FindByMSSV(mssv string) ([]Model, error)
}
