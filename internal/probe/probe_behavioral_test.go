package probe

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// DiscoveryResult — struct fields and validation
// ---------------------------------------------------------------------------

func TestDiscoveryResult_StructFields(t *testing.T) {
	dev := DiscoveryResult{
		IP:        "192.168.1.1",
		MAC:       "00:11:22:33:44:55",
		Hostname:  "workstation.local",
		Vendor:    "ExampleCorp",
		Type:      "desktop",
		Ports:     []string{"22/tcp", "443/tcp"},
		Interface: "eth0",
		Via:       "arp",
		Labels:    map[string]string{"role": "server"},
	}
	if dev.IP != "192.168.1.1" {
		t.Errorf("IP = %q, want 192.168.1.1", dev.IP)
	}
	if dev.MAC != "00:11:22:33:44:55" {
		t.Errorf("MAC = %q, want 00:11:22:33:44:55", dev.MAC)
	}
	if dev.Hostname != "workstation.local" {
		t.Errorf("Hostname = %q, want workstation.local", dev.Hostname)
	}
	if dev.Vendor != "ExampleCorp" {
		t.Errorf("Vendor = %q, want ExampleCorp", dev.Vendor)
	}
	if dev.Type != "desktop" {
		t.Errorf("Type = %q, want desktop", dev.Type)
	}
	if len(dev.Ports) != 2 {
		t.Errorf("Ports len = %d, want 2", len(dev.Ports))
	}
	if dev.Interface != "eth0" {
		t.Errorf("Interface = %q, want eth0", dev.Interface)
	}
	if dev.Via != "arp" {
		t.Errorf("Via = %q, want arp", dev.Via)
	}
	if dev.Labels["role"] != "server" {
		t.Errorf("Labels[role] = %q, want server", dev.Labels["role"])
	}
}

func TestDiscoveryResult_ZeroValues(t *testing.T) {
	dev := DiscoveryResult{}
	if dev.IP != "" {
		t.Error("IP default should be empty")
	}
	if dev.MAC != "" {
		t.Error("MAC default should be empty")
	}
	if dev.Hostname != "" {
		t.Error("Hostname default should be empty")
	}
	if dev.Type != "" {
		t.Error("Type default should be empty")
	}
	if dev.Via != "" {
		t.Error("Via default should be empty")
	}
	if dev.Interface != "" {
		t.Error("Interface default should be empty")
	}
}

