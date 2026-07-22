package discord

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/binding"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/mark"
	"thuanle/cse-mark/internal/domain/student"
	"thuanle/cse-mark/internal/usecases/identity"
)

// --- student-facing fakes ---

type stubIdentity struct {
	bound      binding.Model
	boundErr   error
	startErr   error
	verifyRes  identity.BindResult
	verifyErr  error
	startEmail string
	verifyOTP  string
}

func (s *stubIdentity) BindStart(_ context.Context, _ platform, uid, email string) error {
	s.startEmail = email
	_ = uid
	return s.startErr
}
func (s *stubIdentity) BindVerify(_ context.Context, _ platform, _ string, otp string) (identity.BindResult, error) {
	s.verifyOTP = otp
	return s.verifyRes, s.verifyErr
}
func (s *stubIdentity) GetBinding(_ platform, _ string) (binding.Model, error) {
	return s.bound, s.boundErr
}

// platform/identity use string platform values; mirror the interface signature.
// The delivery identityAPI uses plain string args, so redefine a thin type here.
type platform = string

type stubMarkRepo struct {
	marks map[string]string // key courseId|studentId -> blob
	err   error
}

func (m *stubMarkRepo) GetMark(courseId, studentId string) (string, error) {
	if m.marks != nil {
		if v, ok := m.marks[courseId+"|"+studentId]; ok {
			return v, nil
		}
	}
	return "", mark.ErrNotFound
}
func (m *stubMarkRepo) RemoveMarksByCourseId(string) error                       { return nil }
func (m *stubMarkRepo) AddCourseMarks(string, []map[string]string) error         { return nil }
func (m *stubMarkRepo) RemoveCourseMarks(string) error                           { return nil }
func (m *stubMarkRepo) ListStudentIds(string) ([]string, error)                  { return nil, nil }

type stubStudentRepo struct{ byMSSV map[string]student.Model }

func (s *stubStudentRepo) Upsert(student.Model) error                       { return nil }
func (s *stubStudentRepo) FindByEmail(string) (student.Model, error)        { return student.Model{}, student.ErrNotFound }
func (s *stubStudentRepo) FindByMSSV(mssv string) (student.Model, error) {
	if s.byMSSV != nil {
		if v, ok := s.byMSSV[mssv]; ok {
			return v, nil
		}
	}
	return student.Model{}, student.ErrNotFound
}

type stubCourseRepo struct {
	all []course.Model
}

func (c *stubCourseRepo) FindCoursesUpdatedAfter(time.Time) ([]course.Model, error) {
	return c.all, nil
}
func (c *stubCourseRepo) UpdateCourseRecordCount(string, int) error              { return nil }
func (c *stubCourseRepo) FindCoursesManagedByUser(string) ([]course.Model, error) { return nil, nil }
func (c *stubCourseRepo) FindCourseById(string) (course.Model, error)            { return course.Model{}, course.ErrNotFound }
func (c *stubCourseRepo) UpdateCourseLink(string, string, int64, string) error   { return nil }
func (c *stubCourseRepo) RemoveCourse(string) error                              { return nil }

// newStudentSvc builds a Service with student deps wired.
func newStudentSvc(ident identityAPI, mr mark.Repository, sr student.Repository, cr course.Repository) (*Service, *fakeGW) {
	gw := &fakeGW{}
	return &Service{
		cfg:         &configs.Config{DiscordAdminIds: []string{"a"}},
		gw:          gw,
		admin:       &adminChecker{ids: []string{"a"}},
		identity:    ident,
		markRepo:    mr,
		studentRepo: sr,
		courseRepo:  cr,
	}, gw
}

func TestBind_OpensEmailModal(t *testing.T) {
	ident := &stubIdentity{boundErr: binding.ErrNotFound}
	s, gw := newStudentSvc(ident, nil, nil, nil)
	s.handleBind(cmdInteraction("u1", "bind"))
	if len(gw.responds) != 1 {
		t.Fatal("want 1 respond")
	}
	if gw.responds[0].resp.Type != discordgo.InteractionResponseModal {
		t.Errorf("want modal, got type %v", gw.responds[0].resp.Type)
	}
}

func TestBind_AlreadyBound(t *testing.T) {
	ident := &stubIdentity{bound: binding.Model{MSSV: "2212345", Verified: true}}
	s, gw := newStudentSvc(ident, nil, nil, nil)
	s.handleBind(cmdInteraction("u1", "bind"))
	c := gw.responds[0].resp.Data.Content
	if !strings.Contains(c, "2212345") {
		t.Errorf("should report existing bind: %q", c)
	}
}

