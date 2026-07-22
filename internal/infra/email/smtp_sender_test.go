package email

import (
	"strings"
	"testing"

	"thuanle/cse-mark/internal/domain/email"
)

func TestOtpMessageBody_ContainsOtpAndHeaders(t *testing.T) {
	msg := string(otpMessageBody("bot@cse.local", "sv@hcmut.edu.vn", "987651"))

	// The OTP must appear exactly once in the body.
	if got := strings.Count(msg, "987651"); got != 1 {
		t.Fatalf("otp occurrences: want 1, got %d", got)
	}
	// Required headers for a well-formed message.
	for _, want := range []string{"From: bot@cse.local", "To: sv@hcmut.edu.vn", "Subject:", "MIME-Version: 1.0", "Content-Type: text/plain; charset=UTF-8"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n---\n%s", want, msg)
		}
	}
	// Headers and body separated by a blank line (RFC 5322).
	if !strings.Contains(msg, "\r\n\r\n") {
		t.Errorf("message missing blank line between headers and body")
	}
}

func TestSmtpSender_IsImplicitTLS(t *testing.T) {
	cases := []struct {
		port int
		want bool
	}{
		{465, true},
		{587, false},
		{25, false},
	}
	for _, tc := range cases {
		s := &SmtpSender{port: tc.port}
		if got := s.isImplicitTLS(); got != tc.want {
			t.Errorf("port %d: want implicitTLS=%v, got %v", tc.port, tc.want, got)
		}
	}
}

func TestSmtpSender_AuthSkippedWhenNoCredentials(t *testing.T) {
	// With no username/password, auth() is a no-op and must not even try to
	// talk to a client. We assert the early return without constructing a real
	// smtp.Client (which would need a connection). This guards the "open relay
	// / local MTA without auth" path used in some dev setups.
	s := &SmtpSender{username: "", password: ""}
	if err := s.auth(nil); err != nil {
		t.Fatalf("auth with nil client and no creds should be no-op, got %v", err)
	}
}

func TestSmtpSender_ImplementsSender(t *testing.T) {
	// Compile-time assertion that both concrete types satisfy the port.
	var _ email.Sender = (*SmtpSender)(nil)
	var _ email.Sender = (*LogSender)(nil)
}
