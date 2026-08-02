package probe

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// DiscoveryResult represents a discovered network device
type DiscoveryResult struct {
	IP        string            `json:"ip"`
	MAC       string            `json:"mac"`
	Hostname  string            `json:"hostname"`
	Vendor    string            `json:"vendor"`
	Type      string            `json:"type"`
	Ports     []string          `json:"ports,omitempty"`
	Interface string            `json:"interface,omitempty"`
	Via       string            `json:"via"` // arp, lldp, cdp, snmp, icmp
	Labels    map[string]string `json:"labels,omitempty"`
}

// DiscoveryEngine handles network discovery
type DiscoveryEngine struct {
	probe   *Probe
	subnets []string
	scanner *Scanner
}

// NewDiscoveryEngine creates a new discovery engine
func NewDiscoveryEngine(p *Probe, subnets []string) *DiscoveryEngine {
	return &DiscoveryEngine{
		probe:   p,
		subnets: subnets,
		scanner: NewScanner(p),
	}
}

// Run starts the discovery process
func (de *DiscoveryEngine) Run(ctx context.Context) {
	de.probe.Logger.Info("starting network discovery")
	de.runDiscovery(ctx)

	ticker := time.NewTicker(de.probe.Config.DiscoveryInterval)
	if de.probe.Config.DiscoveryInterval == 0 {
		ticker = time.NewTicker(1 * time.Hour)
	}
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			de.runDiscovery(ctx)
		}
	}
}

// runDiscovery performs a complete discovery cycle
func (de *DiscoveryEngine) runDiscovery(ctx context.Context) {
	de.probe.Logger.Info("running network discovery cycle")

	var allDevices []DiscoveryResult

	// Discover via ARP
	arpDevices := de.discoverARP(ctx)
	allDevices = append(allDevices, arpDevices...)
	de.probe.Logger.Info("ARP discovery complete", zap.Int("devices", len(arpDevices)))

	// Discover via ICMP ping scan
	for _, subnet := range de.subnets {
		if subnet == "" {
			continue
		}
		pingDevices := de.pingScan(ctx, subnet, 2*time.Second)
		allDevices = append(allDevices, pingDevices...)
		de.probe.Logger.Info("ping scan complete", zap.String("subnet", subnet), zap.Int("devices", len(pingDevices)))
	}

	// Discover via SNMP
	snmpDevices := de.discoverSNMPNeighbors()
	allDevices = append(allDevices, snmpDevices...)
	de.probe.Logger.Info("SNMP neighbor discovery complete", zap.Int("devices", len(snmpDevices)))

	// Deduplicate and publish
	deduped := de.deduplicate(allDevices)

	for _, d := range deduped {
		de.publishDiscovery(d)
	}
}

// discoverARP scans ARP tables to discover local network devices
func (de *DiscoveryEngine) discoverARP(ctx context.Context) []DiscoveryResult {
	if runtime.GOOS == "linux" {
		return de.discoverARPLinux(ctx)
	} else if runtime.GOOS == "windows" {
		return de.discoverARPWindows(ctx)
	} else if runtime.GOOS == "darwin" {
		return de.discoverARPUnix(ctx)
	}
	return nil
}

func (de *DiscoveryEngine) discoverARPLinux(ctx context.Context) []DiscoveryResult {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		de.probe.Logger.Warn("failed to read ARP table", zap.Error(err))
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var results []DiscoveryResult

	for _, line := range lines[1:] {
		if ctx.Err() != nil {
			break
		}

		parts := strings.Fields(line)
		if len(parts) < 6 {
			continue
		}

		ip := parts[0]
		mac := parts[3]
		flag := parts[2]

		if flag != "0x2" { // Complete entry
			continue
		}

		// Get vendor from MAC OUI
		vendor := getVendorFromMAC(mac)

		// Get hostname via reverse DNS
		hostname, _ := net.LookupAddr(ip)
		hostStr := ""
		if len(hostname) > 0 {
			hostStr = strings.TrimSuffix(hostname[0], ".")
		}

		// Get interface info
		iface := de.getInterfaceForIP(ip)

		results = append(results, DiscoveryResult{
			IP:        ip,
			MAC:       normalizeMAC(mac),
			Hostname:  hostStr,
			Vendor:    vendor,
			Type:      "device",
			Interface: iface,
			Via:       "arp",
			Labels: map[string]string{
				"discovery_method": "arp_table",
				"os":               "linux",
			},
		})
	}

	return results
}

func (de *DiscoveryEngine) discoverARPWindows(ctx context.Context) []DiscoveryResult {
	cmd := exec.Command("arp", "-a")
	output, err := cmd.Output()
	if err != nil {
		de.probe.Logger.Warn("failed to run arp -a", zap.Error(err))
		return nil
	}

	var results []DiscoveryResult
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if ctx.Err() != nil {
			break
		}

		if !strings.HasPrefix(line, "  ") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 3 {
			ip := parts[0]
			mac := parts[1]

			vendor := getVendorFromMAC(mac)

			results = append(results, DiscoveryResult{
				IP:       ip,
				MAC:      normalizeWindowsMAC(mac),
				Vendor:   vendor,
				Type:     "device",
				Via:      "arp",
				Labels: map[string]string{
					"discovery_method": "arp_table",
					"os":               "windows",
				},
			})
		}
	}

	return results
}

