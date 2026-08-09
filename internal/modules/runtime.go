package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrModuleDisabled     = errors.New("module is not enabled")
	ErrRouteNotDeclared   = errors.New("module route is not declared")
	ErrMethodNotDeclared  = errors.New("module route method is not declared")
	ErrPermissionMismatch = errors.New("module route permission mismatch")
	ErrRuntimeUnavailable = errors.New("module runtime unavailable")
)

type Invocation struct {
	Method     string
	Path       string
	Permission string
	Body       []byte
}

type InvocationResult struct {
	StatusCode int
	Body       []byte
}

// Runtime is the narrow execution boundary between the Strata control plane and
// an out-of-process add-on. Implementations must not receive direct database,
// unrestricted NATS, or in-process package access through this interface.
type Runtime interface {
	Health(ctx context.Context, module InstalledModule) error
	Invoke(ctx context.Context, module InstalledModule, invocation Invocation) (InvocationResult, error)
}

type SupervisorOptions struct {
	InvocationTimeout time.Duration
	FailureThreshold  int
}

type Supervisor struct {
	registry  *Registry
	runtime   Runtime
	timeout   time.Duration
	threshold int

	mu       sync.Mutex
	failures map[string]int
}

func NewSupervisor(registry *Registry, runtime Runtime, options SupervisorOptions) (*Supervisor, error) {
	if registry == nil {
		return nil, errors.New("module registry is required")
	}
	if runtime == nil {
		return nil, errors.New("module runtime is required")
	}
	timeout := options.InvocationTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	threshold := options.FailureThreshold
	if threshold <= 0 {
		threshold = 3
	}
	return &Supervisor{
		registry:  registry,
		runtime:   runtime,
		timeout:   timeout,
		threshold: threshold,
		failures:  make(map[string]int),
	}, nil
}

func (s *Supervisor) Health(ctx context.Context, id string) error {
	module, err := s.enabledModule(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := s.runtime.Health(ctx, module); err != nil {
		return fmt.Errorf("%w: health check: %v", ErrRuntimeUnavailable, err)
	}
	return nil
}

func (s *Supervisor) Invoke(ctx context.Context, id string, invocation Invocation) (InvocationResult, error) {
	module, err := s.enabledModule(id)
	if err != nil {
		return InvocationResult{}, err
	}

	route, err := declaredRoute(module.Manifest, invocation.Method, invocation.Path)
	if err != nil {
		return InvocationResult{}, err
	}
	if invocation.Permission != route.Permission {
		return InvocationResult{}, ErrPermissionMismatch
	}
	if route.Permission != "" {
		if err := s.registry.RequirePermission(id, route.Permission); err != nil {
			return InvocationResult{}, err
		}
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	result, err := s.runtime.Invoke(ctx, module, invocation)
	if err != nil {
		s.recordFailure(id, err)
		return InvocationResult{}, fmt.Errorf("%w: invoke: %v", ErrRuntimeUnavailable, err)
	}
	s.resetFailures(id)
	return result, nil
}

func (s *Supervisor) enabledModule(id string) (InstalledModule, error) {
	module, err := s.registry.Get(id)
	if err != nil {
		return InstalledModule{}, err
	}
	if module.State == StateQuarantined {
		return InstalledModule{}, ErrQuarantined
	}
	if module.State != StateEnabled {
		return InstalledModule{}, ErrModuleDisabled
	}
	return module, nil
}

func declaredRoute(manifest Manifest, method, path string) (Route, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	for _, route := range manifest.Routes {
		if route.Path != path {
			continue
		}
		for _, declaredMethod := range route.Methods {
			if strings.EqualFold(declaredMethod, method) {
				return route, nil
			}
		}
		return Route{}, ErrMethodNotDeclared
	}
	return Route{}, ErrRouteNotDeclared
}

func (s *Supervisor) recordFailure(id string, cause error) {
	s.mu.Lock()
	s.failures[id]++
	count := s.failures[id]
	s.mu.Unlock()
	if count < s.threshold {
		return
	}
	reason := fmt.Sprintf("runtime failure threshold reached after %d consecutive failures: %v", count, cause)
	_, _ = s.registry.Quarantine(id, reason)
}

func (s *Supervisor) resetFailures(id string) {
	s.mu.Lock()
	delete(s.failures, id)
	s.mu.Unlock()
}
