package modules

import (
	"database/sql"
	"errors"
)

// PostgresWASIRuntimeOptions contains only the externally configurable runtime
// limits. Broker registration is intentionally not caller-controlled: the
// production composition below owns the reviewed capability set.
type PostgresWASIRuntimeOptions struct {
	Root       string
	MaxIOBytes int
}

// NewPostgresWASIRuntime composes the production WASI execution boundary with
// the reviewed PostgreSQL-backed broker capabilities. Callers receive no raw
// database handle through the runtime and cannot replace or append broker
// operations through this constructor.
func NewPostgresWASIRuntime(db *sql.DB, registry *Registry, options PostgresWASIRuntimeOptions) (*WASIRuntime, error) {
	if db == nil {
		return nil, errors.New("module runtime PostgreSQL database is required")
	}
	if registry == nil {
		return nil, errors.New("module runtime registry is required")
	}

	deviceResolver, err := NewPostgresBrokerDeviceResolver(db)
	if err != nil {
		return nil, err
	}
	deviceGet, err := NewDeviceGetBrokerOperation(deviceResolver)
	if err != nil {
		return nil, err
	}
	broker, err := NewCapabilityBroker(registry, []BrokerOperation{deviceGet}, CapabilityBrokerOptions{MaxIOBytes: options.MaxIOBytes})
	if err != nil {
		return nil, err
	}
	return NewWASIRuntime(WASIRuntimeOptions{
		Root:       options.Root,
		MaxIOBytes: options.MaxIOBytes,
		Broker:     broker,
	})
}
