package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func big2bytes(n int) []byte {
	if n == 0 {
		return []byte{0}
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte(n & 0xff)}, out...)
		n >>= 8
	}
	return out
}

func mustKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	return k
}

// sign builds an RS256 JWT with the given header and claim overrides.
func sign(t *testing.T, key *rsa.PrivateKey, kid string, alg string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": alg, "typ": "JWT"}
	if kid != "" {
		header["kid"] = kid
	}
	h, _ := json.Marshal(header)
	p, _ := json.Marshal(claims)
	signingInput := b64(h) + "." + b64(p)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signingInput + "." + b64(sig)
}

func jwksJSON(t *testing.T, kid string, key *rsa.PublicKey) []byte {
	t.Helper()
	e := big2bytes(key.E)
	doc := map[string]any{"keys": []map[string]any{{
		"kty": "RSA", "kid": kid, "alg": "RS256",
		"n": b64(key.N.Bytes()), "e": b64(e),
	}}}
	b, _ := json.Marshal(doc)
	return b
}

func TestStaticVerifier(t *testing.T) {
	v := StaticVerifier{Tokens: map[string]Principal{"tok": {Subject: "alice", Roles: []string{RoleKasse}}}}
	if _, err := v.Verify(context.Background(), ""); !errors.Is(err, ErrMissingToken) {
		t.Errorf("empty token: %v", err)
	}
	if _, err := v.Verify(context.Background(), "nope"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("unknown token: %v", err)
	}
	p, err := v.Verify(context.Background(), "tok")
	if err != nil || p.Subject != "alice" || !p.Has(RoleKasse) || p.Has(RoleAdmin) {
		t.Fatalf("verify: %+v %v", p, err)
	}
	if err := RequireRoles(p, RoleAdmin); err == nil {
		t.Error("RequireRoles should reject a missing role")
	}
	if err := RequireRoles(p, RoleKasse, RoleAdmin); err != nil {
		t.Errorf("RequireRoles HasAny: %v", err)
	}
	if err := RequireRoles(p); err != nil {
		t.Errorf("RequireRoles with no roles: %v", err)
	}
}

func newVerifier(t *testing.T, key *rsa.PrivateKey, kid string) *JWKVerifier {
	t.Helper()
	ks, err := ParseJWKS(jwksJSON(t, kid, &key.PublicKey))
	if err != nil {
		t.Fatalf("jwks: %v", err)
	}
	return &JWKVerifier{Keys: ks, Issuer: "https://idp.example.org", Audience: "campus-erp"}
}

func TestJWKVerifierHappyPath(t *testing.T) {
	key := mustKey(t)
	v := newVerifier(t, key, "k1")
	token := sign(t, key, "k1", "RS256", map[string]any{
		"sub": "alice", "iss": "https://idp.example.org", "aud": "campus-erp",
		"exp": time.Now().Add(time.Hour).Unix(), "email": "a@uni.example", "name": "Alice",
		"roles": []string{RoleKasse, RoleReader},
	})
	p, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if p.Subject != "alice" || p.Email != "a@uni.example" || !p.Has(RoleKasse) {
		t.Fatalf("principal: %+v", p)
	}
}

func TestJWKVerifierRejectsBadTokens(t *testing.T) {
	key := mustKey(t)
	v := newVerifier(t, key, "k1")
	base := map[string]any{"sub": "alice", "iss": "https://idp.example.org", "aud": "campus-erp", "exp": time.Now().Add(time.Hour).Unix()}

	cases := []struct {
		name  string
		token string
		want  error
	}{
		{"empty", "", ErrMissingToken},
		{"not three parts", "a.b", ErrInvalidToken},
		{"wrong signature", sign(t, mustKey(t), "k1", "RS256", base), ErrInvalidToken},
		{"bad alg", sign(t, key, "k1", "HS256", base), ErrInvalidToken},
		{"unknown kid", sign(t, key, "zzz", "RS256", base), ErrInvalidToken},
	}
	for _, tc := range cases {
		if _, err := v.Verify(context.Background(), tc.token); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}

	expired := map[string]any{"sub": "a", "iss": "https://idp.example.org", "aud": "campus-erp", "exp": time.Now().Add(-time.Hour).Unix()}
	if _, err := v.Verify(context.Background(), sign(t, key, "k1", "RS256", expired)); !errors.Is(err, ErrExpired) {
		t.Errorf("expired: %v", err)
	}
	wrongIss := map[string]any{"sub": "a", "iss": "https://evil.example", "aud": "campus-erp", "exp": time.Now().Add(time.Hour).Unix()}
	if _, err := v.Verify(context.Background(), sign(t, key, "k1", "RS256", wrongIss)); !errors.Is(err, ErrWrongIssuer) {
		t.Errorf("issuer: %v", err)
	}
	wrongAud := map[string]any{"sub": "a", "iss": "https://idp.example.org", "aud": "other", "exp": time.Now().Add(time.Hour).Unix()}
	if _, err := v.Verify(context.Background(), sign(t, key, "k1", "RS256", wrongAud)); !errors.Is(err, ErrWrongAudience) {
		t.Errorf("audience: %v", err)
	}
	noSub := map[string]any{"iss": "https://idp.example.org", "aud": "campus-erp", "exp": time.Now().Add(time.Hour).Unix()}
	if _, err := v.Verify(context.Background(), sign(t, key, "k1", "RS256", noSub)); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("missing sub: %v", err)
	}
}

