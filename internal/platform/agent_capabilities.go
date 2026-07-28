package platform

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

const maxCapabilityPayloadSize = 1 << 18

type AgentCapability struct {
	ID                string                 `json:"id"`
	DeviceID          string                 `json:"device_id"`
	MSPID             string                 `json:"msp_id"`
	AgentVersion      string                 `json:"agent_version"`
	ProtocolVersion   int                    `json:"protocol_version"`
	OS                string                 `json:"os"`
	Arch              string                 `json:"arch"`
	SupportedJobTypes []string               `json:"supported_job_types"`
	Features          map[string]interface{} `json:"features"`
	InventorySchema   int                    `json:"inventory_schema"`
	LastUpdated       string                 `json:"last_updated"`
}

func (s *APIServer) handleReportCapabilities(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	tokenUse, _ := r.Context().Value(ctxKeyTokenUse).(string)
	authUserID, _ := r.Context().Value(ctxKeyUserID).(string)

	if tokenUse != "agent" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "agent token required"})
		return
	}

	if deviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device id required"})
		return
	}
	if _, err := uuid.Parse(deviceID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
		return
	}

	var devMSPID, devClientID, devSiteID, devTenantID string
	var devStatus string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT msp_id::text, COALESCE(client_id::text,''), COALESCE(site_id::text,''),
		       COALESCE(tenant_id::text,''), status
		FROM devices WHERE id = $1
	`, deviceID).Scan(&devMSPID, &devClientID, &devSiteID, &devTenantID, &devStatus)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
		return
	}
	if devStatus == "disabled" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "device is disabled"})
		return
	}

	var agentMSPID, agentDeviceID string
	err = s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT d.msp_id::text, d.id::text
		FROM agent_registrations ar
		JOIN devices d ON d.id = ar.device_id
		WHERE ar.agent_id = $1 AND ar.approved = true AND ar.device_id = $2
	`, authUserID, deviceID).Scan(&agentMSPID, &agentDeviceID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "agent not registered or approved for this device"})
		return
	}

	var mspActive bool
	if err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT is_active FROM msp_tenants WHERE id = $1`, devMSPID).Scan(&mspActive); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "msp check failed"})
		return
	}
	if !mspActive {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "msp is suspended"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCapabilityPayloadSize)

	var req struct {
		AgentVersion      string                 `json:"agent_version"`
		ProtocolVersion   int                    `json:"protocol_version"`
		OS                string                 `json:"os"`
		Arch              string                 `json:"arch"`
		SupportedJobTypes []string               `json:"supported_job_types"`
		Features          map[string]interface{} `json:"features"`
		InventorySchema   int                    `json:"inventory_schema"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if strings.TrimSpace(req.AgentVersion) == "" || len(req.AgentVersion) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid agent_version required"})
		return
	}
	if req.ProtocolVersion != 1 || req.InventorySchema != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported protocol or inventory schema version"})
		return
	}
	validOS := map[string]bool{"linux": true, "windows": true, "darwin": true}
	validArch := map[string]bool{"amd64": true, "arm64": true}
	if !validOS[req.OS] || !validArch[req.Arch] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported os or architecture"})
		return
	}
	if len(req.SupportedJobTypes) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "too many job types"})
		return
	}

	registeredTypes := make(map[string]bool)
	for _, operation := range operationRegistry {
		registeredTypes[operation.JobType] = true
	}
	validTypes := make([]string, 0, len(req.SupportedJobTypes))
	seenTypes := make(map[string]bool)
	for _, jt := range req.SupportedJobTypes {
		if !registeredTypes[jt] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported job type"})
			return
		}
		if !seenTypes[jt] {
			seenTypes[jt] = true
			validTypes = append(validTypes, jt)
		}
	}
	if validTypes == nil {
		validTypes = []string{}
	}

	if req.SupportedJobTypes == nil {
		req.SupportedJobTypes = []string{}
	}

	featuresJSON, _ := json.Marshal(req.Features)
	if featuresJSON == nil {
		featuresJSON = []byte("{}")
	}

	jobTypesStr := "{" + strings.Join(validTypes, ",") + "}"
	if len(validTypes) == 0 {
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
	`, deviceID, devMSPID, req.AgentVersion, req.ProtocolVersion,
		req.OS, req.Arch, jobTypesStr, string(featuresJSON), req.InventorySchema)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store capabilities failed"})
		return
	}

	result, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE devices SET last_capability_update = NOW() WHERE id = $1
	`, deviceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update capability timestamp failed"})
		return
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device capability update failed"})
		return
	}

	targets, err := json.Marshal([]string{deviceID})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode audit targets failed"})
		return
	}
	if err := writeEndpointAuditEvidence(r, s.requestDB(r), &EndpointAuditEntry{
		MSPID: devMSPID, ClientID: devClientID, SiteID: devSiteID, DeviceID: deviceID,
		ActorUserID: authUserID, ActorRole: "agent", RequestSource: "agent",
		Action: "endpoint.capability.reported", Targets: targets,
		ApprovalState: "none", StateTransition: "capabilities_updated",
		ResultSummary: fmt.Sprintf("agent %s reported %d capabilities", req.AgentVersion, len(validTypes)),
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write audit evidence failed"})
		return
	}

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
