package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	agentRegistryKey  = "strata:agents" // #nosec G101
	agentPrefix       = "agent:"        // #nosec G101
	tokenBlacklistKey = "strata:token:blacklist"
	tokenPrefix       = "token:"
	configCacheKey    = "strata:config:cache"
	configPrefix      = "config:"
)

type AgentInfo struct {
	AgentID    string            `json:"agent_id"`
	TenantID   string            `json:"tenant_id"`
	MSPID      string            `json:"msp_id"`
	ClientID   string            `json:"client_id"`
	DeviceID   string            `json:"device_id"`
	Host       string            `json:"host"`
	Port       int               `json:"port"`
	LastSeen   time.Time         `json:"last_seen"`
	Status     string            `json:"status"`
	OS         string            `json:"os"`
	OSVersion  string            `json:"os_version"`
	AgentVer   string            `json:"agent_version"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type TokenBlacklistEntry struct {
	TokenID   string    `json:"token_id"`
	RevokedAt time.Time `json:"revoked_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ConfigCacheEntry struct {
	ConfigKey string                 `json:"config_key"`
	Version   int                    `json:"version"`
	Payload   map[string]interface{} `json:"payload"`
	UpdatedAt time.Time              `json:"updated_at"`
}

func (c *Client) RegisterAgent(ctx context.Context, info AgentInfo) error {
	info.LastSeen = time.Now().UTC()
	if info.Metadata == nil {
		info.Metadata = make(map[string]string)
	}
	info.Metadata["registered_at"] = info.LastSeen.Format(time.RFC3339)
	info.Metadata["updated_at"] = info.LastSeen.Format(time.RFC3339)

	key := agentPrefix + info.AgentID
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal agent info: %w", err)
	}

	if err := c.rdb.HSet(ctx, agentRegistryKey, key, data).Err(); err != nil {
		return fmt.Errorf("hset agent: %w", err)
	}
	if err := c.rdb.HSet(ctx, fmt.Sprintf("strata:agents:tenant:%s", info.TenantID), info.AgentID, key).Err(); err != nil {
		return fmt.Errorf("hset tenant index: %w", err)
	}
	if err := c.rdb.SAdd(ctx, "strata:agents:msp:"+info.MSPID, info.AgentID).Err(); err != nil {
		return fmt.Errorf("sadd msp index: %w", err)
	}
	if err := c.rdb.SAdd(ctx, "strata:agents:client:"+info.ClientID, info.AgentID).Err(); err != nil {
		return fmt.Errorf("sadd client index: %w", err)
	}
	if err := c.rdb.Expire(ctx, agentRegistryKey, 7*24*time.Hour).Err(); err != nil {
		return fmt.Errorf("expire agent registry: %w", err)
	}
	return nil
}

func (c *Client) UpdateAgentHeartbeat(ctx context.Context, agentID string) error {
	now := time.Now().UTC()
	key := agentPrefix + agentID
	if err := c.rdb.HSet(ctx, agentRegistryKey,
		key+".last_seen", now.Format(time.RFC3339),
		key+".status", "online",
		key+".metadata.updated_at", now.Format(time.RFC3339),
	).Err(); err != nil {
		return fmt.Errorf("update agent heartbeat: %w", err)
	}
	return nil
}

func (c *Client) GetAgent(ctx context.Context, agentID string) (*AgentInfo, error) {
	data, err := c.rdb.HGet(ctx, agentRegistryKey, agentPrefix+agentID).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("agent %s not found", agentID)
		}
		return nil, fmt.Errorf("get agent: %w", err)
	}

	var info AgentInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		return nil, fmt.Errorf("unmarshal agent info: %w", err)
	}
	return &info, nil
}

func (c *Client) ListAgentsForTenant(ctx context.Context, tenantID string) ([]AgentInfo, error) {
	agentIDs, err := c.rdb.HKeys(ctx, fmt.Sprintf("strata:agents:tenant:%s", tenantID)).Result()
	if err != nil {
		return nil, fmt.Errorf("list tenant agents: %w", err)
	}

	if len(agentIDs) == 0 {
		return []AgentInfo{}, nil
	}

	var agents []AgentInfo
	for _, agentID := range agentIDs {
		data, err := c.rdb.HGet(ctx, agentRegistryKey, agentID).Result()
		if err != nil {
			continue
		}
		var info AgentInfo
		if err := json.Unmarshal([]byte(data), &info); err != nil {
			continue
		}
		agents = append(agents, info)
	}
	return agents, nil
}

