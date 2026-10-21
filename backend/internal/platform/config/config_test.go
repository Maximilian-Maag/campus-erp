package config

import "testing"

func TestFromEnvDefaults(t *testing.T) {
	c, err := FromEnv(func(string) string { return "" })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.HTTPAddr != ":8080" || c.Role != RoleAPI || c.Tenant != "demo" {
		t.Fatalf("defaults wrong: %+v", c)
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
