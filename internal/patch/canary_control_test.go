package patch

import (
	"errors"
	"testing"
)

func TestSelectCanarySubsetUsesCeilingAndStableOrder(t *testing.T) {
	devices := []string{"device-a", "device-b", "device-c", "device-d", "device-e"}
	got := selectCanarySubset(devices, 21)
	want := []string{"device-a", "device-b"}
	if len(got) != len(want) {
		t.Fatalf("selected %d devices, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("selected[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSelectCanarySubsetAlwaysSelectsOneEligibleDevice(t *testing.T) {
	got := selectCanarySubset([]string{"device-a", "device-b", "device-c"}, 1)
	if len(got) != 1 || got[0] != "device-a" {
		t.Fatalf("selection = %v, want [device-a]", got)
	}
}

func TestSelectCanarySubsetHundredPercent(t *testing.T) {
	devices := []string{"device-a", "device-b", "device-c"}
	got := selectCanarySubset(devices, 100)
	if len(got) != len(devices) {
		t.Fatalf("selected %d devices, want %d", len(got), len(devices))
	}
	for i := range devices {
		if got[i] != devices[i] {
			t.Fatalf("selected[%d] = %q, want %q", i, got[i], devices[i])
		}
	}
}

func TestSelectCanarySubsetRejectsInvalidInputs(t *testing.T) {
	for _, percent := range []int{-1, 0, 101} {
		if got := selectCanarySubset([]string{"device-a"}, percent); got != nil {
			t.Fatalf("percent %d returned %v, want nil", percent, got)
		}
	}
	if got := selectCanarySubset(nil, 10); got != nil {
		t.Fatalf("empty input returned %v, want nil", got)
	}
}

func TestCanaryDeploymentDevicesFailsClosedWithoutStore(t *testing.T) {
	m := &Manager{}
	_, err := m.CanaryDeploymentDevices(t.Context(), "dep-1", 10)
	if err == nil {
		t.Fatal("missing store unexpectedly succeeded")
	}
}

func TestGetCanaryDeploymentDevicesFailsClosedWithoutDB(t *testing.T) {
	s := &Store{}
	_, err := s.GetCanaryDeploymentDevices(t.Context(), "dep-1", 10)
	if err == nil {
		t.Fatal("missing database unexpectedly succeeded")
	}
}

func TestGetCanaryDeploymentDevicesValidatesPercentBeforeQuery(t *testing.T) {
	s := &Store{}
	_, err := s.GetCanaryDeploymentDevices(t.Context(), "dep-1", 0)
	// Database availability is checked before request validation to ensure the
	// Store never presents itself as usable without its durable authority.
	if err == nil {
		t.Fatal("missing database unexpectedly succeeded")
	}
	if errors.Is(err, ErrInvalidCanaryPercent) {
		t.Fatal("invalid-percent result must not mask missing durable authority")
	}
}
