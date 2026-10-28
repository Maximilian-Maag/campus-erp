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

func bearerToken(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return parts[1]
	}
	return ""
}