func (de *DiscoveryEngine) discoverARPUnix(ctx context.Context) []DiscoveryResult {
	cmd := exec.Command("arp", "-a", "-n")
	output, err := cmd.Output()
	if err != nil {
		de.probe.Logger.Warn("failed to run arp -a", zap.Error(err))
		return nil
	}

	var results []DiscoveryResult
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if ctx.Err() != nil {
			break
		}

		if !strings.Contains(line, "(") || !strings.Contains(line, ")") {
			continue
		}

		// Parse format: host (ip) at mac [type]
		parts := strings.Fields(line)
		if len(parts) >= 4 {
			host := strings.TrimSuffix(parts[0], ":")
			ip := strings.Trim(parts[1], "()")
			mac := parts[3]

			vendor := getVendorFromMAC(mac)

			results = append(results, DiscoveryResult{
				IP:       ip,
				MAC:      normalizeMAC(mac),
				Hostname: host,
				Vendor:   vendor,
				Type:     "device",
				Via:      "arp",
				Labels: map[string]string{
					"discovery_method": "arp_table",
					"os":               "darwin",
				},
			})
		}
	}

	return results
}

// pingScan performs ICMP ping scanning to discover live hosts
func (de *DiscoveryEngine) pingScan(ctx context.Context, subnet string, timeout time.Duration) []DiscoveryResult {
	ip, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		de.probe.Logger.Warn("invalid subnet", zap.String("subnet", subnet), zap.Error(err))
		return nil
	}

	var results []DiscoveryResult
	ip = ip.Mask(ipNet.Mask)

	// Calculate range
	network := ip
	broadcast := make([]byte, 4)
	copy(broadcast, network)
	for i := 0; i < 4; i++ {
		broadcast[i] = network[i] | ^ipNet.Mask[i]
	}

	start := binary.BigEndian.Uint32(network)
	end := binary.BigEndian.Uint32(broadcast)

	sem := make(chan struct{}, 100)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for addr := start + 1; addr < end; addr++ {
		select {
		case <-ctx.Done():
			return results
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(a uint32) {
			defer wg.Done()
			defer func() { <-sem }()

			ip := make([]byte, 4)
			binary.BigEndian.PutUint32(ip, a)
			hostIP := net.IP(ip).String()

			if de.pingHost(ctx, hostIP, timeout) {
				// Perform service detection
				device := de.discoverDevice(ctx, hostIP, timeout)

				mu.Lock()
				results = append(results, device)
				mu.Unlock()
			}
		}(addr)
	}

	wg.Wait()
	return results
}

// pingHost sends an ICMP echo request to check if a host is alive
func (de *DiscoveryEngine) pingHost(ctx context.Context, host string, timeout time.Duration) bool {
	// Try raw ICMP
	if conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0"); err == nil {
		defer conn.Close()

		wm := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   os.Getpid() & 0xffff,
				Seq:  1,
				Data: []byte("Strata RMM"),
			},
		}

		buf, err := wm.Marshal(nil)
		if err != nil {
			return false
		}

		conn.SetDeadline(time.Now().Add(timeout))

		if _, err := conn.WriteTo(buf, &net.IPAddr{IP: net.ParseIP(host)}); err != nil {
			return false
		}

		reply := make([]byte, 64)
		if _, _, err := conn.ReadFrom(reply); err == nil {
			return true
		}

		return false
	}

	// Fallback to system ping
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", host)
	err := cmd.Run()
	return err == nil
}

// discoverDevice performs device discovery on a live host
func (de *DiscoveryEngine) discoverDevice(ctx context.Context, host string, timeout time.Duration) DiscoveryResult {
	device := DiscoveryResult{
		IP:     host,
		Via:    "icmp",
		Labels: map[string]string{"discovery_method": "icmp_ping"},
	}

	// Get hostname
	if hostname, err := net.LookupAddr(host); err == nil && len(hostname) > 0 {
		device.Hostname = strings.TrimSuffix(hostname[0], ".")
	}

	// Get MAC address
	if mac, err := de.getMACAddress(ctx, host); err == nil {
		device.MAC = mac
		device.Vendor = getVendorFromMAC(mac)
	}

	// Scan common ports
	commonPorts := []int{22, 80, 443, 8080, 3306, 5432, 6379, 21, 23, 53}
	ports, _ := de.scanner.ScanPorts(ctx, host, commonPorts, "tcp")

	var portList []string
	for _, p := range ports {
		if p.State == "open" {
			portList = append(portList, fmt.Sprintf("%d/%s", p.Port, p.Service))
		}
	}
	device.Ports = portList

	// Get interface
	device.Interface = de.getInterfaceForIP(host)

	return device
}

