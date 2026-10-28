package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/auth"
)

// okHandler stands in for a route handler: if it runs, authorisation passed.
func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

// authzServer builds a server whose verifier knows one token per role and a mux
// that mounts authorize/authorizeRead over stub handlers.
func authzServer(t *testing.T) http.Handler {
	t.Helper()
	srv := New(Deps{Auth: auth.StaticVerifier{Tokens: map[string]auth.Principal{
		"kasse":  {Subject: "k", Roles: []string{auth.RoleKasse}},
		"klr":    {Subject: "l", Roles: []string{auth.RoleKLR}},
		"admin":  {Subject: "a", Roles: []string{auth.RoleAdmin}},
		"reader": {Subject: "r", Roles: []string{auth.RoleReader}},
		"none":   {Subject: "n"},
	}}})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cashbook/days", srv.authorize(okHandler, auth.RoleKasse))
	mux.HandleFunc("GET /api/cashbook/days/{id}", srv.authorizeRead(okHandler, auth.RoleKasse))
	mux.HandleFunc("GET /api/lookups/{ns}", srv.authorize(okHandler)) // open to any caller
	return srv.authenticate(mux)
}

func TestAuthorizeRoleMatrix(t *testing.T) {
	h := authzServer(t)
	cases := []struct {
		name, token, method, path string
		want                      int
	}{
		{"module role writes its route", "kasse", "POST", "/api/cashbook/days", 200},
		{"module role reads its route", "kasse", "GET", "/api/cashbook/days/1", 200},
		{"admin writes anything", "admin", "POST", "/api/cashbook/days", 200},
		{"reader may read", "reader", "GET", "/api/cashbook/days/1", 200},
		{"reader may not write", "reader", "POST", "/api/cashbook/days", 403},
		{"wrong module is forbidden", "klr", "POST", "/api/cashbook/days", 403},
		{"wrong module read is forbidden", "klr", "GET", "/api/cashbook/days/1", 403},
		{"no role is forbidden", "none", "POST", "/api/cashbook/days", 403},
		{"empty role list admits any caller", "none", "GET", "/api/lookups/x", 200},
		{"missing token is unauthenticated", "", "POST", "/api/cashbook/days", 401},
		{"bad token is unauthenticated", "bogus", "POST", "/api/cashbook/days", 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// authorize must not trust the context: reached without authenticate (defence in
// depth) it still refuses an unauthenticated caller rather than running the route.
func TestAuthorizeWithoutPrincipalIsUnauthenticated(t *testing.T) {
	srv := New(Deps{Auth: auth.StaticVerifier{Tokens: map[string]auth.Principal{}}})
	rec := httptest.NewRecorder()
	srv.authorize(okHandler, auth.RoleKasse)(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// With no verifier configured (local development) the gate is a pass-through,
// matching authenticate.
func TestAuthorizeDisabledIsPassThrough(t *testing.T) {
	srv := New(Deps{})
	for _, h := range []http.HandlerFunc{srv.authorize(okHandler, auth.RoleKasse), srv.authorizeRead(okHandler, auth.RoleKasse)} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	}
}
