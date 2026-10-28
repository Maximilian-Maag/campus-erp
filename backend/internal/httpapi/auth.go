package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/auth"
)

// principalFrom returns the authenticated principal attached by authenticate.
func principalFrom(ctx context.Context) (auth.Principal, bool) {
	p, ok := ctx.Value(principalKey).(auth.Principal)
	return p, ok
}

// authenticate enforces a bearer token on the API surface. Health/readiness and
// the OpenAPI document stay open so probes and tooling work without a token.
// When no verifier is configured the middleware is a pass-through (dev only).
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		token := bearerToken(r)
		principal, err := s.auth.Verify(r.Context(), token)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, principal)))
	})
}

// authorize wraps a route handler so only an authenticated principal holding one
// of the roles may call it (ADR-0003). It is the per-route role gate that sits
// behind authenticate: authenticate proves *who* the caller is, authorize checks
// *what they may do*.
//
// With no verifier configured (local development) it is a pass-through, matching
// authenticate. With auth enabled a caller that reaches the route without a
// principal (defence in depth, if a route is ever mounted outside authenticate) is
// rejected as unauthenticated; a principal lacking every listed role gets 403.
// An empty role list means "any authenticated caller". When module roles are
// listed, erp-admin is always accepted too, so a route lists only the *module*
// role that owns it.
func (s *Server) authorize(h http.HandlerFunc, roles ...string) http.HandlerFunc {
	allowed := roles
	if len(roles) > 0 {
		allowed = append([]string{auth.RoleAdmin}, roles...)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			h(w, r)
			return
		}
		p, ok := principalFrom(r.Context())
		if !ok {
			writeProblem(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if err := auth.RequireRoles(p, allowed...); err != nil {
			writeProblem(w, http.StatusForbidden, "forbidden")
			return
		}
		h(w, r)
	}
}

// authorizeRead gates a read route: the owning module role, any reader, or admin.
func (s *Server) authorizeRead(h http.HandlerFunc, module string) http.HandlerFunc {
	return s.authorize(h, module, auth.RoleReader)
}

func bearerToken(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return parts[1]
	}
	return ""
}
