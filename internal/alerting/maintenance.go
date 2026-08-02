package alerting

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type MaintenanceWindow struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	DeviceIDs []string  `json:"device_ids,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type MaintenanceEngine struct {
	mu      sync.RWMutex
	windows map[string]*MaintenanceWindow
	store   maintenanceStore
	now     func() time.Time
}

type maintenanceStore interface {
	SaveMaintenanceWindow(ctx context.Context, window *MaintenanceWindow) error
	ListMaintenanceWindows(ctx context.Context, tenantID string) ([]*MaintenanceWindow, error)
	DeleteMaintenanceWindow(ctx context.Context, id string) error
	GetActiveMaintenanceWindows(ctx context.Context, tenantID string, deviceID string) ([]*MaintenanceWindow, error)
}

func NewMaintenanceEngine(store maintenanceStore) *MaintenanceEngine {
	return &MaintenanceEngine{
		windows: make(map[string]*MaintenanceWindow),
		store:   store,
		now:     time.Now,
	}
}

func (m *MaintenanceEngine) Start(ctx context.Context) error {
	windows, err := m.store.ListMaintenanceWindows(ctx, "")
	if err != nil {
		return err
	}
	m.mu.Lock()
	for _, w := range windows {
		m.windows[w.ID] = w
	}
	m.mu.Unlock()
	return nil
}

func (m *MaintenanceEngine) LoadWindows(windows []*MaintenanceWindow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range windows {
		m.windows[w.ID] = w
	}
}

func (m *MaintenanceEngine) CreateWindow(ctx context.Context, tenantID, name string, startTime, endTime time.Time, deviceIDs []string) (*MaintenanceWindow, error) {
	id := uuid.New().String()
	window := &MaintenanceWindow{
		ID:        id,
		TenantID:  tenantID,
		Name:      name,
		StartTime: startTime,
		EndTime:   endTime,
		DeviceIDs: deviceIDs,
		CreatedAt: m.now(),
	}
	if err := m.store.SaveMaintenanceWindow(ctx, window); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.windows[id] = window
	m.mu.Unlock()
	return window, nil
}

func (m *MaintenanceEngine) DeleteWindow(ctx context.Context, id string) error {
	m.mu.Lock()
	delete(m.windows, id)
	m.mu.Unlock()
	return m.store.DeleteMaintenanceWindow(ctx, id)
}

func (m *MaintenanceEngine) ListWindows(ctx context.Context, tenantID string) ([]*MaintenanceWindow, error) {
	m.mu.RLock()
	windows := make([]*MaintenanceWindow, 0, len(m.windows))
	for _, w := range m.windows {
		if tenantID == "" || w.TenantID == tenantID {
			windows = append(windows, w)
		}
	}
	m.mu.RUnlock()
	return windows, nil
}

func (m *MaintenanceEngine) IsInMaintenance(tenantID, deviceID string, checkTime time.Time) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.windows {
		if w.TenantID != tenantID {
			continue
		}
		if len(w.DeviceIDs) > 0 {
			deviceMatch := false
			for _, d := range w.DeviceIDs {
				if d == deviceID {
					deviceMatch = true
					break
				}
			}
			if !deviceMatch {
				continue
			}
		}
		if (checkTime.Equal(w.StartTime) || checkTime.After(w.StartTime)) &&
			(checkTime.Equal(w.EndTime) || checkTime.Before(w.EndTime)) {
			return true
		}
	}
	return false
}

func (m *MaintenanceEngine) GetActiveWindows(ctx context.Context, tenantID, deviceID string) ([]*MaintenanceWindow, error) {
	now := m.now()
	m.mu.RLock()
	var windows []*MaintenanceWindow
	for _, w := range m.windows {
		if w.TenantID != tenantID {
			continue
		}
		if len(w.DeviceIDs) > 0 {
			deviceMatch := false
			for _, d := range w.DeviceIDs {
				if d == deviceID {
					deviceMatch = true
					break
				}
			}
			if !deviceMatch {
				continue
			}
		}
		if (now.Equal(w.StartTime) || now.After(w.StartTime)) &&
			(now.Equal(w.EndTime) || now.Before(w.EndTime)) {
			windows = append(windows, w)
		}
	}
	m.mu.RUnlock()
	return windows, nil
}
