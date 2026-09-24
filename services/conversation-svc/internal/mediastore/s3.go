package mediastore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config defines connection parameters for an S3/MinIO compatible object store.
type S3Config struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Region    string
}

// S3Store implements Store using MinIO / S3.
type S3Store struct {
	client *minio.Client
	bucket string
}

// NewS3Store creates and initializes an S3/MinIO storage backend, ensuring the bucket exists.
func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return nil, errors.New("mediastore: s3 endpoint is required")
	}
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, errors.New("mediastore: s3 bucket is required")
	}

	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("mediastore: initialize s3 client: %w", err)
	}

	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("mediastore: check bucket %q: %w", bucket, err)
	}
	if !exists {
		err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: cfg.Region})
		if err != nil {
			return nil, fmt.Errorf("mediastore: create bucket %q: %w", bucket, err)
		}
	}

	return &S3Store{
		client: client,
		bucket: bucket,
	}, nil
}

// Put uploads an object to the S3 bucket.
func (s *S3Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, opts)
	if err != nil {
		return fmt.Errorf("mediastore: upload s3 object: %w", err)
	}
	return nil
}

// Get retrieves an object from the S3 bucket.
func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("mediastore: request s3 object: %w", err)
	}

	// Verify the object exists by inspecting its Stat.
	_, err = obj.Stat()
	if err != nil {
		_ = obj.Close()
		var errResp minio.ErrorResponse
		if errors.As(err, &errResp) && (errResp.Code == "NoSuchKey" || errResp.StatusCode == 404) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("mediastore: stat s3 object: %w", err)
	}

	return obj, nil
}

// Delete removes an object from the S3 bucket.
func (s *S3Store) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		var errResp minio.ErrorResponse
		if errors.As(err, &errResp) && (errResp.Code == "NoSuchKey" || errResp.StatusCode == 404) {
			return nil
		}
		return fmt.Errorf("mediastore: delete s3 object: %w", err)
	}
	return nil
}

// Exists checks if an object exists in the S3 bucket.
func (s *S3Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	var errResp minio.ErrorResponse
	if errors.As(err, &errResp) && (errResp.Code == "NoSuchKey" || errResp.StatusCode == 404) {
		return false, nil
	}
	return false, fmt.Errorf("mediastore: stat s3 object: %w", err)
}
