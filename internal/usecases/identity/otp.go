package identity

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// generateOTP returns a zero-padded numeric OTP of the given length using a
// cryptographically secure source. It panics only if the system CSPRNG fails,
// which is a fatal environment error (the alternative — silently reusing a
// predictable code — would break OTP security). Callers pass Config.OtpLen.
func generateOTP(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("otp length must be positive, got %d", length)
	}
	// Build a value in [0, 10^length) and zero-pad, so every code is exactly
	// `length` digits regardless of leading zeros.
	upper := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(length)), nil)
	n, err := rand.Int(rand.Reader, upper)
	if err != nil {
		return "", fmt.Errorf("generate otp: %w", err)
	}
	return fmt.Sprintf("%0*d", length, n), nil
}
