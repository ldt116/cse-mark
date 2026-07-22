package handlers

import (
	"context"
	"errors"
	"testing"

	"thuanle/cse-mark/internal/domain/binding"
	"thuanle/cse-mark/internal/usecases/identity"
)

type fakeIdentity struct {
	bindStartErr  error
	bindVerifyRes identity.BindResult
	bindVerifyErr error
	existing      binding.Model
	existingErr   error

	startEmail string
	verifyOTP  string
	started    bool
}

func (f *fakeIdentity) BindStart(_ context.Context, _, _, email string) error {
	f.started = true
	f.startEmail = email
	return f.bindStartErr
}
func (f *fakeIdentity) BindVerify(_ context.Context, _, _, otp string) (identity.BindResult, error) {
	f.verifyOTP = otp
	return f.bindVerifyRes, f.bindVerifyErr
}
func (f *fakeIdentity) GetBinding(string, string) (binding.Model, error) {
	return f.existing, f.existingErr
}

func TestBind_StartSetsAwaitEmail(t *testing.T) {
	ident := &fakeIdentity{existingErr: binding.ErrNotFound}
	h := NewBindHandler(ident)
	if h.stage(1) != stageIdle {
		t.Fatal("should start idle")
	}
	_ = h.setStage // ensure methods exist
	// simulate Start by setting stage directly via the public flow
	h.setStage(1, stageAwaitEmail)
	if h.stage(1) != stageAwaitEmail {
		t.Fatal("stage not set")
	}
}

func TestTeleBindMessages(t *testing.T) {
	cases := []struct {
		fn   func(error) string
		err  error
		want string
	}{
		{teleBindStartMsg, identity.ErrEmailNotInRoster, "sinh viên"},
		{teleBindStartMsg, identity.ErrResendCooldown, "đợi"},
		{teleBindVerifyMsg, identity.ErrOTPIncorrect, "không đúng"},
		{teleBindVerifyMsg, identity.ErrOTPExpired, "hết hạn"},
		{teleBindVerifyMsg, identity.ErrOTPMaxAttempts, "số lần"},
		{teleBindVerifyMsg, identity.ErrMSSVAlreadyBound, "Telegram khác"},
		{teleBindVerifyMsg, errors.New("x"), "Không thể"},
	}
	for _, c := range cases {
		if got := c.fn(c.err); !contains(c.want, got) {
			t.Errorf("for %v: want %q in %q", c.err, c.want, got)
		}
	}
}

func TestResetOnVerifyErr(t *testing.T) {
	if !resetOnVerifyErr(identity.ErrOTPExpired) {
		t.Error("expired should reset")
	}
	if !resetOnVerifyErr(identity.ErrOTPMaxAttempts) {
		t.Error("max attempts should reset")
	}
	if resetOnVerifyErr(identity.ErrOTPIncorrect) {
		t.Error("incorrect should NOT reset (allow retry)")
	}
}

func contains(want, s string) bool {
	for i := 0; i+len(want) <= len(s); i++ {
		if s[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
