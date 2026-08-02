package update

import (
	"encoding/json"
	"fmt"
	"time"
)

type Channel string

const (
	ChannelStable Channel = "stable"
	ChannelBeta   Channel = "beta"
	ChannelAlpha  Channel = "alpha"
)

type Manifest struct {
	Version         string                      `json:"version"`
	ReleaseDate     time.Time                   `json:"release_date"`
	Channel         Channel                     `json:"channel"`
	MinAgentVersion string                      `json:"min_agent_version"`
	RollbackVersion string                      `json:"rollback_version,omitempty"`
	Platforms       map[string]PlatformArtifact `json:"platforms"`
	Changelog       string                      `json:"changelog"`
	Signature       CosignSignature             `json:"signature"`
}

type PlatformArtifact struct {
	URL          string `json:"url"`
	Checksum     string `json:"checksum"`
	ChecksumType string `json:"checksum_type"`
	Size         int64  `json:"size"`
}

type CosignSignature struct {
	BundleURL string `json:"bundle_url"`
	CertURL   string `json:"cert_url,omitempty"`
	Algorithm string `json:"algorithm"`
}

type UpdateState struct {
	CurrentVersion  string    `json:"current_version"`
	PendingVersion  string    `json:"pending_version,omitempty"`
	RollbackVersion string    `json:"rollback_version,omitempty"`
	LastCheck       time.Time `json:"last_check"`
	Status          Status    `json:"status"`
	FailedAttempts  int       `json:"failed_attempts"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Status string

const (
	StatusUpToDate    Status = "up_to_date"
	StatusPending     Status = "pending"
	StatusDownloading Status = "downloading"
	StatusReady       Status = "ready"
	StatusApplying    Status = "applying"
	StatusFailed      Status = "failed"
	StatusRolledBack  Status = "rolled_back"
)

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Version == "" {
		return nil, fmt.Errorf("manifest missing version")
	}
	if len(m.Platforms) == 0 {
		return nil, fmt.Errorf("manifest missing platforms")
	}
	return &m, nil
}

func (m *Manifest) ArtifactFor(goos, goarch string) (*PlatformArtifact, error) {
	key := fmt.Sprintf("%s/%s", goos, goarch)
	altKey := fmt.Sprintf("%s/%s", goos, goarch)
	if a, ok := m.Platforms[key]; ok {
		return &a, nil
	}
	if a, ok := m.Platforms[altKey]; ok {
		return &a, nil
	}
	for k, a := range m.Platforms {
		if k == goos || k == fmt.Sprintf("%s/%s", goos, goarch) {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("no artifact for %s/%s", goos, goarch)
}
