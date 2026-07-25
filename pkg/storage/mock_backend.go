package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type MockBackend struct {
	mu       sync.RWMutex
	objects  map[string]*mockObject
	callLog  []string
}

type mockObject struct {
	data      []byte
	meta      map[string]string
	createdAt time.Time
}

func NewMockBackend() *MockBackend {
	return &MockBackend{
		objects: make(map[string]*mockObject),
	}
}

func (b *MockBackend) Upload(ctx context.Context, key string, r io.Reader, opts UploadOptions) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.callLog = append(b.callLog, "Upload:"+key)

	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	if opts.Checksum != nil {
		h := sha256.Sum256(data)
		actual := fmt.Sprintf("%x", h)
		if *opts.Checksum != actual {
			return "", fmt.Errorf("checksum mismatch: expected %s, got %s", *opts.Checksum, actual)
		}
	}

	b.objects[key] = &mockObject{
		data:      data,
		meta:      opts.Metadata,
		createdAt: time.Now(),
	}
	return key, nil
}

func (b *MockBackend) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	b.callLog = append(b.callLog, "Download:"+key)

	obj, ok := b.objects[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return io.NopCloser(bytes.NewReader(obj.data)), nil
}

func (b *MockBackend) Delete(ctx context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.callLog = append(b.callLog, "Delete:"+key)

	if _, ok := b.objects[key]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	delete(b.objects, key)
	return nil
}

func (b *MockBackend) Exists(ctx context.Context, key string) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	b.callLog = append(b.callLog, "Exists:"+key)
	_, ok := b.objects[key]
	return ok, nil
}

func (b *MockBackend) PresignedURL(ctx context.Context, key string, opts PresignedOptions) (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	b.callLog = append(b.callLog, "PresignedURL:"+key)

	if _, ok := b.objects[key]; !ok {
		return "", fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return "mock://" + key, nil
}

func (b *MockBackend) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	b.callLog = append(b.callLog, "Stat:"+key)

	obj, ok := b.objects[key]
	if !ok {
		return ObjectInfo{}, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return ObjectInfo{
		Key:          key,
		Size:         int64(len(obj.data)),
		LastModified: obj.createdAt,
		Metadata:     obj.meta,
	}, nil
}

func (b *MockBackend) List(ctx context.Context, prefix string, opts ListOptions) ([]ObjectInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	b.callLog = append(b.callLog, "List:"+prefix)

	var result []ObjectInfo
	for k, v := range b.objects {
		if strings.HasPrefix(k, prefix) {
			result = append(result, ObjectInfo{
				Key:          k,
				Size:         int64(len(v.data)),
				LastModified: v.createdAt,
				Metadata:     v.meta,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key < result[j].Key
	})

	maxKeys := opts.MaxKeys
	if maxKeys > 0 && len(result) > maxKeys {
		result = result[:maxKeys]
	}
	return result, nil
}

func (b *MockBackend) Close() error {
	b.callLog = append(b.callLog, "Close")
	return nil
}

func (b *MockBackend) Calls() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]string, len(b.callLog))
	copy(result, b.callLog)
	return result
}

func (b *MockBackend) ResetCallLog() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.callLog = nil
}
