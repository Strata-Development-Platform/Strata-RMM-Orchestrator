package platform

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

const maxSoftwareResultErrorBytes = 4096

type SoftwareEngine struct {
	nc     *nats.Conn
	db     *sql.DB
	logger *zap.Logger
}

func NewSoftwareEngine(nc *nats.Conn, db *sql.DB, logger *zap.Logger) *SoftwareEngine {
	return &SoftwareEngine{nc: nc, db: db, logger: logger}
}

func (s *APIServer) handleCreatePackage(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	var req struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		Description   string `json:"description"`
		Platform      string `json:"platform"`
		PackageType   string `json:"package_type"`
		SourceURL     string `json:"source_url"`
		Checksum      string `json:"checksum"`
		InstallArgs   string `json:"install_args"`
		UninstallArgs string `json:"uninstall_args"`
		DetectCommand string `json:"detect_command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.SourceURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and source_url required"})
		return
	}
	if req.Platform == "" {
		req.Platform = "all"
	}
	if !validSoftwarePackageType(req.PackageType) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported package_type"})
		return
	}

	var pkgID string
	var createdAt time.Time
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		INSERT INTO software_packages (tenant_id, name, version, description, platform, package_type, source_url, checksum, install_args, uninstall_args, detect_command)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`, tenantID, req.Name, req.Version, req.Description, req.Platform, req.PackageType,
		req.SourceURL, req.Checksum, req.InstallArgs, req.UninstallArgs, req.DetectCommand).Scan(&pkgID, &createdAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id": pkgID, "name": req.Name, "version": req.Version,
		"package_type": req.PackageType, "created_at": createdAt,
	})
}

func validSoftwarePackageType(value string) bool {
	switch value {
	case "msi", "exe", "deb", "rpm", "appimage", "script":
		return true
	default:
		return false
	}
}

func (s *APIServer) handleListPackages(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, name, version, description, platform, package_type, source_url, checksum, install_args, created_at, updated_at
		FROM software_packages WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var pkgs []map[string]interface{}
	for rows.Next() {
		var id, name, version, desc, platform, pkgType, srcURL, checksum, installArgs string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &version, &desc, &platform, &pkgType, &srcURL, &checksum, &installArgs, &createdAt, &updatedAt); err != nil {
			continue
		}
		pkgs = append(pkgs, map[string]interface{}{
			"id": id, "name": name, "version": version, "description": desc,
			"platform": platform, "package_type": pkgType, "source_url": srcURL,
			"checksum": checksum, "install_args": installArgs,
			"created_at": createdAt, "updated_at": updatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"packages": pkgs})
}

func (s *APIServer) handleDeletePackage(w http.ResponseWriter, r *http.Request) {
	pkgID := r.PathValue("pkgID")
	tenantID := r.PathValue("tenantID")
	_, err := s.requestDB(r).ExecContext(r.Context(), `DELETE FROM software_packages WHERE id = $1 AND tenant_id = $2`, pkgID, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *APIServer) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	s.handleCreateDurableSoftwareDeployment(w, r)
}

func (s *APIServer) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT d.id, d.name, sp.name as package_name, d.status, d.schedule_type, d.scheduled_for, d.created_at, d.completed_at,
		       (SELECT COUNT(*) FROM software_deployment_targets t WHERE t.deployment_id = d.id) as total,
		       (SELECT COUNT(*) FROM software_deployment_targets t WHERE t.deployment_id = d.id AND t.status = 'success') as success_count,
		       (SELECT COUNT(*) FROM software_deployment_targets t WHERE t.deployment_id = d.id AND t.status = 'failed') as fail_count
		FROM software_deployments d
		JOIN software_packages sp ON d.package_id = sp.id
		WHERE d.tenant_id = $1
		ORDER BY d.created_at DESC
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var deployments []map[string]interface{}
	for rows.Next() {
		var id, name, pkgName, status, schedType string
		var createdAt time.Time
		var scheduledNull, completedNull sql.NullTime
		var total, successCount, failCount int

		if err := rows.Scan(&id, &name, &pkgName, &status, &schedType, &scheduledNull, &createdAt, &completedNull, &total, &successCount, &failCount); err != nil {
			continue
		}
		d := map[string]interface{}{
			"id": id, "name": name, "package_name": pkgName, "status": status,
			"schedule_type": schedType, "total": total,
			"success_count": successCount, "fail_count": failCount,
			"created_at": createdAt,
		}
		if scheduledNull.Valid {
			d["scheduled_for"] = scheduledNull.Time
		}
		if completedNull.Valid {
			d["completed_at"] = completedNull.Time
		}
		deployments = append(deployments, d)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"deployments": deployments})
}

