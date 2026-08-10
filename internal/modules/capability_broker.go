package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const defaultBrokerIOBytes = 1 << 20

var (
	ErrBrokerOperationUnknown  = errors.New("module broker operation is not registered")
	ErrBrokerVersionMismatch   = errors.New("module broker version does not match current registry version")
	ErrBrokerInputTooLarge     = errors.New("module broker input exceeds limit")
	ErrBrokerOutputTooLarge    = errors.New("module broker output exceeds limit")
	ErrBrokerScopeInvalid      = errors.New("module broker invocation scope is invalid")
)

// BrokerRequest is assembled by trusted Strata host code. The guest supplies
// only opaque input bytes. It does not select its permission or resource scope.
type BrokerRequest struct {
	Operation string
	Scope     ResourceScope
	Input     []byte
}

// BrokerHandler performs one explicitly registered host capability after the
// broker has validated module lifecycle, version, permission, scope, and input
// bounds. Implementations must still resolve any concrete resource identifiers
// in Input against authoritative platform storage before acting on them.
type BrokerHandler func(context.Context, InstalledModule, ResourceScope, []byte) ([]byte, error)

// BrokerOperation binds a stable host operation name to one manifest permission
// and one implementation. Permissions are host-declared rather than guest-
// declared so a module cannot self-select a stronger capability.
type BrokerOperation struct {
	Name       string
	Permission string
	Handler    BrokerHandler
}

type CapabilityBrokerOptions struct {
	MaxIOBytes int
}

// CapabilityBroker is the single authorization choke point for future WASI
// host functions. It intentionally exposes no raw network, filesystem,
// database, message-bus, or secret handles.
type CapabilityBroker struct {
	registry   *Registry
	operations map[string]BrokerOperation
	maxIOBytes int
}

func NewCapabilityBroker(registry *Registry, operations []BrokerOperation, options CapabilityBrokerOptions) (*CapabilityBroker, error) {
	if registry == nil {
		return nil, errors.New("module registry is required")
	}
	maxIOBytes := options.MaxIOBytes
	if maxIOBytes <= 0 {
		maxIOBytes = defaultBrokerIOBytes
	}
	registered := make(map[string]BrokerOperation, len(operations))
	for _, operation := range operations {
		name := strings.TrimSpace(operation.Name)
		if name == "" {
			return nil, errors.New("module broker operation name is required")
		}
		if _, exists := registered[name]; exists {
			return nil, fmt.Errorf("duplicate module broker operation %q", name)
		}
		if _, known := knownPermissions[operation.Permission]; !known {
			return nil, fmt.Errorf("module broker operation %q uses unknown permission %q", name, operation.Permission)
		}
		if operation.Handler == nil {
			return nil, fmt.Errorf("module broker operation %q handler is required", name)
		}
		operation.Name = name
		registered[name] = operation
	}
	return &CapabilityBroker{registry: registry, operations: registered, maxIOBytes: maxIOBytes}, nil
}

func (b *CapabilityBroker) Call(ctx context.Context, module InstalledModule, request BrokerRequest) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("module broker context is required")
	}
	operation, ok := b.operations[strings.TrimSpace(request.Operation)]
	if !ok {
		return nil, ErrBrokerOperationUnknown
	}
	if len(request.Input) > b.maxIOBytes {
		return nil, ErrBrokerInputTooLarge
	}
	if err := validateServiceScope(request.Scope.MSPID, request.Scope.ClientID, request.Scope.SiteID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBrokerScopeInvalid, err)
	}

	current, err := b.registry.Get(module.Manifest.ID)
	if err != nil {
		return nil, err
	}
	if current.State == StateQuarantined {
		return nil, ErrQuarantined
	}
	if current.State != StateEnabled {
		return nil, ErrModuleDisabled
	}
	if current.Manifest.Version != module.Manifest.Version {
		return nil, ErrBrokerVersionMismatch
	}
	if err := b.registry.RequirePermission(module.Manifest.ID, operation.Permission); err != nil {
		return nil, err
	}

	output, err := operation.Handler(ctx, current, request.Scope, append([]byte(nil), request.Input...))
	if err != nil {
		return nil, err
	}
	if len(output) > b.maxIOBytes {
		return nil, ErrBrokerOutputTooLarge
	}
	return append([]byte(nil), output...), nil
}
