package storage

import (
	"context"
	"io"
	"encoding/json"
	"testing"
	"time"
)

// TestEncryptionTypeConstants verifies all EncryptionType constant values
func TestEncryptionTypeConstants(t *testing.T) {
	constants := map[EncryptionType]string{
		EncryptionNone:   "none",
		EncryptionSSE:    "sse-s3",
		EncryptionKMS:    "sse-kms",
		EncryptionSSEC:   "sse-c",
	}

	for etype, expected := range constants {
		if string(etype) != expected {
			t.Errorf("EncryptionType %q = %q, want %q", etype, etype, expected)
		}
	}
}

// TestUploadOptions_StructFields verifies all UploadOptions struct fields
func TestUploadOptions_StructFields(t *testing.T) {
	contentType := "application/pdf"
	checksum := "abc123"

	opts := UploadOptions{
		ContentType:      contentType,
		ContentEncoding:  "gzip",
		Metadata:         map[string]string{"author": "test"},
		Encryption:       EncryptionConfig{Type: EncryptionSSE},
		PartSize:         5 * 1024 * 1024, // 5MB
		Checksum:         &checksum,
		ContentLength:    1024,
		ContentLengthSet: true,
	}

	if opts.ContentType != contentType {
		t.Errorf("UploadOptions.ContentType = %q, want %q", opts.ContentType, contentType)
	}
	if opts.ContentEncoding != "gzip" {
		t.Errorf("UploadOptions.ContentEncoding = %q, want %q", opts.ContentEncoding, "gzip")
	}
	if len(opts.Metadata) != 1 || opts.Metadata["author"] != "test" {
		t.Errorf("UploadOptions.Metadata = %v, want map[author:test]", opts.Metadata)
	}
	if opts.Encryption.Type != EncryptionSSE {
		t.Errorf("UploadOptions.Encryption.Type = %q, want %q", opts.Encryption.Type, EncryptionSSE)
	}
	if opts.PartSize != 5*1024*1024 {
		t.Errorf("UploadOptions.PartSize = %d, want %d", opts.PartSize, 5*1024*1024)
	}
	if opts.Checksum == nil || *opts.Checksum != checksum {
		t.Errorf("UploadOptions.Checksum = %v, want %q", opts.Checksum, checksum)
	}
	if opts.ContentLength != 1024 {
		t.Errorf("UploadOptions.ContentLength = %d, want %d", opts.ContentLength, 1024)
	}
	if !opts.ContentLengthSet {
		t.Error("UploadOptions.ContentLengthSet should be true")
	}
}

// TestUploadOptions_ZeroValues verifies UploadOptions with zero values
func TestUploadOptions_ZeroValues(t *testing.T) {
	opts := UploadOptions{}

	if opts.ContentType != "" {
		t.Error("UploadOptions.ContentType should be empty by default")
	}
	if opts.ContentEncoding != "" {
		t.Error("UploadOptions.ContentEncoding should be empty by default")
	}
	if opts.Metadata != nil {
		t.Error("UploadOptions.Metadata should be nil by default")
	}
	if opts.Encryption.Type != "" {
		t.Error("UploadOptions.Encryption.Type should be empty by default")
	}
	if opts.PartSize != 0 {
		t.Error("UploadOptions.PartSize should be 0 by default")
	}
	if opts.Checksum != nil {
		t.Error("UploadOptions.Checksum should be nil by default")
	}
	if opts.ContentLength != 0 {
		t.Error("UploadOptions.ContentLength should be 0 by default")
	}
	if opts.ContentLengthSet {
		t.Error("UploadOptions.ContentLengthSet should be false by default")
	}
}

// TestPresignedOptions_StructFields verifies all PresignedOptions struct fields
func TestPresignedOptions_StructFields(t *testing.T) {
	method := "GET"
	expiry := 1 * time.Hour
	disposition := "attachment; filename=test.pdf"
	headers := map[string]string{"X-Custom": "value"}

	opts := PresignedOptions{
		Method:      method,
		Expiry:      expiry,
		ContentType: "application/pdf",
		Disposition: disposition,
		Headers:     headers,
	}

	if opts.Method != method {
		t.Errorf("PresignedOptions.Method = %q, want %q", opts.Method, method)
	}
	if opts.Expiry != expiry {
		t.Errorf("PresignedOptions.Expiry = %v, want %v", opts.Expiry, expiry)
	}
	if opts.ContentType != "application/pdf" {
		t.Errorf("PresignedOptions.ContentType = %q, want %q", opts.ContentType, "application/pdf")
	}
	if opts.Disposition != disposition {
		t.Errorf("PresignedOptions.Disposition = %q, want %q", opts.Disposition, disposition)
	}
	if len(opts.Headers) != 1 || opts.Headers["X-Custom"] != "value" {
		t.Errorf("PresignedOptions.Headers = %v, want map[X-Custom:value]", opts.Headers)
	}
}