func (s *APIServer) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	deployID := r.PathValue("deployID")
	var deploy struct {
		ID, Name, Status, SchedType string
		ScheduledFor, CompletedAt   sql.NullTime
		CreatedAt                   time.Time
	}
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, name, status, schedule_type, scheduled_for, completed_at, created_at
		FROM software_deployments WHERE id = $1 AND tenant_id = $2
	`, deployID, tenantID).Scan(&deploy.ID, &deploy.Name, &deploy.Status, &deploy.SchedType,
		&deploy.ScheduledFor, &deploy.CompletedAt, &deploy.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "deployment not found"})
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT t.device_id, d.hostname, t.status, t.error_message, t.duration_ms, t.started_at, t.completed_at
		FROM software_deployment_targets t
		JOIN software_deployments sd ON sd.id = t.deployment_id
		JOIN devices d ON t.device_id = d.id AND d.tenant_id = sd.tenant_id
		WHERE t.deployment_id = $1 AND sd.tenant_id = $2
	`, deployID, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var targets []map[string]interface{}
	for rows.Next() {
		var devID, hostname, status, errMsg string
		var duration int
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&devID, &hostname, &status, &errMsg, &duration, &startedAt, &completedAt); err != nil {
			continue
		}
		t := map[string]interface{}{
			"device_id": devID, "hostname": hostname, "status": status,
			"error_message": errMsg, "duration_ms": duration,
		}
		if startedAt.Valid {
			t["started_at"] = startedAt.Time
		}
		if completedAt.Valid {
			t["completed_at"] = completedAt.Time
		}
		targets = append(targets, t)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": deploy.ID, "name": deploy.Name, "status": deploy.Status,
		"targets": targets,
	})
}

func parseSoftwareResultSubject(subject string) (tenantID, agentID string, ok bool) {
	parts := strings.Split(subject, ".")
	if len(parts) != 6 || parts[0] != "tenant" || parts[2] != "agent" || parts[4] != "software" || parts[5] != "result" {
		return "", "", false
	}
	if parts[1] == "" || parts[3] == "" || strings.ContainsAny(parts[1], "*> ") || strings.ContainsAny(parts[3], "*> ") {
		return "", "", false
	}
	return parts[1], parts[3], true
}

func boundedSoftwareResultError(value string) string {
	if len(value) > maxSoftwareResultErrorBytes {
		return value[:maxSoftwareResultErrorBytes]
	}
	return value
}

func (s *APIServer) handleSoftwareResultNATS(msg *nats.Msg) {
	tenantID, agentID, ok := parseSoftwareResultSubject(msg.Subject)
	if !ok {
		s.logger.Warn("reject malformed software result subject", zap.String("subject", msg.Subject))
		return
	}

	var result struct {
		Type         string `json:"type"`
		DeploymentID string `json:"deployment_id"`
		Action       string `json:"action"`
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message"`
		DurationMs   int64  `json:"duration_ms"`
	}
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return
	}
	if result.Type != "software_result" || result.DeploymentID == "" || (result.Action != "install" && result.Action != "uninstall") {
		return
	}
	if result.Status != "success" && result.Status != "failed" {
		return
	}
	if result.DurationMs < 0 {
		return
	}
	result.ErrorMessage = boundedSoftwareResultError(result.ErrorMessage)

	tx, err := s.db.DB().Begin()
	if err != nil {
		s.logger.Error("begin software result transaction", zap.Error(err))
		return
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	res, err := tx.Exec(`
		UPDATE software_deployment_targets AS t
		SET status = $1, error_message = $2, duration_ms = $3, completed_at = $4
		FROM software_deployments AS d, devices AS dv
		WHERE t.deployment_id = $5
		  AND d.id = t.deployment_id
		  AND d.tenant_id = $6
		  AND dv.id = t.device_id
		  AND dv.tenant_id = d.tenant_id
		  AND dv.agent_id::text = $7
		  AND t.status IN ('pending', 'deploying')
	`, result.Status, result.ErrorMessage, result.DurationMs, now, result.DeploymentID, tenantID, agentID)
	if err != nil {
		s.logger.Error("update deployment target", zap.Error(err))
		return
	}
	rows, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		s.logger.Error("inspect software deployment target update", zap.Error(rowsErr))
		return
	}
	if rows == 0 {
		var existingStatus string
		err := tx.QueryRow(`
			SELECT t.status
			FROM software_deployment_targets t
			JOIN software_deployments d ON d.id = t.deployment_id
			JOIN devices dv ON dv.id = t.device_id AND dv.tenant_id = d.tenant_id
			WHERE t.deployment_id = $1 AND d.tenant_id = $2 AND dv.agent_id::text = $3
		`, result.DeploymentID, tenantID, agentID).Scan(&existingStatus)
		if err != nil {
			s.logger.Warn("software result did not match an authorized target",
				zap.String("tenant_id", tenantID), zap.String("agent_id", agentID), zap.String("deployment_id", result.DeploymentID))
			return
		}
		if existingStatus != result.Status {
			s.logger.Warn("reject conflicting terminal software result",
				zap.String("tenant_id", tenantID), zap.String("agent_id", agentID), zap.String("deployment_id", result.DeploymentID),
				zap.String("existing_status", existingStatus), zap.String("result_status", result.Status))
			return
		}
	}

	if _, err := tx.Exec(`
		UPDATE software_deployments AS d
		SET status = CASE
			WHEN EXISTS (SELECT 1 FROM software_deployment_targets t WHERE t.deployment_id = d.id AND t.status = 'failed') THEN 'failed'
			ELSE 'completed'
		END,
		completed_at = $2
		WHERE d.id = $1
		  AND d.tenant_id = $3
		  AND NOT EXISTS (
			SELECT 1 FROM software_deployment_targets t
			WHERE t.deployment_id = d.id AND t.status IN ('pending', 'deploying')
		  )
	`, result.DeploymentID, now, tenantID); err != nil {
		s.logger.Error("update aggregate software deployment", zap.Error(err))
		return
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("commit software deployment result", zap.Error(err))
	}
}
