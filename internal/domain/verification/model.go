package verification

import "time"

// Model is a pending OTP verification. PlatformUserID is the primary key (_id).
// Expiry is a BSON Date so the TTL index on `expiry` can delete it automatically.
// Attempts counts failed OTP submissions for brute-force protection; the identity
// use case invalidates the OTP once it reaches Config.OtpMaxAttempts (without
// deleting the record, so the TTL still enforces the resend cooldown).
type Model struct {
	PlatformUserID string    `json:"platform_user_id" bson:"_id"`
	Email          string    `json:"email"            bson:"email"`
	OTP            string    `json:"otp"              bson:"otp"`
	Expiry         time.Time `json:"expiry"           bson:"expiry"`
	Attempts       int       `json:"attempts"         bson:"attempts"`
}
