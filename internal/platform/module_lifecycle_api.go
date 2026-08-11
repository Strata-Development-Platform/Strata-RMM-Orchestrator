package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/modules"
)

const (
	moduleLifecycleCollectionPath = "/api/v2/platform/modules"
	moduleLifecycleItemPrefix     = "/api/v2/platform/modules/"
	moduleLifecycleBodyLimit      = 1 << 20
	moduleLifecycleMaxReason      = 1024
	moduleLifecycleTxAttempts     = 3
)

type moduleLifecycleReasonRequest struct {
	Reason string `json:"reason"`
}

// isModuleLifecycleRequest recognizes only the operator lifecycle and
// platform-global invocation routes explicitly implemented below. Unknown
// methods/actions remain unclassified and therefore fail closed in the platform
// access-control layer.
func isModuleLifecycleRequest(method, path string) bool {
	if method == http.MethodPost && path == moduleLifecycleCollectionPath {
		return true
	}
	if method == http.MethodPost {
		if _, _, ok := moduleDeviceInvocationTarget(path); ok {
			return true
		}
	}
	if !strings.HasPrefix(path, moduleLifecycleItemPrefix) {
		return false
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 6 || parts[0] != "api" || parts[1] != "v2" || parts[2] != "platform" || parts[3] != "modules" || strings.TrimSpace(parts[4]) == "" {
		return false
	}
	switch parts[5] {
	case "enable", "disable", "quarantine", "upgrade", "rollback":
		return method == http.MethodPost
	case "uninstall":
		return method == http.MethodDelete
	default:
		return false
	}
}

func moduleLifecycleID(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 6 || parts[0] != "api" || parts[1] != "v2" || parts[2] != "platform" || parts[3] != "modules" {
		return "", "", false
	}
	id := strings.TrimSpace(parts[4])
	action := strings.TrimSpace(parts[5])
	if id == "" || action == "" {
		return "", "", false
	}
	return id, action, true
}

// serveModuleLifecycle is reached only after withAccessControl has established
// an authenticated user principal and platform-global authorization. It repeats
// those checks as defense in depth before any mutation, invocation, or audit write.
func (s *APIServer) serveModuleLifecycle(w http.ResponseWriter, r *http.Request) {
	actor, ok := moduleLifecycleActor(r)
	if !ok {
		writeAuthorizationDenied(w)
		return
	}
	if s == nil || s.db == nil || s.db.DB() == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "module lifecycle database unavailable"})
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == moduleLifecycleCollectionPath {
		s.handleModuleInstall(w, r, actor)
		return
	}
	if r.Method == http.MethodPost {
		if moduleID, deviceID, ok := moduleDeviceInvocationTarget(r.URL.Path); ok {
			s.handleModuleDeviceInvocation(w, r, moduleID, deviceID)
			return
		}
	}
	id, action, ok := moduleLifecycleID(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "module lifecycle route not found"})
		return
	}
	switch {
	case r.Method == http.MethodPost && action == "enable":
		s.handleModuleEnable(w, r, actor, id)
	case r.Method == http.MethodPost && action == "disable":
		s.handleModuleDisable(w, r, actor, id)
	case r.Method == http.MethodPost && action == "quarantine":
		s.handleModuleQuarantine(w, r, actor, id)
	case r.Method == http.MethodPost && action == "upgrade":
		s.handleModuleUpgrade(w, r, actor, id)
	case r.Method == http.MethodPost && action == "rollback":
		s.handleModuleRollback(w, r, actor, id)
	case r.Method == http.MethodDelete && action == "uninstall":
		s.handleModuleUninstall(w, r, actor, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "module lifecycle route not found"})
	}
}

func moduleLifecycleActor(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}
	authorization, ok := r.Context().Value(ctxKeyAuthorization).(AuthorizationResult)
	if !ok || !authorization.IsPlatformGlobal() {
		return "", false
	}
	actor, ok := r.Context().Value(ctxKeyUserID).(string)
	actor = strings.TrimSpace(actor)
	return actor, ok && actor != ""
}

