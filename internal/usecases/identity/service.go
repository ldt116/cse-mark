package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/binding"
	"thuanle/cse-mark/internal/domain/email"
	"thuanle/cse-mark/internal/domain/student"
	"thuanle/cse-mark/internal/domain/verification"
)

// clock is the time source, abstracted so tests can control expiry/cooldown
// deterministically. Production uses the system clock.
type clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// Service implements the email→OTP→MSSV bind flow shared by Telegram and
// Discord. It depends on the roster (student repo) as the trust source, the
// verification repo for in-flight OTPs, the binding repo for the result, and an
// email sender to deliver the OTP. See SRS-v2.md §6 and architecture.md §3.2.
type Service struct {
	studentRepo student.Repository
	verifyRepo  verification.Repository
	bindingRepo binding.Repository
	sender      email.Sender
	cfg         *configs.Config
	clock       clock
}

// NewService wires the identity use case with the system clock.
func NewService(
	studentRepo student.Repository,
	verifyRepo verification.Repository,
	bindingRepo binding.Repository,
	sender email.Sender,
	config *configs.Config,
) *Service {
	return &Service{
		studentRepo: studentRepo,
		verifyRepo:  verifyRepo,
		bindingRepo: bindingRepo,
		sender:      sender,
		cfg:         config,
		clock:       systemClock{},
	}
}

// BindStart validates the email against the roster, enforces the resend
// cooldown, generates an OTP, persists a verification record, and emails the
// OTP. Returns ErrEmailNotInRoster if the email is unknown, or
// ErrResendCooldown if a live OTP for this user/email is still within its TTL.
//
// Platform and platformUserID identify the chat account requesting the bind
// (e.g. "telegram"+chatID, "discord"+userID); they key the verification record
// and ultimately the binding.
func (s *Service) BindStart(ctx context.Context, platform, platformUserID, emailAddr string) error {
	emailAddr = strings.TrimSpace(strings.ToLower(emailAddr))

	// 1. Trust source: the email must exist in the roster. A non-roster email
	//    ends the flow immediately (SRS §6.1.2). Case-folded for a tolerant
	//    match against roster emails.
	stu, err := s.studentRepo.FindByEmail(emailAddr)
	if err != nil {
		if errors.Is(err, student.ErrNotFound) {
			return ErrEmailNotInRoster
		}
		return err
	}

	// 2. Resend cooldown: if the platform user still has a live OTP (expiry in
	//    the future), refuse rather than overwrite. We check expiry>now, NOT
	//    record existence, because the Mongo TTL index deletes asynchronously
	//    and a record may briefly outlive its expiry (SRS §6.1.1).
	if existing, err := s.verifyRepo.Find(platformUserID); err == nil && existing.Expiry.After(s.clock.Now()) {
		return ErrResendCooldown
	} else if err != nil && !errors.Is(err, verification.ErrNotFound) {
		return err
	}

	// 3. Generate and persist the OTP. Upsert resets the attempt counter to 0
	//    and overwrites any expired record. The unique index on
	//    verifications.email is the Sybil defense: two concurrent binds for the
	//    same email from different platform users cannot both create a record.
	otp, err := generateOTP(s.cfg.OtpLen)
	if err != nil {
		return err
	}
	expiry := s.clock.Now().Add(s.cfg.OtpTtl)
	if err := s.verifyRepo.Upsert(verification.Model{
		PlatformUserID: platformUserID,
		Email:          stu.Email,
		OTP:            otp,
		Expiry:         expiry,
	}); err != nil {
		// A duplicate-key on the unique email index surfaces here if another
		// live OTP for the same email exists — treat it as a cooldown so the
		// caller sees a coherent "wait" message rather than a raw DB error.
		if isDuplicateKeyErr(err) {
			return ErrResendCooldown
		}
		return err
	}

	// 4. Deliver the OTP. We persist first so the cooldown is already in place
	//    even if delivery fails partway; a delivery failure propagates and the
	//    user can retry after the TTL window.
	if err := s.sender.SendOTP(ctx, stu.Email, otp); err != nil {
		return err
	}
	log.Info().
		Str("platform", platform).
		Str("platformUserID", platformUserID).
		Str("email", stu.Email).
		Msg("OTP sent")
	return nil
}

