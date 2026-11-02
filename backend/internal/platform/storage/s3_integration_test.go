package storage_test

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/storage"
)

// TestS3RoundTrip exercises the S3 store against a real S3 API. By default it serves an
// in-process S3-compatible server (gofakes3), so the S3 path is exercised on every run
// and cannot rot; set ERP_S3_TEST_ENDPOINT to point it at an external endpoint (MinIO,
// a university S3) instead.
func TestS3RoundTrip(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("ERP_S3_TEST_ENDPOINT"))
	if endpoint == "" {
		srv := httptest.NewServer(gofakes3.New(s3mem.New()).Server())
		defer srv.Close()
		endpoint = strings.TrimPrefix(srv.URL, "http://")
	}

	ctx := context.Background()
	s, err := storage.NewS3(endpoint,
		envOr("ERP_S3_BUCKET", "campus-erp-test"),
		envOr("ERP_S3_ACCESS_KEY", "erp"),
		envOr("ERP_S3_SECRET_KEY", "erp-erp-erp"), false)
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	if err := s.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if err := s.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket must be idempotent: %v", err)
	}

	if err := s.Save(ctx, "sepa/sepa-7.xml", []byte("<Document/>")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get(ctx, "sepa/sepa-7.xml")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "<Document/>" {
		t.Fatalf("content = %q", got)
	}

	if err := s.Save(ctx, "nested/deep/key.csv", []byte("a,b\n")); err != nil {
		t.Fatalf("Save nested: %v", err)
	}
	if got, err := s.Get(ctx, "nested/deep/key.csv"); err != nil || string(got) != "a,b\n" {
		t.Fatalf("nested round trip = %q, %v", got, err)
	}

	if _, err := s.Get(ctx, "sepa/missing.xml"); err == nil {
		t.Fatal("a missing key must fail")
	}
	if err := s.Save(ctx, "../escape", []byte("x")); err == nil {
		t.Fatal("an escaping key must be refused")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
