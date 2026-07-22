package identity

import (
	"crypto/subtle"
	"strings"

	"go.mongodb.org/mongo-driver/mongo"
)

// constantTimeEq compares the expected and submitted OTP in constant time to
// avoid timing side-channels in OTP verification. Both inputs are trimmed of
// surrounding whitespace since paste/typing can introduce stray spaces.
func constantTimeEq(want, got string) bool {
	return subtle.ConstantTimeCompare(
		[]byte(strings.TrimSpace(want)),
		[]byte(strings.TrimSpace(got)),
	) == 1
}

// isDuplicateKeyErr reports whether err is a Mongo E11000 duplicate-key error,
// which is how the unique indexes (verifications.email, bindings platform+mssv)
// surface a constraint violation to this package.
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	// mongo.WriteException and the single-command form both wrap E11000 as a
	// CommandError/WriteException carrying mongo.IsDuplicateKeyError. Use the
	// driver's own detector so we track its error shape, not a string match.
	if mongo.IsDuplicateKeyError(err) {
		return true
	}
	// Some write paths surface a *mongo.BulkWriteException for E11000; the
	// helper above already covers it, but keep a defensive string check for any
	// driver version that reports E11000 without the typed wrapper.
	return strings.Contains(err.Error(), "E11000")
}
