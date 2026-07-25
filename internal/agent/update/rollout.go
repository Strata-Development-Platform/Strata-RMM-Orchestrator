package update

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

type RolloutConfig struct {
	Channel         Channel  `json:"channel"`
	RolloutPercent  int      `json:"rollout_percent"`
	Paused          bool     `json:"paused"`
	ApprovedVersion string   `json:"approved_version,omitempty"`
	BlockedVersions []string `json:"blocked_versions,omitempty"`
}

type RolloutCommand struct {
	Action  string          `json:"action"`
	Config  *RolloutConfig  `json:"config,omitempty"`
	Version string          `json:"version,omitempty"`
}

type RolloutManager struct {
	nc       *nats.Conn
	store    *Store
	client   *Client
	config   RolloutConfig
	mu       sync.RWMutex
	agentID  string
	tenantID string
}

func NewRolloutManager(nc *nats.Conn, store *Store, client *Client, agentID, tenantID string) *RolloutManager {
	return &RolloutManager{
		nc:       nc,
		store:    store,
		client:   client,
		agentID:  agentID,
		tenantID: tenantID,
		config: RolloutConfig{
			Channel:        ChannelStable,
			RolloutPercent: 100,
		},
	}
}

func (r *RolloutManager) SubscribeCommands(ctx context.Context) error {
	subject := fmt.Sprintf("tenant.%s.rollout.%s", r.tenantID, r.agentID)
	sub, err := r.nc.Subscribe(subject, func(msg *nats.Msg) {
		var cmd RolloutCommand
		if err := json.Unmarshal(msg.Data, &cmd); err != nil {
			return
		}

		switch cmd.Action {
		case "set_config":
			if cmd.Config != nil {
				r.mu.Lock()
				r.config = *cmd.Config
				r.mu.Unlock()
			}
		case "update":
			r.triggerUpdate(cmd.Version)
		case "rollback":
			if err := r.client.Rollback(); err != nil {
				r.publishStatus("rollback_failed", err.Error())
			} else {
				r.publishStatus("rollback_complete", "")
			}
		case "pause":
			r.mu.Lock()
			r.config.Paused = true
			r.mu.Unlock()
		case "resume":
			r.mu.Lock()
			r.config.Paused = false
			r.mu.Unlock()
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe rollout: %w", err)
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
	}()

	return nil
}

func (r *RolloutManager) ShouldApply(version string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.config.Paused {
		return false
	}

	for _, blocked := range r.config.BlockedVersions {
		if blocked == version {
			return false
		}
	}

	if r.config.ApprovedVersion != "" && r.config.ApprovedVersion != version {
		return false
	}

	if r.config.RolloutPercent < 100 {
		h := hashAgentID(r.agentID)
		percent := int(h % 100)
		return percent < r.config.RolloutPercent
	}

	return true
}

func (r *RolloutManager) triggerUpdate(version string) {
	if !r.ShouldApply(version) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	manifest := &Manifest{
		Version: version,
	}

	binaryPath, err := r.client.Download(ctx, manifest)
	if err != nil {
		r.publishStatus("download_failed", err.Error())
		return
	}

	r.publishStatus("downloaded", version)

	if err := r.client.Apply(binaryPath); err != nil {
		r.publishStatus("apply_failed", err.Error())
		return
	}

	r.publishStatus("applied", version)

	if err := r.client.VerifyAndSwitch(); err != nil {
		r.publishStatus("verify_failed", err.Error())

		if rbErr := r.client.Rollback(); rbErr != nil {
			r.publishStatus("rollback_failed", rbErr.Error())
		} else {
			r.publishStatus("rollback_complete", r.client.CurrentVersion())
		}
		return
	}

	r.publishStatus("update_complete", version)
}

func (r *RolloutManager) publishStatus(status, detail string) {
	subject := fmt.Sprintf("tenant.%s.rollout.status.%s", r.tenantID, r.agentID)
	data, _ := json.Marshal(map[string]string{
		"agent_id":  r.agentID,
		"status":    status,
		"detail":    detail,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	r.nc.Publish(subject, data)
}

func hashAgentID(agentID string) uint64 {
	h := sha256.Sum256([]byte(agentID))
	return binary.BigEndian.Uint64(h[:8])
}