// TestPresignedOptions_ZeroValues verifies PresignedOptions with zero values
func TestPresignedOptions_ZeroValues(t *testing.T) {
	opts := PresignedOptions{}

	if opts.Method != "" {
		t.Error("PresignedOptions.Method should be empty by default")
	}
	if opts.Expiry != 0 {
		t.Error("PresignedOptions.Expiry should be 0 by default")
	}
	if opts.ContentType != "" {
		t.Error("PresignedOptions.ContentType should be empty by default")
	}
	if opts.Disposition != "" {
		t.Error("PresignedOptions.Disposition should be empty by default")
	}
	if opts.Headers != nil {
		t.Error("PresignedOptions.Headers should be nil by default")
	}
}

// TestEncryptionConfig_StructFields verifies all EncryptionConfig struct fields
func TestEncryptionConfig_StructFields(t *testing.T) {
	keyID := "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"
	customerKey := []byte("customer-provided-key")

	config := EncryptionConfig{
		Type:        EncryptionKMS,
		KMSKeyID:    keyID,
		CustomerKey: customerKey,
	}

	if config.Type != EncryptionKMS {
		t.Errorf("EncryptionConfig.Type = %q, want %q", config.Type, EncryptionKMS)
	}
	if config.KMSKeyID != keyID {
		t.Errorf("EncryptionConfig.KMSKeyID = %q, want %q", config.KMSKeyID, keyID)
	}
	if len(config.CustomerKey) != len(customerKey) {
		t.Errorf("EncryptionConfig.CustomerKey length = %d, want %d", len(config.CustomerKey), len(customerKey))
	}
}

// TestEncryptionConfig_ZeroValues verifies EncryptionConfig with zero values
func TestEncryptionConfig_ZeroValues(t *testing.T) {
	config := EncryptionConfig{}

	if config.Type != "" {
		t.Error("EncryptionConfig.Type should be empty by default")
	}
	if config.KMSKeyID != "" {
		t.Error("EncryptionConfig.KMSKeyID should be empty by default")
	}
	if config.CustomerKey != nil {
		t.Error("EncryptionConfig.CustomerKey should be nil by default")
	}
}

// TestObjectInfo_StructFields verifies all ObjectInfo struct fields
func TestObjectInfo_StructFields(t *testing.T) {
	now := time.Now()
	metadata := map[string]string{"author": "test"}

	info := ObjectInfo{
		Key:            "reports/tenant-001/2024-01-01-120000.pdf",
		Size:           1048576,
		LastModified:   now,
		ETag:           "abc123def456",
		ContentType:    "application/pdf",
		Metadata:       metadata,
		Encryption:     EncryptionSSE,
		ChecksumSHA256: "sha256hash",
	}

	if info.Key != "reports/tenant-001/2024-01-01-120000.pdf" {
		t.Errorf("ObjectInfo.Key = %q, want %q", info.Key, "reports/tenant-001/2024-01-01-120000.pdf")
	}
	if info.Size != 1048576 {
		t.Errorf("ObjectInfo.Size = %d, want %d", info.Size, 1048576)
	}
	if info.ETag != "abc123def456" {
		t.Errorf("ObjectInfo.ETag = %q, want %q", info.ETag, "abc123def456")
	}
	if info.ContentType != "application/pdf" {
		t.Errorf("ObjectInfo.ContentType = %q, want %q", info.ContentType, "application/pdf")
	}
	if len(info.Metadata) != 1 || info.Metadata["author"] != "test" {
		t.Errorf("ObjectInfo.Metadata = %v, want map[author:test]", info.Metadata)
	}
	if info.Encryption != EncryptionSSE {
		t.Errorf("ObjectInfo.Encryption = %q, want %q", info.Encryption, EncryptionSSE)
	}
	if info.ChecksumSHA256 != "sha256hash" {
		t.Errorf("ObjectInfo.ChecksumSHA256 = %q, want %q", info.ChecksumSHA256, "sha256hash")
	}
}

