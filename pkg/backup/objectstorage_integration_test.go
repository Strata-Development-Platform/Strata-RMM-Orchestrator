//go:build integration

package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestObjectStorageRecovery_RoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	endpoint := firstNonEmpty(os.Getenv("TEST_MINIO_ENDPOINT"), os.Getenv("S3_ENDPOINT"))
	if endpoint == "" {
		t.Skip("TEST_MINIO_ENDPOINT is required for object-storage integration tests")
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	accessKey := firstNonEmpty(os.Getenv("TEST_MINIO_ACCESS_KEY"), os.Getenv("MINIO_ROOT_USER"), "minioadmin")
	secretKey := firstNonEmpty(os.Getenv("TEST_MINIO_SECRET_KEY"), os.Getenv("MINIO_ROOT_PASSWORD"), "minioadmin")
	region := firstNonEmpty(os.Getenv("TEST_MINIO_REGION"), "us-east-1")
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	source := newMinIOTestBackend(t, ctx, endpoint, accessKey, secretKey, region, "recovery-source-"+suffix)
	target := newMinIOTestBackend(t, ctx, endpoint, accessKey, secretKey, region, "recovery-target-"+suffix)

	large := bytes.Repeat([]byte("bounded-stream-content-"), 128*1024)
	objects := []struct {
		key         string
		data        []byte
		contentType string
		tenant      string
	}{
		{"tenant-a/empty.bin", nil, "application/octet-stream", "tenant-a"},
		{"tenant-a/nested/report.txt", []byte("tenant A report"), "text/plain", "tenant-a"},
		{"tenant-b/binary.dat", []byte{0, 1, 2, 3, 254, 255}, "application/octet-stream", "tenant-b"},
		{"tenant-b/large.bin", large, "application/octet-stream", "tenant-b"},
	}
	for _, object := range objects {
		_, err := source.Upload(ctx, object.key, bytes.NewReader(object.data), storage.UploadOptions{
			ContentType:   object.contentType,
			ContentLength: int64(len(object.data)),
			Metadata: map[string]string{
				"tenant-id": object.tenant,
				"purpose":   "phase8c-integration",
			},
		})
		require.NoError(t, err)
	}

	var artifact bytes.Buffer
	recovery, err := NewObjectStorageRecovery(source, target)
	require.NoError(t, err)
	manifest, err := recovery.Backup(ctx, &artifact)
	require.NoError(t, err)
	require.NotEmpty(t, artifact.Bytes(), "backup must contain actual object bytes")

	require.NoError(t, recovery.Restore(ctx, bytes.NewReader(artifact.Bytes())))
	require.NoError(t, recovery.Verify(ctx, manifest))
	assertRestoredObjects(t, ctx, target, objects)

	// Retrying the same restore must reconcile rather than duplicate or corrupt.
	require.NoError(t, recovery.Restore(ctx, bytes.NewReader(artifact.Bytes())))
	require.NoError(t, recovery.Verify(ctx, manifest))
	assertRestoredObjects(t, ctx, target, objects)

	listed, err := target.List(ctx, "", storage.ListOptions{MaxKeys: 2})
	require.NoError(t, err)
	require.Len(t, listed, len(objects))
}

func TestObjectStorageRecovery_DetectsCorruptRestoredObject(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	endpoint := firstNonEmpty(os.Getenv("TEST_MINIO_ENDPOINT"), os.Getenv("S3_ENDPOINT"))
	if endpoint == "" {
		t.Skip("TEST_MINIO_ENDPOINT is required for object-storage integration tests")
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	accessKey := firstNonEmpty(os.Getenv("TEST_MINIO_ACCESS_KEY"), os.Getenv("MINIO_ROOT_USER"), "minioadmin")
	secretKey := firstNonEmpty(os.Getenv("TEST_MINIO_SECRET_KEY"), os.Getenv("MINIO_ROOT_PASSWORD"), "minioadmin")
	region := firstNonEmpty(os.Getenv("TEST_MINIO_REGION"), "us-east-1")
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	source := newMinIOTestBackend(t, ctx, endpoint, accessKey, secretKey, region, "recovery-corrupt-source-"+suffix)
	target := newMinIOTestBackend(t, ctx, endpoint, accessKey, secretKey, region, "recovery-corrupt-target-"+suffix)

	_, err := source.Upload(ctx, "tenant-a/data.bin", bytes.NewReader([]byte("original")), storage.UploadOptions{
		ContentType: "application/octet-stream",
		Metadata:    map[string]string{"tenant-id": "tenant-a"},
	})
	require.NoError(t, err)

	var artifact bytes.Buffer
	recovery, err := NewObjectStorageRecovery(source, target)
	require.NoError(t, err)
	manifest, err := recovery.Backup(ctx, &artifact)
	require.NoError(t, err)
	require.NoError(t, recovery.Restore(ctx, bytes.NewReader(artifact.Bytes())))

	_, err = target.Upload(ctx, "tenant-a/data.bin", bytes.NewReader([]byte("tampered")), storage.UploadOptions{
		ContentType: "application/octet-stream",
		Metadata:    map[string]string{"tenant-id": "tenant-a"},
	})
	require.NoError(t, err)
	require.Error(t, recovery.Verify(ctx, manifest))
}

func newMinIOTestBackend(t *testing.T, ctx context.Context, endpoint, accessKey, secretKey, region, bucket string) storage.Backend {
	t.Helper()
	backend, err := storage.NewBackend(ctx, storage.Config{
		Type:       "minio",
		Endpoint:   endpoint,
		Bucket:     bucket,
		Region:     region,
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		UseSSL:     strings.HasPrefix(firstNonEmpty(os.Getenv("TEST_MINIO_ENDPOINT"), os.Getenv("S3_ENDPOINT")), "https://"),
		PartSize:   5 * 1024 * 1024,
		MaxRetries: 3,
		Timeout:    30 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	return backend
}

func assertRestoredObjects(t *testing.T, ctx context.Context, target storage.Backend, objects []struct {
	key         string
	data        []byte
	contentType string
	tenant      string
}) {
	t.Helper()
	for _, object := range objects {
		reader, err := target.Download(ctx, object.key)
		require.NoError(t, err, object.key)
		got, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		require.NoError(t, readErr, object.key)
		require.NoError(t, closeErr, object.key)
		require.Equal(t, object.data, got, object.key)

		info, err := target.Stat(ctx, object.key)
		require.NoError(t, err, object.key)
		require.Equal(t, int64(len(object.data)), info.Size, object.key)
		require.Equal(t, object.contentType, info.ContentType, object.key)
		require.Equal(t, object.tenant, metadataValue(info.Metadata, "tenant-id"), object.key)
		require.Equal(t, "phase8c-integration", metadataValue(info.Metadata, "purpose"), object.key)
	}
}

func metadataValue(metadata map[string]string, wanted string) string {
	for key, value := range metadata {
		if strings.EqualFold(key, wanted) {
			return value
		}
	}
	return ""
}
