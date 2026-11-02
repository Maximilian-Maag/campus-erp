package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3 keeps artefacts in an S3-compatible bucket (MinIO on-prem, or a university S3).
type S3 struct {
	client *minio.Client
	bucket string
}

// NewS3 returns a store for the bucket at endpoint. Credentials are the static
// access-key pair; secure selects TLS.
func NewS3(endpoint, bucket, accessKey, secretKey string, secure bool) (*S3, error) {
	endpoint, bucket = strings.TrimSpace(endpoint), strings.TrimSpace(bucket)
	if endpoint == "" {
		return nil, ErrEndpointRequired
	}
	if bucket == "" {
		return nil, ErrBucketRequired
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: s3 client: %w", err)
	}
	return &S3{client: client, bucket: bucket}, nil
}

// Bucket is the bucket the store writes to.
func (s *S3) Bucket() string { return s.bucket }

// EnsureBucket creates the bucket when it does not exist yet. It is idempotent, so a
// role can call it on every start.
func (s *S3) EnsureBucket(ctx context.Context) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("storage: bucket %q: %w", s.bucket, err)
	}
	if ok {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("storage: create bucket %q: %w", s.bucket, err)
	}
	return nil
}

// Save writes content under key.
func (s *S3) Save(ctx context.Context, key string, content []byte) error {
	clean, err := CheckKey(key)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return ErrContentEmpty
	}
	_, err = s.client.PutObject(ctx, s.bucket, clean, bytes.NewReader(content),
		int64(len(content)), minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return fmt.Errorf("storage: save %q: %w", key, err)
	}
	return nil
}

// Get reads the content stored under key.
func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	clean, err := CheckKey(key)
	if err != nil {
		return nil, err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, clean, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage: get %q: %w", key, err)
	}
	defer obj.Close()
	content, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("storage: get %q: %w", key, err)
	}
	return content, nil
}
