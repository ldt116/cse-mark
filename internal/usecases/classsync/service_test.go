package classsync

import (
	"context"
	"errors"
	"sync"
	"testing"

	"thuanle/cse-mark/internal/domain/binding"
	"thuanle/cse-mark/internal/domain/discordmapping"
	"thuanle/cse-mark/internal/domain/mark"
)

// --- fakes ---

type fakeMappings struct {
	all []discordmapping.Model
}

func (f *fakeMappings) Upsert(discordmapping.Model) error { return nil }
func (f *fakeMappings) Find(string) (discordmapping.Model, error) {
	return discordmapping.Model{}, discordmapping.ErrNotFound
}
func (f *fakeMappings) Remove(string) error { return nil }
func (f *fakeMappings) ListAll() ([]discordmapping.Model, error) {
	return f.all, nil
}

type fakeMarks struct {
	students map[string][]string // courseId -> mssvs
}

func (f *fakeMarks) GetMark(string, string) (string, error)                 { return "", mark.ErrNotFound }
func (f *fakeMarks) RemoveMarksByCourseId(string) error                     { return nil }
func (f *fakeMarks) AddCourseMarks(string, []map[string]string) error       { return nil }
func (f *fakeMarks) RemoveCourseMarks(string) error                         { return nil }
func (f *fakeMarks) ListStudentIds(courseId string) ([]string, error) {
	return f.students[courseId], nil
}

// fakeBindings holds MSSV -> discord user id (only verified discord bindings).
type fakeBindings struct {
	mu  sync.Mutex
	all map[string]string // mssv -> discordUserID
}

func (f *fakeBindings) DiscordUserID(mssv string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if uid, ok := f.all[mssv]; ok {
		return uid, nil
	}
	return "", nil
}

// recorderBot tracks AssignRole/RemoveRole calls and serves MembersWithRole.
type recorderBot struct {
	mu           sync.Mutex
	members      map[string][]string // roleID -> current member userIDs
	assigned     []string            // roleID per successful assign call
	removed      []string            // roleID per successful remove call
	assignErr    error
	assignTried  []string // every assign attempt (success or failure), for cycle-resilience tests
	removeTried  []string
}

func (b *recorderBot) EnsureRole(context.Context, string) (string, error)    { return "", nil }
func (b *recorderBot) EnsureChannel(context.Context, string, string) (string, error) {
	return "", nil
}
func (b *recorderBot) AssignRole(_ context.Context, userID, roleID string) error {
	b.mu.Lock()
	b.assignTried = append(b.assignTried, roleID+":"+userID)
	b.mu.Unlock()
	if b.assignErr != nil {
		return b.assignErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.assigned = append(b.assigned, roleID+":"+userID)
	b.members[roleID] = append(b.members[roleID], userID)
	return nil
}
func (b *recorderBot) RemoveRole(_ context.Context, userID, roleID string) error {
	b.mu.Lock()
	b.removeTried = append(b.removeTried, roleID+":"+userID)
	defer b.mu.Unlock()
	b.removed = append(b.removed, roleID+":"+userID)
	var next []string
	for _, u := range b.members[roleID] {
		if u != userID {
			next = append(next, u)
		}
	}
	b.members[roleID] = next
	return nil
}
func (b *recorderBot) MembersWithRole(_ context.Context, roleID string) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.members[roleID]))
	copy(out, b.members[roleID])
	return out, nil
}

// --- tests ---

func TestReconcile_AddsEnrolledRemovesUnenrolled(t *testing.T) {
	maps := &fakeMappings{all: []discordmapping.Model{{CourseId: "C1", DiscordRoleId: "r1"}}}
	marks := &fakeMarks{students: map[string][]string{"C1": {"m1", "m2"}}}
	bnd := &fakeBindings{all: map[string]string{"m1": "u1", "m2": "u2"}}
	bot := &recorderBot{members: map[string][]string{"r1": {"u9"}}} // u9 is stale

	s := NewService(maps, marks, bnd, bot, 0)
	s.ReconcileOnce(context.Background())

	// u1, u2 should be assigned; u9 removed.
	if !contains(bot.assigned, "r1:u1") || !contains(bot.assigned, "r1:u2") {
		t.Errorf("expected u1,u2 assigned; got %v", bot.assigned)
	}
	if !contains(bot.removed, "r1:u9") {
		t.Errorf("expected u9 removed; got %v", bot.removed)
	}
}

func TestReconcile_Idempotent(t *testing.T) {
	maps := &fakeMappings{all: []discordmapping.Model{{CourseId: "C1", DiscordRoleId: "r1"}}}
	marks := &fakeMarks{students: map[string][]string{"C1": {"m1"}}}
	bnd := &fakeBindings{all: map[string]string{"m1": "u1"}}
	bot := &recorderBot{members: map[string][]string{"r1": {"u1"}}} // already correct

	s := NewService(maps, marks, bnd, bot, 0)
	s.ReconcileOnce(context.Background())

	if len(bot.assigned) != 0 || len(bot.removed) != 0 {
		t.Errorf("no-op expected; assigned=%v removed=%v", bot.assigned, bot.removed)
	}
}

