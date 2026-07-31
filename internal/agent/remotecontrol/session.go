// Package remotecontrol provides built-in remote desktop capabilities
// without requiring RDP, VNC, or SSH. The agent captures screen frames,
// encodes them as JPEG, and sends them via NATS. Keyboard and mouse input
// is received from NATS and injected into the OS.
package remotecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type SessionState string

const (
	SessionPending SessionState = "pending"
	SessionActive  SessionState = "active"
	SessionClosing SessionState = "closing"
	SessionClosed  SessionState = "closed"
)

type RemoteCommand struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Action    string `json:"action"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Quality   int    `json:"quality,omitempty"`
	FPS       int    `json:"fps,omitempty"`
}

type InputEvent struct {
	Type   string  `json:"type"` // mousemove, mousedown, mouseup, keydown, keyup
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Button int     `json:"button,omitempty"` // 0=left, 1=middle, 2=right
	Key    string  `json:"key,omitempty"`    // key name or code
	Mod    int     `json:"mod,omitempty"`    // bitmask: 1=shift, 2=ctrl, 4=alt, 8=meta
}

type FrameHeader struct {
	SessionID string `json:"session_id"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Seq       int64  `json:"seq"`
	Timestamp int64  `json:"ts"`
}

type Session struct {
	ID         string
	TenantID   string
	DeviceID   string
	State      SessionState
	Width      int
	Height     int
	Quality    int
	FPS        int
	FrameSeq   int64
	LastActive time.Time

	nc       *nats.Conn
	logger   *zap.Logger
	capturer Capturer
	injector InputInjector

	mu       sync.Mutex
	inputSub *nats.Subscription
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

type Capturer interface {
	Init() error
	Capture() (*image.RGBA, error)
	Close() error
}

type InputInjector interface {
	Init() error
	SendMouseMove(x, y float64) error
	SendMouseClick(button int, down bool) error
	SendKey(key string, down bool, mod int) error
	Close() error
}

func NewSession(id, tenantID, deviceID string, nc *nats.Conn, logger *zap.Logger) *Session {
	return &Session{
		ID:       id,
		TenantID: tenantID,
		DeviceID: deviceID,
		State:    SessionPending,
		Width:    1280,
		Height:   720,
		Quality:  70,
		FPS:      5,
		nc:       nc,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

func (s *Session) Start(capturer Capturer, injector InputInjector) error {
	s.mu.Lock()
	s.State = SessionActive
	s.capturer = capturer
	s.injector = injector
	s.mu.Unlock()

	s.logger.Info("remote session started",
		zap.String("session_id", s.ID),
		zap.Int("width", s.Width),
		zap.Int("fps", s.FPS),
	)

	if err := capturer.Init(); err != nil {
		return fmt.Errorf("capturer init: %w", err)
	}
	if err := injector.Init(); err != nil {
		capturer.Close()
		return fmt.Errorf("injector init: %w", err)
	}

	inputSubject := fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.input", s.TenantID, s.DeviceID, s.ID)
	sub, err := s.nc.Subscribe(inputSubject, s.handleInput)
	if err != nil {
		capturer.Close()
		injector.Close()
		return fmt.Errorf("subscribe input: %w", err)
	}
	s.mu.Lock()
	s.inputSub = sub
	s.mu.Unlock()

	ctrlSubject := fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.ctrl", s.TenantID, s.DeviceID, s.ID)
	s.nc.Subscribe(ctrlSubject, s.handleControl)

	s.wg.Add(1)
	go s.frameLoop()

	return nil
}

func (s *Session) Stop() {
	s.mu.Lock()
	if s.State != SessionActive {
		s.mu.Unlock()
		return
	}
	s.State = SessionClosing
	s.mu.Unlock()

	close(s.stopCh)
	s.wg.Wait()

	if s.inputSub != nil {
		s.inputSub.Unsubscribe()
	}
	if s.capturer != nil {
		s.capturer.Close()
	}
	if s.injector != nil {
		s.injector.Close()
	}

	s.mu.Lock()
	s.State = SessionClosed
	s.mu.Unlock()

	s.logger.Info("remote session closed", zap.String("session_id", s.ID))
}

func (s *Session) frameLoop() {
	defer s.wg.Done()

	frameInterval := time.Second / time.Duration(s.FPS)
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	frameSubject := fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.frame", s.TenantID, s.DeviceID, s.ID)

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.sendFrame(frameSubject)
		}
	}
}

func (s *Session) sendFrame(subject string) {
	s.mu.Lock()
	if s.State != SessionActive {
		s.mu.Unlock()
		return
	}
	capturer := s.capturer
	s.mu.Unlock()

	img, err := capturer.Capture()
	if err != nil {
		s.logger.Debug("capture frame", zap.Error(err))
		return
	}

	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: s.Quality}); err != nil {
		s.logger.Debug("encode frame", zap.Error(err))
		return
	}

	s.mu.Lock()
	s.FrameSeq++
	seq := s.FrameSeq
	s.LastActive = time.Now()
	s.mu.Unlock()

	header := FrameHeader{
		SessionID: s.ID,
		Width:     img.Bounds().Dx(),
		Height:    img.Bounds().Dy(),
		Seq:       seq,
		Timestamp: time.Now().UnixMilli(),
	}
	headerJSON, _ := json.Marshal(header)

	msg := append(headerJSON, '\n')
	msg = append(msg, buf.Bytes()...)

	if err := s.nc.Publish(subject, msg); err != nil {
		s.logger.Debug("publish frame", zap.Error(err))
	}
}

