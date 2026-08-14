package patch

import (
	"errors"
	"testing"
	"time"
)

func TestMaintenanceWindowDeadlineRejectsMalformedClocks(t *testing.T) {
	now := time.Date(2026, time.August, 14, 10, 0, 0, 0, time.Local)
	for _, window := range []string{
		"9:00-17:00",
		"09:0-17:00",
		"09:00-7:00",
		"09:00-17:0",
		"24:00-01:00",
		"aa:00-17:00",
	} {
		if _, err := maintenanceWindowDeadline(now, window); !errors.Is(err, ErrInvalidMaintenanceWindow) {
			t.Fatalf("deadline for malformed window %q returned %v, want ErrInvalidMaintenanceWindow", window, err)
		}
	}
}

func TestMaintenanceWindowDeadlineAcceptsTrimmedStrictWindow(t *testing.T) {
	now := time.Date(2026, time.August, 14, 10, 0, 0, 0, time.Local)
	deadline, err := maintenanceWindowDeadline(now, " 09:00-17:00 ")
	if err != nil {
		t.Fatalf("valid trimmed maintenance window rejected: %v", err)
	}
	want := time.Date(2026, time.August, 14, 17, 0, 0, 0, time.Local)
	if !deadline.Equal(want) {
		t.Fatalf("deadline = %v, want %v", deadline, want)
	}
}
