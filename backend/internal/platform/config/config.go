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

	// OIDC (ADR-0003). Required for the api role: the interface is a web application on a
	// host, so every /api/* request is proved against an IdP's keys and the roles in the
	// token decide what the caller may do. A build with no issuer would answer anyone who
	// can reach the Service, which is why config.Load refuses it rather than logging it.
	OIDCIssuer   string
	OIDCAudience string
	OIDCJWKSURL  string

	// TenantDSN is a DSN template containing "{tenant}". When set the API serves
	// its tenants multi-tenant (ADR-0002); otherwise it runs single-tenant
	// against DSN.
	TenantDSN string

	// LegacyDSN is the DSN of the legacy database the ETL datasets read from
	// (docs/19). When empty the ETL source is the main pool (dev/seed).
	LegacyDSN string
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

		LegacyDSN: get("ERP_LEGACY_DATABASE_URL"),
	}
	switch c.Role {
	case RoleAPI, RoleWorker, RoleConnectors:
	default:
		return Config{}, fmt.Errorf("config: unknown role %q", c.Role)
	}
	// The api answers requests on behalf of people, so it does not run without a way to tell
	// who they are: an issuer to check tokens against and the keys to check them with. The
	// worker and the connectors serve nobody and need neither.
	if c.Role == RoleAPI {
		if c.OIDCIssuer == "" || c.OIDCJWKSURL == "" {
			return Config{}, fmt.Errorf(
				"config: ERP_OIDC_ISSUER and ERP_OIDC_JWKS_URL are required for the api role: " +
					"without them the api would answer /api/* without a token (ADR-0003)")
		}
	}
	return c, nil
}

func defaultTo(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
