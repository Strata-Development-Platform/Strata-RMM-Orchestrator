//go:build integration

package repository

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

func TestS3RepositoryRoundTrip(t *testing.T) {
	endpoint := firstEnvironment("TEST_MINIO_ENDPOINT", "S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_MINIO_ENDPOINT is required")
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	accessKey := environmentOr("TEST_MINIO_ACCESS_KEY", "minioadmin")
	secretKey := environmentOr("TEST_MINIO_SECRET_KEY", "minioadmin")
	region := environmentOr("TEST_MINIO_REGION", "us-east-1")
	bucket := fmt.Sprintf("repository-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	require.NoError(t, err)
	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = client.DeleteBucket(context.Background(), &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	repo, err := NewS3Repository(client, bucket)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = repo.DeleteBackupSet(context.Background(), "backup-integration", "integration-cleanup")
	})
	set := BackupSet{ID: "backup-integration", EnvironmentID: "test", StartedAt: time.Now().UTC()}
	require.NoError(t, repo.CreateBackupSet(ctx, set))
	artifact := []byte("encrypted-artifact")
	cipherDigest := DigestBase64(artifact)
	require.NoError(t, repo.WriteComponent(ctx, set.ID, "postgresql.enc", bytes.NewReader(artifact)))
	require.NoError(t, repo.FinalizeComponent(ctx, set.ID, "postgresql.enc", "plain", cipherDigest, int64(len(artifact)), 5))
	manifest := &Manifest{
		Version:       ManifestVersion,
		BackupSetID:   set.ID,
		EnvironmentID: set.EnvironmentID,
		StartedAt:     set.StartedAt,
		CompletedAt:   time.Now().UTC(),
		ProtectedAt:   time.Now().UTC(),
		RequiredComponents: []ComponentRef{{
			ID:               "postgresql.enc",
			Type:             ComponentDatabase,
			ArtifactLoc:      "postgresql.enc",
			PlaintextDigest:  DigestBase64([]byte("plain")),
			CiphertextDigest: cipherDigest,
			Encryption:       "aes-256-gcm",
			KeyID:            "key-test",
			Verified:         true,
		}},
		VerificationStatus: "verified",
	}
	require.NoError(t, repo.FinalizeBackupSet(ctx, manifest))
	require.NoError(t, repo.VerifyIntegrity(ctx, set.ID, "postgresql.enc", cipherDigest))
	readManifest, err := repo.ReadManifest(ctx, set.ID)
	require.NoError(t, err)
	require.Equal(t, manifest.BackupSetID, readManifest.BackupSetID)
	sets, err := repo.ListBackupSets(ctx)
	require.NoError(t, err)
	require.Len(t, sets, 1)
}

func firstEnvironment(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}

func environmentOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
