package platform

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/modules"
)

const (
	moduleInvocationAction      = "invoke"
	moduleInvocationTargetKind  = "device"
	moduleRuntimeRootEnv        = "STRATA_MODULE_ROOT"
	moduleInvocationMaxIOBytes  = 1 << 20
)

type moduleDeviceInvocationRequest struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body,omitempty"`
}

func moduleDeviceInvocationTarget(path string) (moduleID, deviceID string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 8 ||
		parts[0] != "api" || parts[1] != "v2" || parts[2] != "platform" || parts[3] != "modules" ||
		strings.TrimSpace(parts[4]) == "" || parts[5] != moduleInvocationAction || parts[6] != moduleInvocationTargetKind ||
		strings.TrimSpace(parts[7]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[4]), strings.TrimSpace(parts[7]), true
}

// handleModuleDeviceInvocation is the first host->WASI production invocation
// surface. It is reached only through the platform-global module lifecycle gate.
// The target device's organizational scope is resolved from authoritative
// PostgreSQL state and never accepted from request JSON, headers, or query data.
func (s *APIServer) handleModuleDeviceInvocation(w http.ResponseWriter, r *http.Request, moduleID, deviceID string) {
	if s == nil || s.db == nil || s.db.DB() == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "module invocation database unavailable"})
		return
	}

	var request moduleDeviceInvocationRequest
	if err := decodeModuleLifecycleJSON(w, r, &request); err != nil {
		return
	}
	request.Method = strings.ToUpper(strings.TrimSpace(request.Method))
	request.Path = strings.TrimSpace(request.Path)
	if request.Method == "" || request.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "method and path are required"})
		return
	}
	if !strings.HasPrefix(request.Path, "/api/modules/"+moduleID+"/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invocation path must belong to requested module"})
		return
	}

	root := strings.TrimSpace(os.Getenv(moduleRuntimeRootEnv))
	if root == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "module runtime root is not configured"})
		return
	}

	snapshot, err := s.loadDurableModuleSnapshot(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "module registry unavailable"})
		return
	}
	registry := modules.NewRegistry()
	if err := registry.ReplaceSnapshot(snapshot); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "module registry invalid"})
		return
	}

	resolver, err := modules.NewPostgresBrokerDeviceResolver(s.db.DB())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "module device resolver unavailable"})
		return
	}
	device, err := resolver.ResolveBrokerDevice(r.Context(), deviceID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "module device target unavailable"})
		return
	}

	runtime, err := modules.NewPostgresWASIRuntime(s.db.DB(), registry, modules.PostgresWASIRuntimeOptions{
		Root:       root,
		MaxIOBytes: moduleInvocationMaxIOBytes,
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "module runtime unavailable"})
		return
	}
	supervisor, err := modules.NewSupervisor(registry, runtime, modules.SupervisorOptions{})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "module supervisor unavailable"})
		return
	}

	result, err := supervisor.InvokeDeclared(r.Context(), moduleID, request.Method, request.Path, request.Body, device.Scope)
	if err != nil {
		writeModuleInvocationError(w, err)
		return
	}
	if result.StatusCode < 100 || result.StatusCode > 599 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "module returned invalid status"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(result.StatusCode)
	_, _ = w.Write(result.Body)
}

func writeModuleInvocationError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	message := "module invocation failed"
	switch {
	case errors.Is(err, modules.ErrNotFound):
		status, message = http.StatusNotFound, "module not found"
	case errors.Is(err, modules.ErrModuleDisabled), errors.Is(err, modules.ErrQuarantined):
		status, message = http.StatusConflict, "module is not executable"
	case errors.Is(err, modules.ErrRouteNotDeclared), errors.Is(err, modules.ErrMethodNotDeclared):
		status, message = http.StatusBadRequest, "module route is not declared"
	case errors.Is(err, modules.ErrPermissionDenied), errors.Is(err, modules.ErrPermissionMismatch):
		status, message = http.StatusForbidden, "module permission denied"
	case errors.Is(err, modules.ErrRuntimeUnavailable):
		status, message = http.StatusBadGateway, "module runtime failed"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
