// Package tenant resolves the tenant of a request and provides the per-tenant
// database pools (ADR-0002: database-per-tenant, tenant resolved from the
// request host, never a path segment).
//
// The tenant is the first DNS label of the host, e.g. "uni-x.erp.example.org"
// -> "uni-x". The same rule is used by the Angular app (frontend tenancy), so a
// request and its rendering agree on the tenant.
package tenant

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// label is a DNS label: lowercase alphanumeric and hyphens, no leading/trailing
// hyphen, at most 63 characters.
var label = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// Resolve extracts the tenant name from a host (which may carry a port). The
// first DNS label is the tenant; anything that is not a valid label is rejected.
func Resolve(host string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(host))
	h, _, _ = strings.Cut(h, ":") // drop a port
	name, _, _ := strings.Cut(h, ".")
	if !label.MatchString(name) {
		return "", fmt.Errorf("tenant: invalid host %q", host)
	}
	return name, nil
}

type ctxKey struct{}

// WithTenant attaches the resolved tenant to the context.
func WithTenant(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, ctxKey{}, name)
}

// From returns the tenant attached to the context.
func From(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(ctxKey{}).(string)
	return name, ok
}
