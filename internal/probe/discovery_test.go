package probe

import (
	"context"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNormalizeMAC(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"00-11-22-33-44-55", "00:11:22:33:44:55"},
		{"0011.2233.4455", "00:11:22:33:44:55"},
		{"001122334455", "00:11:22:33:44:55"},
		{"aa:bb:cc:dd:ee:ff", "AA:BB:CC:DD:EE:FF"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeMAC(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeMAC(%s) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetVendorFromMAC(t *testing.T) {
	tests := []struct {
		mac      string
		expected string
	}{
		{"00:1C:73:00:00:00", "Dell"},
		{"00:50:56:00:00:00", "VMware"},
		{"00:15:5D:00:00:00", "Microsoft"},
		{"AA:BB:CC:DD:EE:FF", "unknown"}, // Unknown OUI
	}

	for _, tt := range tests {
		t.Run(tt.mac, func(t *testing.T) {
			result := getVendorFromMAC(tt.mac)
			if result != tt.expected {
				t.Errorf("getVendorFromMAC(%s) = %s; want %s", tt.mac, result, tt.expected)
			}
		})
	}
}

func TestNewDiscoveryEngine(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		ProbeID:           "test-probe",
		TenantID:          "test-tenant",
		DiscoveryEnabled:  true,
		DiscoverySubnets:  []string{"192.168.1.0/24"},
		DiscoveryInterval: 1 * time.Hour,
	}

	// Create a test engine (without NATS connection)
	de := NewDiscoveryEngine(&Probe{
		ID:       cfg.ProbeID,
		TenantID: cfg.TenantID,
		Config:   cfg,
		Logger:   logger,
	}, cfg.DiscoverySubnets)

	if de == nil {
		t.Fatal("NewDiscoveryEngine returned nil")
	}
	if de.probe == nil {
		t.Error("DiscoveryEngine.probe is nil")
	}
	if len(de.subnets) != 1 {
		t.Errorf("Expected 1 subnet, got %d", len(de.subnets))
	}
}

func TestDeduplicate(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		ProbeID:           "test-probe",
		TenantID:          "test-tenant",
		DiscoveryEnabled:  true,
		DiscoverySubnets:  []string{"192.168.1.0/24"},
		DiscoveryInterval: 1 * time.Hour,
	}

	p := &Probe{
		ID:       cfg.ProbeID,
		TenantID: cfg.TenantID,
		Config:   cfg,
		Logger:   logger,
	}

	de := NewDiscoveryEngine(p, cfg.DiscoverySubnets)

	// Test with duplicates
	devices := []DiscoveryResult{
		{IP: "192.168.1.1", MAC: "00:11:22:33:44:55"},
		{IP: "192.168.1.2", MAC: "00:11:22:33:44:56"},
		{IP: "192.168.1.1", MAC: "00:11:22:33:44:55"}, // Duplicate by IP
		{IP: "192.168.1.3", MAC: "00:11:22:33:44:55"}, // Same MAC as first - should be deduped
	}

	deduped := de.deduplicate(devices)

	// MAC deduplication means we get 2 unique devices (by MAC)
	if len(deduped) != 2 {
		t.Errorf("Expected 2 unique devices, got %d", len(deduped))
	}

	// Test deduplication by MAC
	macDevices := []DiscoveryResult{
		{IP: "192.168.1.1", MAC: "00:11:22:33:44:55"},
		{IP: "192.168.1.2", MAC: "00:11:22:33:44:55"}, // Same MAC
	}

	dedupedMac := de.deduplicate(macDevices)
	if len(dedupedMac) != 1 {
		t.Errorf("Expected 1 device after MAC deduplication, got %d", len(dedupedMac))
	}
}

func TestDetermineDeviceType(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		ProbeID:           "test-probe",
		TenantID:          "test-tenant",
		DiscoveryEnabled:  true,
		DiscoverySubnets:  []string{"192.168.1.0/24"},
		DiscoveryInterval: 1 * time.Hour,
	}

	p := &Probe{
		ID:       cfg.ProbeID,
		TenantID: cfg.TenantID,
		Config:   cfg,
		Logger:   logger,
	}

	de := NewDiscoveryEngine(p, cfg.DiscoverySubnets)

	tests := []struct {
		result   DiscoveryResult
		expected string
	}{
		{
			DiscoveryResult{IP: "192.168.1.1", Ports: []string{"22/tcp", "443/tcp"}},
			"router",
		},
		{
			DiscoveryResult{IP: "192.168.1.2", Ports: []string{"3306/tcp"}},
			"database",
		},
		{
			DiscoveryResult{IP: "192.168.1.3", Ports: []string{"5432/tcp"}},
			"database",
		},
		{
			DiscoveryResult{IP: "192.168.1.4", Ports: []string{"8080/tcp"}},
			"server",
		},
		{
			DiscoveryResult{IP: "192.168.1.5", MAC: "00:11:22:33:44:55"},
			"device",
		},
		{
			DiscoveryResult{IP: "192.168.1.6"},
			"host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.result.IP, func(t *testing.T) {
			result := de.determineDeviceType(tt.result)
			if result != tt.expected {
				t.Errorf("determineDeviceType(%v) = %s; want %s", tt.result, result, tt.expected)
			}
		})
	}
}

