package platform

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const maxInventoryPayloadSize = 1 << 20

type InventoryResultPayload struct {
	ResultID       string                 `json:"result_id"`
	SchemaVersion  int                    `json:"schema_version"`
	DeviceID       string                 `json:"device_id"`
	AgentID        string                 `json:"agent_id"`
	JobID          string                 `json:"job_id"`
	TargetID       string                 `json:"target_id"`
	CorrelationID  string                 `json:"correlation_id"`
	CollectionTime string                 `json:"collection_time"`
	Data           map[string]interface{} `json:"data"`
	Failure        string                 `json:"failure,omitempty"`
}

func (s *APIServer) handleSubmitInventoryResult(w http.ResponseWriter, r *http.Request) {
	pathDeviceID := r.PathValue("deviceID")
	tokenUse, _ := r.Context().Value(ctxKeyTokenUse).(string)
	authUserID, _ := r.Context().Value(ctxKeyUserID).(string)

	if tokenUse != "agent" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "agent token required"})
		return
	}

	if pathDeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device id required"})
		return
	}
	if _, err := uuid.Parse(pathDeviceID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxInventoryPayloadSize)

	var payload InventoryResultPayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	// Validate payload structural fields
	if _, err := uuid.Parse(payload.ResultID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid result_id required"})
		return
	}
	if err := validateDeviceInventoryResult(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if payload.SchemaVersion != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid schema_version"})
		return
	}
	if payload.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload device_id required"})
		return
	}
	if payload.DeviceID != pathDeviceID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload device_id does not match path"})
		return
	}
	if payload.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload agent_id required"})
		return
	}
	if payload.AgentID != authUserID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "authenticated agent does not match payload agent_id"})
		return
	}

	// Validate collection timestamp
	collectionTime, err := time.Parse(time.RFC3339, payload.CollectionTime)
	if payload.CollectionTime != "" && err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid collection_time"})
		return
	}
	if payload.CollectionTime == "" {
		collectionTime = time.Now().UTC()
	}
	if !collectionTime.IsZero() && collectionTime.After(time.Now().UTC().Add(5*time.Minute)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "collection_time cannot be in the future"})
		return
	}

	if payload.Data == nil {
		payload.Data = make(map[string]interface{})
	}

	// Verify device exists and get scope
	var devMSPID, devClientID, devSiteID, devStatus string
	err = s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT msp_id::text, COALESCE(client_id::text,''), COALESCE(site_id::text,''), status
		FROM devices WHERE id = $1
	`, pathDeviceID).Scan(&devMSPID, &devClientID, &devSiteID, &devStatus)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
		return
	}

	// Verify agent registration belongs to this device
	var regMSPID, regDeviceID string
	err = s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT COALESCE(d.msp_id::text, ''), COALESCE(d.id::text, '')
		FROM agent_registrations ar
		JOIN devices d ON d.id = ar.device_id
		WHERE ar.agent_id = $1 AND ar.approved = true AND ar.device_id = $2
	`, authUserID, pathDeviceID).Scan(&regMSPID, &regDeviceID)
	if err != nil || regDeviceID != pathDeviceID || regMSPID != devMSPID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "agent not registered or approved for this device"})
		return
	}

	// Verify job/target ownership if provided
	if payload.JobID == "" || payload.TargetID == "" || payload.CorrelationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job_id, target_id and correlation_id required"})
		return
	}
	if payload.JobID != "" {
		if _, err := uuid.Parse(payload.JobID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid job_id"})
			return
		}
		if payload.TargetID != "" {
			if _, err := uuid.Parse(payload.TargetID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target_id"})
				return
			}
			var jobDeviceID, jobMSPID, targetAgentID, jobCorrelationID, jobType, targetStatus string
			err = s.requestDB(r).QueryRowContext(r.Context(), `
				SELECT jt.device_id::text, j.msp_id::text, COALESCE(jt.agent_id,''),
				       COALESCE(j.correlation_id,''), j.type, jt.status
				FROM job_targets jt JOIN jobs j ON j.id = jt.job_id
				WHERE jt.id = $1 AND jt.job_id = $2
			`, payload.TargetID, payload.JobID).Scan(&jobDeviceID, &jobMSPID, &targetAgentID,
				&jobCorrelationID, &jobType, &targetStatus)
			if err != nil || jobDeviceID != pathDeviceID || jobMSPID != devMSPID ||
				targetAgentID != authUserID || jobCorrelationID != payload.CorrelationID ||
				jobType != "device.refresh" {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "job/target identity mismatch"})
				return
			}
			if targetStatus != "dispatched" && targetStatus != "running" {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "target cannot accept inventory in current state"})
				return
			}
		}
	}

	// Determine staleness
	payloadBytes, _ := json.Marshal(payload)
	payloadHash := fmt.Sprintf("%x", sha256.Sum256(payloadBytes))

	isFailure := payload.Failure != ""
	var lastSuccessTime *time.Time
	if !isFailure {
		var t sql.NullTime
		err := s.requestDB(r).QueryRowContext(r.Context(), `
			SELECT MAX(collection_time) FROM inventory_results
			WHERE device_id = $1 AND accepted = true AND is_failure = false
		`, pathDeviceID).Scan(&t)
		if err == nil && t.Valid {
			lastSuccessTime = &t.Time
		}
	}

	// The middleware-provided tenant transaction makes inventory, device,
	// target, and audit mutations atomic.
	tx := s.requestDB(r)

	var existingHash string
	err = tx.QueryRowContext(r.Context(), `
		SELECT payload_hash FROM inventory_results WHERE id = $1 AND device_id = $2 AND msp_id = $3
	`, payload.ResultID, pathDeviceID, devMSPID).Scan(&existingHash)
	if err == nil {
		if existingHash != payloadHash {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "result_id already used for different content"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"accepted": true, "duplicate": true, "inventory_id": payload.ResultID})
		return
	}
	if err != nil && err != sql.ErrNoRows {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "inventory idempotency lookup failed"})
		return
	}

	stale := !isFailure && lastSuccessTime != nil && collectionTime.Before(*lastSuccessTime)
	accepted := !isFailure && !stale

	resultID := payload.ResultID
	_, err = tx.ExecContext(r.Context(), `
		INSERT INTO inventory_results
			(id, device_id, msp_id, job_id, target_id, correlation_id, schema_version,
			 payload, payload_hash, collection_time, is_stale, is_failure, failure_message, accepted)
		VALUES ($1, $2, $3, NULLIF($4,'')::uuid, NULLIF($5,'')::uuid, NULLIF($6,''),
		        $7, $8, $9, $10, $11, $12, $13, $14)
	`, resultID, pathDeviceID, devMSPID, payload.JobID, payload.TargetID,
		payload.CorrelationID, payload.SchemaVersion, string(payloadBytes),
		payloadHash, collectionTime, stale, isFailure, payload.Failure, accepted)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store inventory failed"})
		return
	}

	if accepted {
		hostname, _ := payload.Data["hostname"].(string)
		osStr, _ := payload.Data["os"].(string)
		osVersion, _ := payload.Data["os_version"].(string)
		arch, _ := payload.Data["arch"].(string)

		var cpuCores int64
		if v, ok := payload.Data["cpu_cores"].(float64); ok {
			cpuCores = int64(v)
		}
		var ramMB, diskMB int64
		if v, ok := payload.Data["memory_mb"].(float64); ok {
			ramMB = int64(v)
		}
		if v, ok := payload.Data["disk_mb"].(float64); ok {
			diskMB = int64(v)
		}

		result, err := tx.ExecContext(r.Context(), `
			UPDATE devices SET
				hostname = CASE WHEN $2 != '' THEN $2 ELSE hostname END,
				os = CASE WHEN $3 != '' THEN $3 ELSE os END,
				os_version = CASE WHEN $4 != '' THEN $4 ELSE os_version END,
				arch = CASE WHEN $5 != '' THEN $5 ELSE arch END,
				cpu_cores = CASE WHEN $6 > 0 THEN $6::int ELSE cpu_cores END,
				ram_total_mb = CASE WHEN $7 > 0 THEN $7::bigint ELSE ram_total_mb END,
				disk_total_mb = CASE WHEN $8 > 0 THEN $8::bigint ELSE disk_total_mb END,
				inventory_last_success = $9,
				inventory_fresh = true,
				updated_at = NOW()
			WHERE id = $1
		`, pathDeviceID, hostname, osStr, osVersion, arch, cpuCores, ramMB, diskMB, collectionTime)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update device from inventory failed"})
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device not updated"})
			return
		}

		// Update job/target state if this was a refresh job
		if payload.JobID != "" && payload.TargetID != "" {
			targetResult, err := tx.ExecContext(r.Context(), `
				UPDATE job_targets SET status='succeeded', completed_at=NOW()
				WHERE id=$1 AND job_id=$2 AND agent_id=$3 AND status IN ('dispatched','running')
			`, payload.TargetID, payload.JobID, authUserID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update target failed"})
				return
			}
			if affected, err := targetResult.RowsAffected(); err != nil || affected != 1 {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "target state changed before inventory acceptance"})
				return
			}
		}
	}
	

	// Audit evidence
	entry := EndpointAuditEntry{
		MSPID: devMSPID, ClientID: devClientID, SiteID: devSiteID,
		DeviceID: pathDeviceID, ActorUserID: authUserID,
		ActorRole: "agent", RequestSource: "agent",
		Action: "endpoint.inventory.submitted",
		JobID:  payload.JobID, CorrelationID: payload.CorrelationID,
		ApprovalState: "none",
		StateTransition: func() string {
			if accepted {
				return "accepted"
			}
			if stale {
				return "rejected_stale"
			}
			return "rejected"
		}(),
		ResultSummary: func() string {
			if accepted {
				return "inventory accepted"
			}
			if stale {
				return "inventory rejected: stale"
			}
			if isFailure {
				return "collection failure: " + payload.Failure
			}
			return "inventory rejected"
		}(),
	}
	if err := writeEndpointAuditEvidence(r, tx, &entry); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write audit evidence failed"})
		return
	}

	resp := map[string]interface{}{
		"accepted":     accepted,
		"stale":        stale,
		"is_failure":   isFailure,
		"inventory_id": resultID,
	}
	if stale {
		resp["message"] = "inventory rejected: newer collection exists"
	} else if isFailure {
		resp["message"] = "collection failure recorded"
	} else {
		resp["message"] = "inventory accepted"
	}
	writeJSON(w, http.StatusOK, resp)
}

func validateDeviceInventoryResult(payload *InventoryResultPayload) error {
	if payload.SchemaVersion < 1 {
		return fmt.Errorf("invalid schema_version: %d", payload.SchemaVersion)
	}
	if payload.DeviceID == "" {
		return fmt.Errorf("device_id is required")
	}
	if err := validateUUID(payload.DeviceID); err != nil {
		return fmt.Errorf("invalid device_id: %w", err)
	}
	if payload.AgentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	collectionTime, err := time.Parse(time.RFC3339, payload.CollectionTime)
	if err != nil && payload.CollectionTime != "" {
		return fmt.Errorf("invalid collection_time: %w", err)
	}
	if !collectionTime.IsZero() && collectionTime.After(time.Now().Add(5*time.Minute)) {
		return fmt.Errorf("collection_time cannot be in the future")
	}
	return nil
}

func validateUUID(s string) error {
	if _, err := uuid.Parse(s); err != nil {
		return err
	}
	return nil
}