func TestJWKVerifierAudienceArrayAndStringRoles(t *testing.T) {
	key := mustKey(t)
	v := newVerifier(t, key, "k1")
	token := sign(t, key, "k1", "RS256", map[string]any{
		"sub": "bob", "iss": "https://idp.example.org", "aud": []string{"other", "campus-erp"},
		"exp": time.Now().Add(time.Hour).Unix(), "roles": RoleAdmin,
	})
	p, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !p.Has(RoleAdmin) {
		t.Fatalf("string roles: %+v", p)
	}
}

func TestJWKVerifierLeewayAndNbf(t *testing.T) {
	key := mustKey(t)
	v := newVerifier(t, key, "k1")
	v.Leeway = 60 * time.Second
	v.Now = func() time.Time { return time.Unix(1_000_000, 0) }
	// exp 30s in the past relative to Now, within leeway.
	tok := sign(t, key, "k1", "RS256", map[string]any{"sub": "a", "iss": "https://idp.example.org", "aud": "campus-erp", "exp": 1_000_000 - 30})
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Errorf("leeway should accept: %v", err)
	}
	// nbf 120s in the future → invalid.
	tok2 := sign(t, key, "k1", "RS256", map[string]any{"sub": "a", "iss": "https://idp.example.org", "aud": "campus-erp", "exp": 1_000_000 + 1000, "nbf": 1_000_000 + 120})
	if _, err := v.Verify(context.Background(), tok2); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("nbf: %v", err)
	}
}

func TestParseJWKSRejectsGarbage(t *testing.T) {
	if _, err := ParseJWKS([]byte("not json")); err == nil {
		t.Error("expected error for non-JSON")
	}
	if _, err := ParseJWKS([]byte(`{"keys":[{"kty":"EC","crv":"P-256"}]}`)); err == nil {
		t.Error("expected error when no RSA keys")
	}
}

func TestStaticKeySetFallbackWithoutKid(t *testing.T) {
	key := mustKey(t)
	ks, _ := ParseJWKS(jwksJSON(t, "k1", &key.PublicKey))
	if _, err := ks.Key(context.Background(), ""); err != nil {
		t.Errorf("single key without kid should resolve: %v", err)
	}
	two := &StaticKeySet{Keys: map[string]*rsa.PublicKey{"a": &key.PublicKey, "b": &key.PublicKey}}
	if _, err := two.Key(context.Background(), ""); err == nil {
		t.Error("ambiguous kid with two keys should error")
	}
}

func TestFreshBoundary(t *testing.T) {
	base := time.Unix(1_000_000, 0)
	ttl := time.Minute
	if fresh(base, base, 0) {
		t.Error("zero ttl must be stale")
	}
	if !fresh(base, base.Add(-ttl+time.Nanosecond), ttl) {
		t.Error("just inside the ttl must be fresh")
	}
	if fresh(base, base.Add(-ttl), ttl) {
		t.Error("exactly at the ttl must be stale")
	}
}

