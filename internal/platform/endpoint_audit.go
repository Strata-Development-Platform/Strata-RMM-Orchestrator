package platform

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type EndpointAuditEntry struct {
	ID                 string          `json:"id"`
	MSPID              string          `json:"msp_id"`
	ClientID           string          `json:"client_id,omitempty"`
	SiteID             string          `json:"site_id,omitempty"`
	DeviceID           string          `json:"device_id,omitempty"`
	ActorUserID        string          `json:"actor_user_id"`
	ActorRole          string          `json:"actor_role,omitempty"`
	SupportGrantID     string          `json:"support_grant_id,omitempty"`
	RequestSource      string          `json:"request_source"`
	NormalizedIP       string          `json:"normalized_ip,omitempty"`
	Action             string          `json:"action"`
	Targets            json.RawMessage `json:"targets"`
	Reason             string          `json:"reason,omitempty"`
	RequestHash        string          `json:"request_hash,omitempty"`
	IdempotencyKey     string          `json:"idempotency_key,omitempty"`
	PolicySnapshot     json.RawMessage `json:"policy_snapshot,omitempty"`
	ApprovalState      string          `json:"approval_state"`
	ApprovalDecisions  json.RawMessage `json:"approval_decisions,omitempty"`
	JobID              string          `json:"job_id,omitempty"`
	TargetID           string          `json:"target_id,omitempty"`
	CorrelationID      string          `json:"correlation_id,omitempty"`
	ScheduleAt         *time.Time      `json:"schedule_at,omitempty"`
	MaintenanceWindow  json.RawMessage `json:"maintenance_window,omitempty"`
	StateTransition    string          `json:"state_transition"`
	AgentReceiptAt     *time.Time      `json:"agent_receipt_at,omitempty"`
	ExecutionStartedAt *time.Time      `json:"execution_started_at,omitempty"`
	ExecutionResult    json.RawMessage `json:"execution_result,omitempty"`
	ExitCode           *int            `json:"exit_code,omitempty"`
	ResultSummary      string          `json:"result_summary,omitempty"`
	FailureReason      string          `json:"failure_reason,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}

//nolint:unused
func writeEndpointAuditEvidence(r *http.Request, db dbExecutor, entry *EndpointAuditEntry) error {
	if entry.MSPID == "" {
		return fmt.Errorf("msp_id is required for audit evidence")
	}

	targetsJSON := entry.Targets
	if targetsJSON == nil {
		targetsJSON = json.RawMessage("[]")
	}
	policySnap := entry.PolicySnapshot
	if policySnap == nil {
		policySnap = json.RawMessage("{}")
	}
	approvalDec := entry.ApprovalDecisions
	if approvalDec == nil {
		approvalDec = json.RawMessage("[]")
	}
	mwJSON := entry.MaintenanceWindow
	if mwJSON == nil {
		mwJSON = json.RawMessage("null")
	}
	execResult := entry.ExecutionResult
	if execResult == nil {
		execResult = json.RawMessage("null")
	}

	_, err := db.ExecContext(r.Context(), `
		INSERT INTO endpoint_audit_evidence
			(id, msp_id, client_id, site_id, device_id, actor_user_id, actor_role,
			 support_grant_id, request_source, normalized_ip, action, targets, reason,
			 request_hash, idempotency_key, policy_snapshot, approval_state,
			 approval_decisions, job_id, target_id, correlation_id, schedule_at,
			 maintenance_window, state_transition, agent_receipt_at,
			 execution_started_at, execution_result, exit_code, result_summary,
			 failure_reason)
		VALUES (gen_random_uuid(), $1, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid,
		        NULLIF($4,'')::uuid, $5, $6, NULLIF($7,''), $8, NULLIF($9,''), $10, $11,
		        $12, $13, NULLIF($14,''), $15, $16, $17, NULLIF($18,''), NULLIF($19,''),
		        NULLIF($20,''), $21, $22, $23, $24, $25, $26, $27, $28, $29)
	`, entry.MSPID, entry.ClientID, entry.SiteID, entry.DeviceID,
		entry.ActorUserID, entry.ActorRole, entry.SupportGrantID,
		entry.RequestSource, entry.NormalizedIP, entry.Action, string(targetsJSON),
		entry.Reason, entry.RequestHash, entry.IdempotencyKey, string(policySnap),
		entry.ApprovalState, string(approvalDec), entry.JobID, entry.TargetID,
		entry.CorrelationID, entry.ScheduleAt, string(mwJSON),
		entry.StateTransition, entry.AgentReceiptAt, entry.ExecutionStartedAt,
		string(execResult), entry.ExitCode, entry.ResultSummary, entry.FailureReason)
	return err
}

func (s *APIServer) handleListEndpointAuditEvidence(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	limit := parseInt(r.URL.Query().Get("limit"), 50)
	if limit > 200 {
		limit = 200
	}
	offset := parseInt(r.URL.Query().Get("offset"), 0)
	deviceID := r.URL.Query().Get("device_id")
	action := r.URL.Query().Get("action")
	jobID := r.URL.Query().Get("job_id")

	query := `SELECT id::text, msp_id::text, actor_user_id, action, targets::text,
	                 state_transition, correlation_id, job_id, result_summary,
	                 failure_reason, created_at
	          FROM endpoint_audit_evidence WHERE msp_id = $1`
	args := []interface{}{mspID}
	argIdx := 2

	if deviceID != "" {
		query += fmt.Sprintf(" AND device_id::text = $%d", argIdx)
		args = append(args, deviceID)
		argIdx++
	}
	if action != "" {
		query += fmt.Sprintf(" AND action = $%d", argIdx)
		args = append(args, action)
		argIdx++
	}
	if jobID != "" {
		query += fmt.Sprintf(" AND job_id = $%d", argIdx)
		args = append(args, jobID)
		argIdx++
	}
	query += " ORDER BY created_at DESC LIMIT $%d OFFSET $%d"
	query = fmt.Sprintf(query, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.requestDB(r).QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer func() { _ = rows.Close() }()

	type evidenceRow struct {
		ID              string `json:"id"`
		MSPID           string `json:"msp_id"`
		ActorUserID     string `json:"actor_user_id"`
		Action          string `json:"action"`
		Targets         string `json:"targets"`
		StateTransition string `json:"state_transition"`
		CorrelationID   string `json:"correlation_id,omitempty"`
		JobID           string `json:"job_id,omitempty"`
		ResultSummary   string `json:"result_summary"`
		FailureReason   string `json:"failure_reason"`
		CreatedAt       time.Time `json:"created_at"`
	}

	var results []evidenceRow
	for rows.Next() {
		var e evidenceRow
		var jobID sql.NullString
		if err := rows.Scan(&e.ID, &e.MSPID, &e.ActorUserID, &e.Action, &e.Targets,
			&e.StateTransition, &e.CorrelationID, &jobID, &e.ResultSummary,
			&e.FailureReason, &e.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		if jobID.Valid {
			e.JobID = jobID.String
		}
		results = append(results, e)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan error"})
		return
	}
	if results == nil {
		results = []evidenceRow{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"evidence": results})
}

//nolint:unused
func isAuditEvidenceUpdateDenied(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || err == nil
}
