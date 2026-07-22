package identity

import "errors"

// Result of a successful bind verification, returned to the delivery layer so it
// can render a platform-appropriate confirmation (e.g. listing the Discord roles
// just granted). Only the fields the caller needs are exposed.
type BindResult struct {
	// MSSV the account is now bound to.
	MSSV string
	// Student name from the roster, for the confirmation message.
	Name string
	// Email that was verified.
	Email string
}

// Sentinel errors for the bind flow. Each maps to a distinct user-facing
// outcome; the delivery layer turns them into localized messages (see
// SRS-v2.md §6.1 and commands.md error notes).
var (
	// ErrEmailNotInRoster: the email is not in the roster CSV → bind ends. This
	// is the trust-source check: only roster emails may bind.
	ErrEmailNotInRoster = errors.New("email not in roster")
	// ErrResendCooldown: a live OTP exists for this platform user (or, via the
	// unique-email index, for this email) and its cooldown has not elapsed. The
	// user must wait rather than request again.
	ErrResendCooldown = errors.New("otp resend cooldown active")
	// ErrNoPendingOTP: no verification in progress for this platform user. The
	// user tried to verify without first starting a bind, or the record expired.
	ErrNoPendingOTP = errors.New("no pending otp")
	// ErrOTPMaxAttempts: the OTP was invalidated after too many failed attempts.
	// The record is kept until expiry to force the cooldown before a new OTP.
	ErrOTPMaxAttempts = errors.New("otp max attempts exceeded")
	// ErrOTPExpired: the OTP exists but has passed its expiry time.
	ErrOTPExpired = errors.New("otp expired")
	// ErrOTPIncorrect: the submitted code does not match. The attempt counter
	// was incremented; repeated wrong submissions lead to ErrOTPMaxAttempts.
	ErrOTPIncorrect = errors.New("otp incorrect")
	// ErrMSSVAlreadyBound: the MSSV already has a verified binding on this
	// platform (1:1:1 constraint, platform+mssv unique index). Refused rather
	// than silently rebinding to a different chat account.
	ErrMSSVAlreadyBound = errors.New("mssv already bound on this platform")
)
