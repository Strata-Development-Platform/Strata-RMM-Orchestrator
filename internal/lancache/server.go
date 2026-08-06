package lancache

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// Server manages the LAN cache for package distribution.
type Server struct {
	mu       sync.RWMutex
	cache    map[string]*CacheEntry
	config   CacheConfig
	logger   *zap.Logger
	nats     *nats.Conn
	stopped  chan struct{}
}

// NewServer creates a new LAN cache server.
func NewServer(config CacheConfig, logger *zap.Logger, nc *nats.Conn) *Server {
	return &Server{
		cache:    make(map[string]*CacheEntry),
		config:   config,
		logger:   logger,
		nats:     nc,
		stopped:  make(chan struct{}),
	}
}

// Start starts the LAN cache server.
func (s *Server) Start() error {
	if err := s.config.Validate(); err != nil {
		return fmt.Errorf("invalid cache config: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("starting LAN cache server",
			zap.String("mode", string(s.config.CacheMode)),
			zap.String("addr", s.config.ListenAddr),
			zap.Int("port", s.config.ListenPort),
			zap.Int("max_cache_size_gb", s.config.MaxCacheSizeGB),
			zap.Bool("p2p_enabled", s.config.P2PEnabled),
		)
	}

	// Start periodic cleanup goroutine
	go s.cleanupLoop()

	return nil
}

// Stop stops the LAN cache server.
func (s *Server) Stop() {
	close(s.stopped)
	if s.logger != nil {
		s.logger.Info("stopping LAN cache server")
	}
}

// AddCacheEntry adds a package to the cache.
func (s *Server) AddCacheEntry(req *CreateCacheRequest) (*CacheEntry, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	entry := &CacheEntry{
		ID:            fmt.Sprintf("cache-%s", uuid.New().String()[:8]),
		TenantID:      req.TenantID,
		PackageName:   req.PackageName,
		PackageType:   req.PackageType,
		Version:       req.Version,
		Checksum:      req.Checksum,
		SizeBytes:     req.SizeBytes,
		Status:        CacheStatusAvailable,
		DownloadCount: 0,
		CreatedAt:     time.Now(),
		LastAccessed:  time.Now(),
		SourceURL:     req.SourceURL,
	}

	// Set expiration
	if s.config.EntryTTL > 0 {
		expiresAt := time.Now().Add(s.config.EntryTTL)
		entry.ExpiresAt = &expiresAt
	}

	s.mu.Lock()
	s.cache[entry.ID] = entry
	s.mu.Unlock()

	if s.logger != nil {
		s.logger.Info("added cache entry",
			zap.String("entry_id", entry.ID),
			zap.String("package", entry.PackageName),
			zap.String("version", entry.Version),
			zap.Int64("size_bytes", entry.SizeBytes),
		)
	}

	// Publish cache event via NATS
	if s.nats != nil {
		subject := fmt.Sprintf("tenant.%s.lancache.entry.added", req.TenantID)
		data, _ := json.Marshal(map[string]interface{}{
			"entry_id":     entry.ID,
			"package_name": entry.PackageName,
			"version":      entry.Version,
			"size_bytes":   entry.SizeBytes,
			"timestamp":    time.Now().Format(time.RFC3339),
		})
		if err := s.nats.Publish(subject, data); err != nil {
			s.logger.Warn("failed to publish cache event", zap.Error(err))
		}
	}

	return entry, nil
}

// GetCacheEntry retrieves a cache entry by ID.
func (s *Server) GetCacheEntry(entryID string) (*CacheEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.cache[entryID]
	if !ok {
		return nil, fmt.Errorf("cache entry not found: %s", entryID)
	}
	entry.LastAccessed = time.Now()
	return entry, nil
}

// FindCacheEntry finds a cached package by name and version.
func (s *Server) FindCacheEntry(tenantID, packageName, version string) *CacheEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, entry := range s.cache {
		if entry.TenantID == tenantID && entry.PackageName == packageName && entry.Version == version {
			if entry.Status == CacheStatusAvailable && !entry.isExpired() {
				entry.LastAccessed = time.Now()
				entry.DownloadCount++
				return entry
			}
		}
	}
	return nil
}

// ListCacheEntries returns all cache entries for a tenant.
func (s *Server) ListCacheEntries(tenantID string) []*CacheEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var entries []*CacheEntry
	for _, entry := range s.cache {
		if entry.TenantID == tenantID {
			entries = append(entries, entry)
		}
	}
	return entries
}

// EvictCacheEntry removes a cache entry.
func (s *Server) EvictCacheEntry(entryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.cache[entryID]
	if !ok {
		return fmt.Errorf("cache entry not found: %s", entryID)
	}
	entry.Status = CacheStatusEvicting
	delete(s.cache, entryID)
	if s.logger != nil {
		s.logger.Info("evicted cache entry", zap.String("entry_id", entryID))
	}
	return nil
}

// cleanupLoop periodically cleans up expired cache entries.
func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopped:
			return
		}
	}
}

// cleanup removes expired and corrupted cache entries.
func (s *Server) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, entry := range s.cache {
		if entry.Status == CacheStatusExpired || entry.Status == CacheStatusCorrupted || entry.isExpired() {
			if s.logger != nil {
				s.logger.Debug("cleaning up expired entry",
					zap.String("entry_id", id),
					zap.String("package", entry.PackageName),
				)
			}
			delete(s.cache, id)
		}
	}
}

// isExpired checks if a cache entry has expired.
func (e *CacheEntry) isExpired() bool {
	if e.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*e.ExpiresAt)
}

// GetStats returns cache statistics.
func (s *Server) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalSize := int64(0)
	totalDownloads := 0
	statusCounts := map[string]int{}

	for _, entry := range s.cache {
		totalSize += entry.SizeBytes
		totalDownloads += entry.DownloadCount
		statusCounts[string(entry.Status)]++
	}

	return map[string]interface{}{
		"total_entries":    len(s.cache),
		"total_size_bytes": totalSize,
		"total_downloads":  totalDownloads,
		"status_counts":    statusCounts,
		"cache_mode":       string(s.config.CacheMode),
		"max_cache_size":   fmt.Sprintf("%d GB", s.config.MaxCacheSizeGB),
	}
}

// HandleServeHTTP handles HTTP requests for cached packages.
func (s *Server) HandleServeHTTP(w http.ResponseWriter, r *http.Request) {
	entryID := r.PathValue("entryID")
	if entryID == "" {
		http.Error(w, `{"error":"missing entry ID"}`, http.StatusBadRequest)
		return
	}

	entry, err := s.GetCacheEntry(entryID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}

	if entry.Status != CacheStatusAvailable {
		http.Error(w, `{"error":"entry not available"}`, http.StatusServiceUnavailable)
		return
	}

	// In production, this would stream the file from storage
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", entry.SizeBytes))
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "serving",
		"entry_id":     entryID,
		"package":      entry.PackageName,
		"version":      entry.Version,
		"size_bytes":   entry.SizeBytes,
		"download_url": entry.DownloadURL,
	}); err != nil {
		s.logger.Error("failed to encode response", zap.Error(err))
	}
}

// HandleCacheStats handles HTTP request for cache statistics.
func (s *Server) HandleCacheStats(w http.ResponseWriter, r *http.Request) {
	stats := s.GetStats()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		s.logger.Error("failed to encode stats", zap.Error(err))
	}
}
