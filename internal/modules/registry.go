package modules

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

type State string

const (
	StateInstalled   State = "installed"
	StateEnabled     State = "enabled"
	StateDisabled    State = "disabled"
	StateQuarantined State = "quarantined"
)

var (
	ErrNotFound        = errors.New("module not found")
	ErrAlreadyExists   = errors.New("module already installed")
	ErrQuarantined     = errors.New("module is quarantined")
	ErrPermissionDenied = errors.New("module permission denied")
)

type InstalledModule struct {
	Manifest    Manifest  `json:"manifest"`
	State       State     `json:"state"`
	InstalledAt time.Time `json:"installed_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Reason      string    `json:"reason,omitempty"`
}

type Registry struct {
	mu      sync.RWMutex
	modules map[string]InstalledModule
	now     func() time.Time
}

func NewRegistry() *Registry {
	return &Registry{modules: make(map[string]InstalledModule), now: time.Now}
}

func (r *Registry) Install(manifest Manifest) (InstalledModule, error) {
	if err := manifest.Validate(); err != nil {
		return InstalledModule{}, fmt.Errorf("validate module manifest: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.modules[manifest.ID]; exists {
		return InstalledModule{}, ErrAlreadyExists
	}
	now := r.now().UTC()
	installed := InstalledModule{Manifest: manifest, State: StateInstalled, InstalledAt: now, UpdatedAt: now}
	r.modules[manifest.ID] = installed
	return installed, nil
}

func (r *Registry) Enable(id string) (InstalledModule, error) {
	return r.transition(id, func(module InstalledModule) (InstalledModule, error) {
		if module.State == StateQuarantined {
			return InstalledModule{}, ErrQuarantined
		}
		module.State = StateEnabled
		module.Reason = ""
		return module, nil
	})
}

func (r *Registry) Disable(id, reason string) (InstalledModule, error) {
	return r.transition(id, func(module InstalledModule) (InstalledModule, error) {
		if module.State == StateQuarantined {
			return InstalledModule{}, ErrQuarantined
		}
		module.State = StateDisabled
		module.Reason = reason
		return module, nil
	})
}

func (r *Registry) Quarantine(id, reason string) (InstalledModule, error) {
	if reason == "" {
		reason = "administratively quarantined"
	}
	return r.transition(id, func(module InstalledModule) (InstalledModule, error) {
		module.State = StateQuarantined
		module.Reason = reason
		return module, nil
	})
}

func (r *Registry) Uninstall(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	module, ok := r.modules[id]
	if !ok {
		return ErrNotFound
	}
	if module.State == StateEnabled {
		return errors.New("enabled module must be disabled before uninstall")
	}
	delete(r.modules, id)
	return nil
}

func (r *Registry) Get(id string) (InstalledModule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	module, ok := r.modules[id]
	if !ok {
		return InstalledModule{}, ErrNotFound
	}
	return module, nil
}

func (r *Registry) List() []InstalledModule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]InstalledModule, 0, len(r.modules))
	for _, module := range r.modules {
		result = append(result, module)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Manifest.ID < result[j].Manifest.ID })
	return result
}

func (r *Registry) RequirePermission(id, permission string) error {
	module, err := r.Get(id)
	if err != nil {
		return err
	}
	if module.State != StateEnabled {
		return ErrPermissionDenied
	}
	for _, granted := range module.Manifest.Permissions {
		if granted == permission {
			return nil
		}
	}
	return ErrPermissionDenied
}

func (r *Registry) transition(id string, fn func(InstalledModule) (InstalledModule, error)) (InstalledModule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	module, ok := r.modules[id]
	if !ok {
		return InstalledModule{}, ErrNotFound
	}
	updated, err := fn(module)
	if err != nil {
		return InstalledModule{}, err
	}
	updated.UpdatedAt = r.now().UTC()
	r.modules[id] = updated
	return updated, nil
}
