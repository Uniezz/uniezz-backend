package media

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
)

func newMinioStorage(t *testing.T) *objectStorage {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping object storage integration test in short mode")
	}
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:9000"
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		t.Skipf("Skipping object storage integration test: invalid S3_ENDPOINT %q", endpoint)
	}
	conn, err := net.DialTimeout("tcp", u.Host, time.Second)
	if err != nil {
		t.Skipf("Skipping object storage integration test: %s is not reachable", endpoint)
	}
	_ = conn.Close()

	user := os.Getenv("S3_ACCESS_KEY_ID")
	if user == "" {
		user = "minioadmin"
	}
	secret := os.Getenv("S3_SECRET_ACCESS_KEY")
	if secret == "" {
		secret = "minioadmin"
	}

	storage, err := newObjectStorage(context.Background(), &config.Config{
		S3Region:          "us-east-1",
		S3Bucket:          "uniezz-media",
		S3AccessKeyID:     user,
		S3SecretAccessKey: secret,
		S3Endpoint:        endpoint,
		S3ForcePathStyle:  true,
	})
	if err != nil {
		t.Fatalf("newObjectStorage() error = %v", err)
	}
	if err := storage.checkBucket(context.Background()); err != nil {
		t.Skipf("Skipping object storage integration test: %v", err)
	}
	return storage
}

func TestPresignedUploadWritesOnce(t *testing.T) {
	storage := newMinioStorage(t)
	ctx := context.Background()

	body := []byte("first upload")
	key := objectKey(KindFeed, "image/jpeg")
	t.Cleanup(func() { _ = storage.deleteObject(context.Background(), key) })

	uploadURL, err := storage.presignPut(ctx, key, "image/jpeg", int64(len(body)))
	if err != nil {
		t.Fatalf("presignPut() error = %v", err)
	}

	put := func(content []byte) int {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		for name, value := range uploadHeaders("image/jpeg", int64(len(content))) {
			req.Header.Set(name, value)
		}
		req.ContentLength = int64(len(content))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if status := put(body); status != http.StatusOK {
		t.Fatalf("first PUT status = %d, want 200", status)
	}
	if status := put([]byte("other upload")); status != http.StatusPreconditionFailed {
		t.Fatalf("second PUT status = %d, want 412", status)
	}

	size, contentType, err := storage.headObject(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(body)) || contentType != "image/jpeg" {
		t.Errorf("object = %s %s bytes, want the first upload", contentType, strconv.FormatInt(size, 10))
	}
}