// TestObjectInfo_ZeroValues verifies ObjectInfo with zero values
func TestObjectInfo_ZeroValues(t *testing.T) {
	info := ObjectInfo{}

	if info.Key != "" {
		t.Error("ObjectInfo.Key should be empty by default")
	}
	if info.Size != 0 {
		t.Error("ObjectInfo.Size should be 0 by default")
	}
	if info.ETag != "" {
		t.Error("ObjectInfo.ETag should be empty by default")
	}
	if info.ContentType != "" {
		t.Error("ObjectInfo.ContentType should be empty by default")
	}
	if info.Metadata != nil {
		t.Error("ObjectInfo.Metadata should be nil by default")
	}
	if info.Encryption != "" {
		t.Error("ObjectInfo.Encryption should be empty by default")
	}
	if info.ChecksumSHA256 != "" {
		t.Error("ObjectInfo.ChecksumSHA256 should be empty by default")
	}
}

// TestListOptions_StructFields verifies all ListOptions struct fields
func TestListOptions_StructFields(t *testing.T) {
	opts := ListOptions{
		MaxKeys: 1000,
		Cursor:  "next-page-token",
	}

	if opts.MaxKeys != 1000 {
		t.Errorf("ListOptions.MaxKeys = %d, want %d", opts.MaxKeys, 1000)
	}
	if opts.Cursor != "next-page-token" {
		t.Errorf("ListOptions.Cursor = %q, want %q", opts.Cursor, "next-page-token")
	}
}

// TestListOptions_ZeroValues verifies ListOptions with zero values
func TestListOptions_ZeroValues(t *testing.T) {
	opts := ListOptions{}

	if opts.MaxKeys != 0 {
		t.Error("ListOptions.MaxKeys should be 0 by default")
	}
	if opts.Cursor != "" {
		t.Error("ListOptions.Cursor should be empty by default")
	}
}

// TestConfig_StructFields verifies all Config struct fields
func TestConfig_StructFields(t *testing.T) {
	extra := map[string]string{"custom-field": "value"}

	cfg := Config{
		Type:       "minio",
		Bucket:     "reports-bucket",
		Region:     "us-east-1",
		Endpoint:   "http://localhost:9000",
		AccessKey:  "access-key-id",
		SecretKey:  "secret-access-key",
		UseSSL:     false,
		KMSKeyID:   "arn:aws:kms:...",
		PartSize:   5 * 1024 * 1024,
		MaxRetries: 3,
		Timeout:    30 * time.Second,
		Extra:      extra,
	}

	if cfg.Type != "minio" {
		t.Errorf("Config.Type = %q, want %q", cfg.Type, "minio")
	}
	if cfg.Bucket != "reports-bucket" {
		t.Errorf("Config.Bucket = %q, want %q", cfg.Bucket, "reports-bucket")
	}
	if cfg.Region != "us-east-1" {
		t.Errorf("Config.Region = %q, want %q", cfg.Region, "us-east-1")
	}
	if cfg.Endpoint != "http://localhost:9000" {
		t.Errorf("Config.Endpoint = %q, want %q", cfg.Endpoint, "http://localhost:9000")
	}
	if cfg.AccessKey != "access-key-id" {
		t.Errorf("Config.AccessKey = %q, want %q", cfg.AccessKey, "access-key-id")
	}
	if cfg.SecretKey != "secret-access-key" {
		t.Errorf("Config.SecretKey = %q, want %q", cfg.SecretKey, "secret-access-key")
	}
	if cfg.UseSSL {
		t.Error("Config.UseSSL should be false by default")
	}
	if cfg.KMSKeyID != "arn:aws:kms:..." {
		t.Errorf("Config.KMSKeyID = %q, want %q", cfg.KMSKeyID, "arn:aws:kms:...")
	}
	if cfg.PartSize != 5*1024*1024 {
		t.Errorf("Config.PartSize = %d, want %d", cfg.PartSize, 5*1024*1024)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("Config.MaxRetries = %d, want %d", cfg.MaxRetries, 3)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Config.Timeout = %v, want %v", cfg.Timeout, 30*time.Second)
	}
	if len(cfg.Extra) != 1 || cfg.Extra["custom-field"] != "value" {
		t.Errorf("Config.Extra = %v, want map[custom-field:value]", cfg.Extra)
	}
}

