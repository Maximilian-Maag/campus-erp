// Package auth authenticates requests against the university identity provider
// (OIDC) and maps token claims to a Principal (ADR-0003). It is stdlib-only: the
// JWT is an RS256 token verified against the IdP's JWKS, so no third-party
// dependency is needed for the security-critical path.
//
// The package is deliberately split from the HTTP layer: the transport depends on
// the Verifier interface, so it can be tested with a static verifier and swapped
// for the JWKS-backed one in production.
package auth

import (
	"context"
	"errors"
	"fmt"
)

// Errors callers can map to HTTP status codes.
var (
	ErrMissingToken  = errors.New("auth: missing bearer token")
	ErrInvalidToken  = errors.New("auth: invalid token")
	ErrExpired       = errors.New("auth: token expired")
	ErrWrongIssuer   = errors.New("auth: wrong issuer")
	ErrWrongAudience = errors.New("auth: wrong audience")
)

// Principal is the authenticated caller.
type Principal struct {
	Subject string   // token "sub"
	Email   string   // token "email", when present
	Name    string   // token "name", when present
	Roles   []string // mapped from the roles claim
}

// Has reports whether the principal carries the role.
func (p Principal) Has(role string) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAny reports whether the principal carries any of the roles.
func (p Principal) HasAny(roles ...string) bool {
	for _, r := range roles {
		if p.Has(r) {
			return true
		}
	}
	return false
}

// Verifier turns a bearer token into a Principal.
type Verifier interface {
	Verify(ctx context.Context, token string) (Principal, error)
}

// Roles the ERP recognises. They are coarse: the fine-grained legacy
// sv_rechte_sachb model is mapped onto these at the IdP / token-mapping layer.
const (
	RoleAdmin    = "erp-admin"
	RoleKasse    = "erp-kasse"
	RoleFinanzen = "erp-finanzen"
	RolePersonal = "erp-personal"
	RoleKLR      = "erp-klr"
	RoleReader   = "erp-reader"
)

// RequireRoles returns an error unless the principal holds at least one role.
func RequireRoles(p Principal, roles ...string) error {
	if len(roles) == 0 {
		return nil
	}
	if p.HasAny(roles...) {
		return nil
	}
	return fmt.Errorf("auth: principal %q lacks required role(s) %v", p.Subject, roles)
}

// StaticVerifier accepts a fixed set of tokens and is intended for tests and
// local development. It must never be enabled in production.
type StaticVerifier struct {
	Tokens map[string]Principal
}

// Verify implements Verifier.
func (v StaticVerifier) Verify(_ context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrMissingToken
	}
	p, ok := v.Tokens[token]
	if !ok {
		return Principal{}, ErrInvalidToken
	}
	return p, nil
}
