package probe

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// PortScanResult represents the result of scanning a single port
type PortScanResult struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // "tcp" or "udp"
	State    string `json:"state"`    // "open", "closed", "filtered", "unknown"
	Service  string `json:"service"`
	Version  string `json:"version,omitempty"`
}

// ServiceInfo contains service detection information
type ServiceInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Product string `json:"product,omitempty"`
	Extra   string `json:"extra,omitempty"`
}

// HostInfo represents discovered host information
type HostInfo struct {
	IP          string         `json:"ip"`
	Hostname    string         `json:"hostname"`
	MAC         string         `json:"mac,omitempty"`
	OS          string         `json:"os,omitempty"`
	OpenPorts   []PortScanResult `json:"open_ports,omitempty"`
	Services    []ServiceInfo    `json:"services,omitempty"`
	Uptime      int64            `json:"uptime,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// TopologyInfo represents network topology information
type TopologyInfo struct {
	Interfaces []InterfaceInfo `json:"interfaces"`
	Gateways   []GatewayInfo   `json:"gateways"`
}

// InterfaceInfo represents network interface information
type InterfaceInfo struct {
	Name      string        `json:"name"`
	Index     int           `json:"index"`
	MAC       string        `json:"mac"`
	Addresses []AddressInfo `json:"addresses"`
}

// AddressInfo represents IP address information
type AddressInfo struct {
	IP        string `json:"ip"`
	Network   string `json:"network"`
	broadcast string `json:"broadcast,omitempty"`
}

// GatewayInfo represents gateway information
type GatewayInfo struct {
	IP        string `json:"ip"`
	Interface string `json:"interface"`
	IsDefault bool   `json:"is_default"`
}

// Scanner handles port and service scanning
type Scanner struct {
	probe      *Probe
	timeout    time.Duration
	maxWorkers int
}

// NewScanner creates a new Scanner instance
func NewScanner(p *Probe) *Scanner {
	return &Scanner{
		probe:      p,
		timeout:    2 * time.Second,
		maxWorkers: 100,
	}
}

// ScanPorts performs a TCP SYN scan on the specified host and ports
func (s *Scanner) ScanPorts(ctx context.Context, host string, ports []int, protocol string) ([]PortScanResult, error) {
	var results []PortScanResult

	switch protocol {
	case "tcp", "":
		results, _ = s.scanTCPSYN(ctx, host, ports)
	case "udp":
		results, _ = s.scanUDP(ctx, host, ports)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	return results, nil
}

// scanTCPSYN performs TCP SYN scanning using raw sockets or fallback
func (s *Scanner) scanTCPSYN(ctx context.Context, host string, ports []int) ([]PortScanResult, error) {
	var results []PortScanResult
	var mu sync.Mutex
	sem := make(chan struct{}, s.maxWorkers)
	var wg sync.WaitGroup

	for _, port := range ports {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			defer func() { <-sem }()

			result := s.checkPort(ctx, host, p, "tcp")
			if result != nil {
				mu.Lock()
				results = append(results, *result)
				mu.Unlock()
			}
		}(port)
	}

	wg.Wait()
	return results, nil
}

// checkPort checks if a specific port is open
func (s *Scanner) checkPort(ctx context.Context, host string, port int, protocol string) *PortScanResult {
	conn, err := net.DialTimeout(protocol, fmt.Sprintf("%s:%d", host, port), s.timeout)
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return &PortScanResult{
				Port:     port,
				Protocol: protocol,
				State:    "filtered",
			}
		}
		return &PortScanResult{
			Port:     port,
			Protocol: protocol,
			State:    "closed",
		}
	}
	defer conn.Close()

	// Try to get service information
	service, version := s.detectService(ctx, conn, port, protocol)

	return &PortScanResult{
		Port:     port,
		Protocol: protocol,
		State:    "open",
		Service:  service,
		Version:  version,
	}
}

// scanUDP performs UDP port scanning
func (s *Scanner) scanUDP(ctx context.Context, host string, ports []int) ([]PortScanResult, error) {
	var results []PortScanResult
	var mu sync.Mutex
	sem := make(chan struct{}, s.maxWorkers)
	var wg sync.WaitGroup

	for _, port := range ports {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			defer func() { <-sem }()

			result := s.checkUDPPort(ctx, host, p)
			if result != nil {
				mu.Lock()
				results = append(results, *result)
				mu.Unlock()
			}
		}(port)
	}

	wg.Wait()
	return results, nil
}

// checkUDPPort checks if a UDP port is open
func (s *Scanner) checkUDPPort(ctx context.Context, host string, port int) *PortScanResult {
	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:%d", host, port), s.timeout)
	if err != nil {
		return &PortScanResult{
			Port:     port,
			Protocol: "udp",
			State:    "filtered",
		}
	}
	defer conn.Close()

	// Send a null packet
	conn.Write([]byte{0})
	conn.SetReadDeadline(time.Now().Add(s.timeout))

	// Try to read a response
	buf := make([]byte, 1500)
	_, err = conn.Read(buf)

	if err != nil {
		return &PortScanResult{
			Port:     port,
			Protocol: "udp",
			State:    "open", // UDP is connectionless, open ports may not respond
		}
	}

	service, version := s.detectService(ctx, conn, port, "udp")

	return &PortScanResult{
		Port:     port,
		Protocol: "udp",
		State:    "open",
		Service:  service,
		Version:  version,
	}
}

// detectService attempts to detect the service running on a port
func (s *Scanner) detectService(ctx context.Context, conn net.Conn, port int, protocol string) (service string, version string) {
	// Try common service detection based on port
	switch port {
	case 21:
		service = "ftp"
	case 22:
		service = "ssh"
		version = s.trySSHVersion(conn)
	case 23:
		service = "telnet"
	case 25:
		service = "smtp"
	case 53:
		service = "dns"
		version = s.tryDNSVersion(ctx, conn)
	case 80:
		service = "http"
		version = s.tryHTTPVersion(ctx, conn)
	case 443:
		service = "https"
		version = s.tryHTTPVersion(ctx, conn)
	case 3306:
		service = "mysql"
	case 5432:
		service = "postgresql"
	case 6379:
		service = "redis"
	case 8080:
		service = "http-proxy"
	case 8443:
		service = "https-alt"
	default:
		service = "unknown"
	}

	return service, version
}

// trySSHVersion attempts to get SSH version banner
func (s *Scanner) trySSHVersion(conn net.Conn) string {
	if conn == nil {
		return ""
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return ""
	}

	banner := string(buf[:n])
	if strings.HasPrefix(banner, "SSH-") {
		return strings.TrimSpace(banner)
	}
	return ""
}

// tryDNSVersion attempts to get DNS version
func (s *Scanner) tryDNSVersion(ctx context.Context, conn net.Conn) string {
	if conn == nil {
		return ""
	}
	// Send a simple DNS query for version.bind
	query := []byte{
		0x00, 0x01, // Transaction ID
		0x01, 0x00, // Standard query
		0x00, 0x01, // One question
		0x00, 0x00, // No answers
		0x00, 0x00, // No authorities
		0x00, 0x00, // No additional
		0x07, 'v', 'e', 'r', 's', 'i', 'o', 'n', // Query name
		0x04, 'b', 'i', 'n', 'd',
		0x00, // End of name
		0x00, 0x10, // TXT record
		0x00, 0x00, 0x01, 0x00, // IN class
	}

	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	conn.Write(query)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return ""
	}

	// Parse response (simplified)
	if n > 12 {
		return "BIND"
	}
	return ""
}

// tryHTTPVersion attempts to get HTTP version banner
func (s *Scanner) tryHTTPVersion(ctx context.Context, conn net.Conn) string {
	if conn == nil {
		return ""
	}
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	conn.Write([]byte("HEAD / HTTP/1.0\r\nHost: " + conn.RemoteAddr().String() + "\r\n\r\n"))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return ""
	}

	banner := string(buf[:n])
	lines := strings.Split(banner, "\r\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

// ScanService performs comprehensive service detection
func (s *Scanner) ScanService(ctx context.Context, host string, port int) (*ServiceInfo, error) {
	// Use nmap-style service detection if available
	if nmapPath, err := exec.LookPath("nmap"); err == nil {
		return s.detectWithNmap(ctx, nmapPath, host, port)
	}

	// Fallback to basic detection
	service, version := s.detectService(ctx, nil, port, "tcp")
	return &ServiceInfo{
		Name:    service,
		Version: version,
	}, nil
}

// detectWithNmap uses nmap for service detection
func (s *Scanner) detectWithNmap(ctx context.Context, nmapPath, host string, port int) (*ServiceInfo, error) {
	cmd := exec.CommandContext(ctx, nmapPath, "-sV", "-p", fmt.Sprintf("%d", port), "-Pn", host)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	// Parse nmap output
	result := &ServiceInfo{}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Service detection") {
			continue
		}
		if strings.Contains(line, "TCP/IP") {
			continue
		}

		// Look for service info lines like:
		// 80/tcp  open  http    Apache httpd 2.4.41
		parts := strings.Fields(line)
		if len(parts) >= 4 {
			if parts[0] == fmt.Sprintf("%d/tcp", port) || parts[0] == fmt.Sprintf("%d/udp", port) {
				if len(parts) >= 5 {
					result.Name = strings.Trim(parts[2], " ")
					result.Version = strings.Join(parts[3:], " ")
				} else {
					result.Name = parts[2]
				}
				break
			}
		}
	}

	return result, nil
}

// DiscoverTopology discovers network topology using ICMP and ARP
func (s *Scanner) DiscoverTopology(ctx context.Context, subnet string) (*TopologyInfo, error) {
	// Get network interface information
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get interfaces: %w", err)
	}

	info := &TopologyInfo{
		Interfaces: make([]InterfaceInfo, 0, len(ifaces)),
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		ifaceInfo := InterfaceInfo{
			Name:      iface.Name,
			Index:     iface.Index,
			MAC:       iface.HardwareAddr.String(),
			Addresses: make([]AddressInfo, 0, len(addrs)),
		}

		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if ip := ipNet.IP.To4(); ip != nil {
					ifaceInfo.Addresses = append(ifaceInfo.Addresses, AddressInfo{
						IP:        ip.String(),
						Network:   ipNet.String(),
						broadcast: getBroadcastIP(ipNet),
					})
				}
			}
		}

		info.Interfaces = append(info.Interfaces, ifaceInfo)
	}

	return info, nil
}

// getBroadcastIP calculates broadcast address from network
func getBroadcastIP(ipNet *net.IPNet) string {
	ip := ipNet.IP.To4()
	if ip == nil {
		return ""
	}

	broadcast := make([]byte, 4)
	copy(broadcast, ip)
	for i := 0; i < 4; i++ {
		broadcast[i] = ip[i] | ^ipNet.Mask[i]
	}
	return net.IP(broadcast).String()
}

// GetGateways retrieves default gateway information
func (s *Scanner) GetGateways(ctx context.Context) ([]GatewayInfo, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("gateway discovery not supported on %s", runtime.GOOS)
	}

	// Read routing table from /proc/net/route
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil, fmt.Errorf("failed to read routing table: %w", err)
	}

	var gateways []GatewayInfo
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	for _, line := range lines[1:] { // Skip header
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		// Parse destination and gateway
		dest := parts[1]
		gw := parts[2]

		// Check if this is a default route (0.0.0.0)
		if dest == "00000000" {
			if gatewayIP, err := hexToIP(gw); err == nil {
				iface, err := net.InterfaceByIndex(getInterfaceIndex(parts[0]))
				if err == nil {
					gateways = append(gateways, GatewayInfo{
						IP:        gatewayIP,
						Interface: iface.Name,
						IsDefault: true,
					})
				}
			}
		}
	}

	return gateways, nil
}

func hexToIP(hex string) (string, error) {
	val, err := hexToInt(hex)
	if err != nil {
		return "", err
	}

	ip := make([]byte, 4)
	ip[0] = byte(val >> 24)
	ip[1] = byte(val >> 16)
	ip[2] = byte(val >> 8)
	ip[3] = byte(val)
	return net.IP(ip).String(), nil
}

func hexToInt(hex string) (int, error) {
	var val int
	_, err := fmt.Sscanf(hex, "%x", &val)
	return val, err
}

func getInterfaceIndex(name string) int {
	// Try to parse as integer index - handle empty strings safely
	if name == "" {
		return 0
	}
	
	var idx int
	n, err := fmt.Sscanf(name, "%d", &idx)
	if err != nil || n != 1 {
		return 0
	}
	return idx
}

// DiscoverARP performs ARP table scanning
func (s *Scanner) DiscoverARP(ctx context.Context) ([]DiscoveryResult, error) {
	if runtime.GOOS == "linux" {
		return s.parseARPLinux(ctx)
	} else if runtime.GOOS == "windows" {
		return s.parseARPWindows(ctx)
	}

	return nil, fmt.Errorf("ARP discovery not supported on %s", runtime.GOOS)
}

func (s *Scanner) parseARPLinux(ctx context.Context) ([]DiscoveryResult, error) {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return nil, fmt.Errorf("failed to read ARP table: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var results []DiscoveryResult

	for _, line := range lines[1:] { // Skip header
		if ctx.Err() != nil {
			return results, ctx.Err()
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

		// Get hostname
		hostname, _ := net.LookupAddr(ip)

		results = append(results, DiscoveryResult{
			IP:       ip,
			MAC:      normalizeMAC(mac),
			Hostname: func() string { if len(hostname) > 0 { return strings.TrimSuffix(hostname[0], ".") }; return "" }(),
			Type:     "arp",
			Via:      "arp",
			Labels: map[string]string{
				"discovery_method": "arp_table",
			},
		})
	}

	return results, nil
}

func (s *Scanner) parseARPWindows(ctx context.Context) ([]DiscoveryResult, error) {
	cmd := exec.Command("arp", "-a")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run arp -a: %w", err)
	}

	var results []DiscoveryResult
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if ctx.Err() != nil {
			return results, ctx.Err()
		}

		// Windows arp -a output format:
		// Interface: 192.168.1.100 --- 0xb
		//   Internet Address      Physical Address      Type
		//   192.168.1.1           00-11-22-33-44-55     dynamic

		if !strings.HasPrefix(line, "  ") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 3 {
			ip := parts[0]
			mac := parts[1]

			results = append(results, DiscoveryResult{
				IP:       ip,
				MAC:      normalizeWindowsMAC(mac),
				Type:     "arp",
				Via:      "arp",
				Labels: map[string]string{
					"discovery_method": "arp_table",
				},
			})
		}
	}

	return results, nil
}

// PingScan performs ICMP ping scanning
func (s *Scanner) PingScan(ctx context.Context, subnet string, timeout time.Duration) ([]HostInfo, error) {
	hosts, err := s.pingScanSubnet(ctx, subnet, timeout)
	if err != nil {
		return nil, err
	}

	var results []HostInfo
	for _, host := range hosts {
		info, err := s.getHostInfo(ctx, host, timeout)
		if err != nil {
			// Still include the host with minimal info
			results = append(results, HostInfo{
				IP:     host,
				Labels: map[string]string{"pinged": "true"},
			})
			continue
		}
		results = append(results, info)
	}

	return results, nil
}

func (s *Scanner) pingScanSubnet(ctx context.Context, subnet string, timeout time.Duration) ([]string, error) {
	ip, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		// Try parsing as network address
		if netIP := net.ParseIP(subnet); netIP != nil {
			ipNet = &net.IPNet{
				IP:   netIP,
				Mask: net.CIDRMask(24, 32),
			}
		} else {
			return nil, fmt.Errorf("invalid subnet: %s", subnet)
		}
	}

	var hosts []string
	ip = ip.Mask(ipNet.Mask)

	// Get the network and broadcast addresses
	network := ip
	broadcast := make([]byte, 4)
	copy(broadcast, network)
	for i := 0; i < 4; i++ {
		broadcast[i] = network[i] | ^ipNet.Mask[i]
	}

	// Scan all hosts except network and broadcast
	start := binary.BigEndian.Uint32(network)
	end := binary.BigEndian.Uint32(broadcast)

	sem := make(chan struct{}, s.maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for addr := start + 1; addr < end; addr++ {
		select {
		case <-ctx.Done():
			return hosts, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(a uint32) {
			defer wg.Done()
			defer func() { <-sem }()

			ipBytes := make([]byte, 4)
			ipBytes[0] = byte(a >> 24)
			ipBytes[1] = byte(a >> 16)
			ipBytes[2] = byte(a >> 8)
			ipBytes[3] = byte(a)
			hostIP := net.IP(ipBytes).String()

			if s.pingHost(ctx, hostIP, timeout) {
				mu.Lock()
				hosts = append(hosts, hostIP)
				mu.Unlock()
			}
		}(addr)
	}

	wg.Wait()
	return hosts, nil
}

func (s *Scanner) pingHost(ctx context.Context, host string, timeout time.Duration) bool {
	// Try using raw ICMP first
	if conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0"); err == nil {
		defer conn.Close()

		// Create ICMP echo request
		wm := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   os.Getpid() & 0xffff,
				Seq:  1,
				Data: []byte(" Strata RMM"),
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

	// Fallback to using system ping command
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", host)
	err := cmd.Run()
	return err == nil
}

// getHostInfo performs host discovery and information gathering
func (s *Scanner) getHostInfo(ctx context.Context, host string, timeout time.Duration) (HostInfo, error) {
	info := HostInfo{
		IP:     host,
		Labels: map[string]string{"pinged": "true"},
	}

	// Get hostname
	hostname, err := net.LookupAddr(host)
	if err == nil && len(hostname) > 0 {
		info.Hostname = func() string { if len(hostname) > 0 { return strings.TrimSuffix(hostname[0], ".") }; return "" }()
	}

	// Try to discover MAC via ARP
	if mac, err := getMACAddress(ctx, host); err == nil {
		info.MAC = mac
	}

	// Scan common ports
	commonPorts := []int{22, 80, 443, 8080, 3306, 5432, 6379, 21, 23, 53}
	ports, _ := s.ScanPorts(ctx, host, commonPorts, "tcp")
	info.OpenPorts = ports

	// Get service info
	for _, port := range ports {
		if port.State == "open" {
			svc, _ := s.ScanService(ctx, host, port.Port)
			if svc != nil {
				info.Services = append(info.Services, *svc)
			}
		}
	}

	return info, nil
}

// getMACAddress attempts to get MAC address for an IP
func getMACAddress(ctx context.Context, ip string) (string, error) {
	// Try to read ARP table
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

// RunFullDiscovery performs comprehensive network discovery
func (s *Scanner) RunFullDiscovery(ctx context.Context, subnet string) ([]HostInfo, error) {
	// Step 1: Discover live hosts
	hosts, err := s.PingScan(ctx, subnet, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("ping scan failed: %w", err)
	}

	// Step 2: For each live host, scan ports
	for i := range hosts {
		select {
		case <-ctx.Done():
			return hosts, ctx.Err()
		default:
		}

		if hosts[i].Hostname == "" {
			hosts[i].Hostname = hosts[i].IP
		}
	}

	return hosts, nil
}