// TestConfig_ZeroValues verifies Config with zero values
func TestConfig_ZeroValues(t *testing.T) {
	cfg := Config{}

	if cfg.Type != "" {
		t.Error("Config.Type should be empty by default")
	}
	if cfg.Bucket != "" {
		t.Error("Config.Bucket should be empty by default")
	}
	if cfg.Region != "" {
		t.Error("Config.Region should be empty by default")
	}
	if cfg.Endpoint != "" {
		t.Error("Config.Endpoint should be empty by default")
	}
	if cfg.AccessKey != "" {
		t.Error("Config.AccessKey should be empty by default")
	}
	if cfg.SecretKey != "" {
		t.Error("Config.SecretKey should be empty by default")
	}
	if cfg.UseSSL {
		t.Error("Config.UseSSL should be false by default")
	}
	if cfg.PartSize != 0 {
		t.Error("Config.PartSize should be 0 by default")
	}
	if cfg.MaxRetries != 0 {
		t.Error("Config.MaxRetries should be 0 by default")
	}
	if cfg.Timeout != 0 {
		t.Error("Config.Timeout should be 0 by default")
	}
	if cfg.Extra != nil {
		t.Error("Config.Extra should be nil by default")
	}
}

// TestUploadOptions_JSONRoundTrip verifies UploadOptions serializes/deserializes
func TestUploadOptions_JSONRoundTrip(t *testing.T) {
	contentType := "application/pdf"
	checksum := "abc123"
	original := UploadOptions{
		ContentType:      contentType,
		ContentEncoding:  "gzip",
		Metadata:         map[string]string{"author": "test"},
		Encryption:       EncryptionConfig{Type: EncryptionSSE, KMSKeyID: "key-123"},
		PartSize:         5 * 1024 * 1024,
		Checksum:         &checksum,
		ContentLength:    1024,
		ContentLengthSet: true,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal UploadOptions: %v", err)
	}

	var restored UploadOptions
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal UploadOptions: %v", err)
	}

	if restored.ContentType != original.ContentType {
		t.Errorf("Round-trip UploadOptions.ContentType = %q, want %q", restored.ContentType, original.ContentType)
	}
	if restored.ContentEncoding != original.ContentEncoding {
		t.Errorf("Round-trip UploadOptions.ContentEncoding = %q, want %q", restored.ContentEncoding, original.ContentEncoding)
	}
	if restored.PartSize != original.PartSize {
		t.Errorf("Round-trip UploadOptions.PartSize = %d, want %d", restored.PartSize, original.PartSize)
	}
	if restored.ContentLength != original.ContentLength {
		t.Errorf("Round-trip UploadOptions.ContentLength = %d, want %d", restored.ContentLength, original.ContentLength)
	}
}

// TestPresignedOptions_JSONRoundTrip verifies PresignedOptions serializes/deserializes
func TestPresignedOptions_JSONRoundTrip(t *testing.T) {
	original := PresignedOptions{
		Method:      "GET",
		Expiry:      1 * time.Hour,
		ContentType: "application/pdf",
		Disposition: "attachment; filename=test.pdf",
		Headers:     map[string]string{"X-Custom": "value"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal PresignedOptions: %v", err)
	}

	var restored PresignedOptions
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal PresignedOptions: %v", err)
	}

	if restored.Method != original.Method {
		t.Errorf("Round-trip PresignedOptions.Method = %q, want %q", restored.Method, original.Method)
	}
	if restored.ContentType != original.ContentType {
		t.Errorf("Round-trip PresignedOptions.ContentType = %q, want %q", restored.ContentType, original.ContentType)
	}
}

// TestEncryptionConfig_JSONRoundTrip verifies EncryptionConfig serializes/deserializes
func TestEncryptionConfig_JSONRoundTrip(t *testing.T) {
	original := EncryptionConfig{
		Type:        EncryptionKMS,
		KMSKeyID:    "arn:aws:kms:us-east-1:123456789012:key/12345678",
		CustomerKey: []byte("customer-key"),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal EncryptionConfig: %v", err)
	}

	var restored EncryptionConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal EncryptionConfig: %v", err)
	}

	if restored.Type != original.Type {
		t.Errorf("Round-trip EncryptionConfig.Type = %q, want %q", restored.Type, original.Type)
	}
	if restored.KMSKeyID != original.KMSKeyID {
		t.Errorf("Round-trip EncryptionConfig.KMSKeyID = %q, want %q", restored.KMSKeyID, original.KMSKeyID)
	}
}

