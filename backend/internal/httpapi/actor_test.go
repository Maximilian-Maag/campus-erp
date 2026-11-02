package httpapi

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/auth"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/tenant"
)

func TestActorFromHeader(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := actor(r); got != "anonymous" {
		t.Errorf("actor() = %q, want anonymous", got)
	}
	r.Header.Set("X-Actor", "alice")
	if got := actor(r); got != "alice" {
		t.Errorf("actor() = %q, want alice", got)
	}
}

// TestActorUsesTheAuthenticatedPrincipal: a presented token's subject is the actor,
// and it wins over the dev header.
func TestActorUsesTheAuthenticatedPrincipal(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Actor", "header-should-lose")
	r = r.WithContext(context.WithValue(r.Context(), principalKey, auth.Principal{Subject: "carol"}))
	if got := actor(r); got != "carol" {
		t.Errorf("actor() = %q, want carol (the token subject wins over the header)", got)
	}
}

// TestActorRecordsTheTenant: the tenant a request was resolved for is part of the
// actor, so an audit event says which tenant it belongs to (docs/17).
func TestActorRecordsTheTenant(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Actor", "alice")
	r = r.WithContext(tenant.WithTenant(r.Context(), "uni-a"))
	if got := actor(r); got != "uni-a/alice" {
		t.Errorf("actor() = %q, want uni-a/alice", got)
	}

	// With no caller at all, the tenant still qualifies "anonymous" rather than
	// leaving the event unattributed.
	r = httptest.NewRequest("GET", "/", nil)
	r = r.WithContext(tenant.WithTenant(r.Context(), "uni-b"))
	if got := actor(r); got != "uni-b/anonymous" {
		t.Errorf("actor() = %q, want uni-b/anonymous", got)
	}
}

// TestWithTenantEmptyNameIsNoQualifier: an empty tenant leaves the actor alone.
func TestWithTenantEmptyNameIsNoQualifier(t *testing.T) {
	if got := withTenant(context.Background(), "alice"); got != "alice" {
		t.Errorf("withTenant(no tenant) = %q, want alice", got)
	}
	if got := withTenant(tenant.WithTenant(context.Background(), ""), "alice"); got != "alice" {
		t.Errorf("withTenant(empty tenant) = %q, want alice", got)
	}
}
