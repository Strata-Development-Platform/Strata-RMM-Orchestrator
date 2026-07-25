package storage_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
)

func testBackendContract(t *testing.T, name string, newBackend func() (storage.Backend, func())) {
	t.Run(name, func(t *testing.T) {
		b, cleanup := newBackend()
		defer cleanup()

		ctx := context.Background()
		key := fmt.Sprintf("test/contract/%d", time.Now().UnixNano())
		content := "test content for contract verification"

		t.Run("upload and download", func(t *testing.T) {
			uploadedKey, err := b.Upload(ctx, key, strings.NewReader(content), storage.UploadOptions{
				ContentType: "text/plain",
				Metadata:    map[string]string{"test": "true"},
			})
			if err != nil {
				t.Fatalf("upload: %v", err)
			}
			if uploadedKey != key {
				t.Errorf("key mismatch: got %s, want %s", uploadedKey, key)
			}

			rc, err := b.Download(ctx, key)
			if err != nil {
				t.Fatalf("download: %v", err)
			}
			defer rc.Close()

			buf := new(strings.Builder)
			if _, err := io.Copy(buf, rc); err != nil {
				t.Fatalf("read: %v", err)
			}
			if buf.String() != content {
				t.Errorf("content mismatch: got %q, want %q", buf.String(), content)
			}
		})

		t.Run("exists", func(t *testing.T) {
			exists, err := b.Exists(ctx, key)
			if err != nil {
				t.Fatalf("exists: %v", err)
			}
			if !exists {
				t.Error("expected object to exist")
			}

			exists, err = b.Exists(ctx, "nonexistent")
			if err != nil {
				t.Fatalf("exists nonexistent: %v", err)
			}
			if exists {
				t.Error("expected nonexistent object to not exist")
			}
		})

		t.Run("stat", func(t *testing.T) {
			info, err := b.Stat(ctx, key)
			if err != nil {
				t.Fatalf("stat: %v", err)
			}
			if info.Size != int64(len(content)) {
				t.Errorf("size mismatch: got %d, want %d", info.Size, len(content))
			}
			if info.Key != key {
				t.Errorf("key mismatch: got %s, want %s", info.Key, key)
			}
		})

		t.Run("presigned url", func(t *testing.T) {
			url, err := b.PresignedURL(ctx, key, storage.PresignedOptions{Expiry: time.Hour})
			if err != nil {
				t.Fatalf("presign: %v", err)
			}
			if !strings.HasPrefix(url, "http") && !strings.HasPrefix(url, "file://") && !strings.HasPrefix(url, "mock://") {
				t.Errorf("unexpected url scheme: %s", url)
			}
		})

		t.Run("list", func(t *testing.T) {
			objects, err := b.List(ctx, "test/contract/", storage.ListOptions{})
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			found := false
			for _, o := range objects {
				if o.Key == key {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("key %s not in list results", key)
			}
		})

		t.Run("delete", func(t *testing.T) {
			if err := b.Delete(ctx, key); err != nil {
				t.Fatalf("delete: %v", err)
			}
			exists, err := b.Exists(ctx, key)
			if err != nil {
				t.Fatalf("exists after delete: %v", err)
			}
			if exists {
				t.Error("expected object to be deleted")
			}
		})

		t.Run("not found errors", func(t *testing.T) {
			if _, err := b.Download(ctx, "nonexistent"); err == nil {
				t.Error("expected error for nonexistent download")
			} else if !strings.Contains(err.Error(), "not found") {
				t.Errorf("unexpected error: %v", err)
			}

			if _, err := b.Stat(ctx, "nonexistent"); err == nil {
				t.Error("expected error for nonexistent stat")
			}
		})

		t.Run("upload with checksum", func(t *testing.T) {
			cksmKey := key + "-cksm"
			cksmContent := "checksum-verified-content"
			h := sha256.Sum256([]byte(cksmContent))
			expectedCksm := fmt.Sprintf("%x", h)

			_, err := b.Upload(ctx, cksmKey, strings.NewReader(cksmContent), storage.UploadOptions{
				Checksum: &expectedCksm,
			})
			if err != nil {
				t.Fatalf("upload with checksum: %v", err)
			}

			badCksm := "0000000000000000000000000000000000000000000000000000000000000000"
			_, err = b.Upload(ctx, cksmKey+"-bad", strings.NewReader(cksmContent), storage.UploadOptions{
				Checksum: &badCksm,
			})
			if err == nil {
				t.Error("expected checksum mismatch error")
			}
		})
	})
}

func TestLocalBackend(t *testing.T) {
	testBackendContract(t, "local", func() (storage.Backend, func()) {
		b, err := storage.NewLocalBackend(storage.Config{
			Extra: map[string]string{"path": t.TempDir()},
		})
		if err != nil {
			t.Fatal(err)
		}
		return b, func() { b.Close() }
	})
}

func TestMockBackend(t *testing.T) {
	testBackendContract(t, "mock", func() (storage.Backend, func()) {
		return storage.NewMockBackend(), func() {}
	})
}