// TestObjectInfo_JSONRoundTrip verifies ObjectInfo serializes/deserializes
func TestObjectInfo_JSONRoundTrip(t *testing.T) {
	original := ObjectInfo{
		Key:            "reports/tenant-001/test.pdf",
		Size:           1048576,
		ETag:           "etag-123",
		ContentType:    "application/pdf",
		Metadata:       map[string]string{"author": "test"},
		Encryption:     EncryptionSSE,
		ChecksumSHA256: "sha256hash",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal ObjectInfo: %v", err)
	}

	var restored ObjectInfo
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal ObjectInfo: %v", err)
	}

	if restored.Key != original.Key {
		t.Errorf("Round-trip ObjectInfo.Key = %q, want %q", restored.Key, original.Key)
	}
	if restored.Size != original.Size {
		t.Errorf("Round-trip ObjectInfo.Size = %d, want %d", restored.Size, original.Size)
	}
	if restored.ETag != original.ETag {
		t.Errorf("Round-trip ObjectInfo.ETag = %q, want %q", restored.ETag, original.ETag)
	}
	if restored.ContentType != original.ContentType {
		t.Errorf("Round-trip ObjectInfo.ContentType = %q, want %q", restored.ContentType, original.ContentType)
	}
	if restored.Encryption != original.Encryption {
		t.Errorf("Round-trip ObjectInfo.Encryption = %q, want %q", restored.Encryption, original.Encryption)
	}
}

// TestConfig_JSONRoundTrip verifies Config serializes/deserializes
func TestConfig_JSONRoundTrip(t *testing.T) {
	extra := map[string]string{"custom": "value"}
	original := Config{
		Type:       "s3",
		Bucket:     "my-bucket",
		Region:     "us-west-2",
		Endpoint:   "https://s3.amazonaws.com",
		AccessKey:  "AKIAIOSFODNN7EXAMPLE",
		SecretKey:  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		UseSSL:     true,
		PartSize:   8 * 1024 * 1024,
		MaxRetries: 5,
		Timeout:    60 * time.Second,
		Extra:      extra,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal Config: %v", err)
	}

	var restored Config
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal Config: %v", err)
	}

	if restored.Type != original.Type {
		t.Errorf("Round-trip Config.Type = %q, want %q", restored.Type, original.Type)
	}
	if restored.Bucket != original.Bucket {
		t.Errorf("Round-trip Config.Bucket = %q, want %q", restored.Bucket, original.Bucket)
	}
	if restored.Region != original.Region {
		t.Errorf("Round-trip Config.Region = %q, want %q", restored.Region, original.Region)
	}
	if restored.UseSSL != original.UseSSL {
		t.Errorf("Round-trip Config.UseSSL = %v, want %v", restored.UseSSL, original.UseSSL)
	}
	if restored.PartSize != original.PartSize {
		t.Errorf("Round-trip Config.PartSize = %d, want %d", restored.PartSize, original.PartSize)
	}
}

// TestStorageErrorConstants verifies storage error values
func TestStorageErrorConstants(t *testing.T) {
	if ErrNotFound == nil {
		t.Error("ErrNotFound should not be nil")
	}
	if ErrNotFound.Error() != "object not found" {
		t.Errorf("ErrNotFound.Error() = %q, want %q", ErrNotFound.Error(), "object not found")
	}

	if ErrBucketMissing == nil {
		t.Error("ErrBucketMissing should not be nil")
	}
	if ErrBucketMissing.Error() != "bucket does not exist" {
		t.Errorf("ErrBucketMissing.Error() = %q, want %q", ErrBucketMissing.Error(), "bucket does not exist")
	}

	if ErrInvalidConfig == nil {
		t.Error("ErrInvalidConfig should not be nil")
	}
	if ErrInvalidConfig.Error() != "invalid configuration" {
		t.Errorf("ErrInvalidConfig.Error() = %q, want %q", ErrInvalidConfig.Error(), "invalid configuration")
	}
}

// TestBackendInterface verifies Backend interface exists and has all methods
func TestBackendInterface(t *testing.T) {
	// Verify Backend interface exists with all expected methods
	// This is a compile-time check — if Backend doesn't have these methods,
	// the interface wouldn't satisfy the compile-time check below
	var _ Backend = (*mockBackend)(nil)
}

// mockBackend implements Backend for interface verification
type mockBackend struct{}

func (m *mockBackend) Upload(ctx context.Context, key string, r io.Reader, opts UploadOptions) (string, error) {
	return "", nil
}
func (m *mockBackend) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockBackend) Delete(ctx context.Context, key string) error {
	return nil
}
func (m *mockBackend) Exists(ctx context.Context, key string) (bool, error) {
	return false, nil
}
func (m *mockBackend) PresignedURL(ctx context.Context, key string, opts PresignedOptions) (string, error) {
	return "", nil
}
func (m *mockBackend) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	return ObjectInfo{}, nil
}
func (m *mockBackend) List(ctx context.Context, prefix string, opts ListOptions) ([]ObjectInfo, error) {
	return nil, nil
}
func (m *mockBackend) Close() error {
	return nil
}
