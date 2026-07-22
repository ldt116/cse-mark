package email

import (
	"context"
	"errors"
	"os"

	"thuanle/cse-mark/internal/configs"
	emailport "thuanle/cse-mark/internal/domain/email"
)

// ErrDeliveryNotConfigured is returned by FailingSender and by NewSenderFromConfig
// when no delivery path is available. The bind flow surfaces it as "cannot send
// OTP" rather than silently logging a code and telling the user it was emailed.
var ErrDeliveryNotConfigured = errors.New("otp delivery not configured")

// FailingSender implements email.Sender by always returning ErrDeliveryNotConfigured.
// It is the "fail closed" sender: when SMTP is not configured and LogSender is
// not explicitly opted into, bind refuses rather than pretending to deliver.
type FailingSender struct{}

func (FailingSender) SendOTP(_ context.Context, _ string, _ string) error {
	return ErrDeliveryNotConfigured
}

// SenderOptIn names the explicit env that selects the no-op LogSender for local
// development. It is deliberately not the default: production must deliver real
// mail or fail closed, never log-and-claim.
const SenderOptIn = "OTP_SENDER"

// NewSenderFromConfig selects the email.Sender from config:
//   - OTP_SENDER=log         → LogSender (explicit dev opt-in; logs the code)
//   - SMTP_HOST set          → SmtpSender (real delivery)
//   - otherwise              → FailingSender (bind fails closed)
//
// This prevents the "told the user an OTP was emailed but only wrote it to logs"
// failure in production. OTP_SENDER is read directly from the environment since
// it is a delivery-side, dev-only knob not carried by the shared Config.
func NewSenderFromConfig(cfg *configs.Config) emailport.Sender {
	// Explicit dev opt-in takes precedence so a developer can exercise /bind
	// without SMTP by setting OTP_SENDER=log.
	if os.Getenv(SenderOptIn) == "log" {
		return NewLogSender()
	}
	if cfg.SmtpHost != "" && cfg.SmtpFrom != "" {
		return NewSmtpSender(cfg)
	}
	return FailingSender{}
}

// Compile-time: FailingSender satisfies the port.
var _ emailport.Sender = FailingSender{}
