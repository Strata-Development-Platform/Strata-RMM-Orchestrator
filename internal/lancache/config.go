package lancache

import (
	"fmt"
	"net"
	"time"
)

// CacheMode represents the LAN cache operation mode.
type CacheMode string

const (
	CacheModeStandard   CacheMode = "standard"
	CacheModePeerToPeer CacheMode = "peer_to_peer"
	CacheModeHybrid     CacheMode = "hybrid"
)

// PackageType represents the type of package being cached.
type PackageType string

const (
	PackageChocolatey PackageType = "chocolatey"
	PackageWinget     PackageType = "winget"
	PackageFlatpak    PackageType = "flatpak"
	PackageSnap       PackageType = "snap"
	PackageMSI        PackageType = "msi"
	PackageDEB        PackageType = "deb"
	PackageRPM        PackageType = "rpm"
)

// CacheStatus represents the status of a cache entry.
type CacheStatus string

const (
	CacheStatusAvailable   CacheStatus = "available"
	CacheStatusDownloading CacheStatus = "downloading"
	CacheStatusCorrupted   CacheStatus = "corrupted"
	CacheStatusExpired     CacheStatus = "expired"
	CacheStatusEvicting    CacheStatus = "evicting"
)

// CacheConfig holds LAN cache server configuration.
type CacheConfig struct {
	Enabled         bool          `json:"enabled"`
	CacheMode       CacheMode     `json:"cache_mode"`
	ListenAddr      string        `json:"listen_addr"`
	ListenPort      int           `json:"listen_port"`
	MaxCacheSizeGB  int           `json:"max_cache_size_gb"`
	EntryTTL        time.Duration `json:"entry_ttl"`
	CleanupInterval time.Duration `json:"cleanup_interval"`
	P2PEnabled      bool          `json:"p2p_enabled"`
	P2PPeerLimit    int           `json:"p2p_peer_limit"`
	ProxyEnabled    bool          `json:"proxy_enabled"`
	UpstreamProxy   string        `json:"upstream_proxy"`
	AllowedNetworks []string      `json:"allowed_networks"`
}

// CacheEntry represents a cached package.
type CacheEntry struct {
	ID            string      `json:"id"`
	TenantID      string      `json:"tenant_id"`
	PackageName   string      `json:"package_name"`
	PackageType   PackageType `json:"package_type"`
	Version       string      `json:"version"`
	Checksum      string      `json:"checksum"`
	SizeBytes     int64       `json:"size_bytes"`
	Status        CacheStatus `json:"status"`
	DownloadCount int         `json:"download_count"`
	CreatedAt     time.Time   `json:"created_at"`
	LastAccessed  time.Time   `json:"last_accessed"`
	ExpiresAt     *time.Time  `json:"expires_at,omitempty"`
	SourceURL     string      `json:"source_url,omitempty"`
	DownloadURL   string      `json:"download_url,omitempty"`
	PeerHosts     []string    `json:"peer_hosts,omitempty"`
}

// CreateCacheRequest is the request body for creating a cache entry.
type CreateCacheRequest struct {
	TenantID    string      `json:"tenant_id"`
	PackageName string      `json:"package_name"`
	PackageType PackageType `json:"package_type"`
	Version     string      `json:"version"`
	SourceURL   string      `json:"source_url"`
	Checksum    string      `json:"checksum"`
	SizeBytes   int64       `json:"size_bytes"`
}

// StartCacheRequest is the request body for starting the cache server.
type StartCacheRequest struct {
	CacheMode    CacheMode `json:"cache_mode"`
	ListenAddr   string    `json:"listen_addr"`
	ListenPort   int       `json:"listen_port"`
	MaxCacheSize int       `json:"max_cache_size"`
	P2PEnabled   bool      `json:"p2p_enabled"`
}

// Validate checks the CreateCacheRequest for required fields.
func (r *CreateCacheRequest) Validate() error {
	if r.TenantID == "" {
		return fmt.Errorf("missing tenant_id")
	}
	if r.PackageName == "" {
		return fmt.Errorf("missing package_name")
	}
	if r.PackageType == "" {
		return fmt.Errorf("missing package_type")
	}
	if r.Version == "" {
		return fmt.Errorf("missing version")
	}
	return nil
}

// Validate checks the StartCacheRequest for required fields.
func (r *StartCacheRequest) Validate() error {
	if r.CacheMode == "" {
		return fmt.Errorf("missing cache_mode")
	}
	if r.CacheMode != CacheModeStandard && r.CacheMode != CacheModePeerToPeer && r.CacheMode != CacheModeHybrid {
		return fmt.Errorf("invalid cache_mode: %s", r.CacheMode)
	}
	if r.ListenAddr == "" {
		return fmt.Errorf("missing listen_addr")
	}
	if r.ListenPort <= 0 || r.ListenPort > 65535 {
		return fmt.Errorf("invalid listen_port: %d", r.ListenPort)
	}
	if r.MaxCacheSize <= 0 {
		return fmt.Errorf("missing max_cache_size")
	}
	return nil
}

// Validate checks the CacheConfig for valid configuration.
func (c *CacheConfig) Validate() error {
	if c.CacheMode == "" {
		return fmt.Errorf("missing cache_mode")
	}
	if c.CacheMode != CacheModeStandard && c.CacheMode != CacheModePeerToPeer && c.CacheMode != CacheModeHybrid {
		return fmt.Errorf("invalid cache_mode: %s", c.CacheMode)
	}
	if c.ListenAddr == "" {
		return fmt.Errorf("missing listen_addr")
	}
	if c.ListenPort <= 0 || c.ListenPort > 65535 {
		return fmt.Errorf("invalid listen_port: %d", c.ListenPort)
	}
	if c.MaxCacheSizeGB <= 0 {
		return fmt.Errorf("missing max_cache_size_gb")
	}
	if c.EntryTTL == 0 {
		return fmt.Errorf("missing entry_ttl")
	}
	if c.ProxyEnabled && c.UpstreamProxy != "" {
		if _, err := net.ResolveTCPAddr("tcp", c.UpstreamProxy); err != nil {
			return fmt.Errorf("invalid upstream_proxy: %s", c.UpstreamProxy)
		}
	}
	return nil
}

// DefaultCacheConfig returns the default LAN cache configuration.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled:         true,
		CacheMode:       CacheModeHybrid,
		ListenAddr:      "0.0.0.0",
		ListenPort:      3128,
		MaxCacheSizeGB:  50,
		EntryTTL:        30 * 24 * time.Hour,
		CleanupInterval: time.Hour,
		P2PEnabled:      true,
		P2PPeerLimit:    10,
		ProxyEnabled:    true,
		AllowedNetworks: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
	}
}
