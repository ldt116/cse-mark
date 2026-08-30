package course

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("no courses in result")

type Repository interface {
	// FindCoursesUpdatedAfter returns all courses that have been updated after the given time.
	FindCoursesUpdatedAfter(since time.Time) ([]Model, error)

	//UpdateCourseRecordCount updates the course record in the repository.
	UpdateCourseRecordCount(courseId string, cnt int) error

	//FindCoursesManagedByUser returns all courses managed by the user with the given username.
	FindCoursesManagedByUser(username string) ([]Model, error)

	// FindCourseById returns the course with the given ID.
	FindCourseById(courseId string) (Model, error)

	//UpdateCourseLink updates the course link for the given course ID.
	UpdateCourseLink(courseId string, link string, userId int64, username string) error

	// RemoveCourse removes the course with the given ID.
	RemoveCourse(courseId string) error

	// SetCourseStatus updates only the course status; it must not touch
	// updated_at (that field tracks provisioning activity).
	SetCourseStatus(courseId string, status Status) error

	// FindSyncableCourses returns courses the poller should consider:
	// updated after since, with a link, and not inactive (missing status = active).
	FindSyncableCourses(since time.Time) ([]Model, error)
}
