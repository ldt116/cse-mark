package identity

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/binding"
	"thuanle/cse-mark/internal/domain/student"
	"thuanle/cse-mark/internal/domain/verification"
)

// --- fakes ---

type fakeStudentRepo struct {
	byEmail map[string]student.Model
	err     error // forced error
}

func (f *fakeStudentRepo) Upsert(m student.Model) error { return nil }
func (f *fakeStudentRepo) FindByEmail(e string) (student.Model, error) {
	if f.err != nil {
		return student.Model{}, f.err
	}
	if s, ok := f.byEmail[e]; ok {
		return s, nil
	}
	return student.Model{}, student.ErrNotFound
}
func (f *fakeStudentRepo) FindByMSSV(mssv string) (student.Model, error) {
	for _, s := range f.byEmail {
		if s.MSSV == mssv {
			return s, nil
		}
	}
	return student.Model{}, student.ErrNotFound
}

type fakeVerifyRepo struct {
	mu       sync.Mutex
	records  map[string]verification.Model
	upsertOk bool
	dupErr   bool // Upsert returns a duplicate-key-like error to simulate Sybil index
}

func (f *fakeVerifyRepo) Upsert(m verification.Model) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dupErr {
		return dupKeyErr{}
	}
	rec := m
	if _, exists := f.records[m.PlatformUserID]; exists {
		rec.Attempts = f.records[m.PlatformUserID].Attempts // preserve attempts? Upsert resets to 0 per contract
	}
	rec.Attempts = 0
	f.records[m.PlatformUserID] = rec
	return nil
}
func (f *fakeVerifyRepo) Find(id string) (verification.Model, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.records[id]; ok {
		return r, nil
	}
	return verification.Model{}, verification.ErrNotFound
}
func (f *fakeVerifyRepo) IncrementAttempts(id string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.records[id]
	if !ok {
		return 0, verification.ErrNotFound
	}
	r.Attempts++
	f.records[id] = r
	return r.Attempts, nil
}
func (f *fakeVerifyRepo) FindByEmail(email string) ([]verification.Model, error) {
	return nil, nil
}

type fakeBindingRepo struct {
	mu        sync.Mutex
	byUser    map[string]binding.Model // key platform|user
	byMSSV    map[string]binding.Model // key platform|mssv
	upsertErr error
}

func bkey(a, b string) string { return a + "|" + b }

func (f *fakeBindingRepo) Upsert(m binding.Model) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upsertErr != nil {
		return f.upsertErr
	}
	// enforce unique-ish like the real indexes for the 1:1:1 test
	if ex, ok := f.byMSSV[bkey(m.Platform, m.MSSV)]; ok && ex.PlatformUserID != m.PlatformUserID {
		return dupKeyErr{}
	}
	f.byUser[bkey(m.Platform, m.PlatformUserID)] = m
	f.byMSSV[bkey(m.Platform, m.MSSV)] = m
	return nil
}
func (f *fakeBindingRepo) FindByPlatformUser(p, u string) (binding.Model, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.byUser[bkey(p, u)]; ok {
		return m, nil
	}
	return binding.Model{}, binding.ErrNotFound
}
func (f *fakeBindingRepo) FindByPlatformMSSV(p, mssv string) (binding.Model, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.byMSSV[bkey(p, mssv)]; ok {
		return m, nil
	}
	return binding.Model{}, binding.ErrNotFound
}
func (f *fakeBindingRepo) FindByMSSV(mssv string) ([]binding.Model, error) { return nil, nil }

type dupKeyErr struct{}

func (dupKeyErr) Error() string { return "E11000 duplicate key error" }

// dupKeyErr satisfies the string check in isDuplicateKeyErr.
var _ error = dupKeyErr{}

type capturingSender struct {
	lastOTP string
	calls   int
	err     error
}

func (s *capturingSender) SendOTP(_ context.Context, _ string, otp string) error {
	s.calls++
	s.lastOTP = otp
	return s.err
}

type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

// --- test harness ---

func newTestService(t *testing.T, now time.Time) (*Service, *fakeStudentRepo, *fakeVerifyRepo, *fakeBindingRepo, *capturingSender) {
	t.Helper()
	cfg := &configs.Config{OtpLen: 4, OtpTtl: 15 * time.Minute, OtpMaxAttempts: 3}
	stu := &fakeStudentRepo{byEmail: map[string]student.Model{
		"sv@hcmut.edu.vn": {MSSV: "2212345", Name: "Sinh Vien", Email: "sv@hcmut.edu.vn"},
	}}
	vrf := &fakeVerifyRepo{records: map[string]verification.Model{}}
	bnd := &fakeBindingRepo{byUser: map[string]binding.Model{}, byMSSV: map[string]binding.Model{}}
	snd := &capturingSender{}
	s := &Service{studentRepo: stu, verifyRepo: vrf, bindingRepo: bnd, sender: snd, cfg: cfg, clock: fakeClock{now}}
	return s, stu, vrf, bnd, snd
}

