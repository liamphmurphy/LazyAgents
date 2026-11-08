package blob_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/blob"
	"github.com/liamphmurphy/vesper/server/internal/config"
)

// Runs against a real S3 gateway when TEST_S3_ENDPOINT is set (dev compose
// SeaweedFS, or CI). Keys must match deploy/seaweedfs/s3.json.
func TestRoundTrip(t *testing.T) {
	ep := os.Getenv("TEST_S3_ENDPOINT")
	if ep == "" {
		t.Skip("TEST_S3_ENDPOINT not set")
	}
	s := blob.New(config.S3{Endpoint: ep, Region: "us-east-1", AccessKey: "lazyagents", SecretKey: "lazyagents-secret",
		Buckets: []string{"test-artifacts"}, UsePathStyle: true})
	ctx := context.Background()
	if err := s.EnsureBuckets(ctx); err != nil {
		t.Fatal(err)
	}
	body := "hello, ledger"
	if err := s.Put(ctx, "test-artifacts", "a/b.txt", strings.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	o, err := s.Get(ctx, "test-artifacts", "a/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(o.Body)
	o.Body.Close()
	if string(got) != body || o.ContentType != "text/plain" || o.ContentLength != int64(len(body)) {
		t.Fatalf("got %q %q %d", got, o.ContentType, o.ContentLength)
	}
	if err := s.Delete(ctx, "test-artifacts", "a/b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "test-artifacts", "a/b.txt"); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}
