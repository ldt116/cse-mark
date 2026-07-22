package email

import "context"

// Sender is the port for delivering OTP messages. The identity use case depends
// on this interface; the concrete implementation (SMTP, or a no-op LogSender
// for local/dev without an SMTP server) lives in infra. See docs/v2/architecture.md
// §4.2.
type Sender interface {
	// SendOTP delivers a one-time password to the recipient's email address.
	// It must not leak the OTP in error strings beyond what is necessary.
	SendOTP(ctx context.Context, to string, otp string) error
}
