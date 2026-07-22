package courseadmin

import (
	"context"
	"errors"
	"testing"
	"time"

	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/discordmapping"
)

// fullFakeCourse satisfies the full course.Repository interface.
type fullFakeCourse struct {
	links   map[string]string         // last UpdateCourseLink arg
	courses map[string]course.Model   // for FindCourseById
	err     error
}

func (f *fullFakeCourse) FindCoursesUpdatedAfter(time.Time) ([]course.Model, error) {
	return nil, nil
}
func (f *fullFakeCourse) UpdateCourseRecordCount(string, int) error { return nil }
func (f *fullFakeCourse) FindCoursesManagedByUser(string) ([]course.Model, error) {
	return nil, nil
}
func (f *fullFakeCourse) FindCourseById(courseId string) (course.Model, error) {
	if f.err != nil {
		return course.Model{}, f.err
	}
	if c, ok := f.courses[courseId]; ok {
		return c, nil
	}
	return course.Model{}, course.ErrNotFound
}
func (f *fullFakeCourse) UpdateCourseLink(courseId, link string, _ int64, _ string) error {
	if f.links == nil {
		f.links = map[string]string{}
	}
	f.links[courseId] = link
	return nil
}
func (f *fullFakeCourse) RemoveCourse(string) error { return nil }

type fakeMappingRepo struct {
	saved map[string]discordmapping.Model
}

func (f *fakeMappingRepo) Upsert(m discordmapping.Model) error {
	if f.saved == nil {
		f.saved = map[string]discordmapping.Model{}
	}
	f.saved[m.CourseId] = m
	return nil
}
func (f *fakeMappingRepo) Find(courseId string) (discordmapping.Model, error) {
	if m, ok := f.saved[courseId]; ok {
		return m, nil
	}
	return discordmapping.Model{}, discordmapping.ErrNotFound
}
func (f *fakeMappingRepo) Remove(string) error { return nil }

type fakeImporter struct {
	imported  int
	err       error
	lastLink  string
	lastCours string
}

func (f *fakeImporter) FetchMarkLinkIntoCourse(courseId, link string) (int, error) {
	f.lastLink = link
	f.lastCours = courseId
	if f.err != nil {
		return 0, f.err
	}
	return f.imported, nil
}

type fakeBot struct {
	ensureRoleErr    error
	ensureChannelErr error
}

func (b *fakeBot) EnsureRole(_ context.Context, name string) (string, error) {
	if b.ensureRoleErr != nil {
		return "", b.ensureRoleErr
	}
	return "role:" + name, nil
}
func (b *fakeBot) EnsureChannel(_ context.Context, name, _ string) (string, error) {
	if b.ensureChannelErr != nil {
		return "", b.ensureChannelErr
	}
	return "chan:" + name, nil
}
func (b *fakeBot) AssignRole(context.Context, string, string) error    { return nil }
func (b *fakeBot) RemoveRole(context.Context, string, string) error    { return nil }
func (b *fakeBot) MembersWithRole(context.Context, string) ([]string, error) { return nil, nil }

func rules() *course.Rules { return course.NewRules(&configs.Config{CourseActiveAge: 1}) }

func TestCreate_FullProvision(t *testing.T) {
	cr := &fullFakeCourse{links: map[string]string{}}
	mr := &fakeMappingRepo{}
	imp := &fakeImporter{imported: 42}
	bot := &fakeBot{}
	svc := &Service{courseRepo: cr, mappingRepo: mr, imports: imp, bot: bot, rules: rules()}

	res, err := svc.Create(context.Background(), "CO2003-L01", "https://x.co/m.csv", "admin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Imported != 42 {
		t.Errorf("imported want 42, got %d", res.Imported)
	}
	if res.RoleID != "role:CO2003-L01" {
		t.Errorf("role id: %s", res.RoleID)
	}
	if res.ChannelID != "chan:co2003-l01" {
		t.Errorf("channel id: %s", res.ChannelID)
	}
	if !res.Mapped {
		t.Errorf("expected mapped=true")
	}
	if mr.saved["CO2003-L01"].DiscordRoleId != "role:CO2003-L01" {
		t.Errorf("mapping not saved: %+v", mr.saved)
	}
	if cr.links["CO2003-L01"] != "https://x.co/m.csv" {
		t.Errorf("course link not persisted: %v", cr.links)
	}
}

func TestCreate_RejectsInvalidCourseId(t *testing.T) {
	svc := &Service{rules: rules(), courseRepo: &fullFakeCourse{}, bot: &fakeBot{}, imports: &fakeImporter{}}
	_, err := svc.Create(context.Background(), "bad id!", "https://x.co/m.csv", "a")
	if !errors.Is(err, ErrInvalidCourseId) {
		t.Fatalf("want ErrInvalidCourseId, got %v", err)
	}
}

func TestCreate_RejectsInvalidLink(t *testing.T) {
	svc := &Service{rules: rules(), courseRepo: &fullFakeCourse{}, bot: &fakeBot{}, imports: &fakeImporter{}}
	_, err := svc.Create(context.Background(), "CO2003-L01", "notaurl", "a")
	if !errors.Is(err, ErrInvalidLink) {
		t.Fatalf("want ErrInvalidLink, got %v", err)
	}
}

func TestCreate_PropagatesEnsureRoleError(t *testing.T) {
	svc := &Service{
		rules:      rules(),
		courseRepo: &fullFakeCourse{},
		imports:    &fakeImporter{imported: 1},
		bot:        &fakeBot{ensureRoleErr: errors.New("discord boom")},
	}
	_, err := svc.Create(context.Background(), "CO2003-L01", "https://x.co/m.csv", "a")
	if err == nil || err.Error() != "discord boom" {
		t.Fatalf("want discord boom, got %v", err)
	}
}

func TestSync_ReloadsExistingCourse(t *testing.T) {
	cr := &fullFakeCourse{courses: map[string]course.Model{
		"CO2003-L01": {Id: "CO2003-L01", Link: "https://x.co/m.csv"},
	}}
	imp := &fakeImporter{imported: 7}
	svc := &Service{rules: rules(), courseRepo: cr, imports: imp, bot: &fakeBot{}}
	n, err := svc.Sync(context.Background(), "CO2003-L01", "admin")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if n != 7 {
		t.Errorf("imported want 7, got %d", n)
	}
	if imp.lastLink != "https://x.co/m.csv" {
		t.Errorf("sync should re-import existing link, got %q", imp.lastLink)
	}
}

func TestSync_UnknownCourse(t *testing.T) {
	svc := &Service{rules: rules(), courseRepo: &fullFakeCourse{}, imports: &fakeImporter{}, bot: &fakeBot{}}
	_, err := svc.Sync(context.Background(), "CO9999-L99", "a")
	if !errors.Is(err, course.ErrNotFound) {
		t.Fatalf("want course.ErrNotFound, got %v", err)
	}
}

func TestIsValidURL(t *testing.T) {
	good := []string{"https://x.co/m.csv", "http://example.com/a"}
	bad := []string{"", "notaurl", "ftp://x.co/y", "example.com/x.csv"}
	for _, s := range good {
		if !isValidURL(s) {
			t.Errorf("want valid: %q", s)
		}
	}
	for _, s := range bad {
		if isValidURL(s) {
			t.Errorf("want invalid: %q", s)
		}
	}
}