func (s *APIServer) handleModuleInstall(w http.ResponseWriter, r *http.Request, actor string) {
	prepared, err := prepareSignedModulePackage(w, r)
	if err != nil {
		if !writeSignedModuleInstallError(w, err) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid signed module package"})
		}
		return
	}
	module, err := s.mutateModuleLifecycle(r.Context(), actor, func(registry *modules.Registry, store *modules.SQLStore, tx *sql.Tx) (modules.InstalledModule, error) {
		installed, err := registry.Install(prepared.pkg.Manifest)
		if err != nil {
			return modules.InstalledModule{}, err
		}
		if _, err := modules.MaterializePayloadRetrySafe(prepared.pkg, prepared.payload, modules.MaterializeOptions{Root: prepared.root}); err != nil {
			return modules.InstalledModule{}, err
		}
		if _, err := modules.ActivateMaterializedVersion(prepared.root, prepared.pkg.Manifest.ID, prepared.pkg.Manifest.Version); err != nil {
			return modules.InstalledModule{}, err
		}
		if err := store.Save(r.Context(), tx, installed, actor, "install"); err != nil {
			return modules.InstalledModule{}, err
		}
		return installed, nil
	})
	if err != nil {
		if !writeSignedModuleInstallError(w, err) {
			writeModuleLifecycleError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, module)
}

func (s *APIServer) handleModuleEnable(w http.ResponseWriter, r *http.Request, actor, id string) {
	module, err := s.mutateModuleLifecycle(r.Context(), actor, func(registry *modules.Registry, store *modules.SQLStore, tx *sql.Tx) (modules.InstalledModule, error) {
		updated, err := registry.Enable(id)
		if err != nil {
			return modules.InstalledModule{}, err
		}
		if err := store.Save(r.Context(), tx, updated, actor, "enable"); err != nil {
			return modules.InstalledModule{}, err
		}
		return updated, nil
	})
	if err != nil {
		writeModuleLifecycleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, module)
}

func (s *APIServer) handleModuleDisable(w http.ResponseWriter, r *http.Request, actor, id string) {
	reason, ok := decodeModuleLifecycleReason(w, r)
	if !ok {
		return
	}
	module, err := s.mutateModuleLifecycle(r.Context(), actor, func(registry *modules.Registry, store *modules.SQLStore, tx *sql.Tx) (modules.InstalledModule, error) {
		updated, err := registry.Disable(id, reason)
		if err != nil {
			return modules.InstalledModule{}, err
		}
		if err := store.Save(r.Context(), tx, updated, actor, "disable"); err != nil {
			return modules.InstalledModule{}, err
		}
		return updated, nil
	})
	if err != nil {
		writeModuleLifecycleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, module)
}

func (s *APIServer) handleModuleQuarantine(w http.ResponseWriter, r *http.Request, actor, id string) {
	reason, ok := decodeModuleLifecycleReason(w, r)
	if !ok {
		return
	}
	module, err := s.mutateModuleLifecycle(r.Context(), actor, func(registry *modules.Registry, store *modules.SQLStore, tx *sql.Tx) (modules.InstalledModule, error) {
		updated, err := registry.Quarantine(id, reason)
		if err != nil {
			return modules.InstalledModule{}, err
		}
		if err := store.Save(r.Context(), tx, updated, actor, "quarantine"); err != nil {
			return modules.InstalledModule{}, err
		}
		return updated, nil
	})
	if err != nil {
		writeModuleLifecycleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, module)
}

func (s *APIServer) handleModuleUninstall(w http.ResponseWriter, r *http.Request, actor, id string) {
	reason, ok := decodeModuleLifecycleReason(w, r)
	if !ok {
		return
	}
	if err := s.deleteModuleLifecycle(r.Context(), actor, id, reason); err != nil {
		writeModuleLifecycleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled", "module_id": id})
}

func decodeModuleLifecycleReason(w http.ResponseWriter, r *http.Request) (string, bool) {
	var request moduleLifecycleReasonRequest
	if err := decodeModuleLifecycleJSON(w, r, &request); err != nil {
		return "", false
	}
	reason := strings.TrimSpace(request.Reason)
	if reason == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason is required"})
		return "", false
	}
	if len(reason) > moduleLifecycleMaxReason {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason is too long"})
		return "", false
	}
	return reason, true
}

func decodeModuleLifecycleJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	if r == nil || r.Body == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON request body is required"})
		return errors.New("request body required")
	}
	reader := http.MaxBytesReader(w, r.Body, moduleLifecycleBodyLimit)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must contain exactly one JSON value"})
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

type moduleLifecycleMutation func(*modules.Registry, *modules.SQLStore, *sql.Tx) (modules.InstalledModule, error)

