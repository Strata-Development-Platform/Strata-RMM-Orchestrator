package platform

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type AgentCapability struct {
	ID                string   `json:"id"`
	DeviceID          string   `json:"device_id"`
	MSPID             string   `json:"msp_id"`
	AgentVersion      string   `json:"agent_version"`
	ProtocolVersion   int      `json:"protocol_version"`
	OS                string   `json:"os"`
	Arch              string   `json:"arch"`
	SupportedJobTypes []string `json:"supported_job_types"`
	Features          map[string]interface{} `json:"features"`
	InventorySchema   int      `json:"inventory_schema"`
	LastUpdated       string   `json:"last_updated"`
}

func (s *APIServer) handleReportCapabilities(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if mspID == "" || deviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}

	var req struct {
		AgentVersion      string                 `json:"agent_version"`
		ProtocolVersion   int                    `json:"protocol_version"`
		OS                string                 `json:"os"`
		Arch              string                 `json:"arch"`
		SupportedJobTypes []string               `json:"supported_job_types"`
		Features          map[string]interface{} `json:"features"`
		InventorySchema   int                    `json:"inventory_schema"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.SupportedJobTypes == nil {
		req.SupportedJobTypes = []string{}
	}

	var exists bool
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM devices WHERE id = $1 AND msp_id = $2)
	`, deviceID, mspID).Scan(&exists)
	if err != nil || !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
		return
	}

	featuresJSON, _ := json.Marshal(req.Features)
	if featuresJSON == nil {
		featuresJSON = []byte("{}")
	}

	jobTypesStr := "{" + strings.Join(req.SupportedJobTypes, ",") + "}"
	if len(req.SupportedJobTypes) == 0 {
		jobTypesStr = "{}"
	}

	_, err = s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO agent_capabilities
			(device_id, msp_id, agent_version, protocol_version, os, arch,
			 supported_job_types, features, inventory_schema, last_updated)
		VALUES ($1, $2, $3, $4, $5, $6, $7::text[], $8, $9, NOW())
		ON CONFLICT (device_id)
		DO UPDATE SET agent_version=EXCLUDED.agent_version,
		              protocol_version=EXCLUDED.protocol_version,
		              os=EXCLUDED.os, arch=EXCLUDED.arch,
		              supported_job_types=EXCLUDED.supported_job_types,
		              features=EXCLUDED.features,
		              inventory_schema=EXCLUDED.inventory_schema,
		              last_updated=NOW()
	`, deviceID, mspID, req.AgentVersion, req.ProtocolVersion,
		req.OS, req.Arch, jobTypesStr, string(featuresJSON), req.InventorySchema)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store capabilities failed"})
		return
	}

	_, _ = s.requestDB(r).ExecContext(r.Context(), `
		UPDATE devices SET last_capability_update = NOW() WHERE id = $1
	`, deviceID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "capabilities recorded"})
}

func (s *APIServer) handleGetCapabilities(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if mspID == "" || deviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	var cap AgentCapability
	var jobTypesStr string
	var featuresStr string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT device_id::text, msp_id::text, agent_version, protocol_version, os, arch,
		       array_to_string(supported_job_types, ','), COALESCE(features::text, '{}'),
		       inventory_schema, last_updated::text
		FROM agent_capabilities WHERE device_id = $1
	`, deviceID).Scan(&cap.DeviceID, &cap.MSPID, &cap.AgentVersion, &cap.ProtocolVersion,
		&cap.OS, &cap.Arch, &jobTypesStr, &featuresStr, &cap.InventorySchema, &cap.LastUpdated)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "capabilities not found"})
		return
	}
	if jobTypesStr != "" {
		cap.SupportedJobTypes = strings.Split(jobTypesStr, ",")
	}
	if cap.SupportedJobTypes == nil {
		cap.SupportedJobTypes = []string{}
	}
	if featuresStr != "" && featuresStr != "{}" {
		_ = json.Unmarshal([]byte(featuresStr), &cap.Features)
	}
	if cap.Features == nil {
		cap.Features = make(map[string]interface{})
	}
	writeJSON(w, http.StatusOK, cap)
}

func isActionSupportedByCapabilities(action string, capabilities *AgentCapability) bool {
	if capabilities == nil {
		return false
	}
	op, ok := operationRegistry[action]
	if !ok {
		return false
	}
	for _, jobType := range capabilities.SupportedJobTypes {
		if jobType == op.JobType {
			return true
		}
	}
	return false
}

func loadAgentCapabilities(db dbExecutor, deviceID string) (*AgentCapability, error) {
	var cap AgentCapability
	var jobTypesStr string
	err := db.QueryRow(`
		SELECT device_id::text, msp_id::text, agent_version, protocol_version, os, arch,
		       array_to_string(supported_job_types, ','), inventory_schema
		FROM agent_capabilities WHERE device_id = $1
	`, deviceID).Scan(&cap.DeviceID, &cap.MSPID, &cap.AgentVersion, &cap.ProtocolVersion,
		&cap.OS, &cap.Arch, &jobTypesStr, &cap.InventorySchema)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if jobTypesStr != "" {
		cap.SupportedJobTypes = strings.Split(jobTypesStr, ",")
	}
	if cap.SupportedJobTypes == nil {
		cap.SupportedJobTypes = []string{}
	}
	return &cap, nil
}
