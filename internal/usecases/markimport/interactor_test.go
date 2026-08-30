package markimport

import (
	"testing"
	"time"

	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/downloader"
	"thuanle/cse-mark/internal/domain/mark"
)

type fakeAuthDownloader struct {
	records   [][]string
	err       error
	lastToken string
}

func (f *fakeAuthDownloader) DownloadCSVAuthorized(_ string, token string) ([][]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.lastToken = token
	return f.records, nil
}

// fakeCourseRepo satisfies the full course.Repository interface (T2 shape:
// includes SetCourseStatus + FindSyncableCourses).
type fakeCourseRepo struct {
	statusSetOn   []string // courseIds passed to SetCourseStatus
	recordCntOn   string
	recordCntCall bool
}

func (f *fakeCourseRepo) FindCoursesUpdatedAfter(time.Time) ([]course.Model, error) {
	return nil, nil
}
func (f *fakeCourseRepo) UpdateCourseRecordCount(courseId string, _ int) error {
	f.recordCntOn = courseId
	f.recordCntCall = true
	return nil
}
func (f *fakeCourseRepo) FindCoursesManagedByUser(string) ([]course.Model, error) {
	return nil, nil
}
func (f *fakeCourseRepo) FindCourseById(string) (course.Model, error) {
	return course.Model{}, course.ErrNotFound
}
func (f *fakeCourseRepo) UpdateCourseLink(string, string, int64, string) error { return nil }
func (f *fakeCourseRepo) RemoveCourse(string) error                            { return nil }
func (f *fakeCourseRepo) SetCourseStatus(courseId string, _ course.Status) error {
	f.statusSetOn = append(f.statusSetOn, courseId)
	return nil
}
func (f *fakeCourseRepo) FindSyncableCourses(time.Time) ([]course.Model, error) {
	return nil, nil
}

// fakeMarkRepo satisfies the full mark.Repository interface.
type fakeMarkRepo struct {
	removed   []string
	addedOn   string
	addedRows []map[string]string
	addErr    error
}

func (f *fakeMarkRepo) GetMark(string, string) (string, error) { return "", mark.ErrNotFound }
func (f *fakeMarkRepo) RemoveMarksByCourseId(courseId string) error {
	f.removed = append(f.removed, courseId)
	return nil
}
func (f *fakeMarkRepo) AddCourseMarks(courseId string, marks []map[string]string) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.addedOn = courseId
	f.addedRows = marks
	return nil
}
func (f *fakeMarkRepo) RemoveCourseMarks(string) error { return nil }
func (f *fakeMarkRepo) ListStudentIds(string) ([]string, error) { return nil, nil }

func newSvc(dl downloader.AuthorizedRepository, mk mark.Repository) *Service {
	return NewService(dl, &fakeCourseRepo{}, mk, "")
}

func TestFetchMarkLink_FetchFailKeepsOldMarks(t *testing.T) {
	mk := &fakeMarkRepo{}
	svc := newSvc(&fakeAuthDownloader{err: &downloader.FeedError{Status: 401, Code: "service_token_invalid"}}, mk)
	_, err := svc.FetchMarkLinkIntoCourse("c1", "https://x/f")
	if err == nil {
		t.Fatal("want error")
	}
	if len(mk.removed) != 0 {
		t.Fatalf("RemoveMarksByCourseId called on fetch failure — old marks would be wiped: %v", mk.removed)
	}
}

func TestFetchMarkLink_ParseFailKeepsOldMarks(t *testing.T) {
	mk := &fakeMarkRepo{}
	svc := newSvc(&fakeAuthDownloader{records: [][]string{{"id"}}}, mk) // 1 dòng < 2 → invalid csv structure
	if _, err := svc.FetchMarkLinkIntoCourse("c1", "https://x/f"); err == nil {
		t.Fatal("want parse error")
	}
	if len(mk.removed) != 0 {
		t.Fatalf("RemoveMarksByCourseId called on parse failure: %v", mk.removed)
	}
}

func TestFetchMarkLink_HappyPathReplaces(t *testing.T) {
	mk := &fakeMarkRepo{}
	svc := newSvc(&fakeAuthDownloader{records: [][]string{{"id"}, {"name"}, {"s1", "Alice"}}}, mk)
	n, err := svc.FetchMarkLinkIntoCourse("c1", "https://x/f")
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(mk.removed) != 1 || mk.removed[0] != "c1" {
		t.Fatalf("removed = %v, want [c1]", mk.removed)
	}
	if len(mk.addedRows) != 1 {
		t.Fatalf("added rows = %d, want 1", len(mk.addedRows))
	}
}

func TestFetchMarkLink_ForwardsGvProxyToken(t *testing.T) {
	dl := &fakeAuthDownloader{records: [][]string{{"id"}, {"name"}, {"s1", "Alice"}}}
	svc := NewService(dl, &fakeCourseRepo{}, &fakeMarkRepo{}, "gv-secret")
	if _, err := svc.FetchMarkLinkIntoCourse("c1", "https://x/f"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if dl.lastToken != "gv-secret" {
		t.Fatalf("token forwarded = %q, want %q", dl.lastToken, "gv-secret")
	}
}
