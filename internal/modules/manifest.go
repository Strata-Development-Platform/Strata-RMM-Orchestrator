package modules

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

const CurrentAPIVersion = "v1"

const (
	RuntimeKindWASI       = "wasi"
	RuntimeNetworkNone    = "none"
	RuntimeNetworkBrokered = "brokered"
)

var moduleIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

var knownPermissions = map[string]struct{}{
	"devices.read":       {},
	"devices.write":      {},
	"alerts.read":        {},
	"alerts.write":       {},
	"automation.execute": {},
	"reports.read":       {},
	"reports.write":      {},
	"webhooks.receive":   {},
	"notifications.send": {},
	"inventory.read":     {},
	"inventory.write":    {},
}

type Manifest struct {
	ID          string        `json:"id" yaml:"id"`
	Name        string        `json:"name" yaml:"name"`
	Version     string        `json:"version" yaml:"version"`
	APIVersion  string        `json:"api_version" yaml:"api_version"`
	Publisher   string        `json:"publisher" yaml:"publisher"`
	Description string        `json:"description,omitempty" yaml:"description,omitempty"`
	Permissions []string      `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Events      EventAccess   `json:"events,omitempty" yaml:"events,omitempty"`
	Routes      []Route       `json:"routes,omitempty" yaml:"routes,omitempty"`
	UI          UIExtension   `json:"ui,omitempty" yaml:"ui,omitempty"`
	Runtime     *RuntimeSpec  `json:"runtime,omitempty" yaml:"runtime,omitempty"`
}

type EventAccess struct {
	Subscribe []string `json:"subscribe,omitempty" yaml:"subscribe,omitempty"`
	Publish   []string `json:"publish,omitempty" yaml:"publish,omitempty"`
}

type Route struct {
	Path       string   `json:"path" yaml:"path"`
	Methods    []string `json:"methods" yaml:"methods"`
	Permission string   `json:"permission,omitempty" yaml:"permission,omitempty"`
}

type UIExtension struct {
	Navigation []NavigationItem `json:"navigation,omitempty" yaml:"navigation,omitempty"`
}

type NavigationItem struct {
	Label string `json:"label" yaml:"label"`
	Path  string `json:"path" yaml:"path"`
}

// RuntimeSpec declares the sandbox contract required before a module may be
// executed. Alpha supports WASI only. Omission remains valid for non-executable
// modules and preserves compatibility with existing manifests.
type RuntimeSpec struct {
	Kind           string `json:"kind" yaml:"kind"`
	Entrypoint     string `json:"entrypoint" yaml:"entrypoint"`
	MemoryMB       int    `json:"memory_mb" yaml:"memory_mb"`
	TimeoutSeconds int    `json:"timeout_seconds" yaml:"timeout_seconds"`
	MaxConcurrency int    `json:"max_concurrency" yaml:"max_concurrency"`
	Network        string `json:"network" yaml:"network"`
}

func (m Manifest) Validate() error {
	if !moduleIDPattern.MatchString(m.ID) {
		return errors.New("module id must be lowercase and contain only letters, digits, dots, or hyphens")
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("module name is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return errors.New("module version is required")
	}
	if m.APIVersion != CurrentAPIVersion {
		return fmt.Errorf("unsupported module API version %q", m.APIVersion)
	}
	if strings.TrimSpace(m.Publisher) == "" {
		return errors.New("module publisher is required")
	}
	if m.Runtime != nil {
		if err := m.Runtime.Validate(); err != nil {
			return fmt.Errorf("invalid module runtime: %w", err)
		}
	}

	seenPermissions := make(map[string]struct{}, len(m.Permissions))
	for _, permission := range m.Permissions {
		if _, ok := knownPermissions[permission]; !ok {
			return fmt.Errorf("unknown module permission %q", permission)
		}
		if _, duplicate := seenPermissions[permission]; duplicate {
			return fmt.Errorf("duplicate module permission %q", permission)
		}
		seenPermissions[permission] = struct{}{}
	}

	for _, route := range m.Routes {
		if !strings.HasPrefix(route.Path, "/api/modules/"+m.ID+"/") {
			return fmt.Errorf("route %q must be namespaced under /api/modules/%s/", route.Path, m.ID)
		}
		if len(route.Methods) == 0 {
			return fmt.Errorf("route %q must declare at least one method", route.Path)
		}
		if route.Permission != "" {
			if _, ok := seenPermissions[route.Permission]; !ok {
				return fmt.Errorf("route %q requires undeclared permission %q", route.Path, route.Permission)
			}
		}
	}

	return nil
}

func (r RuntimeSpec) Validate() error {
	if r.Kind != RuntimeKindWASI {
		return fmt.Errorf("unsupported runtime kind %q", r.Kind)
	}
	entrypoint := strings.TrimSpace(r.Entrypoint)
	if entrypoint == "" {
		return errors.New("runtime entrypoint is required")
	}
	if strings.Contains(entrypoint, "\\") || strings.HasPrefix(entrypoint, "/") {
		return errors.New("runtime entrypoint must be a relative slash-separated path")
	}
	clean := path.Clean(entrypoint)
	if clean != entrypoint || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return errors.New("runtime entrypoint must be normalized and contained")
	}
	if !strings.HasSuffix(strings.ToLower(entrypoint), ".wasm") {
		return errors.New("WASI runtime entrypoint must end in .wasm")
	}
	if r.MemoryMB < 16 || r.MemoryMB > 512 {
		return errors.New("runtime memory_mb must be between 16 and 512")
	}
	if r.TimeoutSeconds < 1 || r.TimeoutSeconds > 120 {
		return errors.New("runtime timeout_seconds must be between 1 and 120")
	}
	if r.MaxConcurrency < 1 || r.MaxConcurrency > 32 {
		return errors.New("runtime max_concurrency must be between 1 and 32")
	}
	if r.Network != RuntimeNetworkNone && r.Network != RuntimeNetworkBrokered {
		return fmt.Errorf("unsupported runtime network policy %q", r.Network)
	}
	return nil
}

func KnownPermissions() []string {
	permissions := make([]string, 0, len(knownPermissions))
	for permission := range knownPermissions {
		permissions = append(permissions, permission)
	}
	sort.Strings(permissions)
	return permissions
}
