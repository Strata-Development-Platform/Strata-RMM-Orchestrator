package platform

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/patch"
)

type createPatchDeploymentRequest struct {
	PolicyID   string   `json:"policy_id"`
	DeviceIDs  []string `json:"device_ids"`
	PatchIDs   []string `json:"patch_ids"`
	ScheduleAt string   `json:"schedule_at,omitempty"`
}

func normalizePatchDeploymentIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

// handleCreatePatchDeployment creates a rollout only after resolving scope from
// the authenticated client context and durable device ownership. The body never
// supplies tenant/MSP/client/site/agent identity, and an explicit patch
// selection is mandatory.
func (s *APIServer) handleCreatePatchDeployment(w http.ResponseWriter, r *http.Request) {
	if s.patchMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "patch manager unavailable"})
		return
	}
	requestedMSPID := r.Header.Get("X-MSP-ID")
	clientID := r.URL.Query().Get("client_id")
	if requestedMSPID == "" || clientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "msp_id and client_id required"})
		return
	}
	mspID, ok := s.authorizeClientManage(w, r, clientID)
	if !ok {
		return
	}
	if mspID != requestedMSPID {
		writeAuthorizationDenied(w)
		return
	}

	var req createPatchDeploymentRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid patch deployment request"})
		return
	}
	req.PolicyID = strings.TrimSpace(req.PolicyID)
	req.DeviceIDs = normalizePatchDeploymentIDs(req.DeviceIDs)
	req.PatchIDs = normalizePatchDeploymentIDs(req.PatchIDs)
	if req.PolicyID == "" || len(req.DeviceIDs) == 0 || len(req.PatchIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "policy_id, device_ids, and patch_ids are required"})
		return
	}

	scheduledFor := time.Now().UTC()
	if req.ScheduleAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.ScheduleAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schedule_at must be RFC3339"})
			return
		}
		scheduledFor = parsed.UTC()
	}

	db := s.requestDB(r)
	var tenantID string
	for _, deviceID := range req.DeviceIDs {
		var deviceTenant string
		err := db.QueryRowContext(r.Context(), `
			SELECT tenant_id::text
			FROM devices
			WHERE id::text = $1
			  AND msp_id::text = $2
			  AND client_id::text = $3
			  AND status <> 'disabled'
		`, deviceID, mspID, clientID).Scan(&deviceTenant)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "one or more devices are unavailable or outside the authorized client"})
			return
		}
		if tenantID == "" {
			tenantID = deviceTenant
		} else if tenantID != deviceTenant {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "patch deployment devices must share one tenant scope"})
			return
		}
	}

	var policyTenant string
	var enabled bool
	if err := db.QueryRowContext(r.Context(), `
		SELECT tenant_id::text, enabled
		FROM patch_policies
		WHERE id::text = $1
	`, req.PolicyID).Scan(&policyTenant, &enabled); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "patch policy not found"})
		return
	}
	if !enabled || policyTenant != tenantID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "patch policy is disabled or outside the deployment scope"})
		return
	}

	dep := &patch.Deployment{
		ID:           uuid.NewString(),
		PolicyID:     req.PolicyID,
		TenantID:     tenantID,
		Status:       patch.StatusPending,
		DeviceCount:  len(req.DeviceIDs),
		Pending:      len(req.DeviceIDs),
		ScheduledFor: scheduledFor,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.patchMgr.CreateDeploymentWithPatches(r.Context(), dep, req.DeviceIDs, req.PatchIDs); err != nil {
		if errors.Is(err, patch.ErrNoDeploymentPatches) || errors.Is(err, patch.ErrPatchResultScope) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid patch deployment selection or device scope"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating patch deployment"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"deployment_id": dep.ID,
		"status":        dep.Status,
		"device_count":  dep.DeviceCount,
		"patch_count":   len(req.PatchIDs),
		"scheduled_for": dep.ScheduledFor.Format(time.RFC3339),
	})
}
