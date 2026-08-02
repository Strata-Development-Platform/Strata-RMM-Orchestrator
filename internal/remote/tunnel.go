package remote

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type SessionRecorder struct {
	recordingID string
	session     interface{}
	key         string
	writer      interface{}
	started     time.Time
	done        chan struct{}
	uploadKey   string
	checksum    string
	uploadErr   error
	mu          sync.Mutex
	sizeBytes   int64
	logger      *zap.Logger
	backend     interface{}
}

type Protocol string

const (
	ProtocolRDP Protocol = "rdp"
	ProtocolSSH Protocol = "ssh"
	ProtocolVNC Protocol = "vnc"
)

type TunnelState string

const (
	StatePending    TunnelState = "pending"
	StateActive     TunnelState = "active"
	StateClosed     TunnelState = "closed"
	StateFailed     TunnelState = "failed"
)

type TunnelSession struct {
	ID           string       `json:"id"`
	TenantID     string       `json:"tenant_id"`
	DeviceID     string       `json:"device_id"`
	UserID       string       `json:"user_id"`
	Protocol     Protocol     `json:"protocol"`
	TargetPort   int          `json:"target_port"`
	State        TunnelState  `json:"state"`
	CreatedAt    time.Time    `json:"created_at"`
	ClosedAt     *time.Time   `json:"closed_at,omitempty"`
	BytesUp      int64        `json:"bytes_up"`
	BytesDown    int64        `json:"bytes_down"`
}

type Gateway struct {
	nats         *nats.Conn
	logger       *zap.Logger
	addr         string
	recorder     *Recorder
	recStore     *RecordingStore

	mu           sync.RWMutex
	sessions     map[string]*TunnelSession
	activeRelays map[string]*dataRelay
	activeRecs   map[string]interface{}
}

type dataRelay struct {
	sessionID string
	tenantID  string
	deviceID  string
	upSub     *nats.Subscription
	downSub   *nats.Subscription
	conn      net.Conn
	closeOnce sync.Once
	done      chan struct{}
	mu        sync.Mutex
	bytesUp   int64
	bytesDown int64
}

func NewGateway(nc *nats.Conn, addr string, logger *zap.Logger) *Gateway {
	return &Gateway{
		nats:         nc,
		logger:       logger,
		addr:         addr,
		sessions:     make(map[string]*TunnelSession),
		activeRelays: make(map[string]*dataRelay),
		activeRecs:   make(map[string]interface{}),
	}
}

func (g *Gateway) WithRecorder(r *Recorder) *Gateway {
	g.recorder = r
	return g
}

func (g *Gateway) WithRecordingStore(s *RecordingStore) *Gateway {
	g.recStore = s
	return g
}

func (g *Gateway) Start(ctx context.Context) error {
	g.logger.Info("starting remote access gateway", zap.String("listen_addr", g.addr))

	listener, err := net.Listen("tcp", g.addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go g.handleConnection(conn)
		}
	}()

	return nil
}

func (g *Gateway) handleConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	var req struct {
		Action   string   `json:"action"`
		TenantID string   `json:"tenant_id"`
		DeviceID string   `json:"device_id"`
		Protocol Protocol `json:"protocol"`
		Port     int      `json:"port"`
	}

	if err := json.Unmarshal(buf[:n], &req); err != nil {
		conn.Write([]byte(`{"error":"invalid handshake"}`))
		return
	}

	if req.Port == 0 {
		switch req.Protocol {
		case ProtocolRDP:
			req.Port = 3389
		case ProtocolSSH:
			req.Port = 22
		case ProtocolVNC:
			req.Port = 5900
		}
	}

	session := &TunnelSession{
		ID:         uuid.New().String(),
		TenantID:   req.TenantID,
		DeviceID:   req.DeviceID,
		Protocol:   req.Protocol,
		TargetPort: req.Port,
		State:      StatePending,
		CreatedAt:  time.Now(),
	}

	g.mu.Lock()
	g.sessions[session.ID] = session
	g.mu.Unlock()

	handshakeResp, _ := json.Marshal(map[string]string{
		"session_id": session.ID,
		"status":     "connected",
	})
	conn.Write(handshakeResp)

	relay := &dataRelay{
		sessionID: session.ID,
		tenantID:  req.TenantID,
		deviceID:  req.DeviceID,
		conn:      conn,
		done:      make(chan struct{}),
	}

	upSubject := fmt.Sprintf("tenant.%s.tunnel.%s.up", req.TenantID, session.ID)
	downSubject := fmt.Sprintf("tenant.%s.tunnel.%s.down", req.TenantID, session.ID)

	downSub, err := g.nats.Subscribe(downSubject, func(msg *nats.Msg) {
		relay.mu.Lock()
		relay.bytesDown += int64(len(msg.Data))
		relay.mu.Unlock()
		conn.Write(msg.Data)
	})
	if err != nil {
		g.logger.Error("subscribe down subject", zap.Error(err))
		session.State = StateFailed
		return
	}
	relay.downSub = downSub

	session.State = StateActive
	g.mu.Lock()
	g.activeRelays[session.ID] = relay
	g.mu.Unlock()

	// Send tunnel request to agent
	cmdPayload, _ := json.Marshal(map[string]interface{}{
		"type":       "tunnel_open",
		"session_id": session.ID,
		"port":       req.Port,
		"protocol":   req.Protocol,
	})
	cmdSubject := fmt.Sprintf("tenant.%s.cmd.%s", req.TenantID, req.DeviceID)
	if err := g.nats.Publish(cmdSubject, cmdPayload); err != nil {
		g.logger.Error("send tunnel command", zap.Error(err))
	}

	// Start recording if recorder is configured
	var sessionRec interface{}
	if g.recorder != nil {
		frameCh := make(chan []byte, 100)
		close(frameCh)
		rec, err := g.recorder.RecordRaw(context.Background(), &RecordingSession{}, frameCh)
		if err != nil {
			g.logger.Warn("failed to start recording", zap.Error(err))
		} else {
			sessionRec = rec
			g.mu.Lock()
			g.activeRecs[session.ID] = rec
			g.mu.Unlock()
		}
	}

	// Relay data from client to agent via NATS
	buf = make([]byte, 32768)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			break
		}
		data := make([]byte, n)
		copy(data, buf[:n])

		relay.mu.Lock()
		relay.bytesUp += int64(n)
		relay.mu.Unlock()

		if err := g.nats.Publish(upSubject, data); err != nil {
			g.logger.Warn("publish up data", zap.Error(err))
			break
		}
	}

	g.closeSession(session.ID)

	// Finalize recording
	if sessionRec != nil {
		sessionRec := sessionRec.(*SessionRecorder)
		if sessionRec.writer != nil {
			sessionRec.writer.(*io.PipeWriter).Close()
		}
		<-sessionRec.done
	}
}

