package mark

import "errors"

var ErrNotFound = errors.New("no marks in result")

type Repository interface {
	GetMark(courseId string, studentId string) (string, error)

	RemoveMarksByCourseId(courseId string) error
	AddCourseMarks(courseId string, marks []map[string]string) error
	RemoveCourseMarks(courseId string) error

	// ListStudentIds returns the MSSV (_id) of every mark document in a course's
	// collection. This is the source of enrollment for the Discord role-sync
	// scheduler (SRS §13 Enrollment): a student is enrolled in a class iff their
	// MSSV appears in that class's mark cache.
	ListStudentIds(courseId string) ([]string, error)
}
