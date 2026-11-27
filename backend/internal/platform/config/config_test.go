package config

import (
	"strings"
	"testing"
)

// The api is a web application on a host: it does not run without a way to tell who is
// calling (ADR-0003). Everything else about the defaults is uninteresting next to that.
const (
	issuer = "https://idp.uni.ac.at/realms/campus"
	jwks   = "https://idp.uni.ac.at/realms/campus/protocol/openid-connect/certs"
)

func TestFromEnvDefaults(t *testing.T) {
	env := map[string]string{"ERP_OIDC_ISSUER": issuer, "ERP_OIDC_JWKS_URL": jwks}
	c, err := FromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.HTTPAddr != ":8080" || c.Role != RoleAPI || c.Tenant != "demo" {
		t.Fatalf("defaults wrong: %+v", c)
	}
	if c.OIDCIssuer != issuer || c.OIDCJWKSURL != jwks {
		t.Fatalf("OIDC not read: %+v", c)
	}
}

func TestFromEnvReadsValues(t *testing.T) {
	env := map[string]string{"ERP_HTTP_ADDR": ":9000", "ERP_ROLE": "worker", "ERP_TENANT": "uni-x"}
	c, err := FromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.HTTPAddr != ":9000" || c.Role != RoleWorker || c.Tenant != "uni-x" {
		t.Fatalf("values wrong: %+v", c)
	}
}

func TestFromEnvRejectsUnknownRole(t *testing.T) {
	env := map[string]string{"ERP_ROLE": "admin"}
	if _, err := FromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("unknown role must be rejected")
	}
}

// The one that matters: an api with no issuer would answer every request, so the process
// must not start at all — a 401-shaped client is not a substitute for a refused startup.
func TestFromEnvRefusesAPIWithoutOIDC(t *testing.T) {
	for _, env := range []map[string]string{
		{},                                  // neither
		{"ERP_OIDC_ISSUER": issuer},          // no keys
		{"ERP_OIDC_JWKS_URL": jwks},          // no issuer
		{"ERP_OIDC_ISSUER": "", "ERP_OIDC_JWKS_URL": jwks}, // explicitly empty
	} {
		_, err := FromEnv(func(k string) string { return env[k] })
		if err == nil {
			t.Fatalf("api role must be refused without issuer and jwks: %v", env)
		}
		if !strings.Contains(err.Error(), "ERP_OIDC_JWKS_URL") {
			t.Fatalf("the refusal must name the value to set: %v", err)
		}
	}
}

// The worker and the connectors serve nobody: they run without an IdP, and must keep doing
// so, or every background job would need one.
func TestFromEnvAllowsRolesThatServeNobody(t *testing.T) {
	for _, role := range []string{RoleWorker, RoleConnectors} {
		env := map[string]string{"ERP_ROLE": role}
		if _, err := FromEnv(func(k string) string { return env[k] }); err != nil {
			t.Fatalf("%s must not require OIDC: %v", role, err)
		}
	}
}
