package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Backend struct {
	client     *s3.Client
	uploader   *manager.Uploader
	downloader *manager.Downloader
	bucket     string
	partSize   int64
	presignTTL time.Duration
	kmsKeyID   string
	sseType    types.ServerSideEncryption
}

func NewS3Backend(ctx context.Context, cfg Config) (*S3Backend, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("%w: bucket is required", ErrInvalidConfig)
	}

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		awsCfg.Credentials = credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
	}

	var clientOpts []func(*s3.Options)
	if cfg.Endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, clientOpts...)

	partSize := cfg.PartSize
	if partSize == 0 {
		partSize = 64 * 1024 * 1024
	}

	return &S3Backend{
		client:     client,
		uploader:   manager.NewUploader(client, func(u *manager.Uploader) { u.PartSize = partSize }),
		downloader: manager.NewDownloader(client, func(d *manager.Downloader) { d.PartSize = partSize }),
		bucket:     cfg.Bucket,
		partSize:   partSize,
		presignTTL: time.Hour,
		kmsKeyID:   cfg.KMSKeyID,
		sseType:    types.ServerSideEncryptionAes256,
	}, nil
}

func (b *S3Backend) Upload(ctx context.Context, key string, r io.Reader, opts UploadOptions) (string, error) {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(b.bucket),
		Key:         aws.String(key),
		Body:        r,
		ContentType: aws.String(opts.ContentType),
		Metadata:    opts.Metadata,
	}

	if opts.ContentEncoding != "" {
		input.ContentEncoding = aws.String(opts.ContentEncoding)
	}

	switch opts.Encryption.Type {
	case EncryptionKMS:
		kmsID := opts.Encryption.KMSKeyID
		if kmsID == "" {
			kmsID = b.kmsKeyID
		}
		input.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		if kmsID != "" {
			input.SSEKMSKeyId = aws.String(kmsID)
		}
	case EncryptionSSE:
		input.ServerSideEncryption = b.sseType
	case EncryptionSSEC:
		if len(opts.Encryption.CustomerKey) > 0 {
			input.SSECustomerAlgorithm = aws.String("AES256")
			input.SSECustomerKey = aws.String(base64.StdEncoding.EncodeToString(opts.Encryption.CustomerKey))
		}
	}

	if opts.Checksum != nil {
		expected, err := hex.DecodeString(*opts.Checksum)
		if err == nil {
			hasher := sha256.New()
			tee := io.TeeReader(r, hasher)
			input.Body = tee
			_, err := b.uploader.Upload(ctx, input)
			if err != nil {
				return "", fmt.Errorf("s3 upload: %w", err)
			}
			actual := hasher.Sum(nil)
			if !bytes.Equal(expected, actual) {
				b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: aws.String(b.bucket),
					Key:    aws.String(key),
				})
				return "", fmt.Errorf("checksum mismatch: expected %x, got %x", expected, actual)
			}
			return key, nil
		}
	}

	_, err := b.uploader.Upload(ctx, input)
	if err != nil {
		return "", fmt.Errorf("s3 upload: %w", err)
	}
	return key, nil
}

func (b *S3Backend) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	output, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return nil, fmt.Errorf("s3 get object: %w", err)
	}
	return output.Body, nil
}

func (b *S3Backend) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (b *S3Backend) Exists(ctx context.Context, key string) (bool, error) {
	_, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("s3 head object: %w", err)
	}
	return true, nil
}

func (b *S3Backend) PresignedURL(ctx context.Context, key string, opts PresignedOptions) (string, error) {
	ps := s3.NewPresignClient(b.client)

	method := opts.Method
	if method == "" {
		method = "GET"
	}
	expiry := opts.Expiry
	if expiry == 0 || expiry > b.presignTTL {
		expiry = b.presignTTL
	}

	switch method {
	case "GET":
		req := &s3.GetObjectInput{
			Bucket: aws.String(b.bucket),
			Key:    aws.String(key),
		}
		if opts.ContentType != "" {
			req.ResponseContentType = aws.String(opts.ContentType)
		}
		if opts.Disposition != "" {
			req.ResponseContentDisposition = aws.String(opts.Disposition)
		}
		url, err := ps.PresignGetObject(ctx, req, s3.WithPresignExpires(expiry))
		if err != nil {
			return "", fmt.Errorf("presign get: %w", err)
		}
		return url.URL, nil

	case "PUT":
		req := &s3.PutObjectInput{
			Bucket: aws.String(b.bucket),
			Key:    aws.String(key),
		}
		if opts.ContentType != "" {
			req.ContentType = aws.String(opts.ContentType)
		}
		url, err := ps.PresignPutObject(ctx, req, s3.WithPresignExpires(expiry))
		if err != nil {
			return "", fmt.Errorf("presign put: %w", err)
		}
		return url.URL, nil

	default:
		return "", fmt.Errorf("unsupported presign method: %s", method)
	}
}

func (b *S3Backend) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	output, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return ObjectInfo{}, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return ObjectInfo{}, fmt.Errorf("s3 head object: %w", err)
	}

	encType := EncryptionNone
	if output.ServerSideEncryption != "" {
		switch output.ServerSideEncryption {
		case types.ServerSideEncryptionAwsKms:
			encType = EncryptionKMS
		default:
			encType = EncryptionSSE
		}
	}

	meta := make(map[string]string)
	for k, v := range output.Metadata {
		meta[k] = v
	}

	return ObjectInfo{
		Key:          key,
		Size:         aws.ToInt64(output.ContentLength),
		LastModified: aws.ToTime(output.LastModified),
		ETag:         aws.ToString(output.ETag),
		ContentType:  aws.ToString(output.ContentType),
		Metadata:     meta,
		Encryption:   encType,
	}, nil
}

func (b *S3Backend) List(ctx context.Context, prefix string, opts ListOptions) ([]ObjectInfo, error) {
	maxKeys := opts.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 100
	}
	if maxKeys > math.MaxInt32 {
		maxKeys = math.MaxInt32
	}

	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(b.bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(int32(maxKeys)),
	}

	if opts.Cursor != "" {
		input.StartAfter = aws.String(opts.Cursor)
	}

	var result []ObjectInfo
	paginator := s3.NewListObjectsV2Paginator(b.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 list: %w", err)
		}
		for _, obj := range page.Contents {
			result = append(result, ObjectInfo{
				Key:          aws.ToString(obj.Key),
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
				ETag:         aws.ToString(obj.ETag),
			})
		}
		if len(result) >= int(maxKeys) {
			break
		}
	}
	return result, nil
}

func (b *S3Backend) Close() error {
	return nil
}

func isNotFound(err error) bool {
	var nf *types.NoSuchKey
	var nfb *types.NotFound
	return errors.As(err, &nf) || errors.As(err, &nfb)
}
