package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// KeySet resolves a signing key by its kid.
type KeySet interface {
	Key(ctx context.Context, kid string) (*rsa.PublicKey, error)
}

// ParseJWKS builds an RSA KeySet from a JWKS document (RFC 7517). Only RSA keys
// (the algorithm the IdP uses for RS256) are considered.
func ParseJWKS(data []byte) (*StaticKeySet, error) {
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
			Alg string `json:"alg"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("auth: jwks: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.N == "" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, fmt.Errorf("auth: jwks n: %w", err)
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, fmt.Errorf("auth: jwks e: %w", err)
		}
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("auth: jwks has no RSA keys")
	}
	return &StaticKeySet{Keys: keys}, nil
}

// StaticKeySet is a fixed set of public keys, e.g. parsed from a JWKS document.
type StaticKeySet struct {
	Keys map[string]*rsa.PublicKey
}

// Key implements KeySet.
func (s *StaticKeySet) Key(_ context.Context, kid string) (*rsa.PublicKey, error) {
	if k, ok := s.Keys[kid]; ok {
		return k, nil
	}
	// A single-key set may omit kid in the token.
	if kid == "" && len(s.Keys) == 1 {
		for _, k := range s.Keys {
			return k, nil
		}
	}
	return nil, fmt.Errorf("auth: unknown key id %q", kid)
}

// RemoteKeySet fetches JWKS from an OIDC provider and caches it, refreshing on a
// miss (key rotation) or after the TTL.
type RemoteKeySet struct {
	URL    string
	Client *http.Client
	TTL    time.Duration

	mu        sync.Mutex
	cached    *StaticKeySet
	fetchedAt time.Time
}

// fresh reports whether a cached value fetched at fetchedAt is still within ttl
// as of now. Extracted so the boundary is unit-testable.
func fresh(now, fetchedAt time.Time, ttl time.Duration) bool {
	return now.Sub(fetchedAt) < ttl
}

// Key implements KeySet with a cached fetch.
func (r *RemoteKeySet) Key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cached != nil && fresh(time.Now(), r.fetchedAt, r.TTL) {
		if k, err := r.cached.Key(ctx, kid); err == nil {
			return k, nil
		}
		// fall through to refresh (rotation)
	}
	set, err := r.fetch(ctx)
	if err != nil {
		if r.cached != nil {
			return r.cached.Key(ctx, kid) // serve stale on a fetch error
		}
		return nil, err
	}
	r.cached, r.fetchedAt = set, time.Now()
	return set.Key(ctx, kid)
}

func (r *RemoteKeySet) fetch(ctx context.Context) (*StaticKeySet, error) {
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: jwks fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: jwks fetch: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("auth: jwks read: %w", err)
	}
	return ParseJWKS(body)
}