// getMACAddress attempts to get the MAC address for an IP
func (de *DiscoveryEngine) getMACAddress(ctx context.Context, ip string) (string, error) {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/net/arp")
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines[1:] {
				if strings.Contains(line, ip) {
					parts := strings.Fields(line)
					if len(parts) >= 4 {
						return normalizeMAC(parts[3]), nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("MAC not found")
}

// getInterfaceForIP determines which interface is used for a given IP
func (de *DiscoveryEngine) getInterfaceForIP(ip string) string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	iface, err := net.InterfaceByName(localAddr.IP.String())
	if err != nil {
		return ""
	}
	return iface.Name
}

// discoverSNMPNeighbors discovers network neighbors via SNMP
func (de *DiscoveryEngine) discoverSNMPNeighbors() []DiscoveryResult {
	var results []DiscoveryResult

	for _, t := range de.probe.targets {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		neighbors, err := PollSNMP(ctx, SNMPTarget{
			Host:      t.Host,
			Port:      t.Port,
			Version:   t.Version,
			Community: t.Community,
			V3:        t.V3,
			OIDs:      []string{".1.3.6.1.2.1.4.22.1.2*", ".1.3.6.1.2.1.17.4.3.1.1*"},
		})
		cancel()
		if err != nil {
			continue
		}

		for _, n := range neighbors {
			results = append(results, DiscoveryResult{
				IP:     n.Value,
				Via:    "snmp-arp",
				Labels: map[string]string{"discovered_by": t.Host},
			})
		}
	}

	return results
}

// deduplicate removes duplicate discovery results
func (de *DiscoveryEngine) deduplicate(devices []DiscoveryResult) []DiscoveryResult {
	seen := make(map[string]bool)
	var deduped []DiscoveryResult

	for _, d := range devices {
		key := d.IP
		if d.MAC != "" {
			key = d.MAC
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		// Perform reverse DNS lookup
		if d.IP != "" && d.Hostname == "" {
			if names, err := net.LookupAddr(d.IP); err == nil && len(names) > 0 {
				d.Hostname = strings.TrimSuffix(names[0], ".")
			}
		}

		// Determine device type
		d.Type = de.determineDeviceType(d)

		deduped = append(deduped, d)
	}

	return deduped
}

// determineDeviceType attempts to determine the type of device
func (de *DiscoveryEngine) determineDeviceType(d DiscoveryResult) string {
	// Check ports for common device types
	for _, port := range d.Ports {
		switch {
		case strings.Contains(port, "22"):
			return "router"
		case strings.Contains(port, "3306"):
			return "database"
		case strings.Contains(port, "5432"):
			return "database"
		case strings.Contains(port, "6379"):
			return "cache"
		case strings.Contains(port, "8080"):
			return "server"
		}
	}

	// Default
	if d.MAC != "" {
		return "device"
	}
	return "host"
}

// publishDiscovery sends discovery results via NATS
func (de *DiscoveryEngine) publishDiscovery(d DiscoveryResult) {
	data, err := json.Marshal(d)
	if err != nil {
		de.probe.Logger.Warn("failed to marshal discovery", zap.Error(err))
		return
	}

	subject := fmt.Sprintf("tenant.%s.probe.%s.discovery", de.probe.TenantID, de.probe.ID)
	if err := de.probe.NATS.Publish(subject, data); err != nil {
		de.probe.Logger.Warn("publish discovery", zap.Error(err))
	}
}

// normalizeMAC normalizes MAC addresses to standard format
func normalizeMAC(mac string) string {
	mac = strings.ReplaceAll(mac, "-", ":")
	mac = strings.ReplaceAll(mac, ".", "")
	mac = strings.ToUpper(mac)

	if len(mac) == 12 {
		var result strings.Builder
		for i, c := range mac {
			if i > 0 && i%2 == 0 {
				result.WriteString(":")
			}
			result.WriteRune(c)
		}
		mac = result.String()
	}

	return mac
}

// normalizeWindowsMAC normalizes Windows-style MAC addresses
func normalizeWindowsMAC(mac string) string {
	mac = strings.ReplaceAll(mac, "-", ":")
	return strings.ToUpper(mac)
}

// getVendorFromMAC attempts to determine vendor from MAC OUI
func getVendorFromMAC(mac string) string {
	if len(mac) < 8 {
		return "unknown"
	}

	// Extract OUI (first 6 hex chars, 3 bytes)
	oui := strings.ToUpper(strings.ReplaceAll(mac[:8], ":", ""))

	// Common OUI mappings
	vendors := map[string]string{
		"001C73": "Dell",
		"005056": "VMware",
		"00155D": "Microsoft",
		"000C29": "VMware",
		"001B63": "Apple",
		"B8AE6F": "Apple",
		"001A4B": "Intel",
		"002354": "Intel",
		"002672": "Intel",
		"004096": "HP",
		"001F29": "Intel",
		"000D93": "Dell",
		"001422": "Dell",
		"0015C5": "Dell",
		"002590": "Dell",
		"002667": "Dell",
		"001E67": "Dell",
		"002264": "Dell",
		"0025B3": "Dell",
		"001083": "Cisco",
		"001158": "Cisco",
		"00409B": "Cisco",
		"005089": "Cisco",
		"00000C": "Cisco",
	}

	if vendor, ok := vendors[oui]; ok {
		return vendor
	}

	return "unknown"
}
