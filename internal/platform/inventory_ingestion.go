package platform

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const maxInventoryPayloadSize = 1 << 20

type InventoryResultPayload struct {
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
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if mspID == "" || deviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context or device id"})
		return
	}
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxInventoryPayloadSize)

	var payload InventoryResultPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if payload.SchemaVersion < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid schema_version"})
		return
	}
	if payload.Data == nil {
		payload.Data = make(map[string]interface{})
	}

	var devMSPID, devStatus string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT msp_id::text, status FROM devices WHERE id = $1
	`, deviceID).Scan(&devMSPID, &devStatus)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
		return
	}
	if devMSPID != mspID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "device not in your MSP scope"})
		return
	}

	if payload.AgentID != "" {
		var regMSPID string
		err := s.requestDB(r).QueryRowContext(r.Context(), `
			SELECT COALESCE(d.msp_id::text, '') FROM agent_registrations ar
			JOIN devices d ON d.id = ar.device_id
			WHERE ar.agent_id = $1 AND ar.device_id = $2
		`, payload.AgentID, deviceID).Scan(&regMSPID)
		if err != nil || regMSPID != mspID {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "agent does not belong to this device"})
			return
		}
	}

	payloadBytes, _ := json.Marshal(payload)
	payloadHash := fmt.Sprintf("%x", sha256.Sum256(payloadBytes))

	isFailure := payload.Failure != ""
	var lastSuccessTime *time.Time
	if !isFailure {
		var t sql.NullTime
		err := s.requestDB(r).QueryRowContext(r.Context(), `
			SELECT MAX(collection_time) FROM inventory_results
			WHERE device_id = $1 AND accepted = true AND is_failure = false
		`, deviceID).Scan(&t)
		if err == nil && t.Valid {
			lastSuccessTime = &t.Time
		}
	}

	collectionTime, _ := time.Parse(time.RFC3339, payload.CollectionTime)
	if collectionTime.IsZero() {
		collectionTime = time.Now().UTC()
	}

	stale := false
	if !isFailure && lastSuccessTime != nil && collectionTime.Before(*lastSuccessTime) {
		stale = true
	}

	accepted := !isFailure && !stale

	resultID := uuid.New().String()
	_, err = s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO inventory_results
			(id, device_id, msp_id, job_id, target_id, correlation_id, schema_version,
			 payload, payload_hash, collection_time, is_stale, is_failure, failure_message, accepted)
		VALUES ($1, $2, $3, NULLIF($4,'')::uuid, NULLIF($5,'')::uuid, NULLIF($6,''),
		        $7, $8, $9, $10, $11, $12, $13, $14)
	`, resultID, deviceID, mspID, payload.JobID, payload.TargetID,
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

		var cpuCores float64
		if v, ok := payload.Data["cpu_cores"].(float64); ok {
			cpuCores = v
		}
		var ramMB, diskMB float64
		if v, ok := payload.Data["memory_mb"].(float64); ok {
			ramMB = v
		}
		if v, ok := payload.Data["disk_mb"].(float64); ok {
			diskMB = v
		}

		_, err = s.requestDB(r).ExecContext(r.Context(), `
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
		`, deviceID, hostname, osStr, osVersion, arch, cpuCores, ramMB, diskMB, collectionTime)
		if err != nil {
			s.logger.Warn("update device from inventory", zap.Error(err))
		}

		// Update job/target state if this was a refresh job
		if payload.JobID != "" && payload.TargetID != "" {
			_, _ = s.requestDB(r).ExecContext(r.Context(), `
				UPDATE job_targets SET status='succeeded', completed_at=NOW()
				WHERE id=$1 AND job_id=$2 AND status IN ('queued','dispatched','running')
			`, payload.TargetID, payload.JobID)
		}
	}

	if err := writeEndpointAudit(r, s.requestDB(r), mspID, "endpoint.inventory.submitted", "device:"+deviceID,
		map[string]interface{}{
			"device_id": deviceID, "accepted": accepted, "stale": stale,
			"is_failure": isFailure, "schema_version": payload.SchemaVersion,
			"inventory_result_id": resultID,
		}); err != nil {
		s.logger.Warn("write inventory audit", zap.Error(err))
	}

	resp := map[string]interface{}{
		"accepted":        accepted,
		"stale":           stale,
		"is_failure":      isFailure,
		"inventory_id":    resultID,
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

func safeString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
