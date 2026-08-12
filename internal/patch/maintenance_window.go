package patch

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidMaintenanceWindow = errors.New("invalid patch maintenance window")
	ErrOutsideMaintenanceWindow = errors.New("outside patch maintenance window")
)

// maintenanceWindowDeadline returns the expiry for work queued now. Empty and
// equal-endpoint windows are treated as unrestricted by the legacy policy
// contract and receive a conservative 72-hour durable-job expiry. Configured
// windows expire exactly at the active window boundary so an offline agent
// cannot execute stale patch work after reconnecting outside the window.
func maintenanceWindowDeadline(now time.Time, window string) (time.Time, error) {
	window = strings.TrimSpace(window)
	if window == "" {
		return now.Add(72 * time.Hour), nil
	}
	parts := strings.Split(window, "-")
	if len(parts) != 2 {
		return time.Time{}, ErrInvalidMaintenanceWindow
	}
	startClock, err := time.Parse("15:04", strings.TrimSpace(parts[0]))
	if err != nil {
		return time.Time{}, ErrInvalidMaintenanceWindow
	}
	endClock, err := time.Parse("15:04", strings.TrimSpace(parts[1]))
	if err != nil {
		return time.Time{}, ErrInvalidMaintenanceWindow
	}
	startMinute := startClock.Hour()*60 + startClock.Minute()
	endMinute := endClock.Hour()*60 + endClock.Minute()
	if startMinute == endMinute {
		return now.Add(72 * time.Hour), nil
	}
	if !maintenanceWindowAllows(now, window) {
		return time.Time{}, ErrOutsideMaintenanceWindow
	}

	end := time.Date(now.Year(), now.Month(), now.Day(), endClock.Hour(), endClock.Minute(), 0, 0, now.Location())
	if startMinute > endMinute && now.Hour()*60+now.Minute() >= startMinute {
		end = end.Add(24 * time.Hour)
	}
	if !end.After(now) {
		return time.Time{}, ErrOutsideMaintenanceWindow
	}
	return end, nil
}
