package http

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"thuanle/cse-mark/internal/domain/jwks"
)

// stubJwks is a mutable JWKS test double: the served key set (or raw document
// and failure flag) can be swapped between requests, and every request is
// counted.
type stubJwks struct {
	mu      sync.Mutex
	keys    map[string]ed25519.PublicKey
	raw     string // when set, served verbatim instead of keys
	fail    bool
	counter atomic.Int32
}

func (s *stubJwks) setKeys(keys map[string]ed25519.PublicKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = keys
	s.raw = ""
}

func (s *stubJwks) setRaw(raw string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw = raw
}

func (s *stubJwks) setFail(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = fail
}

func (s *stubJwks) requests() int32 {
	return s.counter.Load()
}

func (s *stubJwks) serve(w http.ResponseWriter, r *http.Request) {
	s.counter.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if s.raw != "" {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, s.raw)
		return
	}
	keys := make([]map[string]string, 0, len(s.keys))
	for kid, pub := range s.keys {
		keys = append(keys, map[string]string{
			"kty": "OKP",
			"crv": "Ed25519",
			"kid": kid,
			"x":   base64.RawURLEncoding.EncodeToString(pub),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
}

func newJwksStub(t *testing.T) (*stubJwks, *httptest.Server) {
	t.Helper()
	s := &stubJwks{}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	return s, srv
}

func mustGenerateKey(t *testing.T) (string, ed25519.PublicKey) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	return fmt.Sprintf("key-%d", time.Now().UnixNano()), pub
}

// longTTL keeps the cache fresh for the whole test, isolating the behavior
// under test from TTL expiry.
const longTTL = time.Minute

// backdateThrottle moves the last fetch attempt outside the throttle window
// so that a fetch SigningKey wants to perform is actually attempted. The
// cache timestamp (fetchedAt) is untouched: the cache stays fresh.
func backdateThrottle(c *JwksClient) {
	c.mu.Lock()
	c.lastFetchAttempt = time.Now().Add(-time.Hour)
	c.mu.Unlock()
}

// expireCache backdates both the cache timestamp (past the TTL — the expiry
// under test) and the throttle timestamp (past the fetch window), simulating
// the passage of time without sleeping.
func expireCache(c *JwksClient) {
	c.mu.Lock()
	c.fetchedAt = time.Now().Add(-time.Hour)
	c.lastFetchAttempt = c.fetchedAt
	c.mu.Unlock()
}

func TestSigningKey_FirstFetchCaches(t *testing.T) {
	kidA, pubA := mustGenerateKey(t)
	stub, srv := newJwksStub(t)
	stub.setKeys(map[string]ed25519.PublicKey{kidA: pubA})

	client := NewJwksClient(srv.URL, 2*time.Second, longTTL)

	got1, err := client.SigningKey(kidA)
	if err != nil {
		t.Fatalf("first SigningKey: %v", err)
	}
	got2, err := client.SigningKey(kidA)
	if err != nil {
		t.Fatalf("second SigningKey: %v", err)
	}

	if !bytes.Equal(got1, pubA) || !bytes.Equal(got2, pubA) {
		t.Fatalf("SigningKey returned wrong key: got %x / %x, want %x", got1, got2, pubA)
	}
	if n := stub.requests(); n != 1 {
		t.Fatalf("expected exactly 1 request (cache hit on second call), got %d", n)
	}
}

func TestSigningKey_TTLExpiryRefetches(t *testing.T) {
	kidA, pubA := mustGenerateKey(t)
	stub, srv := newJwksStub(t)
	stub.setKeys(map[string]ed25519.PublicKey{kidA: pubA})

	client := NewJwksClient(srv.URL, 2*time.Second, 50*time.Millisecond)

	if _, err := client.SigningKey(kidA); err != nil {
		t.Fatalf("SigningKey before expiry: %v", err)
	}
	// Simulate TTL expiry (and the throttle window passing) without a sleep.
	expireCache(client)
	got, err := client.SigningKey(kidA)
	if err != nil {
		t.Fatalf("SigningKey after expiry: %v", err)
	}

	if !bytes.Equal(got, pubA) {
		t.Fatalf("wrong key after refetch: got %x, want %x", got, pubA)
	}
	if n := stub.requests(); n != 2 {
		t.Fatalf("expected 2 requests after TTL expiry, got %d", n)
	}
}

func TestSigningKey_UnknownKidTriggersRefresh(t *testing.T) {
	kidA, pubA := mustGenerateKey(t)
	kidB, pubB := mustGenerateKey(t)
	stub, srv := newJwksStub(t)
	stub.setKeys(map[string]ed25519.PublicKey{kidA: pubA})

	client := NewJwksClient(srv.URL, 2*time.Second, longTTL)

	if _, err := client.SigningKey(kidA); err != nil {
		t.Fatalf("SigningKey(kidA): %v", err)
	}
	if n := stub.requests(); n != 1 {
		t.Fatalf("expected 1 request after first call, got %d", n)
	}

	// Rotation: kid B appears server-side while the cache is still fresh.
	stub.setKeys(map[string]ed25519.PublicKey{kidA: pubA, kidB: pubB})
	// The rotation refresh would be throttled right after the first fetch:
	// move the attempt outside the window, keeping the cache fresh.
	backdateThrottle(client)

	got, err := client.SigningKey(kidB)
	if err != nil {
		t.Fatalf("SigningKey(kidB) should refresh within TTL: %v", err)
	}

	if !bytes.Equal(got, pubB) {
		t.Fatalf("wrong key for kidB: got %x, want %x", got, pubB)
	}
	if n := stub.requests(); n != 2 {
		t.Fatalf("unknown kid must trigger a refresh while cache is fresh: expected 2 requests, got %d", n)
	}
}

func TestSigningKey_UnknownKidAfterRefresh(t *testing.T) {
	kidA, pubA := mustGenerateKey(t)
	stub, srv := newJwksStub(t)
	stub.setKeys(map[string]ed25519.PublicKey{kidA: pubA})

	client := NewJwksClient(srv.URL, 2*time.Second, longTTL)

	if _, err := client.SigningKey(kidA); err != nil {
		t.Fatalf("SigningKey(kidA): %v", err)
	}
	// Outside the throttle window so the rotation refresh is attempted.
	backdateThrottle(client)

	_, err := client.SigningKey("kid-never-exists")
	if !errors.Is(err, jwks.ErrUnknownKid) {
		t.Fatalf("expected ErrUnknownKid, got %v", err)
	}
	if n := stub.requests(); n != 2 {
		t.Fatalf("expected exactly one refresh attempt for unknown kid, got %d requests", n)
	}
}

func TestSigningKey_ServerErrorUnavailable(t *testing.T) {
	kidA, pubA := mustGenerateKey(t)
	stub, srv := newJwksStub(t)
	stub.setKeys(map[string]ed25519.PublicKey{kidA: pubA})

	client := NewJwksClient(srv.URL, 2*time.Second, longTTL)

	if _, err := client.SigningKey(kidA); err != nil {
		t.Fatalf("SigningKey(kidA) before failure: %v", err)
	}

	stub.setFail(true)
	// Outside the throttle window so the failing refresh is attempted.
	backdateThrottle(client)

	_, err := client.SigningKey("kid-missing")
	if !errors.Is(err, jwks.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable on server error, got %v", err)
	}

	// The old cache must survive the failed refresh: kidA is still served
	// without another request.
	got, err := client.SigningKey(kidA)
	if err != nil {
		t.Fatalf("SigningKey(kidA) after failed refresh must hit cache: %v", err)
	}
	if !bytes.Equal(got, pubA) {
		t.Fatalf("cached key changed: got %x, want %x", got, pubA)
	}
	if n := stub.requests(); n != 2 {
		t.Fatalf("expected 2 requests total (initial + failed refresh), got %d", n)
	}
}

func TestSigningKey_SkipsBadEntries(t *testing.T) {
	_, pubGood := mustGenerateKey(t)
	stub, srv := newJwksStub(t)
	stub.setRaw(fmt.Sprintf(`{"keys":[
		{"kty":"OKP","crv":"Ed25519","kid":"bad-x","x":"not!base64!!"},
		{"kty":"OKP","crv":"Ed25519","kid":"good","x":"%s"}
	]}`, base64.RawURLEncoding.EncodeToString(pubGood)))

	client := NewJwksClient(srv.URL, 2*time.Second, longTTL)

	got, err := client.SigningKey("good")
	if err != nil {
		t.Fatalf("good entry must survive a bad sibling: %v", err)
	}
	if !bytes.Equal(got, pubGood) {
		t.Fatalf("wrong key for good entry: got %x, want %x", got, pubGood)
	}

	if _, err := client.SigningKey("bad-x"); !errors.Is(err, jwks.ErrUnknownKid) {
		t.Fatalf("bad entry must be skipped, expected ErrUnknownKid, got %v", err)
	}
}

// TestSigningKey_SkipsWrongShapeEntries pins the refreshLocked guards
// (#44 final review F7): entries that decode to something other than a
// 32-byte Ed25519 public key (here 31 bytes), or carry a wrong kty/crv or an
// empty kid, must be skipped without panic — a hostile or buggy JWKS document
// must not crash the client (ed25519.Verify panics on wrong-length keys, the
// len(raw) != ed25519.PublicKeySize check is what keeps them out).
func TestSigningKey_SkipsWrongShapeEntries(t *testing.T) {
	stub, srv := newJwksStub(t)
	stub.setRaw(fmt.Sprintf(`{"keys":[
		{"kty":"OKP","crv":"Ed25519","kid":"short-x","x":"%s"},
		{"kty":"RSA","crv":"Ed25519","kid":"wrong-kty","x":"%s"},
		{"kty":"OKP","crv":"P-256","kid":"wrong-crv","x":"%s"},
		{"kty":"OKP","crv":"Ed25519","kid":"","x":"%s"}
	]}`,
		base64.RawURLEncoding.EncodeToString(make([]byte, 31)), // one byte short
		base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
		base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
		base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
	))

	client := NewJwksClient(srv.URL, 2*time.Second, longTTL)

	for _, kid := range []string{"short-x", "wrong-kty", "wrong-crv", ""} {
		if _, err := client.SigningKey(kid); !errors.Is(err, jwks.ErrUnknownKid) {
			t.Errorf("SigningKey(%q): %v, want ErrUnknownKid (entry must be skipped, no panic)", kid, err)
		}
	}
}

// TestSigningKey_EmptyKeysetServedFromCache pins #44 final review F3: a JWKS
// document with zero usable keys is still a successful fetch, so the cache
// must treat it as fresh — a second lookup is served from cache (no refetch)
// and fails with ErrUnknownKid, instead of hammering the endpoint on every
// single call for as long as the endpoint keeps returning an empty key set.
func TestSigningKey_EmptyKeysetServedFromCache(t *testing.T) {
	stub, srv := newJwksStub(t)
	stub.setRaw(`{"keys":[]}`)

	client := NewJwksClient(srv.URL, 2*time.Second, longTTL)

	if _, err := client.SigningKey("kid-a"); !errors.Is(err, jwks.ErrUnknownKid) {
		t.Fatalf("first SigningKey on empty key set: %v, want ErrUnknownKid", err)
	}
	if n := stub.requests(); n != 1 {
		t.Fatalf("expected 1 request after first call, got %d", n)
	}

	if _, err := client.SigningKey("kid-a"); !errors.Is(err, jwks.ErrUnknownKid) {
		t.Fatalf("second SigningKey on empty key set: %v, want ErrUnknownKid", err)
	}
	if n := stub.requests(); n != 1 {
		t.Fatalf("empty key set must be served from cache within TTL: expected still 1 request, got %d", n)
	}
}

// TestSigningKey_EmptyKeysetRecoversAfterTtl is the other half of F3: the
// empty-key-set cache is still TTL-bounded, so once the endpoint recovers and
// the TTL expires, the client must pick the recovered keys up — the fail-fast
// path never turns into a permanently stuck empty cache.
func TestSigningKey_EmptyKeysetRecoversAfterTtl(t *testing.T) {
	stub, srv := newJwksStub(t)
	_, pubA := mustGenerateKey(t)

	client := NewJwksClient(srv.URL, 2*time.Second, 50*time.Millisecond)

	stub.setRaw(`{"keys":[]}`)
	if _, err := client.SigningKey("kid-a"); !errors.Is(err, jwks.ErrUnknownKid) {
		t.Fatalf("SigningKey on empty key set: %v, want ErrUnknownKid", err)
	}

	// Simulate TTL expiry (and the throttle window passing) without a sleep.
	expireCache(client)

	// The endpoint recovers with a real key.
	stub.setKeys(map[string]ed25519.PublicKey{"kid-a": pubA})
	got, err := client.SigningKey("kid-a")
	if err != nil {
		t.Fatalf("SigningKey after endpoint recovery and TTL expiry: %v, want nil", err)
	}
	if !bytes.Equal(got, pubA) {
		t.Fatalf("wrong key after recovery: got %x, want %x", got, pubA)
	}
	if n := stub.requests(); n != 2 {
		t.Fatalf("expected 2 requests (initial empty fetch + refetch after TTL), got %d", n)
	}
}

// TestSigningKey_UnknownKidThrottledRefetch pins the fetch throttle against
// kid spam (#44 review N1): while the cache is fresh, unknown kids trigger at
// most one fetch per fetchMinInterval — a second unknown-kid call inside the
// window returns ErrUnknownKid without another outbound request, instead of
// amplifying one request per /marks call.
func TestSigningKey_UnknownKidThrottledRefetch(t *testing.T) {
	kidA, pubA := mustGenerateKey(t)
	stub, srv := newJwksStub(t)
	stub.setKeys(map[string]ed25519.PublicKey{kidA: pubA})

	client := NewJwksClient(srv.URL, 2*time.Second, longTTL)

	_, err := client.SigningKey("kid-stranger-1")
	if !errors.Is(err, jwks.ErrUnknownKid) {
		t.Fatalf("first unknown kid: %v, want ErrUnknownKid", err)
	}
	if n := stub.requests(); n != 1 {
		t.Fatalf("first unknown kid must fetch exactly once, got %d requests", n)
	}

	_, err = client.SigningKey("kid-stranger-2")
	if !errors.Is(err, jwks.ErrUnknownKid) {
		t.Fatalf("second unknown kid: %v, want ErrUnknownKid", err)
	}
	if n := stub.requests(); n != 1 {
		t.Fatalf("second unknown kid within the throttle window must not fetch: expected 1 request, got %d", n)
	}
}

// TestSigningKey_FailedFetchThrottled pins the other half of the throttle: a
// failed fetch (endpoint down, here HTTP 500) counts as an attempt too — the
// immediately following request on a serveless cache gets ErrUnavailable
// without a new outbound request (no back-to-back 15s stalls under the
// mutex); once the window passes the fetch is retried.
func TestSigningKey_FailedFetchThrottled(t *testing.T) {
	kidA, pubA := mustGenerateKey(t)
	stub, srv := newJwksStub(t)
	stub.setFail(true)

	client := NewJwksClient(srv.URL, 2*time.Second, longTTL)

	// Cache has never been populated: the request cannot be served at all.
	if _, err := client.SigningKey(kidA); !errors.Is(err, jwks.ErrUnavailable) {
		t.Fatalf("first call on failing endpoint: %v, want ErrUnavailable", err)
	}
	if n := stub.requests(); n != 1 {
		t.Fatalf("first call must attempt one fetch, got %d requests", n)
	}

	if _, err := client.SigningKey(kidA); !errors.Is(err, jwks.ErrUnavailable) {
		t.Fatalf("immediate retry: %v, want ErrUnavailable", err)
	}
	if n := stub.requests(); n != 1 {
		t.Fatalf("immediate retry must be throttled: expected 1 request, got %d", n)
	}

	// Window passed (backdated): the fetch is retried and now succeeds.
	stub.setFail(false)
	stub.setKeys(map[string]ed25519.PublicKey{kidA: pubA})
	backdateThrottle(client)
	got, err := client.SigningKey(kidA)
	if err != nil {
		t.Fatalf("SigningKey after throttle window: %v, want nil", err)
	}
	if !bytes.Equal(got, pubA) {
		t.Fatalf("wrong key after retry: got %x, want %x", got, pubA)
	}
	if n := stub.requests(); n != 2 {
		t.Fatalf("retry after window must fetch again: expected 2 requests, got %d", n)
	}
}
