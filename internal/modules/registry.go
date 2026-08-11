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
	ErrNotFound          = errors.New("module not found")
	ErrAlreadyExists     = errors.New("module already installed")
	ErrQuarantined       = errors.New("module is quarantined")
	ErrPermissionDenied  = errors.New("module permission denied")
	ErrVersionTransition = errors.New("module version transition is not allowed")
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

// Restore inserts already-persisted lifecycle state without replaying lifecycle
// transitions or changing timestamps. This is used only after persisted data has
// passed manifest/state/timestamp validation.
func (r *Registry) Restore(module InstalledModule) error {
	if err := validatePersistedModule(module); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.modules[module.Manifest.ID]; exists {
		return ErrAlreadyExists
	}
	r.modules[module.Manifest.ID] = module
	return nil
}

// ReplaceSnapshot atomically replaces the registry with a validated durable
// snapshot. Validation happens before the write lock is taken, so a malformed or
// duplicate snapshot cannot partially mutate the live authorization registry.
func (r *Registry) ReplaceSnapshot(snapshot []InstalledModule) error {
	if r == nil {
		return errors.New("module registry is required")
	}
	next := make(map[string]InstalledModule, len(snapshot))
	for _, module := range snapshot {
		if err := validatePersistedModule(module); err != nil {
			return fmt.Errorf("validate module %q in registry snapshot: %w", module.Manifest.ID, err)
		}
		if _, exists := next[module.Manifest.ID]; exists {
			return fmt.Errorf("duplicate module %q in registry snapshot", module.Manifest.ID)
		}
		next[module.Manifest.ID] = module
	}

	r.mu.Lock()
	r.modules = next
	r.mu.Unlock()
	return nil
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

// Upgrade replaces the current manifest with a newly verified release manifest
// while preserving lifecycle state and installation time. Version transitions
// are intentionally forbidden while enabled or quarantined so executable bytes
// and authorization cannot change underneath an active or isolated runtime.
func (r *Registry) Upgrade(id string, manifest Manifest) (InstalledModule, error) {
	return r.replaceManifest(id, manifest)
}

// Rollback restores a previously verified release manifest. The same guarded
// state rules as Upgrade apply; callers must source the manifest from trusted,
// immutable release metadata rather than an untrusted request body.
func (r *Registry) Rollback(id string, manifest Manifest) (InstalledModule, error) {
	return r.replaceManifest(id, manifest)
}

func (r *Registry) replaceManifest(id string, manifest Manifest) (InstalledModule, error) {
	if err := manifest.Validate(); err != nil {
		return InstalledModule{}, fmt.Errorf("validate module manifest: %w", err)
	}
	if manifest.ID != id {
		return InstalledModule{}, fmt.Errorf("%w: manifest id %q does not match module %q", ErrVersionTransition, manifest.ID, id)
	}
	return r.transition(id, func(module InstalledModule) (InstalledModule, error) {
		if module.State == StateEnabled || module.State == StateQuarantined {
			return InstalledModule{}, fmt.Errorf("%w: module state %s", ErrVersionTransition, module.State)
		}
		if module.Manifest.Version == manifest.Version {
			return InstalledModule{}, fmt.Errorf("%w: version %q is already current", ErrVersionTransition, manifest.Version)
		}
		module.Manifest = manifest
		module.Reason = ""
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
