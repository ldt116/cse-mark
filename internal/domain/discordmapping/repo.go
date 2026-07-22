package discordmapping

import "errors"

// ErrNotFound is returned when no mapping matches the query.
var ErrNotFound = errors.New("no discord mapping in result")

// Repository persists the Discord role/channel ids provisioned per course.
type Repository interface {
	// Upsert stores the role/channel ids for a course after they are provisioned.
	Upsert(m Model) error

	// Find returns the mapping for a course.
	Find(courseId string) (Model, error)

	// Remove deletes the mapping for a course.
	Remove(courseId string) error

	// ListAll returns every provisioned mapping. The role-sync scheduler iterates
	// this to reconcile enrollment→role for each course that exists on Discord
	// (SRS §10.3: only courses with a discord_mappings record are synced).
	ListAll() ([]Model, error)
}