func TestReconcile_SkipsUnboundMSSV(t *testing.T) {
	maps := &fakeMappings{all: []discordmapping.Model{{CourseId: "C1", DiscordRoleId: "r1"}}}
	marks := &fakeMarks{students: map[string][]string{"C1": {"m1", "m2"}}}
	bnd := &fakeBindings{all: map[string]string{"m1": "u1"}} // m2 unbound
	bot := &recorderBot{members: map[string][]string{}}

	s := NewService(maps, marks, bnd, bot, 0)
	s.ReconcileOnce(context.Background())

	// only u1 assigned; m2 silently skipped (no error)
	if len(bot.assigned) != 1 || !contains(bot.assigned, "r1:u1") {
		t.Errorf("expected only u1 assigned; got %v", bot.assigned)
	}
}

func TestReconcile_OnlyMappedCoursesProcessed(t *testing.T) {
	maps := &fakeMappings{all: []discordmapping.Model{{CourseId: "C1", DiscordRoleId: "r1"}}}
	// C2 has marks but NO mapping -> must be ignored.
	marks := &fakeMarks{students: map[string][]string{"C1": {"m1"}, "C2": {"m2"}}}
	bnd := &fakeBindings{all: map[string]string{"m1": "u1", "m2": "u2"}}
	bot := &recorderBot{members: map[string][]string{}}

	s := NewService(maps, marks, bnd, bot, 0)
	s.ReconcileOnce(context.Background())

	// Only u1 (C1) assigned; C2 ignored.
	if len(bot.assigned) != 1 || !contains(bot.assigned, "r1:u1") {
		t.Errorf("only mapped course C1 should sync; assigned=%v", bot.assigned)
	}
}

func TestReconcile_AssignErrorDoesNotAbortCycle(t *testing.T) {
	maps := &fakeMappings{all: []discordmapping.Model{
		{CourseId: "C1", DiscordRoleId: "r1"},
		{CourseId: "C2", DiscordRoleId: "r2"},
	}}
	marks := &fakeMarks{students: map[string][]string{"C1": {"m1"}, "C2": {"m2"}}}
	bnd := &fakeBindings{all: map[string]string{"m1": "u1", "m2": "u2"}}
	bot := &recorderBot{members: map[string][]string{}, assignErr: errors.New("boom")}

	s := NewService(maps, marks, bnd, bot, 0)
	s.ReconcileOnce(context.Background()) // must not panic; C2 still attempted

	// Both courses should reach an AssignRole attempt despite the error.
	if !contains(bot.assignTried, "r1:u1") || !contains(bot.assignTried, "r2:u2") {
		t.Errorf("both courses should be attempted despite assign error; tried=%v", bot.assignTried)
	}
}

func TestBindingResolver_OnlyVerifiedDiscord(t *testing.T) {
	repo := &fakeBindingRepo{byMSSV: map[string][]binding.Model{
		"m1": {
			{Platform: "telegram", PlatformUserID: "t1", Verified: true},
			{Platform: "discord", PlatformUserID: "d1", Verified: true},
		},
		"m2": {{Platform: "discord", PlatformUserID: "d2", Verified: false}}, // unverified
	}}
	r := NewBindingResolver(repo)

	if uid, _ := r.DiscordUserID("m1"); uid != "d1" {
		t.Errorf("m1 want d1, got %q", uid)
	}
	if uid, _ := r.DiscordUserID("m2"); uid != "" {
		t.Errorf("unverified discord binding should yield empty, got %q", uid)
	}
	if uid, _ := r.DiscordUserID("m3"); uid != "" {
		t.Errorf("unknown mssv should yield empty, got %q", uid)
	}
}

func contains(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}

// fakeBindingRepo for the resolver test.
type fakeBindingRepo struct {
	byMSSV map[string][]binding.Model
}

func (f *fakeBindingRepo) Upsert(binding.Model) error                                  { return nil }
func (f *fakeBindingRepo) FindByPlatformUser(string, string) (binding.Model, error)    { return binding.Model{}, binding.ErrNotFound }
func (f *fakeBindingRepo) FindByPlatformMSSV(string, string) (binding.Model, error)    { return binding.Model{}, binding.ErrNotFound }
func (f *fakeBindingRepo) FindByMSSV(mssv string) ([]binding.Model, error) {
	if v, ok := f.byMSSV[mssv]; ok {
		return v, nil
	}
	return nil, binding.ErrNotFound
}
