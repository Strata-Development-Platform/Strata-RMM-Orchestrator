package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/groups"
)

func (s *APIServer) handleCreateDeviceGroupV2(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	clientID := r.URL.Query().Get("client_id")
	if mspID == "" || clientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "msp_id and client_id required"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}

	var req struct {
		Name             string          `json:"name"`
		Description      string          `json:"description"`
		FilterExpression json.RawMessage `json:"filter_expression"`
		DeviceIDs        []string        `json:"device_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}

	isSmart := groups.IsSmartGroup(req.FilterExpression)

	var memberIDs pq.StringArray
	if !isSmart {
		memberIDs = pq.StringArray(req.DeviceIDs)
		if len(req.DeviceIDs) == 0 {
			memberIDs = pq.StringArray{}
		}
	}

	if req.FilterExpression == nil {
		req.FilterExpression = json.RawMessage("{}")
	}

	id := uuid.New().String()
	_, err := s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO device_groups (id, msp_id, client_id, name, description, member_ids,
		                           filter_expression, is_smart, member_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 0)
	`, id, mspID, clientID, req.Name, req.Description, memberIDs, req.FilterExpression, isSmart)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":              id,
		"name":            req.Name,
		"is_smart":        isSmart,
		"member_count":    0,
		"status":          "created",
	})
}

func (s *APIServer) handleGetDeviceGroup(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	groupID := r.PathValue("groupID")
	if mspID == "" || groupID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "msp_id and groupID required"})
		return
	}

	var id, name, description, filterExprStr, mspIDStr, clientIDStr string
	var isSmart bool
	var memberCount int
	var lastEvaluated *time.Time
	var createdAt time.Time

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, name, description, filter_expression::text, msp_id, client_id::text,
		       is_smart, member_count, last_evaluated, created_at
		FROM device_groups WHERE id = $1 AND msp_id = $2
	`, groupID, mspID).Scan(&id, &name, &description, &filterExprStr, &mspIDStr, &clientIDStr,
		&isSmart, &memberCount, &lastEvaluated, &createdAt)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	resp := map[string]interface{}{
		"id":           id,
		"name":         name,
		"description":  description,
		"is_smart":     isSmart,
		"member_count": memberCount,
		"created_at":   createdAt.UTC().Format(time.RFC3339),
	}
	if lastEvaluated != nil {
		resp["last_evaluated"] = lastEvaluated.UTC().Format(time.RFC3339)
	}
	if filterExprStr != "" && filterExprStr != "{}" {
		var expr groups.Expression
		if err := json.Unmarshal([]byte(filterExprStr), &expr); err == nil {
			resp["filter_expression"] = expr
		} else {
			resp["filter_expression"] = json.RawMessage(filterExprStr)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *APIServer) handleListDeviceGroupMembers(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	groupID := r.PathValue("groupID")
	if mspID == "" || groupID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "msp_id and groupID required"})
		return
	}

	limit := parseInt(r.URL.Query().Get("limit"), 50)
	if limit > 200 {
		limit = 200
	}
	offset := parseInt(r.URL.Query().Get("offset"), 0)

	// Verify group exists and belongs to this MSP
	var groupExists bool
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM device_groups WHERE id = $1 AND msp_id = $2)
	`, groupID, mspID).Scan(&groupExists)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !groupExists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT gm.device_id, gm.evaluated_at, d.hostname, d.os, d.status, d.last_heartbeat
		FROM group_memberships gm
		JOIN devices d ON gm.device_id = d.id
		WHERE gm.group_id = $1 AND d.msp_id = $2
		ORDER BY d.hostname ASC
		LIMIT $3 OFFSET $4
	`, groupID, mspID, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = rows.Close() }()

	var members []map[string]interface{}
	for rows.Next() {
		var deviceID, hostname, os, status string
		var evaluatedAt time.Time
		var lastHeartbeat *time.Time
		if err := rows.Scan(&deviceID, &evaluatedAt, &hostname, &os, &status, &lastHeartbeat); err != nil {
			continue
		}
		members = append(members, map[string]interface{}{
			"device_id":    deviceID,
			"hostname":     hostname,
			"os":           os,
			"status":       status,
			"evaluated_at": evaluatedAt.UTC().Format(time.RFC3339),
		})
	}
	if members == nil {
		members = []map[string]interface{}{}
	}

	var totalCount int
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM group_memberships WHERE group_id = $1
	`, groupID).Scan(&totalCount); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"members":   members,
		"total":     totalCount,
		"limit":     limit,
		"offset":    offset,
	})
}