func (c *Client) ListAgentsForMSP(ctx context.Context, mspID string) ([]AgentInfo, error) {
	agentIDs, err := c.rdb.SMembers(ctx, "strata:agents:msp:"+mspID).Result()
	if err != nil {
		return nil, fmt.Errorf("list msp agents: %w", err)
	}

	if len(agentIDs) == 0 {
		return []AgentInfo{}, nil
	}

	var agents []AgentInfo
	for _, agentID := range agentIDs {
		data, err := c.rdb.HGet(ctx, agentRegistryKey, agentPrefix+agentID).Result()
		if err != nil {
			continue
		}
		var info AgentInfo
		if err := json.Unmarshal([]byte(data), &info); err != nil {
			continue
		}
		agents = append(agents, info)
	}
	return agents, nil
}

func (c *Client) DeregisterAgent(ctx context.Context, agentID string) error {
	agentInfo, err := c.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}

	if err := c.rdb.HDel(ctx, agentRegistryKey, agentPrefix+agentID).Err(); err != nil {
		return fmt.Errorf("hdel agent: %w", err)
	}
	if agentInfo.TenantID != "" {
		if err := c.rdb.HDel(ctx, fmt.Sprintf("strata:agents:tenant:%s", agentInfo.TenantID), agentID).Err(); err != nil {
			return fmt.Errorf("hdel tenant index: %w", err)
		}
	}
	if agentInfo.MSPID != "" {
		if err := c.rdb.SRem(ctx, "strata:agents:msp:"+agentInfo.MSPID, agentID).Err(); err != nil {
			return fmt.Errorf("srem msp index: %w", err)
		}
	}
	if agentInfo.ClientID != "" {
		if err := c.rdb.SRem(ctx, "strata:agents:client:"+agentInfo.ClientID, agentID).Err(); err != nil {
			return fmt.Errorf("srem client index: %w", err)
		}
	}
	return nil
}

func (c *Client) BlacklistToken(ctx context.Context, entry TokenBlacklistEntry) error {
	now := time.Now().UTC()
	if entry.RevokedAt.IsZero() {
		entry.RevokedAt = now
	}
	if entry.ExpiresAt.IsZero() {
		entry.ExpiresAt = now.Add(24 * time.Hour)
	}

	key := tokenPrefix + entry.TokenID
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal token entry: %w", err)
	}

	ttl := time.Until(entry.ExpiresAt)
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	return c.rdb.Set(ctx, tokenBlacklistKey+":"+key, data, ttl).Err()
}

func (c *Client) IsTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	key := tokenPrefix + tokenID
	_, err := c.rdb.Get(ctx, tokenBlacklistKey+":"+key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check token blacklist: %w", err)
	}
	return true, nil
}

func (c *Client) CacheConfig(ctx context.Context, entry ConfigCacheEntry) error {
	entry.UpdatedAt = time.Now().UTC()
	if entry.Payload == nil {
		entry.Payload = make(map[string]interface{})
	}

	key := configPrefix + entry.ConfigKey
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal config cache entry: %w", err)
	}

	return c.rdb.Set(ctx, configCacheKey+":"+key, data, 15*time.Minute).Err()
}

func (c *Client) GetCachedConfig(ctx context.Context, configKey string) (*ConfigCacheEntry, error) {
	key := configPrefix + configKey
	data, err := c.rdb.Get(ctx, configCacheKey+":"+key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get config cache: %w", err)
	}

	var entry ConfigCacheEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		return nil, fmt.Errorf("unmarshal config cache: %w", err)
	}
	return &entry, nil
}

func (c *Client) InvalidateConfig(ctx context.Context, configKey string) error {
	key := configPrefix + configKey
	return c.rdb.Del(ctx, configCacheKey+":"+key).Err()
}
