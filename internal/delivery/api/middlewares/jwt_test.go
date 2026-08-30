package middlewares

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"thuanle/cse-mark/internal/domain/jwks"
	"thuanle/cse-mark/internal/usecases/assertion"
)

const (
	testIssuer   = "https://student-app.test"
	testAudience = "cse-mark"
	testSubject  = "2111111"
)

// fakeJwks is a hand-rolled jwks.Repository for the middleware tests; err,
// when set, makes every SigningKey call fail (JWKS endpoint down).
type fakeJwks struct {
	keys map[string]ed25519.PublicKey
	err  error
}

func (f *fakeJwks) SigningKey(kid string) (ed25519.PublicKey, error) {
	if f.err != nil {
		return nil, f.err
	}
	key, ok := f.keys[kid]
	if !ok {
		return nil, jwks.ErrUnknownKid
	}
	return key, nil
}

func newTestKeyPair(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	return priv, pub
}

// mintTestToken signs an EdDSA JWT the way the student app does (aud as a
// JSON array, kid in the header).
func mintTestToken(t *testing.T, priv ed25519.PrivateKey, kid, issuer, subject string, audience []string, expiresAt time.Time) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		Audience:  jwt.ClaimStrings(audience),
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func newJwtForTest(repo *fakeJwks) *Jwt {
	return NewJwtMiddleware(assertion.NewService(repo, testIssuer, testAudience))
}

// runHandle builds a gin context by hand around a recorder, points it at a
// request with the given Authorization header, and runs the middleware.
func runHandle(m *Jwt, authHeader string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/marks", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	c.Request = req
	m.Handle(c)
	return w, c
}

func assertRejected(t *testing.T, w *httptest.ResponseRecorder, c *gin.Context, wantStatus int, wantBody string) {
	t.Helper()
	if w.Code != wantStatus {
		t.Errorf("status = %d, want %d", w.Code, wantStatus)
	}
	if got := strings.TrimSpace(w.Body.String()); got != wantBody {
		t.Errorf("body = %s, want %s", got, wantBody)
	}
	if !c.IsAborted() {
		t.Error("context not aborted, want Abort")
	}
}

func TestJwtHandle_MissingHeader(t *testing.T) {
	_, pub := newTestKeyPair(t)
	m := newJwtForTest(&fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": pub}})

	w, c := runHandle(m, "")

	assertRejected(t, w, c, 401, `{"error":"jwt_invalid"}`)
}

func TestJwtHandle_WrongPrefix(t *testing.T) {
	priv, pub := newTestKeyPair(t)
	m := newJwtForTest(&fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": pub}})
	token := mintTestToken(t, priv, "key-a", testIssuer, testSubject, []string{testAudience}, time.Now().Add(5*time.Minute))

	// The student app sends exactly "Bearer "; anything else is malformed.
	for _, h := range []string{
		token,                // bare token, no scheme
		"bearer " + token,    // lowercase scheme
		"Basic dXNlcjpwYXNz", // unrelated scheme
		"Bearer",             // scheme without the trailing space
		"Bearer ",            // empty credentials
	} {
		w, c := runHandle(m, h)
		assertRejected(t, w, c, 401, `{"error":"jwt_invalid"}`)
	}
}

func TestJwtHandle_Expired(t *testing.T) {
	priv, pub := newTestKeyPair(t)
	m := newJwtForTest(&fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": pub}})
	token := mintTestToken(t, priv, "key-a", testIssuer, testSubject, []string{testAudience}, time.Now().Add(-time.Minute))

	w, c := runHandle(m, "Bearer "+token)

	assertRejected(t, w, c, 401, `{"error":"jwt_expired"}`)
}

func TestJwtHandle_JwksUnavailable(t *testing.T) {
	priv, pub := newTestKeyPair(t)
	m := newJwtForTest(&fakeJwks{
		keys: map[string]ed25519.PublicKey{"key-a": pub},
		err:  jwks.ErrUnavailable,
	})
	token := mintTestToken(t, priv, "key-a", testIssuer, testSubject, []string{testAudience}, time.Now().Add(5*time.Minute))

	w, c := runHandle(m, "Bearer "+token)

	assertRejected(t, w, c, 503, `{"error":"jwks_unavailable"}`)
}

func TestJwtHandle_InvalidSignature(t *testing.T) {
	// The JWKS holds pair A's public key; the token is signed by pair B's
	// private key under the same kid.
	_, jwksPub := newTestKeyPair(t)
	otherPriv, _ := newTestKeyPair(t)
	m := newJwtForTest(&fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": jwksPub}})
	token := mintTestToken(t, otherPriv, "key-a", testIssuer, testSubject, []string{testAudience}, time.Now().Add(5*time.Minute))

	w, c := runHandle(m, "Bearer "+token)

	assertRejected(t, w, c, 401, `{"error":"jwt_invalid"}`)
}

// TestJwtHandle_Valid runs the real handler chain: CreateTestContext cannot
// run c.Next through registered handlers, so the success path uses a real
// engine with the route wired exactly like service.go.
func TestJwtHandle_Valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	priv, pub := newTestKeyPair(t)
	m := newJwtForTest(&fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": pub}})
	token := mintTestToken(t, priv, "key-a", testIssuer, testSubject, []string{testAudience}, time.Now().Add(5*time.Minute))

	var reached bool
	var gotSub string
	engine := gin.New()
	engine.GET("/marks", m.Handle, func(c *gin.Context) {
		reached = true
		gotSub = c.GetString("sub")
		c.String(200, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/marks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !reached {
		t.Fatal("next handler did not run, want it to run after a valid token")
	}
	if gotSub != testSubject {
		t.Errorf(`context "sub" = %q, want %q`, gotSub, testSubject)
	}
}

// TestJwtHandle_DoesNotEchoToken guarantees a failure response never echoes
// the token back into the body.
func TestJwtHandle_DoesNotEchoToken(t *testing.T) {
	priv, pub := newTestKeyPair(t)
	m := newJwtForTest(&fakeJwks{keys: map[string]ed25519.PublicKey{"key-a": pub}})
	token := mintTestToken(t, priv, "key-a", testIssuer, testSubject, []string{testAudience}, time.Now().Add(-time.Minute))

	w, _ := runHandle(m, "Bearer "+token)

	if strings.Contains(w.Body.String(), token) {
		t.Error("response body contains the token")
	}
}
