package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalBackend struct {
	rootPath string
}

func NewLocalBackend(cfg Config) (*LocalBackend, error) {
	root := cfg.Extra["path"]
	if root == "" {
		root = "/tmp/strata-recordings"
	}
	if err := os.MkdirAll(root, 0750); err != nil {
		return nil, fmt.Errorf("create root: %w", err)
	}
	return &LocalBackend{rootPath: root}, nil
}

func (b *LocalBackend) Upload(ctx context.Context, key string, r io.Reader, opts UploadOptions) (string, error) {
	path := filepath.Join(b.rootPath, key)
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return "", fmt.Errorf("create dirs: %w", err)
	}

	if opts.Checksum != nil {
		expected, err := hex.DecodeString(*opts.Checksum)
		if err == nil {
			data, err := io.ReadAll(r)
			if err != nil {
				return "", fmt.Errorf("read: %w", err)
			}
			h := sha256.Sum256(data)
			if !bytes.Equal(expected, h[:]) {
				return "", fmt.Errorf("checksum mismatch: expected %x, got %x", expected, h[:])
			}
			if err := os.WriteFile(path, data, 0640); err != nil {
				return "", fmt.Errorf("write file: %w", err)
			}
			return key, nil
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return key, nil
}

func (b *LocalBackend) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(b.rootPath, key)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	return f, nil
}

func (b *LocalBackend) Delete(ctx context.Context, key string) error {
	path := filepath.Join(b.rootPath, key)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

func (b *LocalBackend) Exists(ctx context.Context, key string) (bool, error) {
	path := filepath.Join(b.rootPath, key)
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat file: %w", err)
	}
	return true, nil
}

func (b *LocalBackend) PresignedURL(ctx context.Context, key string, opts PresignedOptions) (string, error) {
	path := filepath.Join(b.rootPath, key)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return "file://" + abs, nil
}

func (b *LocalBackend) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	path := filepath.Join(b.rootPath, key)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ObjectInfo{}, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return ObjectInfo{}, fmt.Errorf("stat file: %w", err)
	}
	return ObjectInfo{
		Key:          key,
		Size:         info.Size(),
		LastModified: info.ModTime(),
	}, nil
}

func (b *LocalBackend) List(ctx context.Context, prefix string, opts ListOptions) ([]ObjectInfo, error) {
	searchPath := filepath.Join(b.rootPath, prefix)
	searchDir := filepath.Dir(searchPath)

	var result []ObjectInfo
	err := filepath.WalkDir(searchDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(b.rootPath, path)
		if err != nil {
			return nil
		}
		if !strings.HasPrefix(rel, prefix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		result = append(result, ObjectInfo{
			Key:          rel,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}

	maxKeys := opts.MaxKeys
	if maxKeys > 0 && len(result) > maxKeys {
		result = result[:maxKeys]
	}
	return result, nil
}

func (b *LocalBackend) Close() error {
	return nil
}
