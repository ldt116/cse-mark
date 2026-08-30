package marksquery

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/mark"
)

// fakeCourseRepo satisfies the full course.Repository interface.
type fakeCourseRepo struct {
	courses    []course.Model
	findCalled bool
	findSince  time.Time
	err        error
}

func (f *fakeCourseRepo) FindCoursesUpdatedAfter(since time.Time) ([]course.Model, error) {
	f.findCalled = true
	f.findSince = since
	if f.err != nil {
		return nil, f.err
	}
	return f.courses, nil
}
func (f *fakeCourseRepo) UpdateCourseRecordCount(string, int) error { return nil }
func (f *fakeCourseRepo) FindCoursesManagedByUser(string) ([]course.Model, error) {
	return nil, nil
}
func (f *fakeCourseRepo) FindCourseById(string) (course.Model, error) {
	return course.Model{}, course.ErrNotFound
}
func (f *fakeCourseRepo) UpdateCourseLink(string, string, int64, string) error { return nil }
func (f *fakeCourseRepo) RemoveCourse(string) error                           { return nil }

// fakeMarkRepo satisfies the full mark.Repository interface.
type fakeMarkRepo struct {
	marks map[string]string // "courseId\x00studentId" -> JSON string
	err   error
}

func (f *fakeMarkRepo) GetMark(courseId string, studentId string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if m, ok := f.marks[courseId+"\x00"+studentId]; ok {
		return m, nil
	}
	return "", mark.ErrNotFound
}
func (f *fakeMarkRepo) RemoveMarksByCourseId(string) error { return nil }
func (f *fakeMarkRepo) AddCourseMarks(string, []map[string]string) error {
	return nil
}
func (f *fakeMarkRepo) RemoveCourseMarks(string) error { return nil }
func (f *fakeMarkRepo) ListStudentIds(string) ([]string, error) {
	return nil, nil
}

const (
	studentId = "2012345"
	c1JSON    = `{"z_total":10,"a_mid":4}` // key order deliberately non-alphabetical to catch re-marshal
	c3JSON    = `{"z_total":9,"a_mid":5}`
)

func newAllCoursesFakes() (*fakeCourseRepo, *fakeMarkRepo) {
	courseRepo := &fakeCourseRepo{
		courses: []course.Model{
			{Id: "c1", UpdatedAt: time.Now().Unix()},      // active, student has marks
			{Id: "c2", UpdatedAt: time.Now().Unix()},      // active, student has no doc -> skip
			{Id: "c3", UpdatedAt: time.Unix(0, 0).Unix()}, // inactive, frozen marks still visible
		},
	}
	markRepo := &fakeMarkRepo{
		marks: map[string]string{
			"c1\x00" + studentId: c1JSON,
			"c3\x00" + studentId: c3JSON,
		},
	}
	return courseRepo, markRepo
}

func TestQuery_AllCourses(t *testing.T) {
	courseRepo, markRepo := newAllCoursesFakes()
	s := NewService(courseRepo, markRepo)

	got, err := s.Query(studentId, "")
	if err != nil {
		t.Fatalf("Query(%q, \"\") error = %v, want nil", studentId, err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (c2 skipped: student has no doc)", len(got))
	}
	want := []struct {
		courseId string
		marks    string
	}{
		{"c1", c1JSON},
		{"c3", c3JSON},
	}
	for i, w := range want {
		if got[i].CourseId != w.courseId {
			t.Errorf("got[%d].CourseId = %q, want %q", i, got[i].CourseId, w.courseId)
		}
		if string(got[i].Marks) != w.marks {
			t.Errorf("got[%d].Marks = %q, want verbatim %q", i, string(got[i].Marks), w.marks)
		}
	}
	if !courseRepo.findSince.Equal(time.Unix(0, 0)) {
		t.Errorf("FindCoursesUpdatedAfter since = %v, want epoch zero (inactive courses included)", courseRepo.findSince)
	}
}

func TestQuery_CourseFilter(t *testing.T) {
	courseRepo, markRepo := newAllCoursesFakes()
	s := NewService(courseRepo, markRepo)

	got, err := s.Query(studentId, "c1")
	if err != nil {
		t.Fatalf("Query(%q, \"c1\") error = %v, want nil", studentId, err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].CourseId != "c1" {
		t.Errorf("got[0].CourseId = %q, want \"c1\"", got[0].CourseId)
	}
	if string(got[0].Marks) != c1JSON {
		t.Errorf("got[0].Marks = %q, want verbatim %q", string(got[0].Marks), c1JSON)
	}
	if courseRepo.findCalled {
		t.Error("FindCoursesUpdatedAfter was called for a single-course query, want not called")
	}
}

func TestQuery_NoMarks(t *testing.T) {
	courseRepo, markRepo := newAllCoursesFakes()
	s := NewService(courseRepo, markRepo)

	got, err := s.Query("9999999", "")
	if err != nil {
		t.Fatalf("Query(\"9999999\", \"\") error = %v, want nil", err)
	}
	if got == nil {
		t.Fatal("got = nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}

func TestQuery_CourseFilterStudentMissing(t *testing.T) {
	courseRepo, markRepo := newAllCoursesFakes()
	s := NewService(courseRepo, markRepo)

	got, err := s.Query("9999999", "c1")
	if err != nil {
		t.Fatalf("Query(\"9999999\", \"c1\") error = %v, want nil", err)
	}
	if got == nil {
		t.Fatal("got = nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}

// TestQuery_WireFormat locks the final JSON wire format (json tags, nil -> [],
// no HTML escaping / re-indentation of the verbatim marks), which the struct
// field assertions above cannot catch.
func TestQuery_WireFormat(t *testing.T) {
	courseRepo, markRepo := newAllCoursesFakes()
	s := NewService(courseRepo, markRepo)

	got, err := s.Query(studentId, "")
	if err != nil {
		t.Fatalf("Query(%q, \"\") error = %v, want nil", studentId, err)
	}

	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal error = %v, want nil", err)
	}
	want := `[{"courseId":"c1","marks":{"z_total":10,"a_mid":4}},{"courseId":"c3","marks":{"z_total":9,"a_mid":5}}]`
	if string(b) != want {
		t.Errorf("wire JSON =\n%s\nwant\n%s", string(b), want)
	}
}

func TestQuery_RepoError(t *testing.T) {
	wantErr := errors.New("connection lost")
	markRepo := &fakeMarkRepo{err: wantErr}
	s := NewService(&fakeCourseRepo{}, markRepo)

	if _, err := s.Query(studentId, "c1"); !errors.Is(err, wantErr) {
		t.Fatalf("Query(%q, \"c1\") error = %v, want %v", studentId, err, wantErr)
	}

	courseRepo := &fakeCourseRepo{courses: []course.Model{{Id: "c1"}}, err: nil}
	s2 := NewService(courseRepo, markRepo)
	if _, err := s2.Query(studentId, ""); !errors.Is(err, wantErr) {
		t.Fatalf("Query(%q, \"\") error = %v, want %v", studentId, err, wantErr)
	}
}
