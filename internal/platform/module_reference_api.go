package platform

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/modules"
)

const (
	referenceModuleID               = "com.example.backup"
	referenceModuleDevicePermission = "devices.read"
	referenceModuleDeviceRoute      = "/api/modules/com.example.backup/devices/{deviceID}"
)

type moduleTargetScopeContextKey struct{}

var moduleAuthorizers sync.Map // map[*APIServer]*modules.APIAuthorizer

// WithModuleAuthorizer configures the dedicated module-service authorization
// broker for this API server. Module credentials remain separate from the
// ordinary user/agent principal path in withAccessControl.
func (s *APIServer) WithModuleAuthorizer(authorizer *modules.APIAuthorizer) *APIServer {
	if s == nil {
		return s
	}
	if authorizer == nil {
		moduleAuthorizers.Delete(s)
		return s
	}
	moduleAuthorizers.Store(s, authorizer)
	return s
}

func (s *APIServer) configuredModuleAuthorizer() *modules.APIAuthorizer {
	if s == nil {
		return nil
	}
	value, ok := moduleAuthorizers.Load(s)
	if !ok {
		return nil
	}
	authorizer, _ := value.(*modules.APIAuthorizer)
	return authorizer
}

// serveReferenceModuleDevice is dispatched by the AccessModule route class.
// It deliberately does not pass module JWTs into validateAndBuildPrincipal.
func (s *APIServer) serveReferenceModuleDevice(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := referenceModuleDeviceID(r.URL.Path)
	if !ok {
		http.Error(w, `{"error":"unclassified module route"}`, http.StatusForbidden)
		return
	}
	r.SetPathValue("deviceID", deviceID)
	r.Pattern = "GET " + referenceModuleDeviceRoute

	handler := newReferenceModuleDeviceHTTPHandler(
		s.configuredModuleAuthorizer(),
		s.resolveReferenceModuleDeviceScope,
		http.HandlerFunc(s.handleReferenceModuleDevice),
	)
	handler.ServeHTTP(w, r)
}

func newReferenceModuleDeviceHTTPHandler(authorizer *modules.APIAuthorizer, resolve ModuleTargetResolver, next http.Handler) http.Handler {
	return WithModuleAuthorization(
		authorizer,
		referenceModuleID,
		referenceModuleDevicePermission,
		resolve,
		next,
	)
}

// resolveReferenceModuleDeviceScope derives the target hierarchy exclusively
// from the devices table and its authoritative MSP/client/site relationships.
// Request headers and query parameters are never accepted as ownership input.
func (s *APIServer) resolveReferenceModuleDeviceScope(r *http.Request) (modules.ResourceScope, error) {
	if s == nil || s.db == nil || s.db.DB() == nil {
		return modules.ResourceScope{}, errors.New("module device scope database unavailable")
	}
	deviceID := strings.TrimSpace(r.PathValue("deviceID"))
	if deviceID == "" {
		return modules.ResourceScope{}, errors.New("module device id is required")
	}

	var scope modules.ResourceScope
	err := s.db.DB().QueryRowContext(r.Context(), `
		SELECT COALESCE(d.msp_id::text, ''),
		       COALESCE(d.client_id::text, ''),
		       COALESCE(d.site_id::text, '')
		FROM devices d
		LEFT JOIN msp_tenants m ON m.id = d.msp_id
		LEFT JOIN client_organizations c ON c.id = d.client_id AND c.msp_id = d.msp_id
		LEFT JOIN sites si ON si.id = d.site_id AND si.client_id = d.client_id
		WHERE d.id = $1
		  AND d.status <> 'disabled'
		  AND (d.msp_id IS NULL OR m.is_active = true)
		  AND (d.client_id IS NULL OR c.is_active = true)
		  AND (d.site_id IS NULL OR si.is_active = true)
	`, deviceID).Scan(&scope.MSPID, &scope.ClientID, &scope.SiteID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return modules.ResourceScope{}, errors.New("module device target not found")
		}
		return modules.ResourceScope{}, err
	}
	if scope.MSPID == "" {
		return modules.ResourceScope{}, errors.New("module device target has no MSP owner")
	}
	if scope.SiteID != "" && scope.ClientID == "" {
		return modules.ResourceScope{}, errors.New("module device target has invalid site hierarchy")
	}

	// Preserve the exact authorized target for the protected handler. The
	// handler re-queries using all resolved ownership columns so an ownership
	// change between authorization and read fails closed rather than leaking.
	ctx := context.WithValue(r.Context(), moduleTargetScopeContextKey{}, scope)
	*r = *r.WithContext(ctx)
	return scope, nil
}

func moduleTargetScopeFromContext(ctx context.Context) (modules.ResourceScope, bool) {
	scope, ok := ctx.Value(moduleTargetScopeContextKey{}).(modules.ResourceScope)
	return scope, ok && scope.MSPID != ""
}

func (s *APIServer) handleReferenceModuleDevice(w http.ResponseWriter, r *http.Request) {
	if _, ok := ModulePrincipalFromContext(r.Context()); !ok {
		http.Error(w, `{"error":"module authorization required"}`, http.StatusUnauthorized)
		return
	}
	target, ok := moduleTargetScopeFromContext(r.Context())
	if !ok || s == nil || s.db == nil || s.db.DB() == nil {
		http.Error(w, `{"error":"module target unavailable"}`, http.StatusForbidden)
		return
	}

	deviceID := strings.TrimSpace(r.PathValue("deviceID"))
	var device struct {
		ID            string     `json:"id"`
		Hostname      string     `json:"hostname"`
		OS            string     `json:"os"`
		Arch          string     `json:"arch"`
		AgentVersion  string     `json:"agent_version"`
		Status        string     `json:"status"`
		LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	}
	err := s.db.DB().QueryRowContext(r.Context(), `
		SELECT d.id::text, d.hostname, d.os, d.arch, d.agent_version, d.status, d.last_heartbeat
		FROM devices d
		WHERE d.id = $1
		  AND d.msp_id::text = $2
		  AND COALESCE(d.client_id::text, '') = $3
		  AND COALESCE(d.site_id::text, '') = $4
		  AND d.status <> 'disabled'
	`, deviceID, target.MSPID, target.ClientID, target.SiteID).Scan(
		&device.ID,
		&device.Hostname,
		&device.OS,
		&device.Arch,
		&device.AgentVersion,
		&device.Status,
		&device.LastHeartbeat,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"module device target unavailable"}`, http.StatusForbidden)
			return
		}
		http.Error(w, `{"error":"module device query failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, device)
}

func referenceModuleDeviceID(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 5 || parts[0] != "api" || parts[1] != "modules" || parts[2] != referenceModuleID || parts[3] != "devices" {
		return "", false
	}
	deviceID := strings.TrimSpace(parts[4])
	if deviceID == "" {
		return "", false
	}
	return deviceID, true
}
