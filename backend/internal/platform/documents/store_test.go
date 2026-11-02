package documents_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/documents"
)

func TestDirStoreSaves(t *testing.T) {
	dir := t.TempDir()
	s, err := documents.NewDirStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(context.Background(), "dunning/7.pdf", []byte("%PDF-1.7")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "dunning", "7.pdf"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "%PDF-1.7" {
		t.Fatalf("content = %q", got)
	}
	if s.Path("dunning/7.pdf") != filepath.Join(dir, "dunning", "7.pdf") {
		t.Fatalf("Path = %q", s.Path("dunning/7.pdf"))
	}
}

func TestDirStoreRefuses(t *testing.T) {
	dir := t.TempDir()
	s, err := documents.NewDirStore("  " + dir + "  ")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		key     string
		content []byte
		want    error
	}{
		{"empty key", "  ", []byte("x"), documents.ErrKeyRequired},
		{"empty content", "a.pdf", nil, documents.ErrContentRequired},
		{"absolute key", "/etc/passwd", []byte("x"), documents.ErrUnsafeKey},
		{"parent key", "../escape.pdf", []byte("x"), documents.ErrUnsafeKey},
		{"nested parent", "a/../../escape.pdf", []byte("x"), documents.ErrUnsafeKey},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := s.Save(context.Background(), c.key, c.content); !errors.Is(err, c.want) {
				t.Fatalf("Save error = %v, want %v", err, c.want)
			}
		})
	}
	if _, err := documents.NewDirStore(""); !errors.Is(err, documents.ErrStoreRootRequired) {
		t.Fatalf("NewDirStore error = %v", err)
	}
}
