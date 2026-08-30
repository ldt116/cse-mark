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
	time.Sleep(60 * time.Millisecond)
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
