// Package objectstore talks to private object storage (ADR 0005).
//
// Only the S3 API is used in hosted environments. MinIO backs the local
// stack; any S3-compatible service can back a hosted environment without a
// code change, which keeps the hosting decision open. A filesystem driver
// serves development machines without MinIO and is refused in production by
// config.Load.
//
// Objects are private: nothing here issues public URLs, and object keys are
// random identifiers that never contain a client or matter name.
package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
)

// Client is the subset of the object storage API the application depends on.
type Client interface {
	BucketExists(ctx context.Context, bucket string) (bool, error)
}

// Blob is private object storage as the application uses it: write an
// object once, read it back, and report readiness.
type Blob interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Check(ctx context.Context, timeout time.Duration) error
}

// ErrNotFound is returned by Get for a key that holds no object.
var ErrNotFound = errors.New("objectstore: object not found")

// OpenBlob builds the configured store: "s3" (MinIO or any S3-compatible
// service) or "fs" (a local directory, development only).
func OpenBlob(cfg config.ObjectStore) (Blob, error) {
	switch cfg.Driver {
	case "", "s3":
		return Open(cfg)
	case "fs":
		return OpenFS(cfg.FSRoot, cfg.Bucket)
	default:
		return nil, fmt.Errorf("unknown object storage driver %q", cfg.Driver)
	}
}

// Store wraps the S3 client together with the bucket it operates on.
type Store struct {
	client Client
	bucket string
	// mc is the full client for reads and writes. It is nil for a store
	// built by NewStore around a stub, which then supports Check only.
	mc *minio.Client
}

var _ Blob = (*Store)(nil)

// Open builds a store from configuration. It does not contact the service, so
// that start-up ordering in the local stack is not a race; use Check for that.
func Open(cfg config.ObjectStore) (*Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create object storage client: %w", err)
	}
	return &Store{client: client, bucket: cfg.Bucket, mc: client}, nil
}

// NewStore builds a store around any client. Tests use it with a stub.
func NewStore(client Client, bucket string) *Store {
	return &Store{client: client, bucket: bucket}
}

// Bucket returns the configured bucket name.
func (s *Store) Bucket() string { return s.bucket }

// Check reports whether the configured bucket is reachable. It is used by the
// readiness endpoint, and it is the proof that the local stack really has
// working object storage rather than a container that merely started.
func (s *Store) Check(ctx context.Context, timeout time.Duration) error {
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	exists, err := s.client.BucketExists(checkCtx, s.bucket)
	if err != nil {
		return fmt.Errorf("object storage not ready: %w", err)
	}
	if !exists {
		return fmt.Errorf("object storage bucket %q does not exist", s.bucket)
	}
	return nil
}

// Put writes an object of exactly size bytes. Objects inherit the bucket's
// private policy; no ACL that could make them public is ever set.
func (s *Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if err := ValidKey(key); err != nil {
		return err
	}
	if s.mc == nil {
		return errors.New("objectstore: store has no read/write client")
	}
	if _, err := s.mc.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

// Get opens an object for reading and returns its size. The caller closes
// the reader.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if err := ValidKey(key); err != nil {
		return nil, 0, err
	}
	if s.mc == nil {
		return nil, 0, errors.New("objectstore: store has no read/write client")
	}
	obj, err := s.mc.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("get object: %w", err)
	}
	// GetObject is lazy; Stat makes the request and surfaces a missing key.
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("stat object: %w", err)
	}
	return obj, info.Size, nil
}
