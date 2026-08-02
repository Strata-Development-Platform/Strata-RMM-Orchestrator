package probe

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
)

// FlowRecord represents a network flow record
type FlowRecord struct {
	Time       time.Time `json:"time"`
	SrcIP      string    `json:"src_ip"`
	DstIP      string    `json:"dst_ip"`
	SrcPort    int       `json:"src_port"`
	DstPort    int       `json:"dst_port"`
	Protocol   string    `json:"protocol"`
	ProtocolID uint8     `json:"protocol_id"`
	Bytes      int64     `json:"bytes"`
	Packets    int64     `json:"packets"`
	DurationMs uint32    `json:"duration_ms"`
	Flags      uint8     `json:"flags,omitempty"`
	InputIf    uint16    `json:"input_if,omitempty"`
	OutputIf   uint16    `json:"output_if,omitempty"`
	SrcMAC     string    `json:"src_mac,omitempty"`
	DstMAC     string    `json:"dst_mac,omitempty"`
	SrcAS      uint32    `json:"src_as,omitempty"`
	DstAS      uint32    `json:"dst_as,omitempty"`
	NextHop    string    `json:"next_hop,omitempty"`
	User       string    `json:"user,omitempty"`
	URL        string    `json:"url,omitempty"`
	Labels     []string  `json:"labels,omitempty"`
}

// FlowCollector handles flow collection (NetFlow, sFlow, IPFIX)
type FlowCollector struct {
	probe     *Probe
	port      int
	protocols []string
	conn      *net.UDPConn
}

// NewFlowCollector creates a new flow collector
func NewFlowCollector(p *Probe, port int, protocols []string) *FlowCollector {
	if port == 0 {
		port = 2055
	}
	if len(protocols) == 0 {
		protocols = []string{"netflow9", "ipfix"}
	}
	return &FlowCollector{
		probe:     p,
		port:      port,
		protocols: protocols,
	}
}

// Start begins listening for flow records
func (fc *FlowCollector) Start(ctx context.Context) {
	addr := &net.UDPAddr{Port: fc.port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fc.probe.Logger.Error("flow collector listen", zap.Error(err))
		return
	}
	fc.conn = conn
	fc.probe.Logger.Info("flow collector listening", zap.Int("port", fc.port))

	buf := make([]byte, 65535)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
				continue
			}
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}

			data := make([]byte, n)
			copy(data, buf[:n])

			// Parse flow based on protocol
			if record := fc.parseFlow(data, addr); record != nil {
				fc.publishFlow(*record)
			}
		}
	}
}

// Stop stops the flow collector
func (fc *FlowCollector) Stop() {
	if fc.conn != nil {
		fc.conn.Close()
	}
}

// parseFlow parses flow records from raw data
func (fc *FlowCollector) parseFlow(data []byte, addr *net.UDPAddr) *FlowRecord {
	if len(data) < 4 {
		return nil
	}

	// Check for NetFlow v5 header (first 2 bytes = version)
	version := int(data[0])<<8 | int(data[1])

	switch version {
	case 5:
		return fc.parseNetFlowV5(data)
	case 9:
		return fc.parseNetFlowV9(data)
	case 10:
		return fc.parseIPFIX(data)
	default:
		// Try to parse as custom format or unknown
		return fc.parseUnknownFlow(data, addr)
	}
}

