package verification

import "time"

// Model is a pending OTP verification. PlatformUserID is the primary key (_id).
// Expiry is a BSON Date so the TTL index on `expiry` can delete it automatically.
type Model struct {
	PlatformUserID string    `json:"platform_user_id" bson:"_id"`
	Email          string    `json:"email"            bson:"email"`
	OTP            string    `json:"otp"              bson:"otp"`
	Expiry         time.Time `json:"expiry"           bson:"expiry"`
}
