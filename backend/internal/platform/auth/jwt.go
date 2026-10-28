package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JWKVerifier verifies RS256 JWTs against a KeySet and the expected issuer and
// audience. It validates alg, signature, exp/nbf (with leeway) and iss/aud, then
// maps the claims to a Principal.
type JWKVerifier struct {
	Keys       KeySet
	Issuer     string
	Audience   string
	RolesClaim string // default "roles"
	Leeway     time.Duration
	Now        func() time.Time // injectable for tests
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Sub   string          `json:"sub"`
	Iss   string          `json:"iss"`
	Aud   json.RawMessage `json:"aud"`
	Exp   int64           `json:"exp"`
	Nbf   int64           `json:"nbf"`
	Email string          `json:"email"`
	Name  string          `json:"name"`
	// Roles is captured dynamically below via the configured claim name.
}

func (v *JWKVerifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

func (v *JWKVerifier) rolesClaim() string {
	if v.RolesClaim != "" {
		return v.RolesClaim
	}
	return "roles"
}

// Verify implements Verifier.
func (v *JWKVerifier) Verify(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrMissingToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Principal{}, ErrInvalidToken
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	var hdr jwtHeader
	if err := json.Unmarshal(headerJSON, &hdr); err != nil {
		return Principal{}, ErrInvalidToken
	}
	if hdr.Alg != "RS256" {
		return Principal{}, fmt.Errorf("%w: unexpected alg %q", ErrInvalidToken, hdr.Alg)
	}
	key, err := v.Keys.Key(ctx, hdr.Kid)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
		return Principal{}, ErrInvalidToken
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	var claims jwtClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return Principal{}, ErrInvalidToken
	}

	now := v.now().Unix()
	if claims.Exp != 0 && now > claims.Exp+int64(v.Leeway.Seconds()) {
		return Principal{}, ErrExpired
	}
	if claims.Nbf != 0 && now+int64(v.Leeway.Seconds()) < claims.Nbf {
		return Principal{}, ErrInvalidToken
	}
	if v.Issuer != "" && claims.Iss != v.Issuer {
		return Principal{}, ErrWrongIssuer
	}
	if v.Audience != "" && !audienceContains(claims.Aud, v.Audience) {
		return Principal{}, ErrWrongAudience
	}
	if claims.Sub == "" {
		return Principal{}, fmt.Errorf("%w: missing sub", ErrInvalidToken)
	}

	roles, err := extractRoles(payloadJSON, v.rolesClaim())
	if err != nil {
		return Principal{}, err
	}
	return Principal{Subject: claims.Sub, Email: claims.Email, Name: claims.Name, Roles: roles}, nil
}

func audienceContains(raw json.RawMessage, want string) bool {
	if len(raw) == 0 {
		return false
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single == want
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		for _, a := range many {
			if a == want {
				return true
			}
		}
	}
	return false
}

func extractRoles(payload []byte, claim string) ([]string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, ErrInvalidToken
	}
	raw, ok := m[claim]
	if !ok {
		return nil, nil
	}
	var roles []string
	if err := json.Unmarshal(raw, &roles); err == nil {
		return roles, nil
	}
	// Some IdPs send a single string.
	var one string
	if err := json.Unmarshal(raw, &one); err == nil && one != "" {
		return []string{one}, nil
	}
	return nil, nil
}
