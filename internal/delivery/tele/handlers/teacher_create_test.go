package handlers

import (
	"strings"
	"testing"

	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/usecases/iam"
	"thuanle/cse-mark/internal/usecases/markimport"
)

// newCreateTeacher is newSyncTeacher with a real (fake-backed) authz service:
// /create (LoadCourseLink) checks CanEditCourse before writing, /sync does not.
func newCreateTeacher(cr *teacherCourseRepo, dl *fakeFeedDownloader) *Teacher {
	mis := markimport.NewService(dl, cr, &fakeMarkRepo{}, "")
	return NewTeacherHandler(cr, &course.Rules{}, nil, iam.NewAuthzService(cr, nil), &fakeMarkRepo{}, mis)
}

func createCtx(courseId, link string) *fakeCtx {
	c := privateCtx(7, 42)
	c.args = []string{courseId, link}
	return c
}

// Issue #43 (final review I-1): /create on an inactive course must re-activate
// it — the fresh link is the admin's intent to bring the course back, and
// without the status flip the poller keeps skipping it (FindSyncableCourses
// filters inactive).
func TestLoadCourseLink_ReactivesInactiveCourse(t *testing.T) {
	cr := &teacherCourseRepo{courses: map[string]course.Model{
		// ByTeleId matches the chat id (7) so CanEditCourse grants.
		"CO2003-L01": {Id: "CO2003-L01", Link: "https://old.co/m.csv", ByTeleId: 7, Status: course.StatusInactive},
	}}
	dl := &fakeFeedDownloader{records: [][]string{{"id"}, {"name"}, {"s1", "Alice"}}}
	h := newCreateTeacher(cr, dl)
	c := createCtx("CO2003-L01", "https://x.co/m.csv")

	if err := h.LoadCourseLink(c); err != nil {
		t.Fatalf("LoadCourseLink: %v", err)
	}
	if len(cr.statusOn) != 1 || cr.statusIds[0] != "CO2003-L01" || cr.statusOn[0] != course.StatusActive {
		t.Fatalf("want SetCourseStatus(CO2003-L01, active) after link update, got ids=%v statuses=%v", cr.statusIds, cr.statusOn)
	}
	if len(c.sent) != 1 || !contains("Store 1 records", c.sent[0]) {
		t.Fatalf("want reply with record count, got %v", c.sent)
	}
}

// Issue #43 (final review M-1): a malformed link argument makes
// url.ParseRequestURI return a *url.Error whose text embeds the full link (a
// proxy token may sit in its path/query). The handler must return a link-free
// error — the middleware echoes "Error: <err>" into the chat.
func TestLoadCourseLink_InvalidLinkArgRedacted(t *testing.T) {
	h := newCreateTeacher(&teacherCourseRepo{}, &fakeFeedDownloader{})

	err := h.LoadCourseLink(createCtx("CO2003-L01", "notaurl?token=SUPERSECRET"))
	if err == nil {
		t.Fatal("want error for malformed link argument")
	}
	if strings.Contains(err.Error(), "SUPERSECRET") {
		t.Fatalf("err leaks link payload: %v", err)
	}
}
