package student

import "errors"

// ErrNotFound is returned when no student matches the query.
var ErrNotFound = errors.New("no student in result")

// Repository persists roster students.
type Repository interface {
	// Upsert inserts or replaces a student by MSSV.
	Upsert(m Model) error

	// FindByEmail resolves a roster email to its student (bind flow: email -> MSSV).
	FindByEmail(email string) (Model, error)

	// FindByMSSV returns the student with the given MSSV.
	FindByMSSV(mssv string) (Model, error)
}