func TestDiscoveryResult_JSONRoundTrip(t *testing.T) {
	original := DiscoveryResult{
		IP:        "10.0.0.1",
		MAC:       "aa:bb:cc:dd:ee:ff",
		Hostname:  "router.local",
		Vendor:    "RouterCo",
		Type:      "router",
		Ports:     []string{"22/tcp"},
		Interface: "eth1",
		Via:       "lldp",
		Labels:    map[string]string{"site": "hq"},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DiscoveryResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.IP != original.IP {
		t.Errorf("IP mismatch: %q != %q", decoded.IP, original.IP)
	}
	if decoded.MAC != original.MAC {
		t.Errorf("MAC mismatch: %q != %q", decoded.MAC, original.MAC)
	}
	if decoded.Via != original.Via {
		t.Errorf("Via mismatch: %q != %q", decoded.Via, original.Via)
	}
	if decoded.Labels["site"] != "hq" {
		t.Errorf("Labels[site] mismatch: %q != %q", decoded.Labels["site"], "hq")
	}
}

func TestDiscoveryResult_ViaValues(t *testing.T) {
	viaValues := []string{"arp", "lldp", "cdp", "snmp", "icmp"}
	for _, via := range viaValues {
		dev := DiscoveryResult{IP: "1.2.3.4", Via: via}
		if dev.Via != via {
			t.Errorf("Via = %q, want %q", dev.Via, via)
		}
	}
}

func TestDiscoveryResult_EmptyPorts(t *testing.T) {
	dev := DiscoveryResult{IP: "1.2.3.4", Ports: nil}
	data, err := json.Marshal(dev)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DiscoveryResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.IP != "1.2.3.4" {
		t.Error("IP mismatch after round-trip")
	}
}

func TestDiscoveryResult_LabelsNil(t *testing.T) {
	dev := DiscoveryResult{IP: "1.2.3.4"}
	if dev.Labels != nil {
		t.Error("Labels should be nil for zero-value DiscoveryResult")
	}
}

func TestDiscoveryResult_PortsEmpty(t *testing.T) {
	dev := DiscoveryResult{IP: "1.2.3.4", Ports: []string{}}
	if len(dev.Ports) != 0 {
		t.Error("Empty Ports should have length 0")
	}
}

func TestDiscoveryResult_MACFormats(t *testing.T) {
	formats := []string{"00:11:22:33:44:55", "AA:BB:CC:DD:EE:FF", "00-11-22-33-44-55", ""}
	for _, m := range formats {
		dev := DiscoveryResult{MAC: m}
		if dev.MAC != m {
			t.Errorf("MAC = %q, want %q", dev.MAC, m)
		}
	}
}

// ---------------------------------------------------------------------------
// FlowRecord — struct fields and validation
// ---------------------------------------------------------------------------

func TestFlowRecord_StructFields(t *testing.T) {
	fr := FlowRecord{
		Time:         time.Now(),
		SrcIP:        "192.168.1.10",
		DstIP:        "10.0.0.1",
		SrcPort:      12345,
		DstPort:      443,
		Protocol:     "TCP",
		ProtocolID:   6,
		Bytes:        1024,
		Packets:      10,
		DurationMs:   500,
		Flags:        0x02,
		InputIf:      1,
		OutputIf:     2,
		SrcMAC:       "aa:bb:cc:dd:ee:01",
		DstMAC:       "aa:bb:cc:dd:ee:02",
		SrcAS:        65000,
		DstAS:        65001,
		NextHop:      "192.168.1.1",
		User:         "admin",
		URL:          "https://example.com",
		Labels:       []string{"external"},
	}
	if fr.SrcIP != "192.168.1.10" {
		t.Errorf("SrcIP = %q, want 192.168.1.10", fr.SrcIP)
	}
	if fr.DstIP != "10.0.0.1" {
		t.Errorf("DstIP = %q, want 10.0.0.1", fr.DstIP)
	}
	if fr.SrcPort != 12345 {
		t.Errorf("SrcPort = %d, want 12345", fr.SrcPort)
	}
	if fr.DstPort != 443 {
		t.Errorf("DstPort = %d, want 443", fr.DstPort)
	}
	if fr.Protocol != "TCP" {
		t.Errorf("Protocol = %q, want TCP", fr.Protocol)
	}
	if fr.ProtocolID != 6 {
		t.Errorf("ProtocolID = %d, want 6", fr.ProtocolID)
	}
	if fr.Bytes != 1024 {
		t.Errorf("Bytes = %d, want 1024", fr.Bytes)
	}
	if fr.Packets != 10 {
		t.Errorf("Packets = %d, want 10", fr.Packets)
	}
	if fr.DurationMs != 500 {
		t.Errorf("DurationMs = %d, want 500", fr.DurationMs)
	}
	if fr.InputIf != 1 {
		t.Errorf("InputIf = %d, want 1", fr.InputIf)
	}
	if fr.OutputIf != 2 {
		t.Errorf("OutputIf = %d, want 2", fr.OutputIf)
	}
	if fr.SrcAS != 65000 {
		t.Errorf("SrcAS = %d, want 65000", fr.SrcAS)
	}
	if fr.DstAS != 65001 {
		t.Errorf("DstAS = %d, want 65001", fr.DstAS)
	}
}

func TestFlowRecord_ZeroValues(t *testing.T) {
	fr := FlowRecord{}
	if fr.SrcIP != "" {
		t.Error("SrcIP default should be empty")
	}
	if fr.DstIP != "" {
		t.Error("DstIP default should be empty")
	}
	if fr.SrcPort != 0 {
		t.Error("SrcPort default should be 0")
	}
	if fr.Bytes != 0 {
		t.Error("Bytes default should be 0")
	}
	if fr.Protocol != "" {
		t.Error("Protocol default should be empty")
	}
}

func TestFlowRecord_JSONRoundTrip(t *testing.T) {
	original := FlowRecord{
		SrcIP:    "10.0.0.1",
		DstIP:    "10.0.0.2",
		SrcPort:  8080,
		DstPort:  80,
		Protocol: "UDP",
		Bytes:    256,
		Packets:  2,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded FlowRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SrcIP != original.SrcIP {
		t.Errorf("SrcIP mismatch: %q != %q", decoded.SrcIP, original.SrcIP)
	}
	if decoded.Protocol != original.Protocol {
		t.Errorf("Protocol mismatch: %q != %q", decoded.Protocol, original.Protocol)
	}
	if decoded.Bytes != original.Bytes {
		t.Errorf("Bytes mismatch: %d != %d", decoded.Bytes, original.Bytes)
	}
}

func TestFlowRecord_NetFlowV5Fields(t *testing.T) {
	fr := FlowRecord{
		ProtocolID: 6,
		SrcAS:      64512,
		DstAS:      65534,
		NextHop:    "172.16.0.1",
		InputIf:    100,
		OutputIf:   200,
	}
	if fr.ProtocolID != 6 {
		t.Error("TCP ProtocolID should be 6")
	}
	if fr.SrcAS != 64512 {
		t.Error("SrcAS mismatch")
	}
	if fr.NextHop != "172.16.0.1" {
		t.Errorf("NextHop = %q, want 172.16.0.1", fr.NextHop)
	}
}

func TestFlowRecord_FlagValues(t *testing.T) {
	flags := []uint8{0x02, 0x10, 0x18, 0x11, 0x04, 0x01}
	for _, f := range flags {
		fr := FlowRecord{Flags: f}
		if fr.Flags != f {
			t.Errorf("Flags = %d, want %d", fr.Flags, f)
		}
	}
}

func TestFlowRecord_EmptyLabels(t *testing.T) {
	fr := FlowRecord{Labels: []string{}}
	if len(fr.Labels) != 0 {
		t.Error("Empty Labels should have length 0")
	}
}

func TestFlowRecord_ZeroDuration(t *testing.T) {
	fr := FlowRecord{DurationMs: 0}
	if fr.DurationMs != 0 {
		t.Error("Zero DurationMs should be valid")
	}
}

func TestFlowRecord_NegativeBytes(t *testing.T) {
	fr := FlowRecord{Bytes: -1}
	if fr.Bytes != -1 {
		t.Error("Negative Bytes should be accepted")
	}
}

// ---------------------------------------------------------------------------
// PortScanResult — struct fields and validation
// ---------------------------------------------------------------------------

func TestPortScanResult_StructFields(t *testing.T) {
	psr := PortScanResult{
		Port:     443,
		Protocol: "tcp",
		State:    "open",
		Service:  "https",
		Version:  "nginx/1.24",
	}
	if psr.Port != 443 {
		t.Errorf("Port = %d, want 443", psr.Port)
	}
	if psr.Protocol != "tcp" {
		t.Errorf("Protocol = %q, want tcp", psr.Protocol)
	}
	if psr.State != "open" {
		t.Errorf("State = %q, want open", psr.State)
	}
	if psr.Service != "https" {
		t.Errorf("Service = %q, want https", psr.Service)
	}
	if psr.Version != "nginx/1.24" {
		t.Errorf("Version = %q, want nginx/1.24", psr.Version)
	}
}

func TestPortScanResult_StateValues(t *testing.T) {
	states := []string{"open", "closed", "filtered", "unknown"}
	for _, state := range states {
		psr := PortScanResult{State: state}
		if psr.State != state {
			t.Errorf("State = %q, want %q", psr.State, state)
		}
	}
}

func TestPortScanResult_ZeroValues(t *testing.T) {
	psr := PortScanResult{}
	if psr.Port != 0 {
		t.Error("Port default should be 0")
	}
	if psr.Protocol != "" {
		t.Error("Protocol default should be empty")
	}
	if psr.Service != "" {
		t.Error("Service default should be empty")
	}
}

func TestPortScanResult_JSONRoundTrip(t *testing.T) {
	original := PortScanResult{
		Port:     8080,
		Protocol: "tcp",
		State:    "open",
		Service:  "http-proxy",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PortScanResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Port != original.Port {
		t.Errorf("Port mismatch: %d != %d", decoded.Port, original.Port)
	}
	if decoded.Service != original.Service {
		t.Errorf("Service mismatch: %q != %q", decoded.Service, original.Service)
	}
}

func TestPortScanResult_VersionOptional(t *testing.T) {
	psr := PortScanResult{Port: 22, Service: "ssh"}
	if psr.Version != "" {
		t.Error("Version should be empty when not set")
	}
}

func TestPortScanResult_UDPProtocol(t *testing.T) {
	psr := PortScanResult{Port: 53, Protocol: "udp", State: "open", Service: "dns"}
	if psr.Protocol != "udp" {
		t.Errorf("Protocol = %q, want udp", psr.Protocol)
	}
}

func TestPortScanResult_FilteredState(t *testing.T) {
	psr := PortScanResult{Port: 3389, Protocol: "tcp", State: "filtered"}
	if psr.State != "filtered" {
		t.Errorf("State = %q, want filtered", psr.State)
	}
}

func TestPortScanResult_MaxPort(t *testing.T) {
	psr := PortScanResult{Port: 65535, Protocol: "tcp", State: "open"}
	if psr.Port != 65535 {
		t.Errorf("Port = %d, want 65535", psr.Port)
	}
}

// ---------------------------------------------------------------------------
// Scanner structs — ServiceInfo, HostInfo, TopologyInfo, InterfaceInfo
// ---------------------------------------------------------------------------

func TestServiceInfo_StructFields(t *testing.T) {
	si := ServiceInfo{
		Name:    "nginx",
		Version: "1.24.0",
		Product: "Nginx Web Server",
		Extra:   "fastcgi enabled",
	}
	if si.Name != "nginx" {
		t.Errorf("Name = %q, want nginx", si.Name)
	}
	if si.Version != "1.24.0" {
		t.Errorf("Version = %q, want 1.24.0", si.Version)
	}
	if si.Product != "Nginx Web Server" {
		t.Errorf("Product = %q, want Nginx Web Server", si.Product)
	}
	if si.Extra != "fastcgi enabled" {
		t.Errorf("Extra = %q, want fastcgi enabled", si.Extra)
	}
}

func TestServiceInfo_ZeroValues(t *testing.T) {
	si := ServiceInfo{}
	if si.Name != "" {
		t.Error("Name default should be empty")
	}
	if si.Version != "" {
		t.Error("Version default should be empty")
	}
}

func TestServiceInfo_JSONRoundTrip(t *testing.T) {
	original := ServiceInfo{Name: "apache", Version: "2.4.57"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ServiceInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name mismatch: %q != %q", decoded.Name, original.Name)
	}
}

func TestHostInfo_StructFields(t *testing.T) {
	hi := HostInfo{
		IP:       "192.168.1.50",
		Hostname: "webserver.local",
		MAC:      "aa:bb:cc:dd:ee:01",
		OS:       "Ubuntu 22.04",
		Uptime:   86400,
		OpenPorts: []PortScanResult{
			{Port: 80, Protocol: "tcp", State: "open"},
			{Port: 443, Protocol: "tcp", State: "open"},
		},
		Services: []ServiceInfo{{Name: "nginx", Version: "1.24.0"}},
		Labels:   map[string]string{"role": "web"},
	}
	if hi.IP != "192.168.1.50" {
		t.Errorf("IP = %q, want 192.168.1.50", hi.IP)
	}
	if hi.Hostname != "webserver.local" {
		t.Errorf("Hostname = %q, want webserver.local", hi.Hostname)
	}
	if hi.OS != "Ubuntu 22.04" {
		t.Errorf("OS = %q, want Ubuntu 22.04", hi.OS)
	}
	if hi.Uptime != 86400 {
		t.Errorf("Uptime = %d, want 86400", hi.Uptime)
	}
	if len(hi.OpenPorts) != 2 {
		t.Errorf("OpenPorts len = %d, want 2", len(hi.OpenPorts))
	}
}

func TestHostInfo_ZeroValues(t *testing.T) {
	hi := HostInfo{}
	if hi.IP != "" {
		t.Error("IP default should be empty")
	}
	if hi.OS != "" {
		t.Error("OS default should be empty")
	}
	if hi.Uptime != 0 {
		t.Error("Uptime default should be 0")
	}
}

func TestHostInfo_JSONRoundTrip(t *testing.T) {
	original := HostInfo{IP: "10.0.0.5", OS: "CentOS 8"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded HostInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.IP != original.IP {
		t.Errorf("IP mismatch: %q != %q", decoded.IP, original.IP)
	}
}

func TestTopologyInfo_StructFields(t *testing.T) {
	ti := TopologyInfo{
		Interfaces: []InterfaceInfo{
			{Name: "eth0", Index: 1},
		},
		Gateways: []GatewayInfo{
			{IP: "192.168.1.1", Interface: "eth0", IsDefault: true},
		},
	}
	if len(ti.Interfaces) != 1 {
		t.Errorf("Interfaces len = %d, want 1", len(ti.Interfaces))
	}
	if len(ti.Gateways) != 1 {
		t.Errorf("Gateways len = %d, want 1", len(ti.Gateways))
	}
	if ti.Gateways[0].IP != "192.168.1.1" {
		t.Errorf("Gateway IP = %q, want 192.168.1.1", ti.Gateways[0].IP)
	}
}

func TestTopologyInfo_ZeroValues(t *testing.T) {
	ti := TopologyInfo{}
	if len(ti.Interfaces) != 0 {
		t.Error("Interfaces default should be empty")
	}
	if len(ti.Gateways) != 0 {
		t.Error("Gateways default should be empty")
	}
}

func TestTopologyInfo_JSONRoundTrip(t *testing.T) {
	original := TopologyInfo{
		Interfaces: []InterfaceInfo{{Name: "lo", Index: 0}},
		Gateways:   []GatewayInfo{},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TopologyInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Interfaces) != 1 {
		t.Error("Interfaces count mismatch")
	}
}

func TestInterfaceInfo_StructFields(t *testing.T) {
	ii := InterfaceInfo{
		Name:    "eth0",
		Index:   2,
		MAC:     "aa:bb:cc:dd:ee:01",
		Addresses: []AddressInfo{
			{IP: "192.168.1.1", Network: "255.255.255.0", Broadcast: "192.168.1.255"},
		},
	}
	if ii.Name != "eth0" {
		t.Errorf("Name = %q, want eth0", ii.Name)
	}
	if ii.Index != 2 {
		t.Errorf("Index = %d, want 2", ii.Index)
	}
	if ii.MAC != "aa:bb:cc:dd:ee:01" {
		t.Errorf("MAC = %q, want aa:bb:cc:dd:ee:01", ii.MAC)
	}
	if len(ii.Addresses) != 1 {
		t.Errorf("Addresses len = %d, want 1", len(ii.Addresses))
	}
}

func TestInterfaceInfo_ZeroValues(t *testing.T) {
	ii := InterfaceInfo{}
	if ii.Name != "" {
		t.Error("Name default should be empty")
	}
	if ii.Index != 0 {
		t.Error("Index default should be 0")
	}
}

func TestInterfaceInfo_JSONRoundTrip(t *testing.T) {
	original := InterfaceInfo{Name: "lo", Index: 0}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded InterfaceInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name mismatch: %q != %q", decoded.Name, original.Name)
	}
}

func TestAddressInfo_StructFields(t *testing.T) {
	ai := AddressInfo{
		IP:        "192.168.1.1",
		Network:   "255.255.255.0",
		Broadcast: "192.168.1.255",
	}
	if ai.IP != "192.168.1.1" {
		t.Errorf("IP = %q, want 192.168.1.1", ai.IP)
	}
	if ai.Network != "255.255.255.0" {
		t.Errorf("Network = %q, want 255.255.255.0", ai.Network)
	}
	if ai.Broadcast != "192.168.1.255" {
		t.Errorf("Broadcast = %q, want 192.168.1.255", ai.Broadcast)
	}
}

func TestAddressInfo_ZeroValues(t *testing.T) {
	ai := AddressInfo{}
	if ai.IP != "" {
		t.Error("IP default should be empty")
	}
	if ai.Network != "" {
		t.Error("Network default should be empty")
	}
}

func TestAddressInfo_JSONRoundTrip(t *testing.T) {
	original := AddressInfo{IP: "10.0.0.1", Network: "255.0.0.0"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded AddressInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.IP != original.IP {
		t.Errorf("IP mismatch: %q != %q", decoded.IP, original.IP)
	}
}

func TestGatewayInfo_StructFields(t *testing.T) {
	gi := GatewayInfo{
		IP:        "192.168.1.1",
		Interface: "eth0",
		IsDefault: true,
	}
	if gi.IP != "192.168.1.1" {
		t.Errorf("IP = %q, want 192.168.1.1", gi.IP)
	}
	if gi.Interface != "eth0" {
		t.Errorf("Interface = %q, want eth0", gi.Interface)
	}
	if !gi.IsDefault {
		t.Error("IsDefault should be true")
	}
}

func TestGatewayInfo_ZeroValues(t *testing.T) {
	gi := GatewayInfo{}
	if gi.IP != "" {
		t.Error("IP default should be empty")
	}
	if gi.Interface != "" {
		t.Error("Interface default should be empty")
	}
}

func TestGatewayInfo_JSONRoundTrip(t *testing.T) {
	original := GatewayInfo{IP: "10.0.0.254", Interface: "eth1", IsDefault: false}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded GatewayInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.IP != original.IP {
		t.Errorf("IP mismatch: %q != %q", decoded.IP, original.IP)
	}
}

// ---------------------------------------------------------------------------
// Probe and Config structs
// ---------------------------------------------------------------------------

func TestProbe_StructFields(t *testing.T) {
	p := &Probe{}
	if p == nil {
		t.Error("Probe should not be nil")
	}
}

func TestConfig_StructFields(t *testing.T) {
	cfg := Config{
		ProbeID:           "probe-001",
		TenantID:          "tenant-abc",
		NATSURL:           "nats://localhost:4222",
		DiscoveryEnabled:  true,
		DiscoverySubnets:  []string{"192.168.1.0/24"},
		FlowEnabled:       true,
		FlowPort:          2055,
		FlowProtocols:     []string{"netflow5", "netflow9", "ipfix"},
		PollInterval:      60,
		DiscoveryInterval: 300,
	}
	if cfg.ProbeID != "probe-001" {
		t.Errorf("ProbeID = %q, want probe-001", cfg.ProbeID)
	}
	if cfg.TenantID != "tenant-abc" {
		t.Errorf("TenantID = %q, want tenant-abc", cfg.TenantID)
	}
	if cfg.NATSURL != "nats://localhost:4222" {
		t.Errorf("NATSURL = %q, want nats://localhost:4222", cfg.NATSURL)
	}
	if !cfg.DiscoveryEnabled {
		t.Error("DiscoveryEnabled should be true")
	}
	if len(cfg.DiscoverySubnets) != 1 {
		t.Errorf("DiscoverySubnets len = %d, want 1", len(cfg.DiscoverySubnets))
	}
	if !cfg.FlowEnabled {
		t.Error("FlowEnabled should be true")
	}
	if cfg.FlowPort != 2055 {
		t.Errorf("FlowPort = %d, want 2055", cfg.FlowPort)
	}
	if len(cfg.FlowProtocols) != 3 {
		t.Errorf("FlowProtocols len = %d, want 3", len(cfg.FlowProtocols))
	}
}

func TestConfig_ZeroValues(t *testing.T) {
	cfg := Config{}
	if cfg.ProbeID != "" {
		t.Error("ProbeID default should be empty")
	}
	if cfg.TenantID != "" {
		t.Error("TenantID default should be empty")
	}
	if cfg.NATSURL != "" {
		t.Error("NATSURL default should be empty")
	}
	if cfg.DiscoveryEnabled {
		t.Error("DiscoveryEnabled default should be false")
	}
	if cfg.FlowEnabled {
		t.Error("FlowEnabled default should be false")
	}
}

func TestConfig_JSONRoundTrip(t *testing.T) {
	original := Config{
		ProbeID:        "probe-002",
		TenantID:       "tenant-def",
		DiscoveryEnabled: true,
		FlowEnabled:    true,
		FlowPort:       9996,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ProbeID != original.ProbeID {
		t.Errorf("ProbeID mismatch: %q != %q", decoded.ProbeID, original.ProbeID)
	}
	if decoded.TenantID != original.TenantID {
		t.Errorf("TenantID mismatch: %q != %q", decoded.TenantID, original.TenantID)
	}
	if decoded.FlowPort != original.FlowPort {
		t.Errorf("FlowPort mismatch: %d != %d", decoded.FlowPort, original.FlowPort)
	}
}

func TestConfig_FlowProtocols(t *testing.T) {
	protocols := []string{"netflow5", "netflow9", "ipfix", "sflow"}
	cfg := Config{FlowProtocols: protocols}
	if len(cfg.FlowProtocols) != 4 {
		t.Errorf("FlowProtocols len = %d, want 4", len(cfg.FlowProtocols))
	}
}

// ---------------------------------------------------------------------------
// SNMPV3Config struct
// ---------------------------------------------------------------------------

func TestSNMPV3Config_StructFields(t *testing.T) {
	cfg := SNMPV3Config{
		Username:  "admin",
		AuthProto: "SHA",
		AuthPass:  "secret",
		PrivProto: "AES",
		PrivPass:  "privsecret",
		Context:   "context1",
	}
	if cfg.Username != "admin" {
		t.Errorf("Username = %q, want admin", cfg.Username)
	}
	if cfg.AuthProto != "SHA" {
		t.Errorf("AuthProto = %q, want SHA", cfg.AuthProto)
	}
	if cfg.PrivProto != "AES" {
		t.Errorf("PrivProto = %q, want AES", cfg.PrivProto)
	}
	if cfg.Context != "context1" {
		t.Errorf("Context = %q, want context1", cfg.Context)
	}
}

func TestSNMPV3Config_ZeroValues(t *testing.T) {
	cfg := SNMPV3Config{}
	if cfg.Username != "" {
		t.Error("Username default should be empty")
	}
	if cfg.AuthProto != "" {
		t.Error("AuthProto default should be empty")
	}
	if cfg.Context != "" {
		t.Error("Context default should be empty")
	}
}

// ---------------------------------------------------------------------------
// SNMPTarget struct
// ---------------------------------------------------------------------------

func TestSNMPTarget_StructFields(t *testing.T) {
	target := SNMPTarget{
		Host:      "192.168.1.1",
		Port:      161,
		Version:   "v2c",
		Community: "public",
		OIDs:      []string{"1.3.6.1.2.1.1.1.0"},
	}
	if target.Host != "192.168.1.1" {
		t.Errorf("Host = %q, want 192.168.1.1", target.Host)
	}
	if target.Port != 161 {
		t.Errorf("Port = %d, want 161", target.Port)
	}
	if target.Version != "v2c" {
		t.Errorf("Version = %q, want v2c", target.Version)
	}
	if target.Community != "public" {
		t.Errorf("Community = %q, want public", target.Community)
	}
	if len(target.OIDs) != 1 {
		t.Errorf("OIDs len = %d, want 1", len(target.OIDs))
	}
}

func TestSNMPTarget_ZeroValues(t *testing.T) {
	target := SNMPTarget{}
	if target.Host != "" {
		t.Error("Host default should be empty")
	}
	if target.Port != 0 {
		t.Error("Port default should be 0")
	}
	if target.Version != "" {
		t.Error("Version default should be empty")
	}
}

func TestSNMPTarget_JSONRoundTrip(t *testing.T) {
	original := SNMPTarget{Host: "10.0.0.1", Community: "private"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded SNMPTarget
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Host != original.Host {
		t.Errorf("Host mismatch: %q != %q", decoded.Host, original.Host)
	}
}

// ---------------------------------------------------------------------------
// Utility functions — behavioral tests
// ---------------------------------------------------------------------------

func TestNormalizeMAC_Valid(t *testing.T) {
	result := normalizeMAC("00:11:22:33:44:55")
	if result != "00:11:22:33:44:55" {
		t.Errorf("normalizeMAC(00:11:22:33:44:55) = %q, want 00:11:22:33:44:55", result)
	}
}

func TestNormalizeMAC_Capitalized(t *testing.T) {
	result := normalizeMAC("AA:BB:CC:DD:EE:FF")
	if result != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("normalizeMAC(AA:BB:CC:DD:EE:FF) = %q, want AA:BB:CC:DD:EE:FF", result)
	}
}

func TestNormalizeMAC_Hyphens(t *testing.T) {
	result := normalizeMAC("00-11-22-33-44-55")
	if result != "00:11:22:33:44:55" {
		t.Errorf("normalizeMAC(00-11-22-33-44-55) = %q, want 00:11:22:33:44:55", result)
	}
}

func TestNormalizeMAC_Empty(t *testing.T) {
	result := normalizeMAC("")
	if result != "" {
		t.Errorf("normalizeMAC(empty) = %q, want empty", result)
	}
}

func TestNormalizeWindowsMAC_Valid(t *testing.T) {
	result := normalizeWindowsMAC("0011.2233.4455")
	if result != "0011.2233.4455" {
		t.Errorf("normalizeWindowsMAC(0011.2233.4455) = %q, want 0011.2233.4455", result)
	}
}

func TestNormalizeWindowsMAC_Capitalized(t *testing.T) {
	result := normalizeWindowsMAC("AABB.CCDD.EEFF")
	if result != "AABB.CCDD.EEFF" {
		t.Errorf("normalizeWindowsMAC(AABB.CCDD.EEFF) = %q, want AABB.CCDD.EEFF", result)
	}
}

func TestNormalizeWindowsMAC_Empty(t *testing.T) {
	result := normalizeWindowsMAC("")
	if result != "" {
		t.Errorf("normalizeWindowsMAC(empty) = %q, want empty", result)
	}
}

func TestGetBroadcastIP_V4(t *testing.T) {
	_, ipNet, _ := net.ParseCIDR("192.168.1.0/24")
	broadcast := getBroadcastIP(ipNet)
	if broadcast != "192.168.1.255" {
		t.Errorf("getBroadcastIP(192.168.1.0/24) = %q, want 192.168.1.255", broadcast)
	}
}

func TestGetBroadcastIP_V4Large(t *testing.T) {
	_, ipNet, _ := net.ParseCIDR("10.0.0.0/8")
	broadcast := getBroadcastIP(ipNet)
	if broadcast != "10.255.255.255" {
		t.Errorf("getBroadcastIP(10.0.0.0/8) = %q, want 10.255.255.255", broadcast)
	}
}

func TestProtocolName_TCP(t *testing.T) {
	name := protocolName(6)
	if name != "tcp" {
		t.Errorf("protocolName(6) = %q, want tcp", name)
	}
}

func TestProtocolName_UDP(t *testing.T) {
	name := protocolName(17)
	if name != "udp" {
		t.Errorf("protocolName(17) = %q, want udp", name)
	}
}

func TestProtocolName_Unknown(t *testing.T) {
	name := protocolName(255)
	if name != "proto_255" {
		t.Errorf("protocolName(255) = %q, want proto_255", name)
	}
}

// ---------------------------------------------------------------------------
// FlowRecord — edge cases
// ---------------------------------------------------------------------------

func TestFlowRecord_LargeValues(t *testing.T) {
	fr := FlowRecord{
		Bytes:      999999999,
		Packets:    999999,
		DurationMs: 3600000,
		SrcPort:    65535,
		DstPort:    1,
	}
	if fr.Bytes != 999999999 {
		t.Error("Large Bytes mismatch")
	}
	if fr.Packets != 999999 {
		t.Error("Large Packets mismatch")
	}
	if fr.SrcPort != 65535 {
		t.Error("Max SrcPort mismatch")
	}
}

func TestFlowRecord_ICMPProtocol(t *testing.T) {
	fr := FlowRecord{Protocol: "ICMP", ProtocolID: 1}
	if fr.ProtocolID != 1 {
		t.Errorf("ICMP ProtocolID = %d, want 1", fr.ProtocolID)
	}
}

func TestFlowRecord_SameIPs(t *testing.T) {
	fr := FlowRecord{SrcIP: "192.168.1.1", DstIP: "192.168.1.1"}
	if fr.SrcIP != fr.DstIP {
		t.Error("Same IP flow should be valid")
	}
}

func TestFlowRecord_ZeroPorts(t *testing.T) {
	fr := FlowRecord{SrcPort: 0, DstPort: 0}
	if fr.SrcPort != 0 || fr.DstPort != 0 {
		t.Error("Zero ports should be valid")
	}
}

// ---------------------------------------------------------------------------
// PortScanResult — edge cases
// ---------------------------------------------------------------------------

func TestPortScanResult_AllStates(t *testing.T) {
	states := []string{"open", "closed", "filtered", "unknown"}
	for _, state := range states {
		psr := PortScanResult{State: state}
		if psr.State != state {
			t.Errorf("State = %q, want %q", psr.State, state)
		}
	}
}

func TestPortScanResult_JSONMarshalEmpty(t *testing.T) {
	psr := PortScanResult{}
	data, err := json.Marshal(psr)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PortScanResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Port != 0 {
		t.Error("Empty PortScanResult round-trip failed")
	}
}

// ---------------------------------------------------------------------------
// DiscoveryResult — edge cases
// ---------------------------------------------------------------------------

func TestDiscoveryResult_JSONMarshalEmpty(t *testing.T) {
	dev := DiscoveryResult{}
	data, err := json.Marshal(dev)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DiscoveryResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.IP != "" {
		t.Error("Empty DiscoveryResult round-trip failed")
	}
}

func TestDiscoveryResult_MultiPort(t *testing.T) {
	dev := DiscoveryResult{
		IP:    "192.168.1.1",
		Ports: []string{"22/tcp", "80/tcp", "443/tcp", "8080/tcp", "3306/tcp"},
	}
	if len(dev.Ports) != 5 {
		t.Errorf("Ports len = %d, want 5", len(dev.Ports))
	}
}

func TestDiscoveryResult_MultiLabel(t *testing.T) {
	dev := DiscoveryResult{
		IP: "192.168.1.1",
		Labels: map[string]string{
			"role": "server",
			"site": "hq",
			"env":  "prod",
			"owner": "ops",
		},
	}
	if len(dev.Labels) != 4 {
		t.Errorf("Labels len = %d, want 4", len(dev.Labels))
	}
}

func TestFlowRecord_JSONMarshalEmpty(t *testing.T) {
	fr := FlowRecord{}
	data, err := json.Marshal(fr)
	if err != nil {
		t.Fatal(err)
	}
	var decoded FlowRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SrcIP != "" {
		t.Error("Empty FlowRecord round-trip failed")
	}
}

func TestPortScanResult_JSONMarshalWithVersion(t *testing.T) {
	psr := PortScanResult{Port: 22, Service: "ssh", Version: "OpenSSH_8.9"}
	data, err := json.Marshal(psr)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PortScanResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Version != "OpenSSH_8.9" {
		t.Errorf("Version mismatch: %q != %q", decoded.Version, "OpenSSH_8.9")
	}
}

func TestDiscoveryResult_ViaICMP(t *testing.T) {
	dev := DiscoveryResult{IP: "192.168.1.1", Via: "icmp"}
	if dev.Via != "icmp" {
		t.Errorf("Via = %q, want icmp", dev.Via)
	}
}

func TestFlowRecord_SNMPProtocol(t *testing.T) {
	fr := FlowRecord{SrcPort: 161, DstPort: 161, Protocol: "UDP"}
	if fr.SrcPort != 161 || fr.DstPort != 161 {
		t.Error("SNMP port 161 mismatch")
	}
}

func TestHostInfo_EmptyPorts(t *testing.T) {
	hi := HostInfo{IP: "192.168.1.1"}
	if hi.OpenPorts != nil {
		t.Error("OpenPorts should be nil for zero-value HostInfo")
	}
}

func TestHostInfo_JSONMarshalEmpty(t *testing.T) {
	hi := HostInfo{}
	data, err := json.Marshal(hi)
	if err != nil {
		t.Fatal(err)
	}
	var decoded HostInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.IP != "" {
		t.Error("Empty HostInfo round-trip failed")
	}
}

func TestTopologyInfo_MultiInterfaces(t *testing.T) {
	ti := TopologyInfo{
		Interfaces: []InterfaceInfo{
			{Name: "eth0", Index: 1},
			{Name: "eth1", Index: 2},
			{Name: "lo", Index: 0},
		},
	}
	if len(ti.Interfaces) != 3 {
		t.Errorf("Interfaces len = %d, want 3", len(ti.Interfaces))
	}
}

func TestInterfaceInfo_MultiAddresses(t *testing.T) {
	ii := InterfaceInfo{
		Name: "eth0",
		Addresses: []AddressInfo{
			{IP: "192.168.1.1", Network: "255.255.255.0"},
			{IP: "fe80::1", Network: "ffff:ffff:ffff:ffff::"},
		},
	}
	if len(ii.Addresses) != 2 {
		t.Errorf("Addresses len = %d, want 2", len(ii.Addresses))
	}
}

func TestAddressInfo_NoBroadcast(t *testing.T) {
	ai := AddressInfo{IP: "192.168.1.1", Network: "255.255.255.0"}
	if ai.Broadcast != "" {
		t.Error("Broadcast should be empty when not set")
	}
}

func TestGatewayInfo_NonDefault(t *testing.T) {
	gi := GatewayInfo{IP: "192.168.2.1", Interface: "eth1", IsDefault: false}
	if gi.IsDefault {
		t.Error("IsDefault should be false")
	}
}

func TestConfig_MultiDiscoverySubnets(t *testing.T) {
	cfg := Config{DiscoverySubnets: []string{"192.168.1.0/24", "10.0.0.0/8", "172.16.0.0/12"}}
	if len(cfg.DiscoverySubnets) != 3 {
		t.Errorf("DiscoverySubnets len = %d, want 3", len(cfg.DiscoverySubnets))
	}
}

func TestConfig_ZeroFlowPort(t *testing.T) {
	cfg := Config{FlowPort: 0}
	if cfg.FlowPort != 0 {
		t.Error("Zero flow port should be valid")
	}
}

func TestSNMPTarget_Version3(t *testing.T) {
	target := SNMPTarget{Host: "10.0.0.1", Version: "v3"}
	if target.Version != "v3" {
		t.Errorf("Version = %q, want v3", target.Version)
	}
}

func TestSNMPV3Config_SHAAuth(t *testing.T) {
	cfg := SNMPV3Config{AuthProto: "SHA", PrivProto: "AES"}
	if cfg.AuthProto != "SHA" {
		t.Errorf("AuthProto = %q, want SHA", cfg.AuthProto)
	}
	if cfg.PrivProto != "AES" {
		t.Errorf("PrivProto = %q, want AES", cfg.PrivProto)
	}
}

func TestDiscoveryResult_ViaSNMP(t *testing.T) {
	dev := DiscoveryResult{IP: "192.168.1.1", Via: "snmp"}
	if dev.Via != "snmp" {
		t.Errorf("Via = %q, want snmp", dev.Via)
	}
}

func TestDiscoveryResult_ViaCDP(t *testing.T) {
	dev := DiscoveryResult{IP: "192.168.1.1", Via: "cdp"}
	if dev.Via != "cdp" {
		t.Errorf("Via = %q, want cdp", dev.Via)
	}
}

func TestFlowRecord_DNSFlow(t *testing.T) {
	fr := FlowRecord{SrcPort: 53214, DstPort: 53, Protocol: "UDP"}
	if fr.DstPort != 53 {
		t.Error("DNS port 53 mismatch")
	}
}

func TestServiceInfo_OnlyName(t *testing.T) {
	si := ServiceInfo{Name: "ssh"}
	if si.Name != "ssh" {
		t.Errorf("Name = %q, want ssh", si.Name)
	}
	if si.Version != "" {
		t.Error("Version should be empty")
	}
}
// test
