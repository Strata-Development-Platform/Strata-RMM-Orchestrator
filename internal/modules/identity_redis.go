package modules

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	strataredis "github.com/strata-rmm/strata-rmm-orchestrator/pkg/redis"
)

const moduleRevocationKeyPrefix = "rmm:modules:revoked:" // #nosec G101 -- Redis key prefix, not a credential

type RedisRevocationStore struct {
	rdb *goredis.Client
}

func NewRedisRevocationStore(client *strataredis.Client) (*RedisRevocationStore, error) {
	if client == nil || client.RDB() == nil {
		return nil, errors.New("module revocation Redis client is required")
	}
	return &RedisRevocationStore{rdb: client.RDB()}, nil
}

func (s *RedisRevocationStore) Revoke(ctx context.Context, tokenID string, expiresAt time.Time) error {
	if tokenID == "" {
		return errors.New("module service token id is required")
	}
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil
	}
	if err := s.rdb.Set(ctx, moduleRevocationKeyPrefix+tokenID, "1", ttl).Err(); err != nil {
		return fmt.Errorf("store module token revocation: %w", err)
	}
	return nil
}

func (s *RedisRevocationStore) IsRevoked(ctx context.Context, tokenID string) (bool, error) {
	if tokenID == "" {
		return false, errors.New("module service token id is required")
	}
	_, err := s.rdb.Get(ctx, moduleRevocationKeyPrefix+tokenID).Result()
	if err == nil {
		return true, nil
	}
	if errors.Is(err, goredis.Nil) {
		return false, nil
	}
	return false, fmt.Errorf("read module token revocation: %w", err)
}
