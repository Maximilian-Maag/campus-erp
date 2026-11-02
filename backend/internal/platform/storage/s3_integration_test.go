package storage_test

import (
	"context"
	"os"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/storage"
)

// TestS3RoundTrip exercises the S3 store against a real endpoint. It is skipped unless
// ERP_S3_TEST_ENDPOINT is set (e.g. the MinIO service in docker-compose).
func TestS3RoundTrip(t *testing.T) {
	endpoint := os.Getenv("ERP_S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("ERP_S3_TEST_ENDPOINT not set")
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

	key := "test/round-trip.txt"
	if err := s.Save(ctx, key, []byte("hallo")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "hallo" {
		t.Fatalf("content = %q", got)
	}
	if _, err := s.Get(ctx, "test/does-not-exist.txt"); err == nil {
		t.Fatal("missing key must fail")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
