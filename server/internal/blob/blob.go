// Package blob wraps the S3 API used for binary artifacts and run logs.
// Only the S3 protocol is used, so SeaweedFS can be swapped for MinIO,
// Garage or AWS S3 by changing configuration.
package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/liamphmurphy/lazyagents/server/internal/config"
)

type Store struct {
	client  *s3.Client
	buckets []string
}

func New(cfg config.S3) *Store {
	client := s3.New(s3.Options{
		Region:       cfg.Region,
		BaseEndpoint: aws.String(cfg.Endpoint),
		UsePathStyle: cfg.UsePathStyle,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
	})
	return &Store{client: client, buckets: cfg.Buckets}
}

// EnsureBuckets creates any configured bucket that doesn't exist yet,
// retrying until ctx is done so the API can start before the S3 gateway is up.
func (s *Store) EnsureBuckets(ctx context.Context) error {
	for _, b := range s.buckets {
		for {
			err := s.ensure(ctx, b)
			if err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("blob: bucket %q: %w", b, err)
			case <-time.After(time.Second):
			}
		}
	}
	return nil
}

func (s *Store) ensure(ctx context.Context, bucket string) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &bucket}); err == nil {
		return nil
	}
	_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &bucket})
	var owned *types.BucketAlreadyOwnedByYou
	var exists *types.BucketAlreadyExists
	if errors.As(err, &owned) || errors.As(err, &exists) {
		return nil
	}
	return err
}

// Object is a stored blob being read.
type Object struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
}

// ErrNotFound means the key doesn't exist.
var ErrNotFound = errors.New("blob: not found")

// Put stores a blob. size must be exact; the S3 API needs it up front.
func (s *Store) Put(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        &bucket,
		Key:           &key,
		Body:          body,
		ContentLength: &size,
		ContentType:   &contentType,
	})
	if err != nil {
		return fmt.Errorf("blob: put %s/%s: %w", bucket, key, err)
	}
	return nil
}

// Get opens a blob for reading. The caller closes Body.
func (s *Store) Get(ctx context.Context, bucket, key string) (Object, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return Object{}, ErrNotFound
		}
		return Object{}, fmt.Errorf("blob: get %s/%s: %w", bucket, key, err)
	}
	o := Object{Body: out.Body, ContentType: aws.ToString(out.ContentType)}
	if out.ContentLength != nil {
		o.ContentLength = *out.ContentLength
	}
	return o, nil
}

// Delete removes a blob; a missing key is not an error.
func (s *Store) Delete(ctx context.Context, bucket, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &bucket, Key: &key})
	return err
}

// Ping checks the gateway is reachable by listing buckets.
func (s *Store) Ping(ctx context.Context) error {
	_, err := s.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	return err
}
