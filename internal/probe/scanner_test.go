package probe

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestScanPorts(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test scanning localhost on a range of ports
	ports := []int{22, 80, 443, 8080}
	results, err := s.ScanPorts(ctx, "127.0.0.1", ports, "tcp")

	if err != nil {
		t.Logf("ScanPorts error (may be expected if services not running): %v", err)
	}

	t.Logf("Scanned %d ports, found %d results", len(ports), len(results))
}

func TestCheckPort(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Test with a closed port (should return closed state)
	result := s.checkPort(ctx, "127.0.0.1", 59999, "tcp")

	if result == nil {
		t.Fatal("checkPort returned nil")
	}

	if result.Port != 59999 {
		t.Errorf("Expected port 59999, got %d", result.Port)
	}

	if result.Protocol != "tcp" {
		t.Errorf("Expected protocol tcp, got %s", result.Protocol)
	}

	// State should be "closed" for a non-listening port
	t.Logf("Port 59999 state: %s", result.State)
}

func TestDetectService(t *testing.T) {
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

	tests := []struct {
		port     int
		protocol string
	}{
		{21, "tcp"}, // FTP
		{22, "tcp"}, // SSH
		{23, "tcp"}, // Telnet
		{25, "tcp"}, // SMTP
		{53, "udp"}, // DNS
		{80, "tcp"}, // HTTP
		{443, "tcp"}, // HTTPS
		{3306, "tcp"}, // MySQL
		{5432, "tcp"}, // PostgreSQL
		{6379, "tcp"}, // Redis
		{8080, "tcp"}, // HTTP Proxy
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.port)), func(t *testing.T) {
			service, version := s.detectService(context.Background(), nil, tt.port, tt.protocol)
			t.Logf("Port %d (%s): service=%s, version=%s", tt.port, tt.protocol, service, version)
		})
	}
}

func TestTrySSHVersion(t *testing.T) {
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

	// Test with nil connection (should return empty string)
	version := s.trySSHVersion(nil)
	if version != "" {
		t.Errorf("Expected empty string for nil conn, got %s", version)
	}
}

func TestTryDNSVersion(t *testing.T) {
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

	// Test with nil connection (should return empty string)
	version := s.tryDNSVersion(context.Background(), nil)
	if version != "" {
		t.Errorf("Expected empty string for nil conn, got %s", version)
	}
}

func TestTryHTTPVersion(t *testing.T) {
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

	// Test with nil connection (should return empty string)
	version := s.tryHTTPVersion(context.Background(), nil)
	if version != "" {
		t.Errorf("Expected empty string for nil conn, got %s", version)
	}
}

func TestHexToIP(t *testing.T) {
	tests := []struct {
		hex      string
		expected string
	}{
		{"C0A80101", "192.168.1.1"},     // 192.168.1.1
		{"0A000001", "10.0.0.1"},        // 10.0.0.1
		{"7F000001", "127.0.0.1"},       // 127.0.0.1
		{"FFFFFFFF", "255.255.255.255"}, // Broadcast
	}

	for _, tt := range tests {
		t.Run(tt.hex, func(t *testing.T) {
			ip, err := hexToIP(tt.hex)
			if err != nil {
				t.Errorf("hexToIP(%s) returned error: %v", tt.hex, err)
				return
			}
			if ip != tt.expected {
				t.Errorf("hexToIP(%s) = %s; want %s", tt.hex, ip, tt.expected)
			}
		})
	}
}

func TestGetMACAddress(t *testing.T) {
	ctx := context.Background()

	// Test with localhost
	mac, err := getMACAddress(ctx, "127.0.0.1")
	if err != nil {
		t.Logf("getMACAddress for 127.0.0.1: %v (may be expected)", err)
	} else {
		t.Logf("MAC for 127.0.0.1: %s", mac)
	}

	// Test with non-existent IP (should fail)
	mac, err = getMACAddress(ctx, "192.168.999.999")
	if err == nil {
		t.Logf("Got MAC for invalid IP: %s", mac)
	}
}

func TestDiscoverTopology(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := s.DiscoverTopology(ctx, "192.168.1.0/24")
	if err != nil {
		t.Logf("DiscoverTopology error (may be expected on this platform): %v", err)
	}

	if info != nil {
		t.Logf("Discovered %d interfaces", len(info.Interfaces))
		for i, iface := range info.Interfaces {
			t.Logf("  Interface %d: %s (%s), %d addresses",
				i, iface.Name, iface.MAC, len(iface.Addresses))
		}
	}
}

func TestGetGateways(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gateways, err := s.GetGateways(ctx)
	if err != nil {
		t.Logf("GetGateways error (may be expected on this platform): %v", err)
	} else {
		t.Logf("Discovered %d gateways", len(gateways))
		for i, gw := range gateways {
			t.Logf("  Gateway %d: %s (iface: %s, default: %v)",
				i, gw.IP, gw.Interface, gw.IsDefault)
		}
	}
}

