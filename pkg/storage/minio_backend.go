package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/encrypt"
)

type MinIOBackend struct {
	client     *minio.Client
	bucket     string
	partSize   uint64
	presignTTL time.Duration
	kmsKeyID   string
}

func NewMinIOBackend(ctx context.Context, cfg Config) (*MinIOBackend, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("%w: minio endpoint is required", ErrInvalidConfig)
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("%w: bucket is required", ErrInvalidConfig)
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket check: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("make bucket: %w", err)
		}
	}

	if cfg.PartSize < 0 {
		return nil, fmt.Errorf("part size must not be negative")
	}
	partSize := uint64(cfg.PartSize)
	if partSize == 0 {
		partSize = 64 * 1024 * 1024
	}

	return &MinIOBackend{
		client:     client,
		bucket:     cfg.Bucket,
		partSize:   partSize,
		presignTTL: time.Hour,
		kmsKeyID:   cfg.KMSKeyID,
	}, nil
}

func (b *MinIOBackend) Upload(ctx context.Context, key string, r io.Reader, opts UploadOptions) (string, error) {
	po := minio.PutObjectOptions{
		ContentType:  opts.ContentType,
		UserMetadata: opts.Metadata,
		PartSize:     b.partSize,
	}

	if opts.Encryption.Type == EncryptionKMS {
		kmsID := opts.Encryption.KMSKeyID
		if kmsID == "" {
			kmsID = b.kmsKeyID
		}
		if kmsID != "" {
			sse, err := encrypt.NewSSEKMS(kmsID, nil)
			if err != nil {
				return "", fmt.Errorf("sse-kms: %w", err)
			}
			po.ServerSideEncryption = sse
		}
	} else if opts.Encryption.Type == EncryptionSSE {
		po.ServerSideEncryption = encrypt.NewSSE()
	}

	if opts.Checksum != nil {
		expected, err := hex.DecodeString(*opts.Checksum)
		if err == nil {
			hasher := sha256.New()
			tee := io.TeeReader(r, hasher)
			_, err := b.client.PutObject(ctx, b.bucket, key, tee, -1, po)
			if err != nil {
				return "", fmt.Errorf("put object: %w", err)
			}
			actual := hasher.Sum(nil)
			if !bytes.Equal(expected, actual) {
				b.client.RemoveObject(ctx, b.bucket, key, minio.RemoveObjectOptions{})
				return "", fmt.Errorf("checksum mismatch: expected %x, got %x", expected, actual)
			}
			return key, nil
		}
	}

	size := int64(-1)
	if opts.ContentLengthSet {
		size = opts.ContentLength
	}
	_, err := b.client.PutObject(ctx, b.bucket, key, r, size, po)
	if err != nil {
		return "", fmt.Errorf("put object: %w", err)
	}
	return key, nil
}

func (b *MinIOBackend) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := b.client.GetObject(ctx, b.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	_, err = obj.Stat()
	if err != nil {
		obj.Close()
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return nil, fmt.Errorf("stat object: %w", err)
	}
	return obj, nil
}

func (b *MinIOBackend) Delete(ctx context.Context, key string) error {
	return b.client.RemoveObject(ctx, b.bucket, key, minio.RemoveObjectOptions{})
}

func (b *MinIOBackend) Exists(ctx context.Context, key string) (bool, error) {
	_, err := b.client.StatObject(ctx, b.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("stat object: %w", err)
	}
	return true, nil
}

func (b *MinIOBackend) PresignedURL(ctx context.Context, key string, opts PresignedOptions) (string, error) {
	method := opts.Method
	if method == "" {
		method = "GET"
	}
	expiry := opts.Expiry
	if expiry == 0 || expiry > b.presignTTL {
		expiry = b.presignTTL
	}

	reqParams := make(map[string][]string)
	if opts.ContentType != "" {
		reqParams["response-content-type"] = []string{opts.ContentType}
	}
	if opts.Disposition != "" {
		reqParams["response-content-disposition"] = []string{opts.Disposition}
	}
	for k, v := range opts.Headers {
		reqParams[k] = []string{v}
	}

	url, err := b.client.PresignedGetObject(ctx, b.bucket, key, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("presign: %w", err)
	}
	return url.String(), nil
}

func (b *MinIOBackend) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	info, err := b.client.StatObject(ctx, b.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return ObjectInfo{}, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return ObjectInfo{}, fmt.Errorf("stat object: %w", err)
	}

	meta := make(map[string]string, len(info.UserMetadata))
	for k, v := range info.UserMetadata {
		meta[k] = v
	}

	return ObjectInfo{
		Key:          info.Key,
		Size:         info.Size,
		LastModified: info.LastModified,
		ETag:         info.ETag,
		ContentType:  info.ContentType,
		Metadata:     meta,
	}, nil
}

func (b *MinIOBackend) List(ctx context.Context, prefix string, opts ListOptions) ([]ObjectInfo, error) {
	maxKeys := opts.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 100
	}

	var result []ObjectInfo
	lo := minio.ListObjectsOptions{
		Prefix:    prefix,
		MaxKeys:   maxKeys,
		Recursive: true,
	}

	for obj := range b.client.ListObjects(ctx, b.bucket, lo) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list objects: %w", obj.Err)
		}
		result = append(result, ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
			ContentType:  obj.ContentType,
		})
	}
	return result, nil
}

func (b *MinIOBackend) Close() error {
	return nil
}
