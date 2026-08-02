package remote

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
)

type RecordingFormat string

const (
	FormatWebM RecordingFormat = "webm"
	FormatMP4  RecordingFormat = "mp4"
	FormatMKV  RecordingFormat = "mkv"
	FormatRaw  RecordingFormat = "raw"
	FormatJPEG RecordingFormat = "jpeg-sequence"
)

type RecordingConfig struct {
	Format        RecordingFormat `yaml:"format"`
	FrameRate     int             `yaml:"frame_rate"`
	Width         int             `yaml:"width"`
	Height        int             `yaml:"height"`
	Preset        string          `yaml:"preset"`
	Bitrate       string          `yaml:"bitrate"`
	Compression   string          `yaml:"compression"`
	RetentionDays int             `yaml:"retention_days"`
}

type RecordingSession struct {
	ID             string          `json:"id"`
	SessionID      string          `json:"session_id"`
	TenantID       string          `json:"tenant_id"`
	DeviceID       string          `json:"device_id"`
	UserID         string          `json:"user_id"`
	StorageKey     string          `json:"storage_key"`
	SizeBytes      int64           `json:"size_bytes"`
	DurationMs     int64           `json:"duration_ms"`
	Format         RecordingFormat `json:"format"`
	ChecksumSHA256 string          `json:"checksum_sha256"`
	FrameCount     int64           `json:"frame_count"`
	FrameRate      int             `json:"frame_rate"`
	Width          int             `json:"width"`
	Height         int             `json:"height"`
	StartTime      time.Time       `json:"start_time"`
	EndTime        *time.Time      `json:"end_time,omitempty"`
	FrameWidth     int             `json:"frame_width"`
	FrameHeight    int             `json:"frame_height"`
}

type Recorder struct {
	backend storage.Backend
	logger  *zap.Logger
	mode    RecorderMode
	prefix  string
	config  RecordingConfig
	mu      sync.Mutex
}

type RecorderMode int

const (
	ModeRaw RecorderMode = iota
	ModeVideo
)

func NewRecorder(backend storage.Backend, logger *zap.Logger) *Recorder {
	return &Recorder{
		backend: backend,
		logger:  logger,
		mode:    ModeRaw,
		prefix:  "recordings",
		config:  DefaultRecordingConfig(),
	}
}

func (r *Recorder) WithMode(mode RecorderMode) *Recorder {
	r.mode = mode
	return r
}

func (r *Recorder) WithConfig(config RecordingConfig) *Recorder {
	r.config = config
	return r
}

func (r *Recorder) RecordRaw(ctx context.Context, session *RecordingSession, frameCh <-chan []byte) (*RecordingSession, error) {
	key := fmt.Sprintf("%s/%s/%s/%s.raw",
		r.prefix, session.TenantID, session.DeviceID, session.ID)

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		h := newHash()
		var totalBytes int64

		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-frameCh:
				if !ok {
					return
				}
				if _, err := pw.Write(frame); err != nil {
					r.logger.Error("write frame", zap.Error(err))
					return
				}
				h.Write(frame)
				totalBytes += int64(len(frame))
				session.SizeBytes = totalBytes
			}
		}
	}()

	uploadedKey, err := r.backend.Upload(ctx, key, pr, storage.UploadOptions{
		ContentType: "application/octet-stream",
		Metadata: map[string]string{
			"tenant_id":    session.TenantID,
			"device_id":    session.DeviceID,
			"session_id":   session.SessionID,
			"recording_id": session.ID,
			"type":         "session-raw",
			"format":       string(FormatRaw),
		},
	})
	if err != nil {
		pr.Close()
		return nil, fmt.Errorf("upload: %w", err)
	}

	endTime := time.Now()
	session.EndTime = &endTime
	session.DurationMs = session.EndTime.Sub(session.StartTime).Milliseconds()
	session.StorageKey = uploadedKey
	session.ChecksumSHA256 = ""

	return session, nil
}

func (r *Recorder) RecordVideo(ctx context.Context, session *RecordingSession, frameCh <-chan []byte) (*RecordingSession, error) {
	key := fmt.Sprintf("%s/%s/%s/%s.webm",
		r.prefix, session.TenantID, session.DeviceID, session.ID)

	pr, pw := io.Pipe()

	type uploadResult struct {
		key   string
		cksum string
		err   error
	}

	uploadCh := make(chan uploadResult, 1)

	go func() {
		h := newHash()
		hashReader := io.TeeReader(pr, h)

		uploadedKey, err := r.backend.Upload(ctx, key, hashReader, storage.UploadOptions{
			ContentType: "video/webm",
			Metadata: map[string]string{
				"tenant_id":    session.TenantID,
				"device_id":    session.DeviceID,
				"session_id":   session.SessionID,
				"recording_id": session.ID,
				"type":         "session-video",
				"format":       string(FormatWebM),
			},
		})
		if err != nil {
			uploadCh <- uploadResult{err: fmt.Errorf("upload: %w", err)}
			return
		}
		pr.Close()
		uploadCh <- uploadResult{key: uploadedKey, cksum: fmt.Sprintf("%x", h.Sum(nil))}
	}()

	session.EndTime = &time.Time{}
	session.DurationMs = session.EndTime.Sub(session.StartTime).Milliseconds()

	pw.Close()

	res := <-uploadCh
	if res.err != nil {
		return nil, res.err
	}

	session.StorageKey = res.key
	session.ChecksumSHA256 = res.cksum

	return session, nil
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return nil
}

func DefaultRecordingConfig() RecordingConfig {
	return RecordingConfig{
		Format:        FormatWebM,
		FrameRate:     30,
		Width:         1920,
		Height:        1080,
		Preset:        "ultrafast",
		Bitrate:       "5M",
		Compression:   "libvpx-vp9",
		RetentionDays: 90,
	}
}

type hash interface {
	Write(p []byte) (n int, err error)
	Sum(b []byte) []byte
	Reset()
	Size() int
	BlockSize() int
}

func newHash() hash {
	return &sha256Hash{}
}

type sha256Hash struct {
	data []byte
}

func (h *sha256Hash) Write(p []byte) (n int, err error) {
	h.data = append(h.data, p...)
	return len(p), nil
}

func (h *sha256Hash) Sum(b []byte) []byte {
	result := make([]byte, len(h.data))
	copy(result, h.data)
	return result
}

func (h *sha256Hash) Reset() {
	h.data = h.data[:0]
}

func (h *sha256Hash) Size() int {
	return len(h.data)
}

func (h *sha256Hash) BlockSize() int {
	return 1
}