// parseNetFlowV5 parses NetFlow v5 records
func (fc *FlowCollector) parseNetFlowV5(data []byte) *FlowRecord {
	if len(data) < 48 {
		return nil
	}

	// NetFlow v5 header (24 bytes)
	count := int(binary.BigEndian.Uint16(data[2:4]))
	if count == 0 || count > 255 {
		return nil
	}

	// Parse first flow record (28 bytes each)
	recordOffset := 24
	if len(data) < recordOffset+28 {
		return nil
	}

	recordData := data[recordOffset : recordOffset+28]

	srcIP := net.IP(recordData[0:4]).String()
	dstIP := net.IP(recordData[4:8]).String()

	// Extract port and protocol info
	srcPort := int(binary.BigEndian.Uint16(recordData[8:10]))
	dstPort := int(binary.BigEndian.Uint16(recordData[10:12]))
	protocol := recordData[13]
	flags := recordData[14]

	// Parse timestamp and counters
	msecFirst := binary.BigEndian.Uint32(recordData[16:20])
	msecLast := binary.BigEndian.Uint32(recordData[20:24])
	bytes := int64(binary.BigEndian.Uint32(recordData[24:28]))
	packets := int64(binary.BigEndian.Uint32(recordData[28:32]))

	return &FlowRecord{
		Time:       time.Now().UTC(),
		SrcIP:      srcIP,
		DstIP:      dstIP,
		SrcPort:    srcPort,
		DstPort:    dstPort,
		Protocol:   protocolName(protocol),
		ProtocolID: protocol,
		Bytes:      bytes,
		Packets:    packets,
		DurationMs: uint32(msecLast - msecFirst),
		Flags:      flags,
	}
}

// parseNetFlowV9 parses NetFlow v9 records
func (fc *FlowCollector) parseNetFlowV9(data []byte) *FlowRecord {
	if len(data) < 20 {
		return nil
	}

	// NetFlow v9 header (20 bytes)
	count := int(binary.BigEndian.Uint16(data[2:4]))
	if count == 0 {
		return nil
	}

	// Parse flowset
	flowsetID := binary.BigEndian.Uint16(data[8:10])

	if flowsetID == 0 || flowsetID == 1 {
		// Template or options flowset - skip for now
		return nil
	}

	// Data flowset - parse flow record
	// This is simplified; full implementation would use templates
	if len(data) < 24 {
		return nil
	}

	// Try to extract IP and port information from data
	recordData := data[20:52]

	if len(recordData) >= 8 {
		srcIP := net.IP(recordData[0:4]).String()
		dstIP := net.IP(recordData[4:8]).String()

		if len(recordData) >= 12 {
			srcPort := int(binary.BigEndian.Uint16(recordData[8:10]))
			dstPort := int(binary.BigEndian.Uint16(recordData[10:12]))

			protocol := uint8(recordData[13])

			if len(recordData) >= 28 {
				bytes := int64(binary.BigEndian.Uint32(recordData[24:28]))
				packets := int64(binary.BigEndian.Uint32(recordData[28:32]))

				return &FlowRecord{
					Time:       time.Now().UTC(),
					SrcIP:      srcIP,
					DstIP:      dstIP,
					SrcPort:    srcPort,
					DstPort:    dstPort,
					Protocol:   protocolName(protocol),
					ProtocolID: protocol,
					Bytes:      bytes,
					Packets:    packets,
				}
			}
		}
	}

	return nil
}

// parseIPFIX parses IPFIX (NetFlow v10) records
func (fc *FlowCollector) parseIPFIX(data []byte) *FlowRecord {
	if len(data) < 10 {
		return nil
	}

	// IPFIX header (10 bytes)
	version := binary.BigEndian.Uint16(data[0:2])
	if version != 10 {
		return nil
	}

	count := int(binary.BigEndian.Uint16(data[4:6]))
	if count == 0 {
		return nil
	}

	// Parse flowset
	flowsetID := binary.BigEndian.Uint16(data[8:10])

	// Template flowset (ID 2)
	if flowsetID == 2 {
		return nil
	}

	// Data flowset
	if len(data) < 24 {
		return nil
	}

	recordData := data[20:52]

	if len(recordData) >= 8 {
		srcIP := net.IP(recordData[0:4]).String()
		dstIP := net.IP(recordData[4:8]).String()

		if len(recordData) >= 12 {
			srcPort := int(binary.BigEndian.Uint16(recordData[8:10]))
			dstPort := int(binary.BigEndian.Uint16(recordData[10:12]))

			protocol := uint8(recordData[13])

			if len(recordData) >= 28 {
				bytes := int64(binary.BigEndian.Uint32(recordData[24:28]))
				packets := int64(binary.BigEndian.Uint32(recordData[28:32]))

				return &FlowRecord{
					Time:       time.Now().UTC(),
					SrcIP:      srcIP,
					DstIP:      dstIP,
					SrcPort:    srcPort,
					DstPort:    dstPort,
					Protocol:   protocolName(protocol),
					ProtocolID: protocol,
					Bytes:      bytes,
					Packets:    packets,
				}
			}
		}
	}

	return nil
}

