package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// lazyPool builds a *pgxpool.Pool without connecting (pgxpool opens lazily), so
// the registry can be tested without a live database.
func lazyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/erp")
	if err != nil {
		t.Fatalf("lazy pool: %v", err)
	}
	return p
}

func TestResolve(t *testing.T) {
	cases := []struct {
		host, want string
		ok         bool
	}{
		{"uni-x.erp.example.org", "uni-x", true},
		{"uni-x.erp.example.org:8443", "uni-x", true},
		{"demo", "demo", true},
		{"DEMO", "demo", true},
		{"  uni-x.erp.example.org ", "uni-x", true},
		{"a-b-9.example", "a-b-9", true},
		{"", "", false},
		{"-bad.example", "", false},
		{"bad-.example", "", false},
		{"Uni_X.example", "", false},
	}
	for _, tc := range cases {
		got, err := Resolve(tc.host)
		if tc.ok && (err != nil || got != tc.want) {
			t.Errorf("Resolve(%q) = %q,%v want %q", tc.host, got, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Errorf("Resolve(%q) should fail", tc.host)
		}
	}
}

func TestContextRoundTrip(t *testing.T) {
	ctx := WithTenant(context.Background(), "uni-x")
	if name, ok := From(ctx); !ok || name != "uni-x" {
		t.Fatalf("From = %q,%v", name, ok)
	}
	if _, ok := From(context.Background()); ok {
		t.Error("From on a bare context should be false")
	}
}

func TestRegistryOpensMigratesAndCaches(t *testing.T) {
	var opens, migrations int
	var opened []string
	open := func(_ context.Context, dsn string) (*pgxpool.Pool, error) {
		opens++
		opened = append(opened, dsn)
		return lazyPool(t), nil
	}
	migrate := func(context.Context, *pgxpool.Pool) (int, error) { migrations++; return 1, nil }

	r, err := NewRegistry("postgres://u:p@h:5432/erp_{tenant}", open, migrate)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := r.Pool(context.Background(), "uni-x"); err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := r.Pool(context.Background(), "uni-x"); err != nil {
		t.Fatalf("pool cached: %v", err)
	}
	if opens != 1 || migrations != 1 {
		t.Errorf("expected one open+migrate (cached), got opens=%d migrations=%d", opens, migrations)
	}
	if opened[0] != "postgres://u:p@h:5432/erp_uni-x" {
		t.Errorf("dsn = %q", opened[0])
	}
	r.Close()
}

func TestRegistryRejectsBadNameAndTemplate(t *testing.T) {
	if _, err := NewRegistry("postgres://h/erp", nil, nil); err == nil {
		t.Error("template without {tenant} must be rejected")
	}
	r, _ := NewRegistry("postgres://h/{tenant}", func(context.Context, string) (*pgxpool.Pool, error) { return lazyPool(t), nil }, func(context.Context, *pgxpool.Pool) (int, error) { return 0, nil })
	if _, err := r.Pool(context.Background(), "Bad Name"); err == nil {
		t.Error("invalid tenant name must be rejected")
	}
}

func TestRegistryMigrateFailureClosesPool(t *testing.T) {
	open := func(context.Context, string) (*pgxpool.Pool, error) { return lazyPool(t), nil }
	migrate := func(context.Context, *pgxpool.Pool) (int, error) { return 0, errors.New("boom") }
	r, _ := NewRegistry("postgres://h/{tenant}", open, migrate)
	if _, err := r.Pool(context.Background(), "uni-x"); err == nil {
		t.Error("migrate failure must surface")
	}
}

func TestRegistryOpenFailure(t *testing.T) {
	open := func(context.Context, string) (*pgxpool.Pool, error) { return nil, errors.New("dial tcp: refused") }
	r, _ := NewRegistry("postgres://h/{tenant}", open, nil)
	if _, err := r.Pool(context.Background(), "uni-x"); err == nil {
		t.Error("open failure must surface")
	}
}
