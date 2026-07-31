package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

var (
	ErrNotFound      = errors.New("object not found")
	ErrBucketMissing = errors.New("bucket does not exist")
	ErrInvalidConfig = errors.New("invalid configuration")
)

type Backend interface {
	Upload(ctx context.Context, key string, r io.Reader, opts UploadOptions) (string, error)
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	PresignedURL(ctx context.Context, key string, opts PresignedOptions) (string, error)
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	List(ctx context.Context, prefix string, opts ListOptions) ([]ObjectInfo, error)
	Close() error
}

type UploadOptions struct {
	ContentType      string
	ContentEncoding  string
	Metadata         map[string]string
	Encryption       EncryptionConfig
	PartSize         int64
	Checksum         *string
	ContentLength    int64
	ContentLengthSet bool
}

type PresignedOptions struct {
	Method      string
	Expiry      time.Duration
	ContentType string
	Disposition string
	Headers     map[string]string
}

type EncryptionConfig struct {
	Type        EncryptionType
	KMSKeyID    string
	CustomerKey []byte
}

type EncryptionType string

const (
	EncryptionNone EncryptionType = "none"
	EncryptionSSE  EncryptionType = "sse-s3"
	EncryptionKMS  EncryptionType = "sse-kms"
	EncryptionSSEC EncryptionType = "sse-c"
)

type ObjectInfo struct {
	Key            string
	Size           int64
	LastModified   time.Time
	ETag           string
	ContentType    string
	Metadata       map[string]string
	Encryption     EncryptionType
	ChecksumSHA256 string
}

type ListOptions struct {
	MaxKeys int
	Cursor  string
}

type Config struct {
	Type       string
	Bucket     string
	Region     string
	Endpoint   string
	AccessKey  string
	SecretKey  string
	UseSSL     bool
	KMSKeyID   string
	PartSize   int64
	MaxRetries int
	Timeout    time.Duration
	Extra      map[string]string
}

func NewBackend(ctx context.Context, cfg Config) (Backend, error) {
	switch cfg.Type {
	case "minio":
		return NewMinIOBackend(ctx, cfg)
	case "s3":
		return NewS3Backend(ctx, cfg)
	case "local":
		return NewLocalBackend(cfg)
	default:
		return nil, fmt.Errorf("%w: unknown storage backend type: %s", ErrInvalidConfig, cfg.Type)
	}
}

func MustNewBackend(ctx context.Context, cfg Config) Backend {
	b, err := NewBackend(ctx, cfg)
	if err != nil {
		panic(err)
	}
	return b
}
