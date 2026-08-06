package lancache

import (
	"testing"
	"time"
)

func TestCreateCacheRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateCacheRequest
		wantErr bool
	}{
		{
			name:    "valid request",
			req:     CreateCacheRequest{TenantID: "t1", PackageName: "chrome", PackageType: "chocolatey", Version: "110.0"},
			wantErr: false,
		},
		{
			name:    "missing tenant_id",
			req:     CreateCacheRequest{PackageName: "chrome"},
			wantErr: true,
		},
		{
			name:    "missing package_name",
			req:     CreateCacheRequest{TenantID: "t1"},
			wantErr: true,
		},
		{
			name:    "missing package_type",
			req:     CreateCacheRequest{TenantID: "t1", PackageName: "chrome"},
			wantErr: true,
		},
		{
			name:    "missing version",
			req:     CreateCacheRequest{TenantID: "t1", PackageName: "chrome", PackageType: "chocolatey"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStartCacheRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     StartCacheRequest
		wantErr bool
	}{
		{
			name:    "valid request",
			req:     StartCacheRequest{CacheMode: "standard", ListenAddr: "0.0.0.0", ListenPort: 3128, MaxCacheSize: 50},
			wantErr: false,
		},
		{
			name:    "missing cache_mode",
			req:     StartCacheRequest{ListenAddr: "0.0.0.0"},
			wantErr: true,
		},
		{
			name:    "invalid cache_mode",
			req:     StartCacheRequest{CacheMode: "invalid"},
			wantErr: true,
		},
		{
			name:    "missing listen_addr",
			req:     StartCacheRequest{CacheMode: "standard"},
			wantErr: true,
		},
		{
			name:    "invalid listen_port",
			req:     StartCacheRequest{CacheMode: "standard", ListenAddr: "0.0.0.0", ListenPort: 0},
			wantErr: true,
		},
		{
			name:    "missing max_cache_size",
			req:     StartCacheRequest{CacheMode: "standard", ListenAddr: "0.0.0.0", ListenPort: 3128},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCacheConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  CacheConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: CacheConfig{
				CacheMode: CacheModeStandard, ListenAddr: "0.0.0.0", ListenPort: 3128,
				MaxCacheSizeGB: 50, EntryTTL: 24 * time.Hour,
			},
			wantErr: false,
		},
		{
			name:    "missing cache_mode",
			config:  CacheConfig{ListenAddr: "0.0.0.0"},
			wantErr: true,
		},
		{
			name:    "invalid cache_mode",
			config:  CacheConfig{CacheMode: "invalid", ListenAddr: "0.0.0.0"},
			wantErr: true,
		},
		{
			name:    "missing listen_addr",
			config:  CacheConfig{CacheMode: CacheModeStandard},
			wantErr: true,
		},
		{
			name:    "invalid listen_port",
			config:  CacheConfig{CacheMode: CacheModeStandard, ListenAddr: "0.0.0.0", ListenPort: 0},
			wantErr: true,
		},
		{
			name:    "missing max_cache_size_gb",
			config:  CacheConfig{CacheMode: CacheModeStandard, ListenAddr: "0.0.0.0", ListenPort: 3128},
			wantErr: true,
		},
		{
			name:    "missing entry_ttl",
			config:  CacheConfig{CacheMode: CacheModeStandard, ListenAddr: "0.0.0.0", ListenPort: 3128, MaxCacheSizeGB: 50},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()
	if config.CacheMode != CacheModeHybrid {
		t.Fatalf("expected hybrid mode, got %s", config.CacheMode)
	}
	if config.ListenPort != 3128 {
		t.Fatalf("expected port 3128, got %d", config.ListenPort)
	}
	if config.MaxCacheSizeGB != 50 {
		t.Fatalf("expected 50 GB, got %d", config.MaxCacheSizeGB)
	}
	if config.EntryTTL != 30*24*time.Hour {
		t.Fatalf("expected 30 days TTL, got %v", config.EntryTTL)
	}
	if !config.P2PEnabled {
		t.Fatal("expected P2P enabled")
	}
}

func TestCacheEntryExpired(t *testing.T) {
	now := time.Now()
	entry := &CacheEntry{
		CreatedAt: now,
		ExpiresAt: &now,
	}
	if !entry.isExpired() {
		t.Fatal("expected entry to be expired")
	}

	entry2 := &CacheEntry{
		CreatedAt: now,
		ExpiresAt: nil,
	}
	if entry2.isExpired() {
		t.Fatal("expected entry without expiration to not be expired")
	}
}

func TestCacheEntryNotExpired(t *testing.T) {
	now := time.Now()
	future := now.Add(24 * time.Hour)
	entry := &CacheEntry{
		CreatedAt: now,
		ExpiresAt: &future,
	}
	if entry.isExpired() {
		t.Fatal("expected entry not to be expired")
	}
}

func TestPackageTypeConstants(t *testing.T) {
	types := []PackageType{
		PackageChocolatey, PackageWinget, PackageFlatpak, PackageSnap,
		PackageMSI, PackageDEB, PackageRPM,
	}
	for _, pt := range types {
		pt := pt
		t.Run(string(pt), func(t *testing.T) {
			if string(pt) == "" {
				t.Fatal("expected non-empty package type")
			}
		})
	}
}

func TestCacheModeConstants(t *testing.T) {
	modes := []CacheMode{CacheModeStandard, CacheModePeerToPeer, CacheModeHybrid}
	for _, m := range modes {
		m := m
		t.Run(string(m), func(t *testing.T) {
			if string(m) == "" {
				t.Fatal("expected non-empty cache mode")
			}
		})
	}
}

func TestCacheStatusConstants(t *testing.T) {
	statuses := []CacheStatus{
		CacheStatusAvailable, CacheStatusDownloading, CacheStatusCorrupted,
		CacheStatusExpired, CacheStatusEvicting,
	}
	for _, s := range statuses {
		s := s
		t.Run(string(s), func(t *testing.T) {
			if string(s) == "" {
				t.Fatal("expected non-empty cache status")
			}
		})
	}
}

func TestCacheEntryFields(t *testing.T) {
	now := time.Now()
	entry := &CacheEntry{
		ID:            "cache-abc123",
		TenantID:      "tenant-1",
		PackageName:   "chrome",
		PackageType:   "chocolatey",
		Version:       "110.0",
		Checksum:      "sha256:abc123",
		SizeBytes:     104857600,
		Status:        CacheStatusAvailable,
		DownloadCount: 5,
		CreatedAt:     now,
		LastAccessed:  now,
		PeerHosts:     []string{"10.0.0.1", "10.0.0.2"},
	}

	if entry.ID != "cache-abc123" {
		t.Fatalf("expected ID cache-abc123, got %s", entry.ID)
	}
	if entry.SizeBytes != 104857600 {
		t.Fatalf("expected size 104857600, got %d", entry.SizeBytes)
	}
	if len(entry.PeerHosts) != 2 {
		t.Fatalf("expected 2 peer hosts, got %d", len(entry.PeerHosts))
	}
}
