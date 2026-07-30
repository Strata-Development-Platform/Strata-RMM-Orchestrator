package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
)

type RecordingFormat string

const (
	FormatMKV RecordingFormat = "mkv"
	FormatMP4 RecordingFormat = "mp4"
	FormatRaw RecordingFormat = "raw"
)

type RecordingResult struct {
	RecordingID    string          `json:"recording_id"`
	SessionID      string          `json:"session_id"`
	TenantID       string          `json:"tenant_id"`
	DeviceID       string          `json:"device_id"`
	UserID         string          `json:"user_id"`
	StorageKey     string          `json:"storage_key"`
	SizeBytes      int64           `json:"size_bytes"`
	Duration       time.Duration   `json:"duration_ms"`
	Format         RecordingFormat `json:"format"`
	ChecksumSHA256 string          `json:"checksum_sha256"`
	StorageBackend string          `json:"storage_backend"`
}

type RecorderMode int

const (
	ModeRaw RecorderMode = iota
	ModeVideo
)

type Recorder struct {
	backend storage.Backend
	logger  *zap.Logger
	mode    RecorderMode
	prefix  string
}

func NewRecorder(backend storage.Backend, logger *zap.Logger) *Recorder {
	return &Recorder{
		backend: backend,
		logger:  logger,
		mode:    ModeRaw,
		prefix:  "recordings",
	}
}

func (r *Recorder) WithMode(mode RecorderMode) *Recorder {
	r.mode = mode
	return r
}

func (r *Recorder) RecordRaw(ctx context.Context, session *TunnelSession) (*SessionRecorder, error) {
	recordingID := uuid.New().String()
	key := fmt.Sprintf("%s/%s/%s/%s.raw",
		r.prefix, session.TenantID, session.DeviceID, recordingID)

	pr, pw := io.Pipe()

	sr := &SessionRecorder{
		recordingID: recordingID,
		session:     session,
		key:         key,
		writer:      pw,
		started:     time.Now(),
		done:        make(chan struct{}),
		logger:      r.logger,
		backend:     r.backend,
	}

	go func() {
		defer close(sr.done)
		h := sha256.New()
		hashedReader := io.TeeReader(pr, h)

		uploadedKey, err := r.backend.Upload(ctx, key, hashedReader, storage.UploadOptions{
			ContentType: "application/octet-stream",
			Metadata: map[string]string{
				"tenant_id":    session.TenantID,
				"device_id":    session.DeviceID,
				"session_id":   session.ID,
				"recording_id": recordingID,
				"type":         "session-raw",
			},
		})
		if err != nil {
			sr.uploadErr = fmt.Errorf("upload: %w", err)
			pr.Close()
			return
		}

		pr.Close()
		sr.uploadKey = uploadedKey
		sr.checksum = hex.EncodeToString(h.Sum(nil))
	}()

	return sr, nil
}

func (r *Recorder) RecordVideo(ctx context.Context, session *TunnelSession, stdin io.Reader) (*RecordingResult, error) {
	recordingID := uuid.New().String()
	key := fmt.Sprintf("%s/%s/%s/%s.%s",
		r.prefix, session.TenantID, session.DeviceID, recordingID, FormatMKV)

	pr, pw := io.Pipe()

	uploadCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type uploadRes struct {
		key   string
		cksum string
		err   error
	}

	uploadCh := make(chan uploadRes, 1)

	go func() {
		h := sha256.New()
		hashedReader := io.TeeReader(pr, h)

		uploadedKey, err := r.backend.Upload(uploadCtx, key, hashedReader, storage.UploadOptions{
			ContentType: "video/x-matroska",
			Metadata: map[string]string{
				"tenant_id":    session.TenantID,
				"device_id":    session.DeviceID,
				"session_id":   session.ID,
				"recording_id": recordingID,
				"type":         "session-video",
			},
		})
		if err != nil {
			uploadCh <- uploadRes{err: fmt.Errorf("upload: %w", err)}
			return
		}
		pr.Close()
		uploadCh <- uploadRes{key: uploadedKey, cksum: hex.EncodeToString(h.Sum(nil))}
	}()

	cmd := exec.CommandContext(ctx,
		"ffmpeg",
		"-y",
		"-f", "rawvideo",
		"-pix_fmt", "yuv420p",
		"-s", "1920x1080",
		"-r", "30",
		"-i", "pipe:0",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-f", "matroska",
		"pipe:1",
	)
	cmd.Stdin = stdin
	cmd.Stdout = pw
	cmd.Stderr = os.Stderr

	start := time.Now()

	if err := cmd.Run(); err != nil {
		pw.Close()
		cancel()
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	pw.Close()

	res := <-uploadCh
	if res.err != nil {
		return nil, res.err
	}

	return &RecordingResult{
		RecordingID:    recordingID,
		SessionID:      session.ID,
		TenantID:       session.TenantID,
		DeviceID:       session.DeviceID,
		UserID:         session.UserID,
		StorageKey:     res.key,
		Duration:       time.Since(start),
		Format:         FormatMKV,
		ChecksumSHA256: res.cksum,
	}, nil
}

type SessionRecorder struct {
	recordingID string
	session     *TunnelSession
	key         string
	writer      *io.PipeWriter
	started     time.Time
	done        chan struct{}
	uploadKey   string
	checksum    string
	uploadErr   error
	mu          sync.Mutex
	sizeBytes   int64
	logger      *zap.Logger
	backend     storage.Backend
}

func (sr *SessionRecorder) Write(p []byte) (int, error) {
	n, err := sr.writer.Write(p)
	if err == nil {
		sr.mu.Lock()
		sr.sizeBytes += int64(n)
		sr.mu.Unlock()
	}
	return n, err
}

func (sr *SessionRecorder) Stop() *RecordingResult {
	sr.writer.Close()
	<-sr.done

	if sr.uploadErr != nil {
		sr.logger.Error("recording upload failed", zap.Error(sr.uploadErr))
		return nil
	}

	return &RecordingResult{
		RecordingID:    sr.recordingID,
		SessionID:      sr.session.ID,
		TenantID:       sr.session.TenantID,
		DeviceID:       sr.session.DeviceID,
		UserID:         sr.session.UserID,
		StorageKey:     sr.uploadKey,
		SizeBytes:      sr.sizeBytes,
		Duration:       time.Since(sr.started),
		Format:         FormatRaw,
		ChecksumSHA256: sr.checksum,
	}
}

type RecorderConfig struct {
	Format        RecordingFormat `yaml:"format"`
	Width         int             `yaml:"width"`
	Height        int             `yaml:"height"`
	FPS           int             `yaml:"fps"`
	Preset        string          `yaml:"preset"`
	RetentionDays int             `yaml:"retention_days"`
}

func DefaultRecorderConfig() RecorderConfig {
	return RecorderConfig{
		Format:        FormatRaw,
		Width:         1920,
		Height:        1080,
		FPS:           30,
		Preset:        "ultrafast",
		RetentionDays: 90,
	}
}
