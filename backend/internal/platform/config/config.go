// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
)

// Config is the process configuration.
type Config struct {
	HTTPAddr string
	Role     string
	Tenant   string

	// OIDC (ADR-0003). When OIDCJWKSURL is set the API requires bearer tokens.
	OIDCIssuer   string
	OIDCAudience string
	OIDCJWKSURL  string

	// TenantDSN is a DSN template containing "{tenant}". When set the API serves
	// its tenants multi-tenant (ADR-0002); otherwise it runs single-tenant
	// against DSN.
	TenantDSN string
}

// Roles a process may run as.
const (
	RoleAPI        = "api"
	RoleWorker     = "worker"
	RoleConnectors = "connectors"
)

// Load reads configuration from the process environment.
func Load() (Config, error) { return FromEnv(os.Getenv) }

// FromEnv builds a Config using get, so it is testable without the environment.
func FromEnv(get func(string) string) (Config, error) {
	c := Config{
		HTTPAddr: defaultTo(get("ERP_HTTP_ADDR"), ":8080"),
		Role:     defaultTo(get("ERP_ROLE"), RoleAPI),
		Tenant:   defaultTo(get("ERP_TENANT"), "demo"),

		OIDCIssuer:   get("ERP_OIDC_ISSUER"),
		OIDCAudience: get("ERP_OIDC_AUDIENCE"),
		OIDCJWKSURL:  get("ERP_OIDC_JWKS_URL"),

		TenantDSN: get("ERP_TENANT_DSN"),
	}
	switch c.Role {
	case RoleAPI, RoleWorker, RoleConnectors:
	default:
		return Config{}, fmt.Errorf("config: unknown role %q", c.Role)
	}
	return c, nil
}

func defaultTo(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
