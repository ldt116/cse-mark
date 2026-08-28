package handlers

import (
	"testing"

	"gopkg.in/telebot.v3"

	"thuanle/cse-mark/internal/domain/binding"
	"thuanle/cse-mark/internal/usecases/identity"
)

// Issue #38: bind must key by sender and be DM-only, so a group chat can never
// become a shared identity credential.

func TestBindStart_GroupChatRefused(t *testing.T) {
	ident := &fakeIdentity{existingErr: binding.ErrNotFound}
	h := NewBindHandler(ident)
	c := groupCtx(42)

	if err := h.Start(c); err != nil {
		t.Fatal(err)
	}
	if len(c.sent) != 1 || !contains("nhắn riêng", c.sent[0]) {
		t.Fatalf("want DM-only notice, got %v", c.sent)
	}
	if h.stage(42) != stageIdle {
		t.Fatal("group /bind must not open a stage")
	}
	if ident.bindingKey != "" {
		t.Fatalf("group /bind must not look up identity, looked up %q", ident.bindingKey)
	}
}

func TestBindStart_PrivateKeysBySender(t *testing.T) {
	ident := &fakeIdentity{existingErr: binding.ErrNotFound}
	h := NewBindHandler(ident)
	// chat id 7 ≠ sender id 42: proves the key is the sender.
	c := privateCtx(7, 42)

	if err := h.Start(c); err != nil {
		t.Fatal(err)
	}
	if ident.bindingKey != "42" {
		t.Fatalf("GetBinding key = %q, want sender 42", ident.bindingKey)
	}
	if h.stage(42) != stageAwaitEmail {
		t.Fatal("stage must be keyed by sender")
	}
	if h.stage(7) != stageIdle {
		t.Fatal("stage must NOT be keyed by chat")
	}
}

func TestBindOnText_GroupTextNeverConsumed(t *testing.T) {
	ident := &fakeIdentity{}
	h := NewBindHandler(ident)
	h.setStage(42, stageAwaitOTP) // user has an in-flight bind from their DM
	c := groupCtx(42)
	c.text = "123456"

	handled, err := h.OnText(c)
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Fatal("group text must fall through (handled=false), never feed the OTP stage")
	}
	if ident.verifyKey != "" {
		t.Fatalf("BindVerify must not run for group text, ran with %q", ident.verifyKey)
	}
	if len(c.sent) != 0 {
		t.Fatalf("group text must not produce a reply, sent %v", c.sent)
	}
}

func TestBindOnText_PrivateVerifiesBySenderKey(t *testing.T) {
	ident := &fakeIdentity{bindVerifyRes: identity.BindResult{MSSV: "2013307", Name: "NGUYEN DUC HUY"}}
	h := NewBindHandler(ident)
	h.setStage(42, stageAwaitOTP)
	c := privateCtx(42, 42)
	c.text = "123456"

	handled, err := h.OnText(c)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("private OTP text must be consumed")
	}
	if ident.verifyKey != "42" {
		t.Fatalf("BindVerify key = %q, want sender 42", ident.verifyKey)
	}
	if h.stage(42) != stageIdle {
		t.Fatal("successful verify must clear the stage")
	}
}

func TestBindOnText_OtherUserStageNotVisible(t *testing.T) {
	ident := &fakeIdentity{}
	h := NewBindHandler(ident)
	h.setStage(99, stageAwaitOTP) // another user mid-bind
	c := privateCtx(42, 42)       // a different user's DM
	c.text = "123456"

	handled, err := h.OnText(c)
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Fatal("text from user 42 must not consume user 99's stage")
	}
	if ident.verifyKey != "" {
		t.Fatal("BindVerify must not run against another user's stage")
	}
}

func TestBindCancel_PrivateChatWorks(t *testing.T) {
	ident := &fakeIdentity{}
	h := NewBindHandler(ident)
	h.setStage(42, stageAwaitEmail)
	c := privateCtx(42, 42)

	if err := h.Cancel(c); err != nil {
		t.Fatal(err)
	}
	if h.stage(42) != stageIdle {
		t.Fatal("cancel must clear the sender's stage")
	}
}

var _ = telebot.ChatPrivate // keep the telebot import honest if helpers change
