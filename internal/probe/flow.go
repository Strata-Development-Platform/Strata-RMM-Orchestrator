package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
)

type FlowRecord struct {
	Time       time.Time `json:"time"`
	SrcIP      string    `json:"src_ip"`
	DstIP      string    `json:"dst_ip"`
	SrcPort    int       `json:"src_port"`
	DstPort    int       `json:"dst_port"`
	Protocol   string    `json:"protocol"`
	Bytes      int64     `json:"bytes"`
	Packets    int64     `json:"packets"`
	DurationMs int       `json:"duration_ms"`
}

type FlowCollector struct {
	probe     *Probe
	port      int
	protocols []string
	conn      *net.UDPConn
}

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

			// Basic flow decoding - extract src/dst IP + port from raw
			// Full NetFlow/IPFIX parsing with tehmaze/netflow would go here
			record := fc.parseFlow(data, addr)
			if record != nil {
				fc.publishFlow(*record)
			}
		}
	}
}

func (fc *FlowCollector) Stop() {
	if fc.conn != nil {
		fc.conn.Close()
	}
}

func (fc *FlowCollector) parseFlow(data []byte, addr *net.UDPAddr) *FlowRecord {
	if len(data) < 4 {
		return nil
	}

	// Check for NetFlow v5 header (first 2 bytes = version)
	version := int(data[0])<<8 | int(data[1])
	if version < 5 || version > 10 {
		return nil
	}

	_ = addr
	return &FlowRecord{
		Time:     time.Now().UTC(),
		Protocol: fmt.Sprintf("netflow-v%d", version),
	}
}

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
		"bytes":       record.Bytes,
		"packets":     record.Packets,
		"duration_ms": record.DurationMs,
	}
	data, _ := json.Marshal(payload)
	subject := fmt.Sprintf("tenant.%s.probe.%s.flow", fc.probe.TenantID, fc.probe.ID)
	if err := fc.probe.NATS.Publish(subject, data); err != nil {
		fc.probe.Logger.Warn("publish flow record", zap.Error(err))
	}
}