func TestPingScan(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test pinging localhost
	hosts, err := s.PingScan(ctx, "127.0.0.1/32", 1*time.Second)
	if err != nil {
		t.Logf("PingScan error: %v", err)
	}

	t.Logf("PingScan found %d hosts", len(hosts))
}

func TestPingHost(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test with non-routable IP
	live := s.pingHost(ctx, "192.0.2.1", 1*time.Second)
	if live {
		t.Logf("192.0.2.1 is live (may be routable in some networks)")
	} else {
		t.Logf("192.0.2.1 is not live (expected)")
	}

	// Test with localhost
	live = s.pingHost(ctx, "127.0.0.1", 1*time.Second)
	t.Logf("127.0.0.1 is live: %v", live)
}

func TestGetHostInfo(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test with localhost
	info, err := s.getHostInfo(ctx, "127.0.0.1", 2*time.Second)
	if err != nil {
		t.Logf("getHostInfo error: %v", err)
	}

	t.Logf("Host info for 127.0.0.1:")
	t.Logf("  IP: %s", info.IP)
	t.Logf("  Hostname: %s", info.Hostname)
	t.Logf("  MAC: %s", info.MAC)
	t.Logf("  Open ports: %d", len(info.OpenPorts))
	t.Logf("  Services: %d", len(info.Services))
}

func TestRunFullDiscovery(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test full discovery on a small subnet
	hosts, err := s.RunFullDiscovery(ctx, "127.0.0.0/30")
	if err != nil {
		t.Logf("RunFullDiscovery error: %v", err)
	}

	t.Logf("Full discovery found %d hosts", len(hosts))
	for i, host := range hosts {
		t.Logf("  Host %d: %s (%s)", i, host.IP, host.Hostname)
		t.Logf("    MAC: %s", host.MAC)
		t.Logf("    Open ports: %d", len(host.OpenPorts))
	}
}

func TestParseARPLinux(t *testing.T) {
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

	ctx := context.Background()
	results, err := s.parseARPLinux(ctx)
	if err != nil {
		t.Logf("parseARPLinux error: %v", err)
	}

	t.Logf("Parsed %d ARP entries", len(results))
	for i, r := range results {
		t.Logf("  ARP %d: %s -> %s (%s)", i, r.IP, r.MAC, r.Hostname)
	}
}

func TestScanTCPSYN(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ports := []int{22, 80, 443, 8080, 59999}
	results, err := s.scanTCPSYN(ctx, "127.0.0.1", ports)

	if err != nil {
		t.Logf("scanTCPSYN error: %v", err)
	}

	t.Logf("TCP SYN scan results:")
	for _, r := range results {
		t.Logf("  Port %d: %s (%s)", r.Port, r.State, r.Service)
	}
}

func TestScanUDP(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ports := []int{53, 123, 161}
	results, err := s.scanUDP(ctx, "127.0.0.1", ports)

	if err != nil {
		t.Logf("scanUDP error: %v", err)
	}

	t.Logf("UDP scan results:")
	for _, r := range results {
		t.Logf("  Port %d: %s (%s)", r.Port, r.State, r.Service)
	}
}

func TestScanService(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test service detection on localhost
	svc, err := s.ScanService(ctx, "127.0.0.1", 22)
	if err != nil {
		t.Logf("ScanService error (may be expected if nmap not available): %v", err)
	}

	if svc != nil {
		t.Logf("Service on port 22: %s %s", svc.Name, svc.Version)
	}
}

func TestPublishDiscovery(t *testing.T) {
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

	de := NewDiscoveryEngine(p, []string{"192.168.1.0/24"})

	// Test publishing without NATS connection (should log warning but not panic)
	device := DiscoveryResult{
		IP:       "192.168.1.1",
		MAC:      "00:11:22:33:44:55",
		Hostname: "test-host",
	}

	de.publishDiscovery(device)
	t.Log("publishDiscovery executed without panic")
}

func TestPublishFlow(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		ProbeID:           "test-probe",
		TenantID:          "test-tenant",
		FlowEnabled:       true,
		FlowPort:          2055,
		DiscoveryInterval: 1 * time.Hour,
	}

	p := &Probe{
		ID:       cfg.ProbeID,
		TenantID: cfg.TenantID,
		Config:   cfg,
		Logger:   logger,
	}

	fc := NewFlowCollector(p, cfg.FlowPort, cfg.FlowProtocols)

	// Test publishing without NATS connection
	record := FlowRecord{
		Time:     time.Now(),
		SrcIP:    "192.168.1.1",
		DstIP:    "10.0.0.1",
		SrcPort:  12345,
		DstPort:  80,
		Protocol: "tcp",
		Bytes:    1024,
		Packets:  10,
	}

	fc.publishFlow(record)
	t.Log("publishFlow executed without panic")
}