func (s *Session) handleInput(msg *nats.Msg) {
	var evt InputEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		return
	}

	s.mu.Lock()
	injector := s.injector
	s.mu.Unlock()

	if injector == nil {
		return
	}

	switch evt.Type {
	case "mousemove":
		injector.SendMouseMove(evt.X, evt.Y)
	case "mousedown":
		injector.SendMouseClick(evt.Button, true)
	case "mouseup":
		injector.SendMouseClick(evt.Button, false)
	case "keydown":
		injector.SendKey(evt.Key, true, evt.Mod)
	case "keyup":
		injector.SendKey(evt.Key, false, evt.Mod)
	}
}

func (s *Session) handleControl(msg *nats.Msg) {
	var cmd RemoteCommand
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch cmd.Action {
	case "resize":
		if cmd.Width > 0 {
			s.Width = cmd.Width
		}
		if cmd.Height > 0 {
			s.Height = cmd.Height
		}
	case "quality":
		if cmd.Quality >= 10 && cmd.Quality <= 100 {
			s.Quality = cmd.Quality
		}
	case "fps":
		if cmd.FPS >= 1 && cmd.FPS <= 30 {
			s.FPS = cmd.FPS
		}
	case "disconnect":
		go s.Stop()
	}
}

type Manager struct {
	nc       *nats.Conn
	logger   *zap.Logger
	tenantID string
	agentID  string
	mu       sync.Mutex
	sessions map[string]*Session
	sub      *nats.Subscription
}

func NewManager(nc *nats.Conn, logger *zap.Logger, tenantID, agentID string) *Manager {
	return &Manager{
		nc:       nc,
		logger:   logger,
		tenantID: tenantID,
		agentID:  agentID,
		sessions: make(map[string]*Session),
	}
}

func (m *Manager) Start(ctx context.Context) error {
	subject := fmt.Sprintf("tenant.%s.cmd.%s", m.tenantID, m.agentID)
	var err error
	m.sub, err = m.nc.Subscribe(subject, func(msg *nats.Msg) {
		var cmd struct {
			Type      string `json:"type"`
			SessionID string `json:"session_id"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			Quality   int    `json:"quality"`
			FPS       int    `json:"fps"`
		}
		if err := json.Unmarshal(msg.Data, &cmd); err != nil {
			return
		}

		switch cmd.Type {
		case "remote_start":
			go m.startSession(cmd.SessionID, cmd.Width, cmd.Height, cmd.Quality, cmd.FPS)
		case "remote_stop":
			m.stopSession(cmd.SessionID)
		}
	})
	return err
}

func (m *Manager) Stop() {
	if m.sub != nil {
		m.sub.Unsubscribe()
	}
	m.mu.Lock()
	for id, s := range m.sessions {
		s.Stop()
		delete(m.sessions, id)
	}
	m.mu.Unlock()
}

func (m *Manager) startSession(sessionID string, width, height, quality, fps int) {
	m.mu.Lock()
	if _, exists := m.sessions[sessionID]; exists {
		m.mu.Unlock()
		return
	}

	session := NewSession(sessionID, m.tenantID, m.agentID, m.nc, m.logger)
	if width > 0 {
		session.Width = width
	}
	if height > 0 {
		session.Height = height
	}
	if quality >= 10 {
		session.Quality = quality
	}
	if fps >= 1 {
		session.FPS = fps
	}
	m.sessions[sessionID] = session
	m.mu.Unlock()

	capturer := NewCapturer()
	injector := NewInjector()

	if err := session.Start(capturer, injector); err != nil {
		m.logger.Error("start remote session", zap.Error(err))
		m.mu.Lock()
		delete(m.sessions, sessionID)
		m.mu.Unlock()
		return
	}

	pubSubj := fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.ctrl", m.tenantID, m.agentID, sessionID)
	m.nc.Publish(pubSubj, []byte(`{"action":"started"}`))
}

func (m *Manager) stopSession(sessionID string) {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	if ok {
		session.Stop()
	}
}
