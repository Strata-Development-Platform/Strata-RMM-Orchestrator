package platform

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/modules"
)

var errModuleVersionStateMismatch = errors.New("module runtime and durable version state do not match")

func (s *APIServer) handleModuleUpgrade(w http.ResponseWriter, r *http.Request, actor, id string) {
	prepared, err := prepareSignedModulePackage(w, r)
	if err != nil {
		if !writeSignedModuleInstallError(w, err) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid signed module package"})
		}
		return
	}
	if prepared.pkg.Manifest.ID != id {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "signed module package does not match requested module"})
		return
	}

	expected, err := modules.ReadActiveVersion(prepared.root, id)
	if err != nil {
		writeModuleVersionError(w, err)
		return
	}
	module, err := s.mutateModuleLifecycle(r.Context(), actor, func(registry *modules.Registry, store *modules.SQLStore, tx *sql.Tx) (modules.InstalledModule, error) {
		current, err := registry.Get(id)
		if err != nil {
			return modules.InstalledModule{}, err
		}
		if current.Manifest.Version != expected.ActiveVersion {
			return modules.InstalledModule{}, errModuleVersionStateMismatch
		}
		updated, err := registry.Upgrade(id, prepared.pkg.Manifest)
		if err != nil {
			return modules.InstalledModule{}, err
		}
		if _, err := modules.MaterializePayloadRetrySafe(prepared.pkg, prepared.payload, modules.MaterializeOptions{Root: prepared.root}); err != nil {
			return modules.InstalledModule{}, err
		}
		if _, err := modules.ActivateMaterializedVersion(prepared.root, id, prepared.pkg.Manifest.Version); err != nil {
			return modules.InstalledModule{}, err
		}
		if err := store.Save(r.Context(), tx, updated, actor, "upgrade"); err != nil {
			return modules.InstalledModule{}, err
		}
		return updated, nil
	})
	if err != nil {
		if !writeSignedModuleInstallError(w, err) {
			writeModuleVersionError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, module)
}

func (s *APIServer) handleModuleRollback(w http.ResponseWriter, r *http.Request, actor, id string) {
	if !requireEmptyModuleVersionBody(w, r) {
		return
	}
	root := strings.TrimSpace(os.Getenv(moduleRuntimeRootEnv))
	if root == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "module runtime is not configured"})
		return
	}
	expected, err := modules.ReadActiveVersion(root, id)
	if err != nil {
		writeModuleVersionError(w, err)
		return
	}
	if expected.PreviousVersion == "" {
		writeModuleVersionError(w, modules.ErrNoRollbackVersion)
		return
	}
	metadata, err := modules.ReadReleaseMetadata(root, id, expected.PreviousVersion)
	if err != nil {
		writeModuleVersionError(w, err)
		return
	}
	if metadata.Manifest.ID != id || metadata.Manifest.Version != expected.PreviousVersion {
		writeModuleVersionError(w, errModuleVersionStateMismatch)
		return
	}

	module, err := s.mutateModuleLifecycle(r.Context(), actor, func(registry *modules.Registry, store *modules.SQLStore, tx *sql.Tx) (modules.InstalledModule, error) {
		current, err := registry.Get(id)
		if err != nil {
			return modules.InstalledModule{}, err
		}
		if current.Manifest.Version != expected.ActiveVersion {
			return modules.InstalledModule{}, errModuleVersionStateMismatch
		}
		updated, err := registry.Rollback(id, metadata.Manifest)
		if err != nil {
			return modules.InstalledModule{}, err
		}
		if _, err := modules.ActivateExpectedPreviousVersion(root, id, expected); err != nil {
			return modules.InstalledModule{}, err
		}
		if err := store.Save(r.Context(), tx, updated, actor, "rollback"); err != nil {
			return modules.InstalledModule{}, err
		}
		return updated, nil
	})
	if err != nil {
		writeModuleVersionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, module)
}

func requireEmptyModuleVersionBody(w http.ResponseWriter, r *http.Request) bool {
	if r == nil || r.Body == nil || r.Body == http.NoBody {
		return true
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid rollback request body"})
		return false
	}
	if len(data) != 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rollback request body must be empty"})
		return false
	}
	return true
}

func writeModuleVersionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, modules.ErrVersionTransition):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "module must be inactive and target a different verified version"})
	case errors.Is(err, modules.ErrNoRollbackVersion):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "module has no rollback version"})
	case errors.Is(err, modules.ErrActivationStateChanged), errors.Is(err, errModuleVersionStateMismatch):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "module version state changed concurrently; retry request"})
	case errors.Is(err, modules.ErrReleaseMetadataInvalid), errors.Is(err, modules.ErrReleaseMetadataConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "module release history is invalid"})
	case errors.Is(err, modules.ErrNoActiveVersion):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "module has no active runtime version"})
	default:
		writeModuleLifecycleError(w, err)
	}
}
