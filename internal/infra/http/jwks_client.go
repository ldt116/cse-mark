package http

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"thuanle/cse-mark/internal/domain/jwks"
)

// jwksMaxBodyBytes caps the JWKS response body to protect against a hostile or
// broken endpoint returning an unbounded document.
const jwksMaxBodyBytes = 1 << 20 // 1 MiB

// JwksClient is a jwks.Repository backed by a remote JWKS endpoint. Keys are
// cached in memory and refreshed when the TTL expires or when a kid is missing
// from a still-fresh cache (key rotation). It never logs: callers own logging.
type JwksClient struct {
	url    string
	client *http.Client
	ttl    time.Duration

	mu        sync.Mutex
	keys      map[string]ed25519.PublicKey
	fetchedAt time.Time
}

// Compile-time check that JwksClient satisfies the domain port.
var _ jwks.Repository = (*JwksClient)(nil)

// NewJwksClient creates a client for the given JWKS URL. Timeout bounds each
// HTTP request; ttl bounds how long a successfully fetched key set is served
// without refetching.
func NewJwksClient(url string, timeout, ttl time.Duration) *JwksClient {
	return &JwksClient{
		url:    url,
		client: &http.Client{Timeout: timeout},
		ttl:    ttl,
	}
}

// SigningKey returns the Ed25519 public key for kid. A fresh cache hit returns
// immediately; a kid missing from a fresh cache triggers one refresh (rotation)
// before giving up with ErrUnknownKid — unless the fresh cache is an empty key
// set, which is served as-is until the TTL expires (no per-call refetching). A
// refresh failure returns ErrUnavailable and leaves the previously cached keys
// intact.
func (c *JwksClient) SigningKey(kid string) (ed25519.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Fresh means "successfully fetched recently", even if that fetch yielded
	// zero usable keys: an empty key set must still be served from cache until
	// the TTL expires, instead of refetching on every call.
	if !c.fetchedAt.IsZero() && time.Since(c.fetchedAt) < c.ttl {
		if key, ok := c.keys[kid]; ok {
			return key, nil
		}
		// The kid may have been rotated in after this cache entry was fetched
		// — refresh once and look again. An empty cached key set, however, was
		// fetched whole within the TTL: a refetch cannot surface the kid and
		// would hammer the endpoint on every call, so fail fast instead.
		if len(c.keys) == 0 {
			return nil, jwks.ErrUnknownKid
		}
		if err := c.refreshLocked(); err != nil {
			return nil, err
		}
		return c.lookupLocked(kid)
	}

	if err := c.refreshLocked(); err != nil {
		return nil, err
	}
	return c.lookupLocked(kid)
}

// lookupLocked resolves kid from the current cache; requires c.mu held.
func (c *JwksClient) lookupLocked(kid string) (ed25519.PublicKey, error) {
	if key, ok := c.keys[kid]; ok {
		return key, nil
	}
	return nil, jwks.ErrUnknownKid
}

// refreshLocked fetches and installs a fresh key set. On any failure it
// returns ErrUnavailable and leaves the cached keys untouched, so previously
// fetched keys stay usable while the endpoint is down; requires c.mu held.
func (c *JwksClient) refreshLocked() error {
	resp, err := c.client.Get(c.url)
	if err != nil {
		return jwks.ErrUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return jwks.ErrUnavailable
	}

	var doc jwksDocument
	if err := json.NewDecoder(io.LimitReader(resp.Body, jwksMaxBodyBytes)).Decode(&doc); err != nil {
		return jwks.ErrUnavailable
	}

	keys := make(map[string]ed25519.PublicKey, len(doc.Keys))
	for _, entry := range doc.Keys {
		if entry.Kty != "OKP" || entry.Crv != "Ed25519" || entry.Kid == "" {
			continue
		}
		// Skip malformed entries silently: a single bad key must not make the
		// whole document unusable.
		raw, err := base64.RawURLEncoding.DecodeString(entry.X)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			continue
		}
		keys[entry.Kid] = ed25519.PublicKey(raw)
	}

	c.keys = keys
	c.fetchedAt = time.Now()
	return nil
}

// jwksDocument is the RFC 7517 JSON Web Key Set, restricted to the Ed25519
// fields the student app emits.
type jwksDocument struct {
	Keys []jwksKey `json:"keys"`
}

type jwksKey struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
}