func TestJWKVerifierExpNbfBoundaries(t *testing.T) {
	key := mustKey(t)
	v := newVerifier(t, key, "k1")
	now := time.Unix(2_000_000, 0)
	v.Now = func() time.Time { return now }
	iss := "https://idp.example.org"

	// exp exactly now, no leeway: accepted (now > exp is false).
	at := sign(t, key, "k1", "RS256", map[string]any{"sub": "a", "iss": iss, "aud": "campus-erp", "exp": now.Unix()})
	if _, err := v.Verify(context.Background(), at); err != nil {
		t.Errorf("exp==now should pass: %v", err)
	}
	justExpired := sign(t, key, "k1", "RS256", map[string]any{"sub": "a", "iss": iss, "aud": "campus-erp", "exp": now.Unix() - 1})
	if _, err := v.Verify(context.Background(), justExpired); !errors.Is(err, ErrExpired) {
		t.Errorf("exp==now-1 should expire: %v", err)
	}
	// nbf exactly now: accepted; nbf now+1: rejected.
	nbfNow := sign(t, key, "k1", "RS256", map[string]any{"sub": "a", "iss": iss, "aud": "campus-erp", "exp": now.Unix() + 100, "nbf": now.Unix()})
	if _, err := v.Verify(context.Background(), nbfNow); err != nil {
		t.Errorf("nbf==now should pass: %v", err)
	}
	nbfFuture := sign(t, key, "k1", "RS256", map[string]any{"sub": "a", "iss": iss, "aud": "campus-erp", "exp": now.Unix() + 100, "nbf": now.Unix() + 1})
	if _, err := v.Verify(context.Background(), nbfFuture); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("nbf==now+1 should fail: %v", err)
	}

	// leeway arithmetic: exp exactly now-leeway is accepted; one second further is not.
	v.Leeway = 60 * time.Second
	atEdge := sign(t, key, "k1", "RS256", map[string]any{"sub": "a", "iss": iss, "aud": "campus-erp", "exp": now.Unix() - 60})
	if _, err := v.Verify(context.Background(), atEdge); err != nil {
		t.Errorf("exp==now-leeway should pass: %v", err)
	}
	pastEdge := sign(t, key, "k1", "RS256", map[string]any{"sub": "a", "iss": iss, "aud": "campus-erp", "exp": now.Unix() - 61})
	if _, err := v.Verify(context.Background(), pastEdge); !errors.Is(err, ErrExpired) {
		t.Errorf("exp==now-leeway-1 should expire: %v", err)
	}
	// nbf arithmetic: now+leeway-1 accepted, now+leeway+1 rejected.
	nbfInLeeway := sign(t, key, "k1", "RS256", map[string]any{"sub": "a", "iss": iss, "aud": "campus-erp", "exp": now.Unix() + 1000, "nbf": now.Unix() + 59})
	if _, err := v.Verify(context.Background(), nbfInLeeway); err != nil {
		t.Errorf("nbf within leeway should pass: %v", err)
	}
	nbfPastLeeway := sign(t, key, "k1", "RS256", map[string]any{"sub": "a", "iss": iss, "aud": "campus-erp", "exp": now.Unix() + 1000, "nbf": now.Unix() + 61})
	if _, err := v.Verify(context.Background(), nbfPastLeeway); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("nbf beyond leeway should fail: %v", err)
	}
}

func TestJWKVerifierRolesNotAList(t *testing.T) {
	key := mustKey(t)
	v := newVerifier(t, key, "k1")
	tok := sign(t, key, "k1", "RS256", map[string]any{"sub": "a", "iss": "https://idp.example.org", "aud": "campus-erp", "exp": time.Now().Add(time.Hour).Unix(), "roles": 42})
	p, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(p.Roles) != 0 {
		t.Errorf("non-list roles should yield none, got %v", p.Roles)
	}
}

func TestRemoteKeySetFetchAndCache(t *testing.T) {
	key := mustKey(t)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksJSON(t, "k1", &key.PublicKey))
	}))
	defer srv.Close()

	rs := &RemoteKeySet{URL: srv.URL, TTL: time.Hour}
	k, err := rs.Key(context.Background(), "k1")
	if err != nil || k == nil {
		t.Fatalf("first fetch: %v", err)
	}
	if _, err := rs.Key(context.Background(), "k1"); err != nil {
		t.Fatalf("cached fetch: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 fetch (cached), got %d", calls)
	}
}

func TestRemoteKeySetServesStaleOnError(t *testing.T) {
	key := mustKey(t)
	ok := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(jwksJSON(t, "k1", &key.PublicKey))
	}))
	defer srv.Close()

	rs := &RemoteKeySet{URL: srv.URL, TTL: time.Nanosecond}
	if _, err := rs.Key(context.Background(), "k1"); err != nil {
		t.Fatalf("warm: %v", err)
	}
	ok = false
	time.Sleep(time.Millisecond) // let the TTL lapse so it refetches (and fails)
	if _, err := rs.Key(context.Background(), "k1"); err != nil {
		t.Errorf("should serve stale key on fetch error: %v", err)
	}
}
