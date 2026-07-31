package reconnect

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func TestDelayCapsExponentialBackoff(t *testing.T) {
	base := 5 * time.Second
	maximum := time.Minute
	if got := Delay(base, maximum, 20, failingReader{}); got != maximum {
		t.Fatalf("expected capped delay %s, got %s", maximum, got)
	}
}

func TestDelayUsesFullJitter(t *testing.T) {
	base := 5 * time.Second
	got := Delay(base, time.Minute, 0, bytes.NewReader(make([]byte, 32)))
	if got < 0 || got > base {
		t.Fatalf("delay outside full-jitter range: %s", got)
	}
}

func TestDelayNormalizesInvalidInputs(t *testing.T) {
	got := Delay(0, 0, -1, failingReader{})
	if got != time.Second {
		t.Fatalf("expected normalized one-second delay, got %s", got)
	}
}
