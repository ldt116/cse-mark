package email

import (
	"context"
	"errors"
	"os"
	"testing"

	"thuanle/cse-mark/internal/configs"
)

func TestNewSenderFromConfig_LogOptIn(t *testing.T) {
	t.Setenv(SenderOptIn, "log")
	s := NewSenderFromConfig(&configs.Config{})
	if _, ok := s.(*LogSender); !ok {
		t.Fatalf("OTP_SENDER=log → want *LogSender, got %T", s)
	}
}

func TestNewSenderFromConfig_SMTPConfigured(t *testing.T) {
	t.Setenv(SenderOptIn, "") // no opt-in
	s := NewSenderFromConfig(&configs.Config{SmtpHost: "smtp.x", SmtpFrom: "bot@x"})
	if _, ok := s.(*SmtpSender); !ok {
		t.Fatalf("SMTP configured → want *SmtpSender, got %T", s)
	}
}

func TestNewSenderFromConfig_FailClosed(t *testing.T) {
	t.Setenv(SenderOptIn, "") // no opt-in, no SMTP
	s := NewSenderFromConfig(&configs.Config{})
	if _, ok := s.(FailingSender); !ok {
		t.Fatalf("nothing configured → want FailingSender, got %T", s)
	}
	if err := s.SendOTP(context.Background(), "x@x", "123"); !errors.Is(err, ErrDeliveryNotConfigured) {
		t.Fatalf("FailingSender must return ErrDeliveryNotConfigured, got %v", err)
	}
}

func TestNewSenderFromConfig_OptInPrecedence(t *testing.T) {
	// Opt-in wins even when SMTP is also configured (dev override).
	t.Setenv(SenderOptIn, "log")
	s := NewSenderFromConfig(&configs.Config{SmtpHost: "smtp.x", SmtpFrom: "bot@x"})
	if _, ok := s.(*LogSender); !ok {
		t.Fatalf("opt-in should win over SMTP, got %T", s)
	}
}

// keep os referenced (t.Setenv covers it, but guard against edits dropping it).
var _ = os.Getenv
