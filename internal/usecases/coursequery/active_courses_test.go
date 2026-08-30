package coursequery

import (
	"testing"
	"time"

	"thuanle/cse-mark/internal/domain/course"
)

type fakeRepo struct {
	courses []course.Model
	called  string // method name of the last call
	since   time.Time
}

func (f *fakeRepo) FindCoursesUpdatedAfter(since time.Time) ([]course.Model, error) {
	f.called = "FindCoursesUpdatedAfter"
	f.since = since
	return f.courses, nil
}
func (f *fakeRepo) FindSyncableCourses(since time.Time) ([]course.Model, error) {
	f.called = "FindSyncableCourses"
	f.since = since
	return f.courses, nil
}
func (f *fakeRepo) UpdateCourseRecordCount(string, int) error             { return nil }
func (f *fakeRepo) FindCoursesManagedByUser(string) ([]course.Model, error) { return nil, nil }
func (f *fakeRepo) FindCourseById(string) (course.Model, error)          { return course.Model{}, nil }
func (f *fakeRepo) UpdateCourseLink(string, string, int64, string) error { return nil }
func (f *fakeRepo) RemoveCourse(string) error                           { return nil }
func (f *fakeRepo) SetCourseStatus(string, course.Status) error         { return nil }

func TestListActiveCourses_UsesSyncableQuery(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewActiveCourseService(repo, &course.Rules{CourseActiveAge: 9 * 30 * 24 * time.Hour})
	if _, err := svc.ListActiveCourses(); err != nil {
		t.Fatal(err)
	}
	if repo.called != "FindSyncableCourses" {
		t.Fatalf("called = %s, want FindSyncableCourses (poll must exclude inactive courses)", repo.called)
	}
}