// --- BindStart ---

func TestBindStart_RejectsNonRosterEmail(t *testing.T) {
	s, _, _, _, _ := newTestService(t, time.Now())
	err := s.BindStart(context.Background(), "discord", "u1", "stranger@hcmut.edu.vn")
	if !errors.Is(err, ErrEmailNotInRoster) {
		t.Fatalf("want ErrEmailNotInRoster, got %v", err)
	}
}

func TestBindStart_LowercasesEmail(t *testing.T) {
	s, _, vrf, _, snd := newTestService(t, time.Now())
	// roster email is lowercase; submit mixed case.
	if err := s.BindStart(context.Background(), "discord", "u1", " SV@HCMUT.edu.vn "); err != nil {
		t.Fatalf("BindStart: %v", err)
	}
	if snd.calls != 1 {
		t.Fatalf("want 1 send, got %d", snd.calls)
	}
	if _, ok := vrf.records["u1"]; !ok {
		t.Fatalf("verification record not stored")
	}
}

func TestBindStart_BlocksResendWithinCooldown(t *testing.T) {
	now := time.Now()
	s, _, vrf, _, _ := newTestService(t, now)
	if err := s.BindStart(context.Background(), "telegram", "t1", "sv@hcmut.edu.vn"); err != nil {
		t.Fatalf("first start: %v", err)
	}
	// Simulate a still-live record (expiry in the future) and try again.
	rec := vrf.records["t1"]
	rec.Expiry = now.Add(10 * time.Minute) // still live
	vrf.records["t1"] = rec

	err := s.BindStart(context.Background(), "telegram", "t1", "sv@hcmut.edu.vn")
	if !errors.Is(err, ErrResendCooldown) {
		t.Fatalf("want ErrResendCooldown, got %v", err)
	}
}

func TestBindStart_AllowsResendAfterExpiry(t *testing.T) {
	now := time.Now()
	s, _, vrf, _, snd := newTestService(t, now)
	if err := s.BindStart(context.Background(), "telegram", "t1", "sv@hcmut.edu.vn"); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Expire the existing record; a new start should succeed and overwrite.
	rec := vrf.records["t1"]
	rec.Expiry = now.Add(-time.Minute) // past
	vrf.records["t1"] = rec

	if err := s.BindStart(context.Background(), "telegram", "t1", "sv@hcmut.edu.vn"); err != nil {
		t.Fatalf("second start after expiry: %v", err)
	}
	if snd.calls != 2 {
		t.Fatalf("want 2 sends, got %d", snd.calls)
	}
}

func TestBindStart_DuplicateEmailIndexMapsToCooldown(t *testing.T) {
	now := time.Now()
	s, _, vrf, _, _ := newTestService(t, now)
	vrf.dupErr = true // simulate the unique-email index rejecting a concurrent bind
	err := s.BindStart(context.Background(), "discord", "u2", "sv@hcmut.edu.vn")
	if !errors.Is(err, ErrResendCooldown) {
		t.Fatalf("want ErrResendCooldown from dup index, got %v", err)
	}
}

// --- BindVerify ---

func TestBindVerify_NoPendingRecord(t *testing.T) {
	s, _, _, _, _ := newTestService(t, time.Now())
	_, err := s.BindVerify(context.Background(), "discord", "nobody", "1234")
	if !errors.Is(err, ErrNoPendingOTP) {
		t.Fatalf("want ErrNoPendingOTP, got %v", err)
	}
}

func TestBindVerify_Expired(t *testing.T) {
	now := time.Now()
	s, _, vrf, _, _ := newTestService(t, now)
	vrf.records["u1"] = verification.Model{
		PlatformUserID: "u1", Email: "sv@hcmut.edu.vn", OTP: "1234",
		Expiry: now.Add(-time.Minute), // expired
	}
	_, err := s.BindVerify(context.Background(), "discord", "u1", "1234")
	if !errors.Is(err, ErrOTPExpired) {
		t.Fatalf("want ErrOTPExpired, got %v", err)
	}
}

