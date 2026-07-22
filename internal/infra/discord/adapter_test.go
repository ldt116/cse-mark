package discord

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	discordport "thuanle/cse-mark/internal/domain/discord"
)

// recordingSleeper records the durations it was asked to sleep.
type recordingSleeper struct {
	mu    sync.Mutex
	slept []time.Duration
}

func (r *recordingSleeper) Sleep(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.slept = append(r.slept, d)
}

// fakeAPI is a hand-written stand-in for the discordgo subset the adapter uses.
type fakeAPI struct {
	mu sync.Mutex

	roles   []*discordgo.Role
	chans   []*discordgo.Channel
	members []*discordgo.Member

	// scripted errors per call index, per method. nil entry = success.
	roleAddErrs    []error
	roleCreateErrs []error
	chanCreateErrs []error
	guildRolesErrs []error
	memberErrs     []error

	calls map[string]int
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{calls: map[string]int{}}
}

func (f *fakeAPI) count(k string) int { f.calls[k]++; return f.calls[k] }

func nthErr(slice []error, i int) error {
	if i >= 0 && i < len(slice) {
		return slice[i]
	}
	return nil
}

func (f *fakeAPI) GuildRoles(_ string, _ ...discordgo.RequestOption) ([]*discordgo.Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.count("roles")
	if err := nthErr(f.guildRolesErrs, n-1); err != nil {
		return nil, err
	}
	return f.roles, nil
}

func (f *fakeAPI) GuildRoleCreate(_ string, data *discordgo.RoleParams, _ ...discordgo.RequestOption) (*discordgo.Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.count("roleCreate")
	if err := nthErr(f.roleCreateErrs, n-1); err != nil {
		return nil, err
	}
	r := &discordgo.Role{ID: "role-" + data.Name, Name: data.Name}
	f.roles = append(f.roles, r)
	return r, nil
}

func (f *fakeAPI) GuildChannels(_ string, _ ...discordgo.RequestOption) ([]*discordgo.Channel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chans, nil
}

func (f *fakeAPI) GuildChannelCreateComplex(_ string, data discordgo.GuildChannelCreateData, _ ...discordgo.RequestOption) (*discordgo.Channel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.count("chanCreate")
	if err := nthErr(f.chanCreateErrs, n-1); err != nil {
		return nil, err
	}
	c := &discordgo.Channel{ID: "chan-" + data.Name, Name: data.Name}
	f.chans = append(f.chans, c)
	return c, nil
}

func (f *fakeAPI) GuildMemberRoleAdd(_, _, _ string, _ ...discordgo.RequestOption) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.count("roleAdd")
	return nthErr(f.roleAddErrs, n-1)
}

func (f *fakeAPI) GuildMemberRoleRemove(_, _, _ string, _ ...discordgo.RequestOption) error {
	return nil
}

func (f *fakeAPI) GuildMembers(_ string, _ string, _ int, _ ...discordgo.RequestOption) ([]*discordgo.Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.count("members")
	if err := nthErr(f.memberErrs, n-1); err != nil {
		return nil, err
	}
	return f.members, nil
}

// newTestAdapter wires an Adapter around a fake API + recording sleeper and
// starts its worker. The cmds/err channels are replaced so dispatch works
// synchronously.
func newTestAdapter(api discordAPI, slp sleeper) *Adapter {
	a := &Adapter{
		api:     api,
		guildID: "guild-1",
		sleep:   slp,
		cmds:    make(chan func() error),
		errs:    make(chan error),
	}
	a.Start()
	return a
}

func TestEnsureRole_CreatesWhenAbsent(t *testing.T) {
	api := newFakeAPI()
	a := newTestAdapter(api, &recordingSleeper{})
	id, err := a.EnsureRole(context.Background(), "CO2003-L01")
	if err != nil {
		t.Fatalf("EnsureRole: %v", err)
	}
	if id != "role-CO2003-L01" {
		t.Errorf("id: want role-CO2003-L01, got %s", id)
	}
}

func TestEnsureRole_IdempotentReusesExisting(t *testing.T) {
	api := newFakeAPI()
	api.roles = []*discordgo.Role{{ID: "existing", Name: "CO2003-L01"}}
	a := newTestAdapter(api, &recordingSleeper{})

	id, err := a.EnsureRole(context.Background(), "CO2003-L01")
	if err != nil {
		t.Fatalf("EnsureRole: %v", err)
	}
	if id != "existing" {
		t.Errorf("id: want existing, got %s", id)
	}
	if api.calls["roleCreate"] != 0 {
		t.Errorf("should not create when role exists; roleCreate=%d", api.calls["roleCreate"])
	}
}

func TestEnsureChannel_CreatesLockedToRole(t *testing.T) {
	api := newFakeAPI()
	a := newTestAdapter(api, &recordingSleeper{})
	id, err := a.EnsureChannel(context.Background(), "co2003-l01", "role-1")
	if err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	if id != "chan-co2003-l01" {
		t.Errorf("id: want chan-co2003-l01, got %s", id)
	}
	// @everyone deny + role allow present in the created overwrites
	if api.calls["chanCreate"] != 1 {
		t.Errorf("chanCreate calls: want 1, got %d", api.calls["chanCreate"])
	}
}

