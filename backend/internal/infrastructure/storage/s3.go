// Package storage implements the media storage port on S3-compatible
// object stores (MinIO, Cloudflare R2, AWS S3) and in memory for tests.
package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config configures the S3 client.
type S3Config struct {
	Endpoint  string // host:port, no scheme
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
	// PublicEndpoint optionally rewrites presigned URLs for browsers (e.g. localhost:9000).
	PublicEndpoint string
	AutoCreate     bool
}

// S3 implements media.Storage.
type S3 struct {
	client     *minio.Client
	signer     *minio.Client
	bucket     string
	region     string
	autoCreate bool

	mu    sync.Mutex
	ready bool // bucket verified/created
}

// startupProbeTimeout bounds the best-effort bucket check done in NewS3.
const startupProbeTimeout = 5 * time.Second

// NewS3 builds the object-store client. The service must start even when the
// store is down, so the bucket is only verified/created best-effort here and
// again lazily before the first write or readiness probe; /ready reports an
// unreachable store as "unavailable".
func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	opts := &minio.Options{Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: cfg.UseSSL, Region: cfg.Region}
	client, err := minio.New(cfg.Endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("storage: client: %w", err)
	}
	signer := client
	if cfg.PublicEndpoint != "" && cfg.PublicEndpoint != cfg.Endpoint {
		if signer, err = minio.New(cfg.PublicEndpoint, opts); err != nil {
			return nil, fmt.Errorf("storage: public client: %w", err)
		}
	}
	s := &S3{client: client, signer: signer, bucket: cfg.Bucket, region: cfg.Region, autoCreate: cfg.AutoCreate}
	pctx, cancel := context.WithTimeout(ctx, startupProbeTimeout)
	defer cancel()
	if err := s.ensure(pctx); err != nil {
		slog.Warn("object storage not reachable at startup; will retry on demand (see /ready)",
			slog.String("endpoint", cfg.Endpoint), slog.String("bucket", cfg.Bucket), slog.Any("error", err))
	}
	return s, nil
}

// ensure verifies (and with AutoCreate, creates) the bucket once.
func (s *S3) ensure(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ready {
		return nil
	}
	if s.autoCreate {
		if err := s.ensureBucket(ctx, s.region); err != nil {
			return err
		}
	} else if _, err := s.client.BucketExists(ctx, s.bucket); err != nil {
		return fmt.Errorf("storage: bucket check: %w", err)
	}
	s.ready = true
	return nil
}

func (s *S3) ensureBucket(ctx context.Context, region string) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("storage: bucket check: %w", err)
	}
	if ok {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: region}); err != nil {
		return fmt.Errorf("storage: create bucket: %w", err)
	}
	return nil
}

// putPartSize is the multipart part size (the S3 minimum). minio-go buffers one part per upload thread, so with
// NumThreads 1 a streamed upload holds about 5 MiB however large the object is (D-015).
const putPartSize = 5 << 20

// Put uploads an object (private ACL by default). size -1 streams a body of unknown length in bounded parts; if r
// fails, minio-go aborts the multipart upload and no object is created.
func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size,
		minio.PutObjectOptions{ContentType: contentType, PartSize: putPartSize, NumThreads: 1})
	return err
}

// Get streams an object.
func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		return nil, err
	}
	return obj, nil
}

// Delete removes an object.
func (s *S3) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// PresignGet returns a short-lived download URL.
func (s *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.signer.PresignedGetObject(ctx, s.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// Ping checks the bucket is reachable (readiness).
func (s *S3) Ping(ctx context.Context) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !ok {
		s.mu.Lock()
		s.ready = false
		s.mu.Unlock()
		return fmt.Errorf("storage: bucket %q does not exist", s.bucket)
	}
	return nil
}
