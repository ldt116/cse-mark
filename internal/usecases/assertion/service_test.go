package assertion

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"thuanle/cse-mark/internal/domain/jwks"
)

const (
	testIssuer   = "https://student-app.test"
	testAudience = "cse-mark"
	testSubject  = "2111111"
)

// fakeJwks is a hand-rolled jwks.Repository. keys maps kid → public key;
// err, when set, short-circuits SigningKey. Tests can swap keys or err
// between calls to simulate key rotation or endpoint failure. calls counts
// SigningKey invocations so tests can assert the parser rejected a token
// before ever hitting the JWKS lookup.
type fakeJwks struct {
	keys  map[string]ed25519.PublicKey
	err   error
	calls int
}

func (f *fakeJwks) SigningKey(kid string) (ed25519.PublicKey, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	key, ok := f.keys[kid]
	if !ok {
		return nil, jwks.ErrUnknownKid
	}
	return key, nil
}

type keyPair struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

func newKeyPair(t *testing.T) keyPair {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	return keyPair{priv: priv, pub: pub}
}

// mintToken signs an EdDSA JWT the way the student app does: aud goes on the
// wire as a JSON array, and the kid header names the signing key.
func mintToken(t *testing.T, priv ed25519.PrivateKey, kid, issuer, subject string, audience []string, expiresAt time.Time) string {
	t.Helper()
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		Audience:  jwt.ClaimStrings(audience),
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(now),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func newService(repo jwks.Repository) *Service {
	return NewService(repo, testIssuer, testAudience)
}

func TestVerify_Valid(t *testing.T) {
	key := newKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": key.pub}}
	token := mintToken(t, key.priv, "key-a", testIssuer, testSubject, []string{testAudience}, time.Now().Add(5*time.Minute))

	sub, err := newService(repo).Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}
	if sub != testSubject {
		t.Errorf("Verify() sub = %q, want %q", sub, testSubject)
	}
}

func TestVerify_Expired(t *testing.T) {
	key := newKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": key.pub}}
	token := mintToken(t, key.priv, "key-a", testIssuer, testSubject, []string{testAudience}, time.Now().Add(-time.Minute))

	_, err := newService(repo).Verify(token)
	if !errors.Is(err, ErrExpired) {
		t.Errorf("Verify() error = %v, want ErrExpired", err)
	}
}

func TestVerify_WrongIssuer(t *testing.T) {
	key := newKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": key.pub}}
	token := mintToken(t, key.priv, "key-a", "https://evil.test", testSubject, []string{testAudience}, time.Now().Add(5*time.Minute))

	_, err := newService(repo).Verify(token)
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("Verify() error = %v, want ErrInvalid", err)
	}
}

// The student app signs aud as a JSON array, so the negative case must also
// use an array (["other-svc"]) to exercise the same claim shape.
func TestVerify_WrongAudience(t *testing.T) {
	key := newKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": key.pub}}
	token := mintToken(t, key.priv, "key-a", testIssuer, testSubject, []string{"other-svc"}, time.Now().Add(5*time.Minute))

	_, err := newService(repo).Verify(token)
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("Verify() error = %v, want ErrInvalid", err)
	}
}

func TestVerify_KeyNotInJwks(t *testing.T) {
	key := newKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": key.pub}}
	token := mintToken(t, key.priv, "key-x", testIssuer, testSubject, []string{testAudience}, time.Now().Add(5*time.Minute))

	_, err := newService(repo).Verify(token)
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("Verify() error = %v, want ErrInvalid", err)
	}
}

func TestVerify_JwksUnavailable(t *testing.T) {
	key := newKeyPair(t)
	repo := &fakeJwks{
		keys: map[string]ed25519.PublicKey{"key-a": key.pub},
		err:  jwks.ErrUnavailable,
	}
	token := mintToken(t, key.priv, "key-a", testIssuer, testSubject, []string{testAudience}, time.Now().Add(5*time.Minute))

	_, err := newService(repo).Verify(token)
	if !errors.Is(err, jwks.ErrUnavailable) {
		t.Errorf("Verify() error = %v, want propagated jwks.ErrUnavailable", err)
	}
}