func TestDiscoverARPLinux(t *testing.T) {
	// Test with mock data
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		ProbeID:           "test-probe",
		TenantID:          "test-tenant",
		DiscoveryEnabled:  true,
		DiscoverySubnets:  []string{"192.168.1.0/24"},
		DiscoveryInterval: 1 * time.Hour,
	}

	p := &Probe{
		ID:       cfg.ProbeID,
		TenantID: cfg.TenantID,
		Config:   cfg,
		Logger:   logger,
	}

	de := NewDiscoveryEngine(p, cfg.DiscoverySubnets)

	// This test would require mocking the /proc/net/arp file
	// For now, just test that the function doesn't panic
	ctx := context.Background()
	results := de.discoverARPLinux(ctx)
	// Results may be empty if /proc/net/arp doesn't exist
	t.Logf("Discovered %d devices via ARP", len(results))
}

func TestDiscoverARPWindows(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		ProbeID:           "test-probe",
		TenantID:          "test-tenant",
		DiscoveryEnabled:  true,
		DiscoverySubnets:  []string{"192.168.1.0/24"},
		DiscoveryInterval: 1 * time.Hour,
	}

	p := &Probe{
		ID:       cfg.ProbeID,
		TenantID: cfg.TenantID,
		Config:   cfg,
		Logger:   logger,
	}

	de := NewDiscoveryEngine(p, cfg.DiscoverySubnets)

	ctx := context.Background()
	results := de.discoverARPWindows(ctx)
	t.Logf("Discovered %d devices via Windows ARP", len(results))
}

func TestDiscoverARPUnix(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		ProbeID:           "test-probe",
		TenantID:          "test-tenant",
		DiscoveryEnabled:  true,
		DiscoverySubnets:  []string{"192.168.1.0/24"},
		DiscoveryInterval: 1 * time.Hour,
	}

	p := &Probe{
		ID:       cfg.ProbeID,
		TenantID: cfg.TenantID,
		Config:   cfg,
		Logger:   logger,
	}

	de := NewDiscoveryEngine(p, cfg.DiscoverySubnets)

	ctx := context.Background()
	results := de.discoverARPUnix(ctx)
	t.Logf("Discovered %d devices via Unix ARP", len(results))
}

func TestNormalizeWindowsMAC(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"00-11-22-33-44-55", "00:11:22:33:44:55"},
		{"aa-bb-cc-dd-ee-ff", "AA:BB:CC:DD:EE:FF"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeWindowsMAC(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeWindowsMAC(%s) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestProtocolName(t *testing.T) {
	tests := []struct {
		proto    uint8
		expected string
	}{
		{1, "icmp"},
		{6, "tcp"},
		{17, "udp"},
		{41, "ipv6"},
		{255, "proto_255"},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.proto)), func(t *testing.T) {
			result := protocolName(tt.proto)
			if result != tt.expected {
				t.Errorf("protocolName(%d) = %s; want %s", tt.proto, result, tt.expected)
			}
		})
	}
}

func TestGetBroadcastIP(t *testing.T) {
	_, ipNet, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatal(err)
	}

	broadcast := getBroadcastIP(ipNet)
	if broadcast != "192.168.1.255" {
		t.Errorf("getBroadcastIP for 192.168.1.0/24 = %s; want 192.168.1.255", broadcast)
	}
}

func TestNewFlowCollector(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		ProbeID:           "test-probe",
		TenantID:          "test-tenant",
		FlowEnabled:       true,
		FlowPort:          2055,
		FlowProtocols:     []string{"netflow9", "ipfix"},
		DiscoveryInterval: 1 * time.Hour,
	}

	p := &Probe{
		ID:       cfg.ProbeID,
		TenantID: cfg.TenantID,
		Config:   cfg,
		Logger:   logger,
	}

	fc := NewFlowCollector(p, cfg.FlowPort, cfg.FlowProtocols)

	if fc == nil {
		t.Fatal("NewFlowCollector returned nil")
	}
	if fc.probe != p {
		t.Error("FlowCollector.probe is not set correctly")
	}
	if fc.port != 2055 {
		t.Errorf("Expected port 2055, got %d", fc.port)
	}
}

func TestNewScanner(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		ProbeID:           "test-probe",
		TenantID:          "test-tenant",
		DiscoveryInterval: 1 * time.Hour,
	}

	p := &Probe{
		ID:       cfg.ProbeID,
		TenantID: cfg.TenantID,
		Config:   cfg,
		Logger:   logger,
	}

	s := NewScanner(p)

	if s == nil {
		t.Fatal("NewScanner returned nil")
	}
	if s.probe != p {
		t.Error("Scanner.probe is not set correctly")
	}
	if s.timeout != 2*time.Second {
		t.Errorf("Expected timeout 2s, got %v", s.timeout)
	}
}