func TestProfile_NotBound(t *testing.T) {
	ident := &stubIdentity{boundErr: binding.ErrNotFound}
	s, gw := newStudentSvc(ident, &stubMarkRepo{}, &stubStudentRepo{}, &stubCourseRepo{})
	s.handleProfile(cmdInteraction("u1", "profile"))
	c := gw.responds[0].resp.Data.Content
	if !strings.Contains(c, "Chưa xác thực") {
		t.Errorf("want not-bound message: %q", c)
	}
}

func TestProfile_ShowsMSSVAndClasses(t *testing.T) {
	ident := &stubIdentity{bound: binding.Model{MSSV: "2212345", Verified: true}}
	sr := &stubStudentRepo{byMSSV: map[string]student.Model{"2212345": {MSSV: "2212345", Name: "SV", Email: "sv@x"}}}
	mr := &stubMarkRepo{marks: map[string]string{"CO2003-L01|2212345": "X"}}
	cr := &stubCourseRepo{all: []course.Model{{Id: "CO2003-L01"}, {Id: "CO9999"}}}
	s, gw := newStudentSvc(ident, mr, sr, cr)

	s.handleProfile(cmdInteraction("u1", "profile"))
	c := gw.responds[0].resp.Data.Content
	for _, want := range []string{"2212345", "SV", "CO2003-L01"} {
		if !strings.Contains(c, want) {
			t.Errorf("profile missing %q: %s", want, c)
		}
	}
	if strings.Contains(c, "CO9999") {
		t.Errorf("non-enrolled course should not appear: %s", c)
	}
}

func TestMark_SingleCourse(t *testing.T) {
	ident := &stubIdentity{bound: binding.Model{MSSV: "2212345", Verified: true}}
	mr := &stubMarkRepo{marks: map[string]string{"CO2003-L01|2212345": `{ "Lab 1": "10" }`}}
	s, gw := newStudentSvc(ident, mr, nil, nil)

	i := cmdInteraction("u1", "mark", optStr(optCourse, "CO2003-L01"))
	s.handleMark(i)
	c := gw.responds[0].resp.Data.Content
	if !strings.Contains(c, "CO2003-L01") {
		t.Errorf("mark should name course: %s", c)
	}
	if gw.responds[0].resp.Data.Flags&discordgo.MessageFlagsEphemeral == 0 {
		t.Error("mark must be ephemeral")
	}
}

func TestMark_NotBound(t *testing.T) {
	ident := &stubIdentity{boundErr: binding.ErrNotFound}
	s, gw := newStudentSvc(ident, &stubMarkRepo{}, nil, nil)
	s.handleMark(cmdInteraction("u1", "mark"))
	if !strings.Contains(gw.responds[0].resp.Data.Content, "Chưa xác thực") {
		t.Errorf("want not-bound: %s", gw.responds[0].resp.Data.Content)
	}
}

func TestMark_MissingForCourse(t *testing.T) {
	ident := &stubIdentity{bound: binding.Model{MSSV: "2212345", Verified: true}}
	mr := &stubMarkRepo{} // no marks
	s, gw := newStudentSvc(ident, mr, nil, nil)
	s.handleMark(cmdInteraction("u1", "mark", optStr(optCourse, "CO2003-L01")))
	if !strings.Contains(gw.responds[0].resp.Data.Content, "Chưa có điểm") {
		t.Errorf("want no-marks message: %s", gw.responds[0].resp.Data.Content)
	}
}

func TestBindStartMsg_KnownErrors(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{identity.ErrEmailNotInRoster, "roster"},
		{identity.ErrResendCooldown, "đợi"},
		{errors.New("other"), "Không thể gửi"},
	}
	for _, tc := range cases {
		if !strings.Contains(bindStartMsg(tc.err), tc.want) {
			t.Errorf("for %v: want %q in %q", tc.err, tc.want, bindStartMsg(tc.err))
		}
	}
}

func TestCollapseJSON(t *testing.T) {
	in := `{
  "Lab 1": "10",
  "Lab 2": "9"
}`
	out := collapseJSON(in)
	if strings.Contains(out, "{") || strings.Contains(out, "}") {
		t.Errorf("braces should be stripped: %q", out)
	}
	if !strings.Contains(out, "Lab 1") {
		t.Errorf("data lost: %q", out)
	}
}
