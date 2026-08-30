package marksync

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/downloader"
	"thuanle/cse-mark/internal/domain/mark"
	"thuanle/cse-mark/internal/usecases/coursequery"
	"thuanle/cse-mark/internal/usecases/markimport"
)

// fakeAuthDownloader returns canned records or a canned error, counting calls
// so tests can assert that the slow-poll window suppressed a fetch.
type fakeAuthDownloader struct {
	err     error
	records [][]string
	calls   int
}

func (f *fakeAuthDownloader) DownloadCSVAuthorized(_ string, _ string) ([][]string, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.records, nil
}

// fakeCourseRepo satisfies the full course.Repository interface and serves
// FindSyncableCourses (consumed by the real coursequery service) while
// recording SetCourseStatus calls with the exact status passed.
type fakeCourseRepo struct {
	courses   []course.Model
	statusIds []string
	statuses  []course.Status
}

func (f *fakeCourseRepo) FindCoursesUpdatedAfter(time.Time) ([]course.Model, error) {
	return nil, nil
}
func (f *fakeCourseRepo) UpdateCourseRecordCount(string, int) error { return nil }
func (f *fakeCourseRepo) FindCoursesManagedByUser(string) ([]course.Model, error) {
	return nil, nil
}
func (f *fakeCourseRepo) FindCourseById(string) (course.Model, error) {
	return course.Model{}, course.ErrNotFound
}
func (f *fakeCourseRepo) UpdateCourseLink(string, string, int64, string) error { return nil }
func (f *fakeCourseRepo) RemoveCourse(string) error                            { return nil }
func (f *fakeCourseRepo) SetCourseStatus(courseId string, status course.Status) error {
	f.statusIds = append(f.statusIds, courseId)
	f.statuses = append(f.statuses, status)
	return nil
}
func (f *fakeCourseRepo) FindSyncableCourses(time.Time) ([]course.Model, error) {
	return f.courses, nil
}

// fakeMarkRepo satisfies the full mark.Repository interface; addCalls counts
// successful imports (proof that a fetch actually reached the store).
type fakeMarkRepo struct {
	addCalls int
	removed  []string
}

func (f *fakeMarkRepo) GetMark(string, string) (string, error) { return "", mark.ErrNotFound }
func (f *fakeMarkRepo) RemoveMarksByCourseId(courseId string) error {
	f.removed = append(f.removed, courseId)
	return nil
}
func (f *fakeMarkRepo) AddCourseMarks(_ string, _ []map[string]string) error {
	f.addCalls++
	return nil
}
func (f *fakeMarkRepo) RemoveCourseMarks(string) error          { return nil }
func (f *fakeMarkRepo) ListStudentIds(string) ([]string, error) { return nil, nil }

var validRecords = [][]string{{"id"}, {"name"}, {"s1", "Alice"}}

// newTestService wires the REAL markimport service against fake ports and the
// REAL coursequery service, with fetchingInterval = 0 so fetchNewMarks never
// sleeps in tests.
func newTestService(dl *fakeAuthDownloader, cr *fakeCourseRepo, mr *fakeMarkRepo) *Service {
	importer := markimport.NewService(dl, cr, mr, "gv-test-token")
	svc := NewService(
		coursequery.NewActiveCourseService(cr, &course.Rules{CourseActiveAge: 30 * 24 * time.Hour}),
		cr, importer)
	svc.fetchingInterval = 0
	return svc
}

func TestSyncCourse_ClassificationAndTransitions(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name          string
		dlErr         error      // error returned by the downloader (nil → records used)
		records       [][]string // records returned when dlErr is nil
		startStatus   course.Status
		seedCounter   int           // consecutiveFailures before the sync
		wantStatus    course.Status // "" = SetCourseStatus must NOT be called
		wantNextDelay bool          // nextAttemptAt == now + 1h after the sync
		wantCounter   int
	}{
		{
			name:          "401 service_token_invalid is config-token: no status change, slow-poll, counter untouched",
			dlErr:         &downloader.FeedError{Status: http.StatusUnauthorized, Code: "service_token_invalid"},
			seedCounter:   2,
			wantCounter:   2,
			wantNextDelay: true,
		},
		{
			name:          "403 owner_token_invalid marks course stale",
			dlErr:         &downloader.FeedError{Status: http.StatusForbidden, Code: "owner_token_invalid"},
			wantStatus:    course.StatusStale,
			wantNextDelay: true,
		},
		{
			name:          "403 sheet_access_denied marks course stale",
			dlErr:         &downloader.FeedError{Status: http.StatusForbidden, Code: "sheet_access_denied"},
			wantStatus:    course.StatusStale,
			wantNextDelay: true,
		},
		{
			name:          "404 sheet_not_found marks course stale",
			dlErr:         &downloader.FeedError{Status: http.StatusNotFound, Code: "sheet_not_found"},
			wantStatus:    course.StatusStale,
			wantNextDelay: true,
		},
		{
			name:       "410 grant_revoked marks course inactive without re-probe delay",
			dlErr:      &downloader.FeedError{Status: http.StatusGone, Code: "grant_revoked"},
			wantStatus: course.StatusInactive,
		},
		{
			name:        "500 is transient: no status change, no delay, counter++",
			dlErr:       &downloader.FeedError{Status: http.StatusInternalServerError, Code: "internal_error"},
			wantCounter: 1,
		},
		{
			name:        "network error is transient",
			dlErr:       errors.New("dial tcp: connection refused"),
			wantCounter: 1,
		},
		{
			name:        "parse failure (1-row csv) is transient",
			records:     [][]string{{"id"}},
			wantCounter: 1,
		},
		{
			name:        "success heals stale course and resets counter",
			records:     validRecords,
			startStatus: course.StatusStale,
			seedCounter: 3,
			wantStatus:  course.StatusActive,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dl := &fakeAuthDownloader{err: tc.dlErr, records: tc.records}
			cr := &fakeCourseRepo{}
			mr := &fakeMarkRepo{}
			svc := newTestService(dl, cr, mr)
			if tc.seedCounter != 0 {
				svc.consecutiveFailures["c1"] = tc.seedCounter
			}

			svc.syncCourse(course.Model{Id: "c1", Link: "https://x/f", Status: tc.startStatus}, now)

			if tc.wantStatus == "" {
				if len(cr.statuses) != 0 {
					t.Fatalf("SetCourseStatus called with %v, want no call", cr.statuses)
				}
			} else if len(cr.statuses) != 1 || cr.statuses[0] != tc.wantStatus || cr.statusIds[0] != "c1" {
				t.Fatalf("SetCourseStatus = %v (ids %v), want [%s] on c1", cr.statuses, cr.statusIds, tc.wantStatus)
			}

			next, ok := svc.nextAttemptAt["c1"]
			if tc.wantNextDelay {
				if !ok || !next.Equal(now.Add(time.Hour)) {
					t.Fatalf("nextAttemptAt[c1] = %v (ok=%v), want %v", next, ok, now.Add(time.Hour))
				}
			} else if !next.IsZero() {
				t.Fatalf("nextAttemptAt[c1] = %v, want none", next)
			}

			if got := svc.consecutiveFailures["c1"]; got != tc.wantCounter {
				t.Fatalf("consecutiveFailures[c1] = %d, want %d", got, tc.wantCounter)
			}
		})
	}
}

