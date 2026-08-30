package handlers

import (
	"errors"
	"strings"
	"testing"
	"time"

	"thuanle/cse-mark/internal/delivery/tele/models"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/usecases/markimport"
)

// teacherCourseRepo satisfies the full course.Repository interface and records
// SetCourseStatus calls for the /sync re-enable assertions (#43).
type teacherCourseRepo struct {
	courses     map[string]course.Model
	statusIds   []string
	statusOn    []course.Status
	recordCntOn []string
}

func (f *teacherCourseRepo) FindCoursesUpdatedAfter(time.Time) ([]course.Model, error) {
	return nil, nil
}
func (f *teacherCourseRepo) UpdateCourseRecordCount(courseId string, _ int) error {
	f.recordCntOn = append(f.recordCntOn, courseId)
	return nil
}
func (f *teacherCourseRepo) FindCoursesManagedByUser(string) ([]course.Model, error) {
	return nil, nil
}
func (f *teacherCourseRepo) FindCourseById(courseId string) (course.Model, error) {
	if c, ok := f.courses[courseId]; ok {
		return c, nil
	}
	return course.Model{}, course.ErrNotFound
}
func (f *teacherCourseRepo) UpdateCourseLink(string, string, int64, string) error { return nil }
func (f *teacherCourseRepo) RemoveCourse(string) error                            { return nil }
func (f *teacherCourseRepo) SetCourseStatus(courseId string, status course.Status) error {
	f.statusIds = append(f.statusIds, courseId)
	f.statusOn = append(f.statusOn, status)
	return nil
}
func (f *teacherCourseRepo) FindSyncableCourses(time.Time) ([]course.Model, error) {
	return nil, nil
}

// fakeFeedDownloader stands in for the (gv-proxy) CSV fetcher inside the real
// markimport.Service wired below.
type fakeFeedDownloader struct {
	records [][]string
	err     error
	lastURL string
}

func (f *fakeFeedDownloader) DownloadCSVAuthorized(url string, _ string) ([][]string, error) {
	f.lastURL = url
	if f.err != nil {
		return nil, f.err
	}
	return f.records, nil
}

// newSyncTeacher builds a Teacher handler against a real markimport.Service
// backed by fakes; renderer and authz are nil because /sync never touches them.
func newSyncTeacher(cr *teacherCourseRepo, dl *fakeFeedDownloader) *Teacher {
	mis := markimport.NewService(dl, cr, &fakeMarkRepo{}, "")
	return NewTeacherHandler(cr, &course.Rules{}, nil, nil, &fakeMarkRepo{}, mis)
}

func syncCtx(courseId string) *fakeCtx {
	c := privateCtx(7, 42)
	c.args = []string{courseId}
	return c
}

// Issue #43: /sync re-enables a stale course (status active) and refetches its
// stored link, replying with the record count.
func TestSyncCourse_ReenablesAndFetches(t *testing.T) {
	cr := &teacherCourseRepo{courses: map[string]course.Model{
		"CO2003-L01": {Id: "CO2003-L01", Link: "https://x.co/m.csv", Status: course.StatusStale},
	}}
	dl := &fakeFeedDownloader{records: [][]string{{"id"}, {"name"}, {"s1", "Alice"}}}
	h := newSyncTeacher(cr, dl)
	c := syncCtx("CO2003-L01")

	if err := h.SyncCourse(c); err != nil {
		t.Fatalf("SyncCourse: %v", err)
	}
	if len(cr.statusOn) != 1 || cr.statusIds[0] != "CO2003-L01" || cr.statusOn[0] != course.StatusActive {
		t.Fatalf("SetCourseStatus = %v/%v, want [CO2003-L01]/[active]", cr.statusIds, cr.statusOn)
	}
	if dl.lastURL != "https://x.co/m.csv" {
		t.Fatalf("fetched %q, want the stored course link", dl.lastURL)
	}
	if len(cr.recordCntOn) != 1 || cr.recordCntOn[0] != "CO2003-L01" {
		t.Fatalf("import did not update record count: %v", cr.recordCntOn)
	}
	if len(c.sent) != 1 || !contains("synced 1 records", c.sent[0]) {
		t.Fatalf("want reply with record count, got %v", c.sent)
	}
}

func TestSyncCourse_InvalidCourseId(t *testing.T) {
	h := newSyncTeacher(&teacherCourseRepo{}, &fakeFeedDownloader{})

	err := h.SyncCourse(syncCtx("123")) // must start with a letter
	var av *models.ArgValueMismatchError
	if !errors.As(err, &av) {
		t.Fatalf("want ArgValueMismatchError, got %v", err)
	}
}

func TestSyncCourse_UnknownCourse(t *testing.T) {
	h := newSyncTeacher(&teacherCourseRepo{}, &fakeFeedDownloader{})

	if err := h.SyncCourse(syncCtx("CO9999-L01")); !errors.Is(err, course.ErrNotFound) {
		t.Fatalf("want course.ErrNotFound, got %v", err)
	}
}

// A course whose stored link is not a valid URI is refused before the status
// flips: re-enabling a broken course would just point the poller at a bad feed.
func TestSyncCourse_RejectsInvalidStoredLink(t *testing.T) {
	cr := &teacherCourseRepo{courses: map[string]course.Model{
		"CO2003-L01": {Id: "CO2003-L01", Link: "notaurl"},
	}}
	h := newSyncTeacher(cr, &fakeFeedDownloader{})

	if err := h.SyncCourse(syncCtx("CO2003-L01")); err == nil {
		t.Fatal("want error for invalid stored link")
	}
	if len(cr.statusOn) != 0 {
		t.Fatalf("status must not flip on invalid link, got %v", cr.statusOn)
	}
}

// Issue #43 (final review M-1): a malformed stored link makes
// url.ParseRequestURI return a *url.Error whose text embeds the full link (a
// proxy token may sit in its path/query). The handler must return a link-free
// sentinel — the middleware echoes "Error: <err>" into the chat.
func TestSyncCourse_InvalidStoredLinkRedacted(t *testing.T) {
	cr := &teacherCourseRepo{courses: map[string]course.Model{
		"CO2003-L01": {Id: "CO2003-L01", Link: "notaurl?token=SUPERSECRET"},
	}}
	h := newSyncTeacher(cr, &fakeFeedDownloader{})

	err := h.SyncCourse(syncCtx("CO2003-L01"))
	if err == nil {
		t.Fatal("want error for malformed stored link")
	}
	if strings.Contains(err.Error(), "SUPERSECRET") {
		t.Fatalf("err leaks stored link: %v", err)
	}
	if !errors.Is(err, models.ErrStoredLinkInvalid) {
		t.Fatalf("want models.ErrStoredLinkInvalid, got %v", err)
	}
	if len(cr.statusOn) != 0 {
		t.Fatalf("status must not flip on invalid link, got %v", cr.statusOn)
	}
}
