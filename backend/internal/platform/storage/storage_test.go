package storage_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/storage"
)

func TestCheckKey(t *testing.T) {
	if got, err := storage.CheckKey("  a/b.pdf  "); err != nil || got != "a/b.pdf" {
		t.Fatalf("CheckKey = %q, %v", got, err)
	}
	cases := []struct {
		key  string
		want error
	}{
		{"  ", storage.ErrKeyRequired},
		{"/etc/passwd", storage.ErrUnsafeKey},
		{"../escape", storage.ErrUnsafeKey},
		{"a/../../escape", storage.ErrUnsafeKey},
	}
	for _, c := range cases {
		if _, err := storage.CheckKey(c.key); !errors.Is(err, c.want) {
			t.Errorf("CheckKey(%q) = %v, want %v", c.key, err, c.want)
		}
	}
}

func TestDirRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.NewDir("  " + dir + "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(context.Background(), "exports/2026/ledger.csv", []byte("a,b\n")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get(context.Background(), "exports/2026/ledger.csv")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "a,b\n" {
		t.Fatalf("content = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "exports", "2026", "ledger.csv")); err != nil {
		t.Fatalf("file not where expected: %v", err)
	}
	if s.Path("exports/2026/ledger.csv") != filepath.Join(dir, "exports", "2026", "ledger.csv") {
		t.Fatalf("Path = %q", s.Path("exports/2026/ledger.csv"))
	}
}

func TestDirRefusals(t *testing.T) {
	s, err := storage.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(context.Background(), "k", nil); !errors.Is(err, storage.ErrContentEmpty) {
		t.Fatalf("empty content = %v", err)
	}
	if err := s.Save(context.Background(), " ", []byte("x")); !errors.Is(err, storage.ErrKeyRequired) {
		t.Fatalf("empty key = %v", err)
	}
	if _, err := s.Get(context.Background(), "../x"); !errors.Is(err, storage.ErrUnsafeKey) {
		t.Fatalf("unsafe get key = %v", err)
	}
	if _, err := s.Get(context.Background(), "missing"); err == nil {
		t.Fatal("getting a missing key must fail")
	}
	if _, err := storage.NewDir(""); !errors.Is(err, storage.ErrRootRequired) {
		t.Fatalf("NewDir empty root = %v", err)
	}
}

func TestNewS3Validation(t *testing.T) {
	if _, err := storage.NewS3("", "b", "k", "s", false); !errors.Is(err, storage.ErrEndpointRequired) {
		t.Fatalf("no endpoint = %v", err)
	}
	if _, err := storage.NewS3("localhost:9000", " ", "k", "s", false); !errors.Is(err, storage.ErrBucketRequired) {
		t.Fatalf("no bucket = %v", err)
	}
	s, err := storage.NewS3("localhost:9000", "erp", "k", "s", false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Bucket() != "erp" {
		t.Fatalf("Bucket = %q", s.Bucket())
	}
}