func (s *APIServer) handleTriggerGroupEvaluate(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	groupID := r.PathValue("groupID")
	if mspID == "" || groupID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "msp_id and groupID required"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}

	// Verify group exists, is smart, and belongs to this MSP
	var isSmart bool
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT is_smart FROM device_groups WHERE id = $1 AND msp_id = $2
	`, groupID, mspID).Scan(&isSmart)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !isSmart {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "group is not a smart group"})
		return
	}

	// Start evaluation in background
	go s.evaluateSingleGroup(groupID, mspID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"group_id":      groupID,
		"status":        "evaluation_started",
		"message":       "group evaluation triggered",
	})
}

func (s *APIServer) handleGetEvaluationStatus(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	groupID := r.PathValue("groupID")
	if mspID == "" || groupID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "msp_id and groupID required"})
		return
	}

	var isSmart bool
	var lastEvaluated *time.Time
	var memberCount int

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT is_smart, last_evaluated, member_count
		FROM device_groups WHERE id = $1 AND msp_id = $2
	`, groupID, mspID).Scan(&isSmart, &lastEvaluated, &memberCount)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	resp := map[string]interface{}{
		"group_id":      groupID,
		"is_smart":      isSmart,
		"member_count":  memberCount,
		"evaluation_pending": !isSmart,
	}
	if isSmart {
		resp["evaluation_pending"] = false
		if lastEvaluated != nil {
			resp["last_evaluated"] = lastEvaluated.UTC().Format(time.RFC3339)
			resp["evaluation_pending"] = false
		} else {
			resp["last_evaluated"] = nil
			resp["evaluation_pending"] = true
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// evaluateSingleGroup performs a single evaluation cycle for one smart group.
func (s *APIServer) evaluateSingleGroup(groupID, mspID string) {
	ctx := s.evaluationContext()

	var clientID uuid.UUID
	var filterExpressionBytes []byte
	err := s.db.DB().QueryRowContext(ctx, `
		SELECT client_id, filter_expression FROM device_groups WHERE id = $1 AND msp_id = $2
	`, groupID, mspID).Scan(&clientID, &filterExpressionBytes)
	if err != nil {
		s.logger.Error("evaluateSingleGroup: query group", zap.String("group_id", groupID), zap.Error(err))
		return
	}

	var expr groups.Expression
	if err := json.Unmarshal(filterExpressionBytes, &expr); err != nil {
		s.logger.Error("evaluateSingleGroup: parse expression", zap.String("group_id", groupID), zap.Error(err))
		return
	}

	rows, err := s.db.DB().QueryContext(ctx, `
		SELECT id, hostname, os, arch, cpu_cores, ram_total_mb, disk_total_mb,
		       agent_version, status, last_heartbeat, created_at,
		       COALESCE(tags::text, '[]'::text),
		       COALESCE(private_ips::text, '[]'::text),
		       COALESCE(public_ip, '')
		FROM devices WHERE msp_id = $1 AND client_id = $2 AND status != 'disabled'
	`, mspID, clientID)
	if err != nil {
		s.logger.Error("evaluateSingleGroup: query devices", zap.String("group_id", groupID), zap.Error(err))
		return
	}
	defer func() { _ = rows.Close() }()

	type deviceInfo struct {
		ID       string
		Hostname string
		OS       string
		Arch     string
		CPUCores int
		RAM      int64
		Disk     int64
		AgentVer string
		Status   string
		HB       *time.Time
		Created  time.Time
		Tags     []string
		Private  []string
		PublicIP string
	}

	var matchedDevices []string
	for rows.Next() {
		var tagsBytes, privateBytes, publicIP string
		var info deviceInfo
		if err := rows.Scan(&info.ID, &info.Hostname, &info.OS, &info.Arch, &info.CPUCores,
			&info.RAM, &info.Disk, &info.AgentVer, &info.Status, &info.HB, &info.Created,
			&tagsBytes, &privateBytes, &publicIP); err != nil {
			continue
		}
		info.PublicIP = publicIP

		tags := parseStringArray(tagsBytes)
		private := parseStringArray(privateBytes)
		info.Tags = tags
		info.Private = private

		dev := groups.Device{
			Hostname:      info.Hostname,
			OS:            info.OS,
			Arch:          info.Arch,
			CPUCores:      info.CPUCores,
			RAMTotalMB:    info.RAM,
			DiskTotalMB:   info.Disk,
			AgentVersion:  info.AgentVer,
			Status:        info.Status,
			LastHeartbeat: info.HB,
			CreatedAt:     info.Created,
			Tags:          tags,
			PublicIP:      publicIP,
			PrivateIPs:    private,
		}
		if matched, err := expr.Evaluate(dev); err == nil && matched {
			matchedDevices = append(matchedDevices, info.ID)
		}
	}

	// Atomically update memberships
	tx, err := s.db.DB().BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error("evaluateSingleGroup: begin tx", zap.String("group_id", groupID), zap.Error(err))
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `DELETE FROM group_memberships WHERE group_id = $1`, groupID)
	if err != nil {
		s.logger.Error("evaluateSingleGroup: delete old memberships", zap.String("group_id", groupID), zap.Error(err))
		return
	}

	for _, devID := range matchedDevices {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO group_memberships (group_id, device_id, evaluated_at)
			VALUES ($1, $2, NOW()) ON CONFLICT DO NOTHING
		`, groupID, devID)
		if err != nil {
			s.logger.Error("evaluateSingleGroup: insert membership", zap.String("group_id", groupID), zap.String("device_id", devID), zap.Error(err))
			return
		}
	}

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
		UPDATE device_groups SET last_evaluated = $1, member_count = $2 WHERE id = $3
	`, now, len(matchedDevices), groupID)
	if err != nil {
		s.logger.Error("evaluateSingleGroup: update group", zap.String("group_id", groupID), zap.Error(err))
		return
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("evaluateSingleGroup: commit", zap.String("group_id", groupID), zap.Error(err))
		return
	}

	s.logger.Info("smart group evaluation complete",
		zap.String("group_id", groupID),
		zap.Int("matched", len(matchedDevices)),
	)
}

func parseStringArray(s string) []string {
	if s == "" || s == "null" {
		return []string{}
	}
	var result []string
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return []string{}
	}
	return result
}

func (s *APIServer) evaluationContext() context.Context {
	ctx := context.Background()
	// Set up RLS context for the evaluation
	// This is called from background goroutines, so we use the DB directly
	return ctx
}

// evaluateAllSmartGroups is the main loop called by SmartGroupSync.
func (s *APIServer) evaluateAllSmartGroups(ctx context.Context) {
	rows, err := s.db.DB().QueryContext(ctx, `
		SELECT id, msp_id, client_id, filter_expression
		FROM device_groups WHERE is_smart = true
	`)
	if err != nil {
		s.logger.Error("evaluateAllSmartGroups: query smart groups", zap.Error(err))
		return
	}
	defer func() { _ = rows.Close() }()

	type smartGroup struct {
		ID               string
		MSPID            string
		ClientID         string
		FilterExpression json.RawMessage
	}

	var groups []smartGroup
	for rows.Next() {
		var g smartGroup
		if err := rows.Scan(&g.ID, &g.MSPID, &g.ClientID, &g.FilterExpression); err != nil {
			continue
		}
		groups = append(groups, g)
	}

	for _, g := range groups {
		select {
		case <-ctx.Done():
			return
		default:
			s.evaluateSingleGroup(g.ID, g.MSPID)
		}
	}
}