func TestSyncCourse_TransientThresholdWarnsOnce(t *testing.T) {
	var buf bytes.Buffer
	orig := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = orig })

	unhealthyCount := func() int { return strings.Count(buf.String(), "feed unhealthy") }

	dl := &fakeAuthDownloader{err: &downloader.FeedError{Status: http.StatusInternalServerError}}
	svc := newTestService(dl, &fakeCourseRepo{}, &fakeMarkRepo{})
	c := course.Model{Id: "c1", Link: "https://x/f"}
	base := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

	// Fails 1-5 stay below the threshold: no warning yet.
	for i := 1; i <= 5; i++ {
		svc.syncCourse(c, base.Add(time.Duration(i)*time.Minute))
		if got := unhealthyCount(); got != 0 {
			t.Fatalf("after fail #%d: \"feed unhealthy\" count = %d, want 0 (below threshold)", i, got)
		}
	}
	// The 6th consecutive transient failure warns exactly once.
	svc.syncCourse(c, base.Add(6*time.Minute))
	if got := unhealthyCount(); got != 1 {
		t.Fatalf("after fail #6: \"feed unhealthy\" count = %d, want 1", got)
	}
	// 7th failure must NOT warn again.
	svc.syncCourse(c, base.Add(7*time.Minute))
	if got := unhealthyCount(); got != 1 {
		t.Fatalf("after fail #7: \"feed unhealthy\" count = %d, want 1", got)
	}

	// Success resets the streak.
	dl.err = nil
	dl.records = validRecords
	svc.syncCourse(c, base.Add(8*time.Minute))
	if got := unhealthyCount(); got != 1 {
		t.Fatalf("after success: \"feed unhealthy\" count = %d, want 1", got)
	}
	if svc.consecutiveFailures["c1"] != 0 {
		t.Fatalf("consecutiveFailures after success = %d, want 0", svc.consecutiveFailures["c1"])
	}

	// 6 more failures → second warning, exactly once more.
	dl.err = &downloader.FeedError{Status: http.StatusInternalServerError}
	for i := 1; i <= 5; i++ {
		svc.syncCourse(c, base.Add(time.Duration(8+i)*time.Minute))
		if got := unhealthyCount(); got != 1 {
			t.Fatalf("post-reset fail #%d: \"feed unhealthy\" count = %d, want 1", i, got)
		}
	}
	svc.syncCourse(c, base.Add(14*time.Minute))
	if got := unhealthyCount(); got != 2 {
		t.Fatalf("after 6 more fails: \"feed unhealthy\" count = %d, want 2", got)
	}
}

func TestFetchNewMarks_SkipsCourseInsideSlowPollWindow(t *testing.T) {
	dl := &fakeAuthDownloader{err: &downloader.FeedError{Status: http.StatusUnauthorized, Code: "service_token_invalid"}}
	cr := &fakeCourseRepo{courses: []course.Model{{Id: "c1", Link: "https://x/f"}}}
	mr := &fakeMarkRepo{}
	svc := newTestService(dl, cr, mr)

	// First sync hits the config-token error → nextAttemptAt[c1] = now + 1h.
	svc.syncCourse(cr.courses[0], time.Now())
	if dl.calls != 1 {
		t.Fatalf("setup: downloader calls = %d, want 1", dl.calls)
	}

	// The next poll cycle lands inside the 1h window → no fetch at all.
	svc.fetchNewMarks()

	if dl.calls != 1 {
		t.Fatalf("downloader calls = %d, want 1 (course inside slow-poll window must not be fetched)", dl.calls)
	}
	if mr.addCalls != 0 {
		t.Fatalf("AddCourseMarks calls = %d, want 0", mr.addCalls)
	}
}
