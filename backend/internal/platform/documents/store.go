package documents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Errors of the document store.
var (
	ErrStoreRootRequired = errors.New("documents: store root is required")
	ErrKeyRequired       = errors.New("documents: document key is required")
	ErrContentRequired   = errors.New("documents: document content is empty")
	ErrUnsafeKey         = errors.New("documents: document key escapes the store")
)

// Store keeps a rendered document under a key.
type Store interface {
	Save(ctx context.Context, key string, content []byte) error
}

// DirStore writes documents below a directory, creating parents as needed. It is the
// local implementation of the seam: object storage replaces it without touching the
// callers.
type DirStore struct{ root string }

// NewDirStore returns a store rooted at dir.
func NewDirStore(dir string) (*DirStore, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, ErrStoreRootRequired
	}
	return &DirStore{root: dir}, nil
}

// Save writes content under key. A key that is empty, absolute or escapes the root is
// refused, so a caller cannot write outside the store.
func (s *DirStore) Save(_ context.Context, key string, content []byte) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return ErrKeyRequired
	}
	if len(content) == 0 {
		return ErrContentRequired
	}
	clean := filepath.Clean(key)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q", ErrUnsafeKey, key)
	}
	full := filepath.Join(s.root, clean)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return fmt.Errorf("documents: store %q: %w", key, err)
	}
	if err := os.WriteFile(full, content, 0o640); err != nil {
		return fmt.Errorf("documents: store %q: %w", key, err)
	}
	return nil
}

// Path returns the file a key maps to (used by tests and operations).
func (s *DirStore) Path(key string) string {
	return filepath.Join(s.root, filepath.Clean(strings.TrimSpace(key)))
}
