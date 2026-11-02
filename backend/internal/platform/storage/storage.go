// Package storage keeps binary artefacts — documents, exports — behind one keyed store.
//
// It is the seam the roles write to instead of touching a filesystem: a directory
// locally, an S3-compatible bucket in a deployment (ADR-0011). Both satisfy Store, so a
// caller does not know which one it has.
package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Errors of the store.
var (
	ErrKeyRequired      = errors.New("storage: key is required")
	ErrContentEmpty     = errors.New("storage: content is empty")
	ErrUnsafeKey        = errors.New("storage: key escapes the store")
	ErrRootRequired     = errors.New("storage: root is required")
	ErrEndpointRequired = errors.New("storage: endpoint is required")
	ErrBucketRequired   = errors.New("storage: bucket is required")
)

// Store keeps content under a key.
type Store interface {
	Save(ctx context.Context, key string, content []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
}

// FromEnv returns the artefact store a role uses: an S3-compatible bucket when
// ERP_S3_ENDPOINT is set (bucket from ERP_S3_BUCKET, created when missing), otherwise a
// directory at root.
func FromEnv(ctx context.Context, root string) (Store, error) {
	if endpoint := strings.TrimSpace(os.Getenv("ERP_S3_ENDPOINT")); endpoint != "" {
		s, err := NewS3(endpoint, os.Getenv("ERP_S3_BUCKET"),
			os.Getenv("ERP_S3_ACCESS_KEY"), os.Getenv("ERP_S3_SECRET_KEY"), false)
		if err != nil {
			return nil, err
		}
		if err := s.EnsureBucket(ctx); err != nil {
			return nil, err
		}
		return s, nil
	}
	return NewDir(root)
}

// CheckKey trims a key and refuses one that is empty or would escape the store.
func CheckKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ErrKeyRequired
	}
	clean := filepath.Clean(key)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrUnsafeKey, key)
	}
	return clean, nil
}