func (s *APIServer) mutateModuleLifecycle(ctx context.Context, actor string, mutation moduleLifecycleMutation) (modules.InstalledModule, error) {
	var lastErr error
	for attempt := 0; attempt < moduleLifecycleTxAttempts; attempt++ {
		module, err := s.runModuleLifecycleMutation(ctx, actor, mutation)
		if err == nil {
			s.invalidateModuleRuntimeRegistry()
			return module, nil
		}
		lastErr = err
		if !retryableModuleLifecycleTransaction(err) {
			break
		}
	}
	return modules.InstalledModule{}, lastErr
}

func (s *APIServer) runModuleLifecycleMutation(ctx context.Context, actor string, mutation moduleLifecycleMutation) (modules.InstalledModule, error) {
	tx, err := s.beginModuleLifecycleTransaction(ctx, actor)
	if err != nil {
		return modules.InstalledModule{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	store := modules.NewSQLStore()
	registry, err := store.RestoreRegistry(ctx, tx)
	if err != nil {
		return modules.InstalledModule{}, err
	}
	module, err := mutation(registry, store, tx)
	if err != nil {
		return modules.InstalledModule{}, err
	}
	if err := tx.Commit(); err != nil {
		return modules.InstalledModule{}, err
	}
	committed = true
	return module, nil
}

func (s *APIServer) deleteModuleLifecycle(ctx context.Context, actor, id, reason string) error {
	var lastErr error
	for attempt := 0; attempt < moduleLifecycleTxAttempts; attempt++ {
		err := s.runModuleLifecycleDelete(ctx, actor, id, reason)
		if err == nil {
			s.invalidateModuleRuntimeRegistry()
			return nil
		}
		lastErr = err
		if !retryableModuleLifecycleTransaction(err) {
			break
		}
	}
	return lastErr
}

func (s *APIServer) runModuleLifecycleDelete(ctx context.Context, actor, id, reason string) error {
	tx, err := s.beginModuleLifecycleTransaction(ctx, actor)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	store := modules.NewSQLStore()
	if err := store.Delete(ctx, tx, id, actor, reason); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *APIServer) beginModuleLifecycleTransaction(ctx context.Context, actor string) (*sql.Tx, error) {
	if s == nil || s.db == nil || s.db.DB() == nil {
		return nil, errors.New("module lifecycle database unavailable")
	}
	if strings.TrimSpace(actor) == "" {
		return nil, errors.New("module lifecycle actor is required")
	}
	tx, err := s.db.DB().BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		SELECT
			set_config('app.role', $1, true),
			set_config('app.user_id', $2, true),
			set_config('app.scope_type', $3, true)
	`, "platform_admin", actor, string(ScopePlatform)); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func retryableModuleLifecycleTransaction(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return pqErr.Code == "40001" || pqErr.Code == "40P01"
}

func (s *APIServer) invalidateModuleRuntimeRegistry() {
	if s == nil {
		return
	}
	value, ok := moduleRuntimes.Load(s)
	if !ok {
		return
	}
	state, ok := value.(*moduleRuntimeState)
	if !ok || state == nil {
		return
	}
	state.mu.Lock()
	state.refreshAfter = time.Time{}
	state.retryAfter = time.Time{}
	state.mu.Unlock()
}

func writeModuleLifecycleError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "module lifecycle operation failed"
	switch {
	case errors.Is(err, modules.ErrNotFound):
		status, message = http.StatusNotFound, "module not found"
	case errors.Is(err, modules.ErrAlreadyExists):
		status, message = http.StatusConflict, "module already installed"
	case errors.Is(err, modules.ErrQuarantined):
		status, message = http.StatusConflict, "quarantined module cannot transition to requested state"
	case errors.Is(err, modules.ErrPermissionDenied):
		status, message = http.StatusForbidden, "module permission denied"
	case retryableModuleLifecycleTransaction(err):
		status, message = http.StatusConflict, "module lifecycle state changed concurrently; retry request"
	default:
		text := strings.ToLower(err.Error())
		if strings.Contains(text, "validate module manifest") || strings.Contains(text, "invalid module") || strings.Contains(text, "manifest") {
			status, message = http.StatusBadRequest, "invalid module manifest"
		} else if strings.Contains(text, "enabled module must be disabled before uninstall") || strings.Contains(text, "invalid enable transition") || strings.Contains(text, "invalid disable transition") {
			status, message = http.StatusConflict, err.Error()
		}
	}
	writeJSON(w, status, map[string]string{"error": message})
}

var _ = fmt.Sprintf