// BindVerify checks the submitted OTP and, on success, persists a verified
// binding. Enforces expiry, max attempts, and the 1:1:1 (platform+mssv)
// constraint. On a wrong OTP the attempt counter is incremented; once it
// reaches the limit the record is considered exhausted but kept until expiry
// (forcing the cooldown). Returns a BindResult on success.
func (s *Service) BindVerify(ctx context.Context, platform, platformUserID, otp string) (BindResult, error) {
	rec, err := s.verifyRepo.Find(platformUserID)
	if err != nil {
		if errors.Is(err, verification.ErrNotFound) {
			return BindResult{}, ErrNoPendingOTP
		}
		return BindResult{}, err
	}

	// 1. Expiry check first: an expired-but-not-yet-deleted record must not
	//    accept the OTP.
	if !rec.Expiry.After(s.clock.Now()) {
		return BindResult{}, ErrOTPExpired
	}

	// 2. Already exhausted by too many wrong attempts. We do NOT delete the
	//    record: keeping it until expiry enforces the cooldown before a new OTP
	//    can be requested (SRS §6.1.1 bullet 3).
	if rec.Attempts >= s.cfg.OtpMaxAttempts {
		return BindResult{}, ErrOTPMaxAttempts
	}

	// 3. Wrong OTP: increment and refuse. When this increment reaches the limit
	//    the *next* verify will hit case 2. We report ErrOTPIncorrect so the
	//    delivery layer can show remaining attempts if desired.
	if !constantTimeEq(rec.OTP, otp) {
		newAttempts, incErr := s.verifyRepo.IncrementAttempts(platformUserID)
		if incErr != nil {
			return BindResult{}, incErr
		}
		if newAttempts >= s.cfg.OtpMaxAttempts {
			return BindResult{}, ErrOTPMaxAttempts
		}
		return BindResult{}, ErrOTPIncorrect
	}

	// 4. Correct OTP. Resolve the roster student for the canonical MSSV/name.
	stu, err := s.studentRepo.FindByEmail(rec.Email)
	if err != nil {
		return BindResult{}, err
	}

	// 5. Enforce 1:1:1: an MSSV may be bound to at most one chat account per
	//    platform. If one exists, refuse rather than silently re-point it.
	if existing, err := s.bindingRepo.FindByPlatformMSSV(platform, stu.MSSV); err == nil {
		// Re-binding the SAME account is idempotent and allowed; binding the
		// MSSV to a DIFFERENT account is refused.
		if existing.PlatformUserID != platformUserID {
			return BindResult{}, ErrMSSVAlreadyBound
		}
	} else if !errors.Is(err, binding.ErrNotFound) {
		return BindResult{}, err
	}

	// 6. Persist the verified binding. The unique indexes enforce (platform,
	//    platform_user_id) and (platform, mssv); a racing conflict surfaces as a
	//    duplicate-key error mapped to ErrMSSVAlreadyBound.
	b := binding.Model{
		Platform:       platform,
		PlatformUserID: platformUserID,
		MSSV:           stu.MSSV,
		Verified:       true,
		BoundAt:        s.clock.Now().Unix(),
	}
	if err := s.bindingRepo.Upsert(b); err != nil {
		if isDuplicateKeyErr(err) {
			return BindResult{}, ErrMSSVAlreadyBound
		}
		return BindResult{}, err
	}

	// 7. Success. We do NOT delete the verification record here; the TTL index
	//    reclaims it. Removing it manually would be fine too, but leaving it
	//    (now expired from the caller's perspective soon) is harmless and
	//    avoids an extra write. See note in BindStart about expiry>now.
	log.Info().
		Str("platform", platform).
		Str("platformUserID", platformUserID).
		Str("mssv", stu.MSSV).
		Msg("Binding verified")
	return BindResult{MSSV: stu.MSSV, Name: stu.Name, Email: stu.Email}, nil
}

// GetBinding resolves a chat account to its MSSV. Used by /mark and /profile to
// avoid requiring the student to re-enter their MSSV. Returns ErrNotBound
// (binding.ErrNotFound propagated) when no verified binding exists.
func (s *Service) GetBinding(platform, platformUserID string) (binding.Model, error) {
	return s.bindingRepo.FindByPlatformUser(platform, platformUserID)
}