func TestVerify_TamperedPayload(t *testing.T) {
	key := newKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": key.pub}}
	token := mintToken(t, key.priv, "key-a", testIssuer, testSubject, []string{testAudience}, time.Now().Add(5*time.Minute))

	// Flip one base64 character in the middle of the payload segment.
	segments := strings.Split(token, ".")
	payload := []byte(segments[1])
	i := len(payload) / 2
	if payload[i] == 'A' {
		payload[i] = 'B'
	} else {
		payload[i] = 'A'
	}
	segments[1] = string(payload)

	_, err := newService(repo).Verify(strings.Join(segments, "."))
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("Verify() error = %v, want ErrInvalid", err)
	}
}

func TestVerify_WrongAlg(t *testing.T) {
	key := newKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": key.pub}}

	claims := jwt.RegisteredClaims{
		Issuer:    testIssuer,
		Audience:  jwt.ClaimStrings{testAudience},
		Subject:   testSubject,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok.Header["kid"] = "key-a"
	token, err := tok.SignedString([]byte("hmac-secret"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	_, err = newService(repo).Verify(token)
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("Verify() error = %v, want ErrInvalid", err)
	}
	// The alg whitelist must reject HS256 before the parser calls the
	// keyfunc. This is the assertion with teeth: without WithValidMethods
	// the keyfunc runs (calls > 0) and this test fails, even though the
	// ed25519-vs-[]byte key mismatch still yields ErrInvalid above.
	if repo.calls != 0 {
		t.Errorf("keyfunc called %d time(s), want 0 — alg whitelist must reject before JWKS lookup", repo.calls)
	}
}

// TestVerify_AlgNone locks the same whitelist from the other side: an
// unsigned alg=none token must be rejected before any JWKS lookup. If
// WithValidMethods is dropped from service.go, the keyfunc is invoked
// (calls > 0) and this test fails, even though golang-jwt would still
// refuse the none signature on its own.
func TestVerify_AlgNone(t *testing.T) {
	key := newKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": key.pub}}

	claims := jwt.RegisteredClaims{
		Issuer:    testIssuer,
		Audience:  jwt.ClaimStrings{testAudience},
		Subject:   testSubject,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tok.Header["kid"] = "key-a"
	token, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	_, err = newService(repo).Verify(token)
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("Verify() error = %v, want ErrInvalid", err)
	}
	if repo.calls != 0 {
		t.Errorf("keyfunc called %d time(s), want 0 — alg whitelist must reject before JWKS lookup", repo.calls)
	}
}

func TestVerify_MissingSub(t *testing.T) {
	key := newKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": key.pub}}
	token := mintToken(t, key.priv, "key-a", testIssuer, "", []string{testAudience}, time.Now().Add(5*time.Minute))

	_, err := newService(repo).Verify(token)
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("Verify() error = %v, want ErrInvalid", err)
	}
}

// TestVerify_MissingExp locks jwt.WithExpirationRequired: a token with valid
// iss/aud/sub and signature but NO exp claim must be rejected as ErrInvalid —
// a token that never expires must not be accepted. Minted inline (not via
// mintToken) because mintToken always sets ExpiresAt.
func TestVerify_MissingExp(t *testing.T) {
	key := newKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": key.pub}}

	claims := jwt.RegisteredClaims{
		Issuer:   testIssuer,
		Audience: jwt.ClaimStrings{testAudience},
		Subject:  testSubject,
		IssuedAt: jwt.NewNumericDate(time.Now()),
		// ExpiresAt deliberately unset.
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = "key-a"
	token, err := tok.SignedString(key.priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	_, err = newService(repo).Verify(token)
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("Verify() error = %v, want ErrInvalid", err)
	}
}

func TestVerify_RotationNewKid(t *testing.T) {
	keyA := newKeyPair(t)
	repo := &fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": keyA.pub}}

	// The JWKS client refreshes after rotation: the key set now only holds
	// the new kid.
	keyB := newKeyPair(t)
	repo.keys = map[string]ed25519.PublicKey{"key-b": keyB.pub}

	token := mintToken(t, keyB.priv, "key-b", testIssuer, testSubject, []string{testAudience}, time.Now().Add(5*time.Minute))
	sub, err := newService(repo).Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}
	if sub != testSubject {
		t.Errorf("Verify() sub = %q, want %q", sub, testSubject)
	}
}
