package probe

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// SyslogMessage represents a parsed syslog message.
type SyslogMessage struct {
	Priority  int       `json:"priority"`
	Severity  string    `json:"severity"`
	Facility  string    `json:"facility"`
	Hostname  string    `json:"hostname"`
	AppName   string    `json:"app_name"`
	ProcID    string    `json:"proc_id"`
	MessageID string    `json:"message_id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	SourceIP  string    `json:"source_ip"`
	DeviceType string   `json:"device_type"`
}

// SyslogConfig holds the configuration for the syslog collector.
type SyslogConfig struct {
	Port      int           `json:"port"`       // UDP port to listen on (default 514)
	Protocol  string        `json:"protocol"`   // udp or tcp (default udp)
	Timeout   time.Duration `json:"timeout"`    // Read timeout (default 30s)
	Buffer    int           `json:"buffer"`     // Message buffer size (default 1000)
}

// SyslogCollector listens for syslog messages and parses them.
type SyslogCollector struct {
	config   SyslogConfig
	conn     *net.UDPConn
	tcpConn  net.Listener
	mu       sync.Mutex
	messages []*SyslogMessage
	handler  MessageHandler
}

// MessageHandler is called when a new syslog message is received.
type MessageHandler func(ctx context.Context, msg *SyslogMessage)

// NewSyslogCollector creates a new syslog collector.
func NewSyslogCollector(cfg SyslogConfig) *SyslogCollector {
	if cfg.Port == 0 {
		cfg.Port = 514
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "udp"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Buffer == 0 {
		cfg.Buffer = 1000
	}

	return &SyslogCollector{
		config: cfg,
		messages: make([]*SyslogMessage, 0, cfg.Buffer),
	}
}

// Start begins listening for syslog messages.
func (c *SyslogCollector) Start(ctx context.Context) error {
	switch c.config.Protocol {
	case "tcp":
		return c.startTCP(ctx)
	default:
		return c.startUDP(ctx)
	}
}

// StartUDP begins listening for UDP syslog messages.
func (c *SyslogCollector) startUDP(ctx context.Context) error {
	addr := &net.UDPAddr{
		Port: c.config.Port,
		IP:   net.IPv4zero,
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}
	c.conn = conn

	go c.receiveUDP(ctx)
	return nil
}

// StartTCP begins listening for TCP syslog messages.
func (c *SyslogCollector) startTCP(ctx context.Context) error {
	addr := &net.TCPAddr{
		Port: c.config.Port,
		IP:   net.IPv4zero,
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen TCP: %w", err)
	}
	c.tcpConn = listener

	go c.acceptTCP(ctx)
	return nil
}

// ReceiveUDP reads UDP packets and parses syslog messages.
func (c *SyslogCollector) receiveUDP(ctx context.Context) {
	buf := make([]byte, 65535)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_ = c.conn.SetReadDeadline(time.Now().Add(c.config.Timeout))
			n, remoteAddr, err := c.conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				continue
			}

			msg := c.parseSyslogMessage(string(buf[:n]), remoteAddr.String())
			c.storeMessage(msg)

			if c.handler != nil {
				c.handler(ctx, msg)
			}
		}
	}
}

// AcceptTCP accepts TCP connections and reads syslog messages.
func (c *SyslogCollector) acceptTCP(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := c.tcpConn.Accept()
			if err != nil {
				continue
			}
			go c.handleTCPConn(ctx, conn)
		}
	}
}

// HandleTCPConn reads a TCP connection for syslog messages.
func (c *SyslogCollector) handleTCPConn(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	buf := make([]byte, 65535)
	_ = conn.SetReadDeadline(time.Now().Add(c.config.Timeout))
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	msg := c.parseSyslogMessage(string(buf[:n]), conn.RemoteAddr().String())
	c.storeMessage(msg)

	if c.handler != nil {
		c.handler(ctx, msg)
	}
}

// ParseSyslogMessage parses a raw syslog message string.
func (c *SyslogCollector) parseSyslogMessage(raw, sourceIP string) *SyslogMessage {
	msg := &SyslogMessage{
		Timestamp: time.Now().UTC(),
		SourceIP:  sourceIP,
	}

	// Parse priority (PRI) if present
	if strings.HasPrefix(raw, "<") {
		if idx := strings.Index(raw, ">"); idx > 0 {
			priStr := raw[1:idx]
			if pri, err := fmt.Sscanf(priStr, "%d", &msg.Priority); err == nil && pri == 1 {
				raw = raw[idx+1:]
			}
		}
	}

	// Parse syslog header (RFC 3164)
	// Format: <PRI>TIMESTAMP HOSTNAME APP[PROC]: MESSAGE
	if len(raw) > 0 {
		// Try to match RFC 3164 timestamp format: "Aug  4 12:00:00" or "Aug 4 12:00:00"
		// The timestamp is always 15 characters: "Mon DD HH:MM:SS" or "Mon  D HH:MM:SS"
		if len(raw) > 15 {
			// Extract timestamp (first 15 chars)
			msg.Timestamp, _ = time.Parse(time.ANSIC, raw[0:15])
			rest := strings.TrimSpace(raw[15:])

			// Rest should be: "HOSTNAME APP[PROC]: MESSAGE"
			if idx := strings.Index(rest, " "); idx > 0 {
				msg.Hostname = rest[:idx]
				rest = strings.TrimSpace(rest[idx+1:])

				if idx2 := strings.Index(rest, " "); idx2 > 0 {
					appPart := rest[:idx2]
					msg.Content = strings.TrimSpace(rest[idx2+1:])

					appParts := strings.SplitN(appPart, "[", 2)
					if len(appParts) >= 1 {
						msg.AppName = appParts[0]
					}
					if len(appParts) >= 2 {
						// Remove trailing "]" and any trailing content after it
						procID := appParts[1]
						if idx := strings.Index(procID, "]"); idx >= 0 {
							procID = procID[:idx]
						}
						msg.ProcID = procID
					}
				}
			}
		}
	}

	// Determine severity and facility from priority
	if msg.Priority >= 0 {
		msg.Facility = getFacility(msg.Priority)
		msg.Severity = getSeverity(msg.Priority)
	}

	// Classify device type based on app name
	msg.DeviceType = classifyDeviceType(msg.AppName)

	return msg
}

// StoreMessage adds a message to the internal buffer.
func (c *SyslogCollector) storeMessage(msg *SyslogMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	capacity := cap(c.messages)

	// If buffer is full, drop oldest message
	if len(c.messages) >= capacity {
		// Create new slice to avoid capacity issues
		newMessages := make([]*SyslogMessage, 0, capacity)
		newMessages = append(newMessages, c.messages[1:]...)
		c.messages = newMessages
	}

	c.messages = append(c.messages, msg)
}

// GetMessages returns a copy of the stored messages.
func (c *SyslogCollector) GetMessages() []*SyslogMessage {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]*SyslogMessage, len(c.messages))
	copy(result, c.messages)
	return result
}

// Stop closes the collector.
func (c *SyslogCollector) Stop() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	if c.tcpConn != nil {
		_ = c.tcpConn.Close()
	}
}

// GetFacility returns the facility name from syslog priority.
func getFacility(priority int) string {
	// Facility = priority >> 3
	facility := priority >> 3
	switch facility {
	case 0:
		return "kern"
	case 1:
		return "user"
	case 2:
		return "mail"
	case 3:
		return "daemon"
	case 4:
		return "auth"
	case 5:
		return "syslog"
	case 6:
		return "lpr"
	case 7:
		return "news"
	case 8:
		return "uucp"
	case 9:
		return "cron"
	case 10:
		return "authpriv"
	case 11:
		return "ftp"
	case 16:
		return "ntp"
	case 17:
		return "security"
	case 18:
		return "console"
	case 20:
		return "local0"
	case 21:
		return "local1"
	case 22:
		return "local2"
	case 23:
		return "local3"
	case 24:
		return "local4"
	case 25:
		return "local5"
	case 26:
		return "local6"
	case 27:
		return "local7"
	default:
		return "unknown"
	}
}

// GetSeverity returns the severity level from syslog priority.
func getSeverity(priority int) string {
	severity := priority & 0x07
	switch severity {
	case 0:
		return "emergency"
	case 1:
		return "alert"
	case 2:
		return "critical"
	case 3:
		return "error"
	case 4:
		return "warning"
	case 5:
		return "notice"
	case 6:
		return "info"
	case 7:
		return "debug"
	default:
		return "unknown"
	}
}

// ClassifyDeviceType classifies the device type based on the app name.
func classifyDeviceType(appName string) string {
	appLower := strings.ToLower(appName)
	switch {
	case strings.Contains(appLower, "firewall") || strings.Contains(appLower, "forti") || strings.Contains(appLower, "palo") || strings.Contains(appLower, "fw"):
		return "firewall"
	case strings.Contains(appLower, "switch") || strings.Contains(appLower, "sw"):
		return "switch"
	case strings.Contains(appLower, "router") || strings.Contains(appLower, "rt") || strings.Contains(appLower, "rtr"):
		return "router"
	case strings.Contains(appLower, "server") || strings.Contains(appLower, "bmc") || strings.Contains(appLower, "ipmi"):
		return "server"
	case strings.Contains(appLower, "ups") || strings.Contains(appLower, "apc"):
		return "ups"
	case strings.Contains(appLower, "san") || strings.Contains(appLower, "storage"):
		return "storage"
	default:
		return "unknown"
	}
}
