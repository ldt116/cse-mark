package middlewares

import (
	"testing"

	"gopkg.in/telebot.v3"
)

// fakeCtx satisfies telebot.Context by embedding the interface (nil) and
// overriding only Sender — everything Whitelist touches. Calling an
// un-overridden method panics, which is the desired loud failure.
type fakeCtx struct {
	telebot.Context
	sender *telebot.User
}

func (f *fakeCtx) Sender() *telebot.User { return f.sender }

// Issue #45 F1: telebot v3 upstream middleware.Whitelist derefs
// c.Sender().ID without a nil check (restrict.go). An anonymous group admin
// sends updates with only sender_chat (no `from`), so Sender() is nil and the
// upstream middleware panics — telebot v3 does not recover, crashing the whole
// process. The repo-local Whitelist must skip such updates silently instead.
func TestWhitelist_NilSenderSkipsWithoutPanic(t *testing.T) {
	mw := Whitelist(42)
	nextCalled := false
	handler := mw(func(c telebot.Context) error {
		nextCalled = true
		return nil
	})

	c := &fakeCtx{sender: nil} // anonymous group admin: no `from` on update

	if err := handler(c); err != nil {
		t.Fatalf("nil sender must be skipped with nil error, got %v", err)
	}
	if nextCalled {
		t.Fatal("nil sender must not reach next")
	}
	// Reaching this line at all proves no panic occurred.
}

func TestWhitelist_AllowedSenderPasses(t *testing.T) {
	mw := Whitelist(7, 42)
	nextCalled := false
	handler := mw(func(c telebot.Context) error {
		nextCalled = true
		return nil
	})

	if err := handler(&fakeCtx{sender: &telebot.User{ID: 42}}); err != nil {
		t.Fatalf("allowed sender: %v", err)
	}
	if !nextCalled {
		t.Fatal("whitelisted sender must reach next")
	}
}

func TestWhitelist_UnknownSenderSkipped(t *testing.T) {
	mw := Whitelist(7, 42)
	nextCalled := false
	handler := mw(func(c telebot.Context) error {
		nextCalled = true
		return nil
	})

	if err := handler(&fakeCtx{sender: &telebot.User{ID: 99}}); err != nil {
		t.Fatalf("unknown sender must be skipped with nil error, got %v", err)
	}
	if nextCalled {
		t.Fatal("unknown sender must not reach next")
	}
}