// parseUnknownFlow attempts to parse unknown flow formats
func (fc *FlowCollector) parseUnknownFlow(data []byte, addr *net.UDPAddr) *FlowRecord {
	// Try to extract IP addresses from raw bytes
	// Look for common patterns in first 64 bytes

	if len(data) < 20 {
		return nil
	}

	// Try to find IPv4 addresses (common at offsets 0-4 and 4-8)
	for i := 0; i <= 16; i += 4 {
		if i+8 > len(data) {
			break
		}

		// Check if bytes look like valid IP addresses
		srcIP := net.IP(data[i : i+4])
		dstIP := net.IP(data[i+4 : i+8])

		if srcIP.To4() != nil && dstIP.To4() != nil {
			// Likely IPv4 addresses
			srcPort := int(binary.BigEndian.Uint16(data[i+8 : i+10]))
			dstPort := int(binary.BigEndian.Uint16(data[i+10 : i+12]))
			protocol := uint8(data[i+9])

			bytes := int64(binary.BigEndian.Uint32(data[i+12 : i+16]))

			return &FlowRecord{
				Time:     time.Now().UTC(),
				SrcIP:    srcIP.String(),
				DstIP:    dstIP.String(),
				SrcPort:  srcPort,
				DstPort:  dstPort,
				Protocol: protocolName(protocol),
				Bytes:    bytes,
			}
		}
	}

	return nil
}

// protocolName returns the name of a protocol given its number
func protocolName(proto uint8) string {
	switch proto {
	case 1:
		return "icmp"
	case 6:
		return "tcp"
	case 17:
		return "udp"
	case 41:
		return "ipv6"
	case 47:
		return "gre"
	case 50:
		return "esp"
	case 51:
		return "ah"
	case 89:
		return "ospf"
	case 112:
		return "vrrp"
	default:
		return fmt.Sprintf("proto_%d", proto)
	}
}

// publishFlow sends flow records via NATS
func (fc *FlowCollector) publishFlow(record FlowRecord) {
	payload := map[string]interface{}{
		"probe_id":    fc.probe.ID,
		"tenant_id":   fc.probe.TenantID,
		"time":        record.Time,
		"src_ip":      record.SrcIP,
		"dst_ip":      record.DstIP,
		"src_port":    record.SrcPort,
		"dst_port":    record.DstPort,
		"protocol":    record.Protocol,
		"protocol_id": record.ProtocolID,
		"bytes":       record.Bytes,
		"packets":     record.Packets,
		"duration_ms": record.DurationMs,
		"flags":       record.Flags,
		"input_if":    record.InputIf,
		"output_if":   record.OutputIf,
	}

	// Add optional fields if present
	if record.SrcMAC != "" {
		payload["src_mac"] = record.SrcMAC
	}
	if record.DstMAC != "" {
		payload["dst_mac"] = record.DstMAC
	}
	if record.NextHop != "" {
		payload["next_hop"] = record.NextHop
	}

	data, err := json.Marshal(payload)
	if err != nil {
		fc.probe.Logger.Warn("failed to marshal flow record", zap.Error(err))
		return
	}

	subject := fmt.Sprintf("tenant.%s.probe.%s.flow", fc.probe.TenantID, fc.probe.ID)
	if err := fc.probe.NATS.Publish(subject, data); err != nil {
		fc.probe.Logger.Warn("publish flow record", zap.Error(err))
	}
}
