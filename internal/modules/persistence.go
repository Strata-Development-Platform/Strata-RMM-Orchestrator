package modules

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// DBTX is intentionally satisfied by *sql.Tx and *sql.DB. Production callers
// should pass an authorization-scoped transaction so existing SET LOCAL app.*
// RLS context remains bound to the same PostgreSQL connection for every query.
type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type SQLStore struct{}

func NewSQLStore() *SQLStore { return &SQLStore{} }

func (s *SQLStore) Save(ctx context.Context, db DBTX, module InstalledModule, actor, action string) error {
	if db == nil {
		return errors.New("module persistence requires a database transaction")
	}
	if err := validatePersistedModule(module); err != nil {
		return err
	}
	if actor == "" {
		return errors.New("module persistence actor is required")
	}
	if !validAuditAction(action) || action == "uninstall" {
		return fmt.Errorf("invalid module persistence action %q", action)
	}

	var previousState sql.NullString
	err := db.QueryRowContext(ctx, `SELECT state FROM addon_modules WHERE module_id=$1`, module.Manifest.ID).Scan(&previousState)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read existing module state: %w", err)
	}
	if err := validateAuditTransition(previousState, module.State, action); err != nil {
		return err
	}

	manifest, err := json.Marshal(module.Manifest)
	if err != nil {
		return fmt.Errorf("marshal module manifest: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO addon_modules (module_id, manifest, state, reason, installed_at, updated_at)
		VALUES ($1, $2::jsonb, $3, $4, $5, $6)
		ON CONFLICT (module_id) DO UPDATE SET
			manifest=EXCLUDED.manifest,
			state=EXCLUDED.state,
			reason=EXCLUDED.reason,
			updated_at=EXCLUDED.updated_at
	`, module.Manifest.ID, string(manifest), string(module.State), module.Reason, module.InstalledAt.UTC(), module.UpdatedAt.UTC())
	if err != nil {
		return fmt.Errorf("persist module: %w", err)
	}

	var previous any
	if previousState.Valid {
		previous = previousState.String
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO addon_module_audit (module_id, action, previous_state, new_state, reason, actor)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, module.Manifest.ID, action, previous, string(module.State), module.Reason, actor); err != nil {
		return fmt.Errorf("append module audit: %w", err)
	}
	return nil
}

func (s *SQLStore) Delete(ctx context.Context, db DBTX, id, actor, reason string) error {
	if db == nil {
		return errors.New("module persistence requires a database transaction")
	}
	if actor == "" {
		return errors.New("module persistence actor is required")
	}
	var state string
	if err := db.QueryRowContext(ctx, `SELECT state FROM addon_modules WHERE module_id=$1 FOR UPDATE`, id).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock module for uninstall: %w", err)
	}
	if State(state) == StateEnabled {
		return errors.New("enabled module must be disabled before uninstall")
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO addon_module_audit (module_id, action, previous_state, new_state, reason, actor)
		VALUES ($1, 'uninstall', $2, NULL, $3, $4)
	`, id, state, reason, actor); err != nil {
		return fmt.Errorf("append module uninstall audit: %w", err)
	}
	result, err := db.ExecContext(ctx, `DELETE FROM addon_modules WHERE module_id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete module: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLStore) List(ctx context.Context, db DBTX) ([]InstalledModule, error) {
	if db == nil {
		return nil, errors.New("module persistence requires a database transaction")
	}
	rows, err := db.QueryContext(ctx, `
		SELECT manifest, state, reason, installed_at, updated_at
		FROM addon_modules ORDER BY module_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list persisted modules: %w", err)
	}
	defer rows.Close()

	var modules []InstalledModule
	for rows.Next() {
		var raw []byte
		var state string
		var module InstalledModule
		if err := rows.Scan(&raw, &state, &module.Reason, &module.InstalledAt, &module.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan persisted module: %w", err)
		}
		if err := json.Unmarshal(raw, &module.Manifest); err != nil {
			return nil, fmt.Errorf("decode persisted module manifest: %w", err)
		}
		module.State = State(state)
		if err := validatePersistedModule(module); err != nil {
			return nil, fmt.Errorf("invalid persisted module %q: %w", module.Manifest.ID, err)
		}
		modules = append(modules, module)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate persisted modules: %w", err)
	}
	return modules, nil
}

func (s *SQLStore) RestoreRegistry(ctx context.Context, db DBTX) (*Registry, error) {
	modules, err := s.List(ctx, db)
	if err != nil {
		return nil, err
	}
	registry := NewRegistry()
	for _, module := range modules {
		if err := registry.Restore(module); err != nil {
			return nil, fmt.Errorf("restore module %q: %w", module.Manifest.ID, err)
		}
	}
	return registry, nil
}

func validatePersistedModule(module InstalledModule) error {
	if err := module.Manifest.Validate(); err != nil {
		return fmt.Errorf("validate persisted manifest: %w", err)
	}
	if !validState(module.State) {
		return fmt.Errorf("invalid persisted module state %q", module.State)
	}
	if module.InstalledAt.IsZero() || module.UpdatedAt.IsZero() {
		return errors.New("persisted module timestamps are required")
	}
	if module.UpdatedAt.Before(module.InstalledAt) {
		return errors.New("persisted module updated_at precedes installed_at")
	}
	return nil
}

func validateAuditTransition(previous sql.NullString, next State, action string) error {
	if !previous.Valid {
		if action != "install" || next != StateInstalled {
			return fmt.Errorf("new module must be persisted as install -> installed, got %s -> %s", action, next)
		}
		return nil
	}
	if action == "install" {
		return errors.New("install audit action is invalid for an existing module")
	}
	previousState := State(previous.String)
	if !validState(previousState) {
		return fmt.Errorf("invalid previous module state %q", previousState)
	}
	switch action {
	case "enable":
		if next != StateEnabled || previousState == StateQuarantined {
			return fmt.Errorf("invalid enable transition %s -> %s", previousState, next)
		}
	case "disable":
		if next != StateDisabled || previousState == StateQuarantined {
			return fmt.Errorf("invalid disable transition %s -> %s", previousState, next)
		}
	case "quarantine":
		if next != StateQuarantined {
			return fmt.Errorf("invalid quarantine transition %s -> %s", previousState, next)
		}
	case "restore":
		if next != previousState {
			return fmt.Errorf("restore action may not change state: %s -> %s", previousState, next)
		}
	default:
		return fmt.Errorf("invalid module persistence action %q", action)
	}
	return nil
}

func validState(state State) bool {
	switch state {
	case StateInstalled, StateEnabled, StateDisabled, StateQuarantined:
		return true
	default:
		return false
	}
}

func validAuditAction(action string) bool {
	switch action {
	case "install", "enable", "disable", "quarantine", "restore", "uninstall":
		return true
	default:
		return false
	}
}
