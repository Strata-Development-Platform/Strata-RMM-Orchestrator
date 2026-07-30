package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Repository stores artifacts in an S3-compatible bucket.
type S3Repository struct {
	client     *s3.Client
	bucket     string
	uploadMgr  *manager.Uploader //nolint:staticcheck
	downloader *manager.Downloader //nolint:staticcheck
}

// NewS3Repository creates a repository backed by an S3 bucket.
func NewS3Repository(client *s3.Client, bucket string) (*S3Repository, error) {
	if bucket == "" {
		return nil, fmt.Errorf("bucket name required")
	}
	return &S3Repository{
		client:     client,
		bucket:     bucket,
	uploadMgr:  manager.NewUploader(client), //nolint:staticcheck
	downloader: manager.NewDownloader(client), //nolint:staticcheck
	}, nil
}

// ProviderName implements Repository.
func (r *S3Repository) ProviderName() string { return "s3" }

func (r *S3Repository) objectKey(backupSetID, componentID string) string {
	if componentID == "" {
		return fmt.Sprintf("backups/%s/manifest.json", backupSetID)
	}
	return fmt.Sprintf("backups/%s/components/%s", backupSetID, componentID)
}

func (r *S3Repository) statusKey(backupSetID, componentID string) string {
	return fmt.Sprintf("backups/%s/components/%s.status.json", backupSetID, componentID)
}

func (r *S3Repository) retentionKey(backupSetID string) string {
	return fmt.Sprintf("backups/%s/retention.json", backupSetID)
}

func (r *S3Repository) CreateBackupSet(ctx context.Context, set BackupSet) error {
	// S3 is eventually consistent but we use put-object for the marker
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.objectKey(set.ID, "")),
		Body:   nil,
		Metadata: map[string]string{
			"x-strata-backupset": "creating",
		},
	})
	return err
}

func (r *S3Repository) WriteComponent(ctx context.Context, backupSetID, componentID string, reader io.Reader) error {
	key := r.objectKey(backupSetID, componentID)

	// Upload with bounded concurrency via manager
	_, err := r.uploadMgr.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
		Body:   reader,
	})
	if err != nil {
		return fmt.Errorf("upload component: %w", err)
	}
	return nil
}

func (r *S3Repository) FinalizeComponent(ctx context.Context, backupSetID, componentID, plaintextDigest, ciphertextDigest string, encryptedSize, originalSize int64) error {
	status := struct {
		ComponentID      string `json:"component_id"`
		PlaintextDigest  string `json:"plaintext_digest"`
		CiphertextDigest string `json:"ciphertext_digest"`
		EncryptedSize    int64  `json:"encrypted_size"`
		OriginalSize     int64  `json:"original_size"`
		Status           string `json:"status"`
	}{
		ComponentID:      componentID,
		PlaintextDigest:  plaintextDigest,
		CiphertextDigest: ciphertextDigest,
		EncryptedSize:    encryptedSize,
		OriginalSize:     originalSize,
		Status:           "finalized",
	}

	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("marshal component status: %w", err)
	}

	_, err = r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.statusKey(backupSetID, componentID)),
		Body:   strings.NewReader(string(data)),
	})
	return err
}

func (r *S3Repository) FinalizeBackupSet(ctx context.Context, manifest *Manifest) error {
	if err := manifest.VerifyManifest(); err != nil {
		return fmt.Errorf("verify manifest: %w", err)
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	// Update the backup set marker to "finalized"
	_, err = r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.objectKey(manifest.BackupSetID, "")),
		Body:   strings.NewReader(string(data)),
		Metadata: map[string]string{
			"x-strata-backupset": "finalized",
		},
	})
	if err != nil {
		return fmt.Errorf("finalize backup set: %w", err)
	}

	return nil
}

func (r *S3Repository) ListBackupSets(ctx context.Context) ([]BackupSet, error) {
	paginator := s3.NewListObjectsV2Paginator(r.client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(r.bucket),
		Prefix:    aws.String("backups/"),
		Delimiter: aws.String("/"),
	})

	var sets []BackupSet
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range page.CommonPrefixes {
			prefix := aws.ToString(obj.Prefix)
			// Extract backup set ID from prefix like "backups/<id>/components/"
			parts := strings.Split(strings.TrimPrefix(prefix, "backups/"), "/")
			if len(parts) >= 1 && parts[0] != "" {
				sets = append(sets, BackupSet{ID: parts[0]})
			}
		}
	}

	// Filter to only finalized sets
	var finalized []BackupSet
	for _, s := range sets {
		_, err := r.ReadManifest(ctx, s.ID)
		if err == nil {
			finalized = append(finalized, s)
		}
	}
	return finalized, nil
}

func (r *S3Repository) ReadManifest(ctx context.Context, backupSetID string) (*Manifest, error) {
	result, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.objectKey(backupSetID, "")),
	})
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	defer result.Body.Close() //nolint:errcheck

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("read manifest body: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	return &m, nil
}

func (r *S3Repository) ReadComponent(ctx context.Context, backupSetID, componentID string) (io.ReadCloser, error) {
	result, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.objectKey(backupSetID, componentID)),
	})
	if err != nil {
		return nil, fmt.Errorf("read component: %w", err)
	}
	return result.Body, nil
}

func (r *S3Repository) VerifyIntegrity(ctx context.Context, backupSetID, componentID, expectedDigest string) error {
	// Read the component and compute its digest
	body, err := r.ReadComponent(ctx, backupSetID, componentID)
	if err != nil {
		return err
	}
	defer body.Close() //nolint:errcheck

	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read component for verification: %w", err)
	}

	actualDigest := DigestBase64(data)
	if actualDigest != expectedDigest {
		return fmt.Errorf("integrity mismatch: expected %s, got %s", expectedDigest, actualDigest)
	}
	return nil
}

func (r *S3Repository) MarkRetention(ctx context.Context, backupSetID, policy string) error {
	data, err := json.Marshal(map[string]string{"policy": policy, "backup_set_id": backupSetID})
	if err != nil {
		return fmt.Errorf("marshal retention: %w", err)
	}

	_, err = r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.retentionKey(backupSetID)),
		Body:   strings.NewReader(string(data)),
	})
	return err
}

func (r *S3Repository) DeleteBackupSet(ctx context.Context, backupSetID, policy string) error {
	if policy == "" {
		return fmt.Errorf("delete policy required")
	}

	paginator := s3.NewListObjectsV2Paginator(r.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(r.bucket),
		Prefix: aws.String(fmt.Sprintf("backups/%s/", backupSetID)),
	})

	var keys []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list objects for deletion: %w", err)
		}
		for _, obj := range page.Contents {
			keys = append(keys, aws.ToString(obj.Key))
		}
	}

	for _, key := range keys {
		_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(r.bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return fmt.Errorf("delete object %s: %w", key, err)
		}
	}
	return nil
}

// S3CompatibleEndpoint wraps configuration for S3-compatible services (MinIO, etc.).
type S3CompatibleEndpoint struct {
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Bucket    string
}

// Validate checks that the endpoint configuration is complete and secure.
func (e *S3CompatibleEndpoint) Validate() error {
	if e.Endpoint == "" {
		return fmt.Errorf("endpoint required")
	}
	if e.Region == "" {
		return fmt.Errorf("region required")
	}
	if e.Bucket == "" {
		return fmt.Errorf("bucket required")
	}
	if e.UseSSL && strings.HasPrefix(e.Endpoint, "http://") {
		return fmt.Errorf("insecure transport: endpoint must use HTTPS when UseSSL is true")
	}
	return nil
}
