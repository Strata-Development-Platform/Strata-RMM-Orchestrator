package platform

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type durableSoftwareTarget struct {
	DeviceID string
	AgentID  string
	MSPID    string
	ClientID string
}

func normalizeSoftwareDeviceIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	return clean
}

// handleCreateDurableSoftwareDeployment creates the legacy software deployment
// view and the generic durable job/outbox records in the request-scoped SQL
// transaction. No broker publish occurs in the HTTP request; the outbox
// dispatcher owns delivery, replay, reconnect recovery, retries, and expiry.
func (s *APIServer) handleCreateDurableSoftwareDeployment(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	var req struct {
		PackageID    string   `json:"package_id"`
		Name         string   `json:"name"`
		DeviceIDs    []string `json:"device_ids"`
		Action       string   `json:"action"`
		ScheduleType string   `json:"schedule_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PackageID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "package_id and device_ids required"})
		return
	}
	req.DeviceIDs = normalizeSoftwareDeviceIDs(req.DeviceIDs)
	if len(req.DeviceIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "package_id and device_ids required"})
		return
	}
	if req.Action == "" {
		req.Action = "install"
	}
	if req.Action != "install" && req.Action != "uninstall" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported action"})
		return
	}
	if req.ScheduleType == "" {
		req.ScheduleType = "now"
	}

	db := s.requestDB(r)
	var pkg struct {
		Name          string
		SourceURL     string
		Checksum      string
		PkgType       string
		InstallArgs   string
		UninstallArgs string
	}
	if err := db.QueryRowContext(r.Context(), `
		SELECT name, source_url, checksum, package_type, install_args, uninstall_args
		FROM software_packages WHERE id = $1 AND tenant_id = $2
	`, req.PackageID, tenantID).Scan(&pkg.Name, &pkg.SourceURL, &pkg.Checksum, &pkg.PkgType, &pkg.InstallArgs, &pkg.UninstallArgs); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "package not found"})
		return
	}
	if !validSoftwarePackageType(pkg.PkgType) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "package has unsupported type"})
		return
	}

	targets := make([]durableSoftwareTarget, 0, len(req.DeviceIDs))
	var canonicalMSPID, canonicalClientID string
	for _, deviceID := range req.DeviceIDs {
		var target durableSoftwareTarget
		target.DeviceID = deviceID
		if err := db.QueryRowContext(r.Context(), `
			SELECT agent_id::text, msp_id::text, client_id::text
			FROM devices
			WHERE id::text = $1 AND tenant_id = $2
			  AND is_active = TRUE AND status <> 'disabled'
			  AND agent_id IS NOT NULL AND msp_id IS NOT NULL AND client_id IS NOT NULL
		`, deviceID, tenantID).Scan(&target.AgentID, &target.MSPID, &target.ClientID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "one or more devices are unavailable or outside the authorized tenant"})
			return
		}
		if canonicalMSPID == "" {
			canonicalMSPID, canonicalClientID = target.MSPID, target.ClientID
		} else if target.MSPID != canonicalMSPID || target.ClientID != canonicalClientID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "software deployment targets must belong to one MSP client"})
			return
		}
		targets = append(targets, target)
	}

	deployName := strings.TrimSpace(req.Name)
	if deployName == "" {
		deployName = fmt.Sprintf("Deploy %s", pkg.Name)
	}
	var deployID string
	if err := db.QueryRowContext(r.Context(), `
		INSERT INTO software_deployments (package_id, tenant_id, name, schedule_type, status)
		VALUES ($1, $2, $3, $4, 'deploying')
		RETURNING id::text
	`, req.PackageID, tenantID, deployName, req.ScheduleType).Scan(&deployID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating software deployment"})
		return
	}

	jobType := "software_" + req.Action
	commandPayload := map[string]interface{}{
		"type":           jobType,
		"deployment_id":  deployID,
		"action":         req.Action,
		"source_url":     pkg.SourceURL,
		"checksum":       pkg.Checksum,
		"install_args":   pkg.InstallArgs,
		"uninstall_args": pkg.UninstallArgs,
		"package_type":   pkg.PkgType,
		"timeout":        600,
	}
	payloadJSON, err := json.Marshal(commandPayload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encoding software command"})
		return
	}

	jobID := uuid.New().String()
	correlationID := uuid.New().String()
	scheduledFor := time.Now().UTC()
	expiresAt := scheduledFor.Add(72 * time.Hour)
	sortedDevices := append([]string(nil), req.DeviceIDs...)
	sort.Strings(sortedDevices)
	fingerprint, err := json.Marshal(struct {
		DeploymentID string   `json:"deployment_id"`
		Type         string   `json:"type"`
		DeviceIDs    []string `json:"device_ids"`
		Payload      []byte   `json:"payload"`
	}{deployID, jobType, sortedDevices, payloadJSON})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encoding durable software job"})
		return
	}
	requestHash := fmt.Sprintf("%x", sha256.Sum256(fingerprint))

	if _, err := db.ExecContext(r.Context(), `
		INSERT INTO jobs (id, msp_id, client_id, created_by, type, status, priority,
		                  payload, max_retries, max_devices, expires_at,
		                  correlation_id, scheduled_for, request_hash)
		VALUES ($1, $2, $3, 'software-api', $4, 'queued', 0, $5, 3, $6, $7, $8, $9, $10)
	`, jobID, canonicalMSPID, canonicalClientID, jobType, payloadJSON, len(targets), expiresAt, correlationID, scheduledFor, requestHash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating durable software job"})
		return
	}

	for _, target := range targets {
		targetID := uuid.New().String()
		if _, err := db.ExecContext(r.Context(), `
			INSERT INTO job_targets (id, job_id, device_id, agent_id, msp_id, status)
			VALUES ($1, $2, $3, $4, $5, 'queued')
		`, targetID, jobID, target.DeviceID, target.AgentID, canonicalMSPID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating durable software target"})
			return
		}
		if _, err := db.ExecContext(r.Context(), `
			INSERT INTO software_deployment_targets (deployment_id, device_id, status, job_id, job_target_id)
			VALUES ($1, $2, 'pending', $3, $4)
		`, deployID, target.DeviceID, jobID, targetID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating software deployment target"})
			return
		}

		outboxPayload, err := json.Marshal(map[string]interface{}{
			"job_id":         jobID,
			"target_id":      targetID,
			"device_id":      target.DeviceID,
			"agent_id":       target.AgentID,
			"msp_id":         canonicalMSPID,
			"correlation_id": correlationID,
			"attempt":        1,
			"issued_at":      scheduledFor.Format(time.RFC3339),
			"expires_at":     expiresAt.Format(time.RFC3339),
			"type":           jobType,
			"payload":        commandPayload,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encoding software dispatch event"})
			return
		}
		if _, err := db.ExecContext(r.Context(), `
			INSERT INTO job_outbox (id, msp_id, aggregate_id, event_type, payload, available_at)
			VALUES (gen_random_uuid(), $1, $2, 'job.dispatch', $3, $4)
		`, canonicalMSPID, jobID, outboxPayload, scheduledFor); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating durable software dispatch event"})
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"deployment_id": deployID,
		"job_id":        jobID,
		"name":          deployName,
		"package":       pkg.Name,
		"action":        req.Action,
		"targets":       len(targets),
		"durable":       true,
	})
}