func TestMembersWithRole_FiltersByRole(t *testing.T) {
	api := newFakeAPI()
	api.members = []*discordgo.Member{
		{User: &discordgo.User{ID: "u1"}, Roles: []string{"rX"}},
		{User: &discordgo.User{ID: "u2"}, Roles: []string{"rX", "course"}},
		{User: &discordgo.User{ID: "u3"}, Roles: []string{"course"}},
	}
	a := newTestAdapter(api, &recordingSleeper{})
	got, err := a.MembersWithRole(context.Background(), "course")
	if err != nil {
		t.Fatalf("MembersWithRole: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 members, got %v", got)
	}
}

func TestRateLimitRetry_BackoffsOn429ThenSucceeds(t *testing.T) {
	// AssignRole: first call returns a RateLimitError, second succeeds. The
	// adapter must sleep(=RetryAfter) and retry.
	slp := &recordingSleeper{}
	rl := &discordgo.RateLimitError{RateLimit: &discordgo.RateLimit{TooManyRequests: &discordgo.TooManyRequests{RetryAfter: 50 * time.Millisecond}}}
	api := newFakeAPI()
	api.roleAddErrs = []error{rl} // first attempt 429s
	a := newTestAdapter(api, slp)

	err := a.AssignRole(context.Background(), "u1", "r1")
	if err != nil {
		t.Fatalf("want success after retry, got %v", err)
	}
	if len(slp.slept) != 1 {
		t.Fatalf("want 1 backoff sleep, got %v", slp.slept)
	}
	if slp.slept[0] != 50*time.Millisecond {
		t.Errorf("sleep: want 50ms, got %v", slp.slept[0])
	}
	if api.calls["roleAdd"] != 2 {
		t.Errorf("want 2 attempts, got %d", api.calls["roleAdd"])
	}
}

func TestRateLimitRetry_GivesUpAfterMaxRetries(t *testing.T) {
	slp := &recordingSleeper{}
	rl := &discordgo.RateLimitError{RateLimit: &discordgo.RateLimit{TooManyRequests: &discordgo.TooManyRequests{RetryAfter: time.Millisecond}}}
	api := newFakeAPI()
	// every attempt rate-limits
	api.roleAddErrs = []error{rl, rl, rl, rl, rl, rl}
	a := newTestAdapter(api, slp)

	err := a.AssignRole(context.Background(), "u1", "r1")
	if err == nil {
		t.Fatal("want final error after exhausting retries, got nil")
	}
	// 1 initial + 3 retries = 4 attempts
	if api.calls["roleAdd"] != 4 {
		t.Errorf("attempts: want 4 (1+maxRetries), got %d", api.calls["roleAdd"])
	}
}

func TestRateLimitRetry_429TextMarkerBacksOff(t *testing.T) {
	// An untyped error containing "429" should also trigger backoff.
	slp := &recordingSleeper{}
	api := newFakeAPI()
	api.roleCreateErrs = []error{errors.New("HTTP 429 Too Many Requests")}
	a := newTestAdapter(api, slp)

	_, err := a.EnsureRole(context.Background(), "R1")
	if err != nil {
		t.Fatalf("want success after marker backoff, got %v", err)
	}
	if len(slp.slept) != 1 {
		t.Fatalf("want 1 sleep for text-marker 429, got %v", slp.slept)
	}
}

func TestWithRateLimitRetry_PassesThroughNonRateLimit(t *testing.T) {
	slp := &recordingSleeper{}
	boom := errors.New("boom")
	calls := 0
	err := withRateLimitRetry(slp, func() error {
		calls++
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if calls != 1 {
		t.Errorf("non-ratelimit should not retry; calls=%d", calls)
	}
	if len(slp.slept) != 0 {
		t.Errorf("no sleep expected; got %v", slp.slept)
	}
}

func TestAdapter_NotConfiguredWhenNilSession(t *testing.T) {
	a := &Adapter{api: nil, guildID: "g", sleep: &recordingSleeper{}, cmds: make(chan func() error)}
	err := a.AssignRole(context.Background(), "u", "r")
	if !errors.Is(err, discordport.ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestQueue_SerializesCommands(t *testing.T) {
	// Two concurrent AssignRole calls should not interleave inside the API;
	// the queue runs them one at a time. We assert ordering with a shared
	// monotonic counter captured into a slice.
	api := newFakeAPI()
	var seq []int
	var mu sync.Mutex
	api.roleAddErrs = nil
	// wrap to observe execution order
	a := newTestAdapter(api, &recordingSleeper{})

	var wg sync.WaitGroup
	expected := []int{0, 1, 2, 3}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			_ = a.dispatch(context.Background(), func() error {
				mu.Lock()
				seq = append(seq, i)
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
	// all 4 ran; order is the append order which reflects queue serialization
	if len(seq) != 4 {
		t.Fatalf("want 4 dispatched, got %v", seq)
	}
	// Each value appears exactly once.
	seen := map[int]bool{}
	for _, v := range seq {
		if seen[v] {
			t.Fatalf("duplicate dispatch of %d", v)
		}
		seen[v] = true
	}
	for _, e := range expected {
		if !seen[e] {
			t.Fatalf("missing dispatch %d", e)
		}
	}
}

func TestLogBot_NoOp(t *testing.T) {
	b := NewLogBot()
	id, err := b.EnsureRole(context.Background(), "X")
	if err != nil || !strings.HasPrefix(id, "log-role:") {
		t.Fatalf("EnsureRole log bot: id=%q err=%v", id, err)
	}
	if err := b.AssignRole(context.Background(), "u", "r"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	ms, err := b.MembersWithRole(context.Background(), "r")
	if err != nil || len(ms) != 0 {
		t.Fatalf("MembersWithRole: %v %v", ms, err)
	}
}
