package email

import (
	"context"

	"github.com/rs/zerolog/log"
)

// LogSender implements Sender without sending any email: it logs the OTP at
// info level. It exists for local development and tests where no SMTP server is
// available, so the bind flow can be exercised end-to-end without real mail.
type LogSender struct{}

// NewLogSender returns a no-op sender.
func NewLogSender() *LogSender { return &LogSender{} }

// SendOTP logs the OTP instead of delivering it. It never returns an error.
func (s *LogSender) SendOTP(_ context.Context, to string, otp string) error {
	log.Info().Str("to", to).Str("otp", otp).Msg("OTP (log sender, not emailed)")
	return nil
}