func (g *Gateway) closeSession(sessionID string) {
	g.mu.Lock()
	relay, ok := g.activeRelays[sessionID]
	if ok {
		delete(g.activeRelays, sessionID)
	}
	rec, hasRec := g.activeRecs[sessionID]
	if hasRec {
		delete(g.activeRecs, sessionID)
	}
	session, hasSession := g.sessions[sessionID]
	g.mu.Unlock()

	if relay != nil {
		relay.close()
	}

	if hasSession {
		now := time.Now()
		session.State = StateClosed
		session.ClosedAt = &now
		if relay != nil {
			session.BytesUp = relay.bytesUp
			session.BytesDown = relay.bytesDown
		}
	}

	if hasRec {
		sessionRec := rec.(*SessionRecorder)
		if sessionRec.writer != nil {
			sessionRec.writer.(*io.PipeWriter).Close()
		}
		<-sessionRec.done
	}
}

func (r *dataRelay) close() {
	r.closeOnce.Do(func() {
		close(r.done)
		if r.upSub != nil {
			r.upSub.Unsubscribe()
		}
		if r.downSub != nil {
			r.downSub.Unsubscribe()
		}
		r.conn.Close()
	})
}

// Agent-side tunnel handler

type AgentTunnel struct {
	nats     *nats.Conn
	logger   *zap.Logger
}

func NewAgentTunnel(nc *nats.Conn, logger *zap.Logger) *AgentTunnel {
	return &AgentTunnel{
		nats:   nc,
		logger: logger,
	}
}

func (at *AgentTunnel) HandleTunnelCommand(msg *nats.Msg, tenantID, agentID string) error {
	var cmd struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
		Port      int    `json:"port"`
		Protocol  string `json:"protocol"`
	}
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		return fmt.Errorf("unmarshal tunnel command: %w", err)
	}

	if cmd.Type != "tunnel_open" {
		return nil
	}

	go at.establishTunnel(tenantID, agentID, cmd.SessionID, cmd.Port)
	return nil
}

func (at *AgentTunnel) establishTunnel(tenantID, agentID, sessionID string, port int) {
	targetAddr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		at.logger.Error("tunnel connect to local service", zap.String("addr", targetAddr), zap.Error(err))
		errorPayload, _ := json.Marshal(map[string]string{
			"type":    "tunnel_error",
			"session": sessionID,
			"error":   err.Error(),
		})
		subject := fmt.Sprintf("tenant.%s.tunnel.%s.down", tenantID, sessionID)
		at.nats.Publish(subject, errorPayload)
		return
	}
	defer conn.Close()

	upSubject := fmt.Sprintf("tenant.%s.tunnel.%s.up", tenantID, sessionID)
	downSubject := fmt.Sprintf("tenant.%s.tunnel.%s.down", tenantID, sessionID)

	// Subscribe to down data (gateway → agent → local service)
	sub, err := at.nats.SubscribeSync(downSubject)
	if err != nil {
		at.logger.Error("subscribe down subject", zap.Error(err))
		return
	}
	defer sub.Unsubscribe()

	// Read from local service and publish up
	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			data := make([]byte, n)
			copy(data, buf[:n])
			at.nats.Publish(upSubject, data)
		}
	}()

	// Read from NATS down and write to local service
	for {
		msg, err := sub.NextMsg(30 * time.Second)
		if err != nil {
			return
		}

		if len(msg.Data) > 0 {
			if _, err := conn.Write(msg.Data); err != nil {
				return
			}
		}
	}
}

// Tunnel stream uses a simple length-prefixed framing for reliability
type framedReader struct {
	reader io.Reader
	buf    []byte
}

func (r *framedReader) ReadFrame() ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r.reader, header); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header)
	if size > 1024*1024 {
		return nil, fmt.Errorf("frame too large: %d", size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r.reader, data); err != nil {
		return nil, err
	}
	return data, nil
}