func TestBindVerify_WrongThenCorrect(t *testing.T) {
	now := time.Now()
	s, _, vrf, _, _ := newTestService(t, now)
	vrf.records["u1"] = verification.Model{
		PlatformUserID: "u1", Email: "sv@hcmut.edu.vn", OTP: "1234",
		Expiry: now.Add(10 * time.Minute),
	}
	// wrong OTP increments attempts
	if _, err := s.BindVerify(context.Background(), "discord", "u1", "0000"); !errors.Is(err, ErrOTPIncorrect) {
		t.Fatalf("want ErrOTPIncorrect, got %v", err)
	}
	if vrf.records["u1"].Attempts != 1 {
		t.Fatalf("attempts want 1, got %d", vrf.records["u1"].Attempts)
	}
	// correct OTP binds
	res, err := s.BindVerify(context.Background(), "discord", "u1", "1234")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if res.MSSV != "2212345" {
		t.Fatalf("mssv want 2212345, got %s", res.MSSV)
	}
}

func TestBindVerify_MaxAttemptsLocksUntilExpiry(t *testing.T) {
	now := time.Now()
	s, _, vrf, _, _ := newTestService(t, now)
	vrf.records["u1"] = verification.Model{
		PlatformUserID: "u1", Email: "sv@hcmut.edu.vn", OTP: "1234",
		Expiry: now.Add(10 * time.Minute),
	}
	// OtpMaxAttempts == 3
	for i := 0; i < 3; i++ {
		_, err := s.BindVerify(context.Background(), "discord", "u1", "0000")
		// 3rd wrong increments to 3 and returns ErrOTPMaxAttempts; first two return ErrOTPIncorrect
		_ = err
	}
	// Now even the CORRECT OTP must be refused (record kept, attempts >= max).
	_, err := s.BindVerify(context.Background(), "discord", "u1", "1234")
	if !errors.Is(err, ErrOTPMaxAttempts) {
		t.Fatalf("after max attempts correct OTP should be refused with ErrOTPMaxAttempts, got %v", err)
	}
}

func TestBindVerify_RefusesMSSVAlreadyBoundToOtherAccount(t *testing.T) {
	now := time.Now()
	s, _, vrf, bnd, _ := newTestService(t, now)
	// Pre-bind the MSSV to a different discord account.
	bnd.byMSSV["discord|2212345"] = binding.Model{Platform: "discord", PlatformUserID: "u-other", MSSV: "2212345"}
	bnd.byUser["discord|u-other"] = binding.Model{Platform: "discord", PlatformUserID: "u-other", MSSV: "2212345"}

	vrf.records["u1"] = verification.Model{
		PlatformUserID: "u1", Email: "sv@hcmut.edu.vn", OTP: "1234",
		Expiry: now.Add(10 * time.Minute),
	}
	_, err := s.BindVerify(context.Background(), "discord", "u1", "1234")
	if !errors.Is(err, ErrMSSVAlreadyBound) {
		t.Fatalf("want ErrMSSVAlreadyBound, got %v", err)
	}
}

func TestBindVerify_IdempotentRebindSameAccount(t *testing.T) {
	now := time.Now()
	s, _, vrf, _, _ := newTestService(t, now)
	vrf.records["u1"] = verification.Model{
		PlatformUserID: "u1", Email: "sv@hcmut.edu.vn", OTP: "1234",
		Expiry: now.Add(10 * time.Minute),
	}
	// First bind succeeds.
	if _, err := s.BindVerify(context.Background(), "discord", "u1", "1234"); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	// Re-verify with a fresh OTP on the SAME account should be allowed (idempotent),
	// so we refresh: store a new live OTP and verify again.
	vrf.records["u1"] = verification.Model{
		PlatformUserID: "u1", Email: "sv@hcmut.edu.vn", OTP: "9999",
		Expiry: now.Add(10 * time.Minute),
	}
	res, err := s.BindVerify(context.Background(), "discord", "u1", "9999")
	if err != nil {
		t.Fatalf("idempotent rebind should succeed, got %v", err)
	}
	if res.MSSV != "2212345" {
		t.Fatalf("mssv mismatch: %s", res.MSSV)
	}
}

func TestGetBinding_NotBound(t *testing.T) {
	s, _, _, _, _ := newTestService(t, time.Now())
	_, err := s.GetBinding("discord", "nobody")
	if !errors.Is(err, binding.ErrNotFound) {
		t.Fatalf("want binding.ErrNotFound, got %v", err)
	}
}

func TestGenerateOTP_LengthAndDigits(t *testing.T) {
	for _, n := range []int{4, 6, 8} {
		otp, err := generateOTP(n)
		if err != nil {
			t.Fatalf("len %d: %v", n, err)
		}
		if len(otp) != n {
			t.Errorf("len %d: got %q (len %d)", n, otp, len(otp))
		}
		for _, r := range otp {
			if r < '0' || r > '9' {
				t.Errorf("len %d: non-digit %q in %q", n, r, otp)
			}
		}
	}
}
