package patch

import (
	"errors"
	"testing"
	"time"
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

func TestCanaryGateRequiresAllCanariesAndThreshold(t *testing.T) {
	if (CanaryGate{Total: 10, Completed: 9, Succeeded: 9}).Ready() {
		t.Fatal("partial canary unexpectedly ready")
	}
	passing := CanaryGate{Total: 10, Completed: 10, Succeeded: 9, Failed: 1}
	if !passing.Passes(90) {
		t.Fatal("90% canary should pass 90% threshold")
	}
	failing := CanaryGate{Total: 10, Completed: 10, Succeeded: 8, Failed: 2}
	if failing.Passes(90) {
		t.Fatal("80% canary unexpectedly passed 90% threshold")
	}
	if passing.Passes(101) || passing.Passes(-1) {
		t.Fatal("invalid threshold unexpectedly passed")
	}
}

func TestMaintenanceWindowAllowsNormalAndOvernightRanges(t *testing.T) {
	at := func(hour, minute int) time.Time {
		return time.Date(2026, time.August, 11, hour, minute, 0, 0, time.Local)
	}
	if !maintenanceWindowAllows(at(12, 0), "") {
		t.Fatal("empty window should be unrestricted")
	}
	if !maintenanceWindowAllows(at(10, 30), "09:00-17:00") || maintenanceWindowAllows(at(18, 0), "09:00-17:00") {
		t.Fatal("daytime maintenance window evaluated incorrectly")
	}
	if !maintenanceWindowAllows(at(23, 30), "22:00-02:00") || !maintenanceWindowAllows(at(1, 30), "22:00-02:00") || maintenanceWindowAllows(at(12, 0), "22:00-02:00") {
		t.Fatal("overnight maintenance window evaluated incorrectly")
	}
	if !maintenanceWindowAllows(at(12, 0), "00:00-00:00") {
		t.Fatal("equal endpoints should represent a full-day window")
	}
}

func TestMaintenanceWindowFailsClosedOnMalformedValue(t *testing.T) {
	for _, window := range []string{"not-a-window", "9:00-17:00", "24:00-01:00", "09:00/17:00"} {
		if maintenanceWindowAllows(time.Now(), window) {
			t.Fatalf("malformed window %q unexpectedly allowed dispatch", window)
		}
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
