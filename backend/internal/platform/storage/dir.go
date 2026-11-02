package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dir keeps artefacts in a directory, creating parents as needed. It is the local
// implementation of the seam (tests, single-host installs).
type Dir struct{ root string }

// NewDir returns a store rooted at root.
func NewDir(root string) (*Dir, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, ErrRootRequired
	}
	return &Dir{root: root}, nil
}

// Path is where a key lives on disk (used by tests and operations).
func (d *Dir) Path(key string) string {
	return filepath.Join(d.root, filepath.Clean(strings.TrimSpace(key)))
}

// Save writes content under key.
func (d *Dir) Save(_ context.Context, key string, content []byte) error {
	clean, err := CheckKey(key)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return ErrContentEmpty
	}
	full := filepath.Join(d.root, clean)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return fmt.Errorf("storage: save %q: %w", key, err)
	}
	if err := os.WriteFile(full, content, 0o640); err != nil {
		return fmt.Errorf("storage: save %q: %w", key, err)
	}
	return nil
}

// Get reads the content stored under key.
func (d *Dir) Get(_ context.Context, key string) ([]byte, error) {
	clean, err := CheckKey(key)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(filepath.Join(d.root, clean))
	if err != nil {
		return nil, fmt.Errorf("storage: get %q: %w", key, err)
	}
	return content, nil
}
