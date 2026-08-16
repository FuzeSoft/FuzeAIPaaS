package artifact

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"fuze-ai-paas/backend/internal/ports"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Store struct {
	cli    *minio.Client
	bucket string
}

func NewS3Store(cfg Config) (*S3Store, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("artifact: s3 backend requires ARTIFACT_S3_ENDPOINT")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("artifact: s3 backend requires ARTIFACT_S3_BUCKET")
	}
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}
	return &S3Store{cli: cli, bucket: cfg.Bucket}, nil
}

func (s *S3Store) Put(ctx context.Context, key string, r io.Reader) (string, error) {
	k, err := normalizeKey(key)
	if err != nil {
		return "", err
	}
	if r == nil {
		return "", fmt.Errorf("artifact: nil reader for key %q", key)
	}
	
	if _, err := s.cli.PutObject(ctx, s.bucket, k, r, -1, minio.PutObjectOptions{}); err != nil {
		return "", err
	}
	return s3URI(s.bucket, k), nil
}

func (s *S3Store) Get(ctx context.Context, uri string) (io.ReadCloser, error) {
	bucket, key, err := s.locate(uri)
	if err != nil {
		return nil, err
	}
	obj, err := s.cli.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	
	if _, err := obj.Stat(); err != nil {
		obj.Close()
		return nil, err
	}
	return obj, nil
}

func (s *S3Store) List(ctx context.Context, prefix string) ([]ports.ArtifactInfo, error) {
	p := ""
	if prefix != "" {
		k, err := normalizeKey(prefix)
		if err != nil {
			return nil, err
		}
		p = k
	}
	out := []ports.ArtifactInfo{}
	for obj := range s.cli.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: p, Recursive: true}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		out = append(out, ports.ArtifactInfo{
			Key:          obj.Key,
			URI:          s3URI(s.bucket, obj.Key),
			Size:         obj.Size,
			LastModified: obj.LastModified,
		})
	}
	return out, nil
}

func (s *S3Store) Delete(ctx context.Context, uri string) error {
	bucket, key, err := s.locate(uri)
	if err != nil {
		return err
	}
	return s.cli.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
}

func (s *S3Store) Presign(ctx context.Context, uri string, ttl time.Duration) (string, error) {
	bucket, key, err := s.locate(uri)
	if err != nil {
		return "", err
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	u, err := s.cli.PresignedGetObject(ctx, bucket, key, ttl, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *S3Store) locate(uri string) (string, string, error) {
	bucket, key, err := parseS3URI(uri)
	if err != nil {
		return "", "", err
	}
	if bucket != s.bucket {
		return "", "", fmt.Errorf("artifact: URI %q belongs to bucket %q, store manages %q", uri, bucket, s.bucket)
	}
	return bucket, key, nil
}

var _ ports.ArtifactStore = (*S3Store)(nil)