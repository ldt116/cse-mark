package handlers

import (
	"testing"

	"thuanle/cse-mark/internal/domain/binding"
	"thuanle/cse-mark/internal/domain/course"
)

type fakeMarkRepo struct {
	mark   string
	getErr error

	gotCourse  string
	gotStudent string
}

func (f *fakeMarkRepo) GetMark(courseId, studentId string) (string, error) {
	f.gotCourse, f.gotStudent = courseId, studentId
	return f.mark, f.getErr
}
func (f *fakeMarkRepo) RemoveMarksByCourseId(string) error               { return nil }
func (f *fakeMarkRepo) AddCourseMarks(string, []map[string]string) error { return nil }
func (f *fakeMarkRepo) RemoveCourseMarks(string) error                   { return nil }
func (f *fakeMarkRepo) ListStudentIds(string) ([]string, error)          { return nil, nil }

// Issue #38: identity-resolved /mark must resolve by sender and be DM-only —
// a group reply would expose the sender's MSSV binding to everyone.

func TestGetMark_GroupChatDirectedToDM(t *testing.T) {
	ident := &fakeIdentity{existing: binding.Model{MSSV: "2013307", Verified: true}}
	mr := &fakeMarkRepo{mark: "HT: 9.0"}
	g := NewGuestHandler(&course.Rules{}, mr, WithGuestIdentity(ident))
	c := groupCtx(42)
	c.args = []string{"CO2003"}

	if err := g.GetMark(c); err != nil {
		t.Fatal(err)
	}
	if ident.bindingKey != "" {
		t.Fatalf("must not resolve identity in a group, resolved %q", ident.bindingKey)
	}
	if mr.gotStudent != "" {
		t.Fatalf("must not query marks in a group, queried %q", mr.gotStudent)
	}
	if len(c.sent) != 1 || !contains("nhắn riêng", c.sent[0]) {
		t.Fatalf("want DM-only notice, got %v", c.sent)
	}
}

func TestGetMark_PrivateResolvesBySender(t *testing.T) {
	ident := &fakeIdentity{existing: binding.Model{MSSV: "2013307", Verified: true}}
	mr := &fakeMarkRepo{mark: "HT: 9.0"}
	g := NewGuestHandler(&course.Rules{}, mr, WithGuestIdentity(ident))
	// chat id 7 ≠ sender id 42: proves the binding lookup uses the sender.
	c := privateCtx(7, 42)
	c.args = []string{"CO2003"}

	if err := g.GetMark(c); err != nil {
		t.Fatal(err)
	}
	if ident.bindingKey != "42" {
		t.Fatalf("GetBinding key = %q, want sender 42", ident.bindingKey)
	}
	if mr.gotCourse != "CO2003" || mr.gotStudent != "2013307" {
		t.Fatalf("GetMark(%q,%q), want (CO2003,2013307)", mr.gotCourse, mr.gotStudent)
	}
	if len(c.sent) != 1 || !contains("9.0", c.sent[0]) {
		t.Fatalf("want marks reply, got %v", c.sent)
	}
}
