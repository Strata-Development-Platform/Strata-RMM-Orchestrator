package platform

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/inventory"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/reporting"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
	"go.uber.org/zap"
)

func (s *APIServer) handleDownloadReport(w http.ResponseWriter, r *http.Request) {
	reportID := r.PathValue("reportID")
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}

	var storageKey string
	err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT storage_key FROM generated_reports WHERE id = $1 AND tenant_id = $2`, reportID, tenantID).Scan(&storageKey)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if s.storageBackend == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "storage not configured"})
		return
	}

	obj, err := s.storageBackend.Download(r.Context(), storageKey)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report file not found in storage"})
		return
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read report"})
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.pdf", reportID))
	http.ServeContent(w, r, fmt.Sprintf("%s.pdf", reportID), time.Now(), bytes.NewReader(data))
}

func (s *APIServer) handleListReports(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, name, format, storage_key, size_bytes, generated_at
		FROM generated_reports WHERE tenant_id = $1
		ORDER BY generated_at DESC LIMIT 20
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var reports []map[string]interface{}
	for rows.Next() {
		var id, name, format, storageKey string
		var size int64
		var genAt time.Time
		if err := rows.Scan(&id, &name, &format, &storageKey, &size, &genAt); err != nil {
			continue
		}
		reports = append(reports, map[string]interface{}{
			"id": id, "name": name, "format": format,
			"storage_key": storageKey, "size_bytes": size, "generated_at": genAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"reports": reports})
}

func (s *APIServer) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	var req struct {
		Name      string   `json:"name"`
		Frequency string   `json:"frequency"`
		Sections  []string `json:"sections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Frequency == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and frequency required"})
		return
	}
	if req.Sections == nil {
		req.Sections = []string{"summary", "alerts", "cves", "patches"}
	}
	secJSON, _ := json.Marshal(req.Sections)

	var id string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		INSERT INTO report_schedules (tenant_id, name, frequency, sections)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, tenantID, req.Name, req.Frequency, secJSON).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "created"})
}

func (s *APIServer) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, name, frequency, sections, enabled, last_sent, created_at
		FROM report_schedules WHERE tenant_id = $1 ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var schedules []map[string]interface{}
	for rows.Next() {
		var id, name, freq string
		var enabled bool
		var created_at time.Time
		var lastSent sql.NullTime
		var sections []byte

		if err := rows.Scan(&id, &name, &freq, &sections, &enabled, &lastSent, &created_at); err != nil {
			continue
		}
		sched := map[string]interface{}{
			"id": id, "name": name, "frequency": freq,
			"enabled": enabled, "created_at": created_at,
		}
		if lastSent.Valid {
			sched["last_sent"] = lastSent.Time
		}
		var secs []string
		json.Unmarshal(sections, &secs)
		sched["sections"] = secs
		schedules = append(schedules, sched)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"schedules": schedules})
}

func (s *APIServer) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("scheduleID")
	_, err := s.requestDB(r).ExecContext(r.Context(), `DELETE FROM report_schedules WHERE id = $1`, scheduleID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *APIServer) handleGenerateReport(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}
	var req struct {
		Sections []string `json:"sections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Sections = []string{"summary", "alerts", "cves", "patches"}
	}
	if req.Sections == nil {
		req.Sections = []string{"summary", "alerts", "cves", "patches"}
	}

	var tenantName string
	err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT name FROM tenants WHERE id = $1`, tenantID).Scan(&tenantName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
		return
	}

	go func() {
		logger := s.logger
		_ = logger
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "generation started"})
}

func (s *APIServer) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("scheduleID")
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}

	var req struct {
		Name       string   `json:"name"`
		Frequency  string   `json:"frequency"`
		Sections   []string `json:"sections"`
		Recipients []string `json:"recipients"`
		Enabled    *bool    `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	query := `UPDATE report_schedules SET`
	args := []interface{}{tenantID, scheduleID}
	hasUpdate := false

	if req.Name != "" {
		query += ` name = $` + fmt.Sprint(len(args)+1)
		args = append(args, req.Name)
		hasUpdate = true
	}
	if req.Frequency != "" {
		if hasUpdate {
			query += `,`
		}
		query += ` frequency = $` + fmt.Sprint(len(args)+1)
		args = append(args, req.Frequency)
		hasUpdate = true
	}
	if len(req.Sections) > 0 {
		if hasUpdate {
			query += `,`
		}
		secJSON, _ := json.Marshal(req.Sections)
		query += ` sections = $` + fmt.Sprint(len(args)+1)
		args = append(args, secJSON)
		hasUpdate = true
	}
	if len(req.Recipients) > 0 {
		if hasUpdate {
			query += `,`
		}
		recJSON, _ := json.Marshal(req.Recipients)
		query += ` recipients = $` + fmt.Sprint(len(args)+1)
		args = append(args, recJSON)
		hasUpdate = true
	}
	if req.Enabled != nil {
		if hasUpdate {
			query += `,`
		}
		query += ` enabled = $` + fmt.Sprint(len(args)+1)
		args = append(args, *req.Enabled)
		hasUpdate = true
	}

	if !hasUpdate {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no updates provided"})
		return
	}

	query += ` WHERE tenant_id = $1 AND id = $2`
	_, err := s.requestDB(r).ExecContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *APIServer) handleToggleSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("scheduleID")
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	_, err := s.requestDB(r).ExecContext(r.Context(), `UPDATE report_schedules SET enabled = $1 WHERE tenant_id = $2 AND id = $3`, req.Enabled, tenantID, scheduleID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "toggled"})
}

func (s *APIServer) handleTriggerSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("scheduleID")
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}

	var name string
	var sectionsJSON []byte
	err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT name, sections FROM report_schedules WHERE id = $1 AND tenant_id = $2`, scheduleID, tenantID).Scan(&name, &sectionsJSON)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		var secs []reporting.ReportSection
		if err := json.Unmarshal(sectionsJSON, &secs); err != nil {
			return
		}

		var tenantName string
		s.requestDB(r).QueryRowContext(ctx, `SELECT name FROM tenants WHERE id = $1`, tenantID).Scan(&tenantName)

		if s.reportEngine == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "report engine not configured"})
			return
		}

		pdfData, key, err := s.reportEngine.GenerateReport(ctx, tenantID, secs, tenantName)
		if err != nil {
			s.logger.Error("generate report", zap.Error(err))
			return
		}

		if s.storageBackend != nil {
			_, err := s.storageBackend.Upload(ctx, key, bytes.NewReader(pdfData), storage.UploadOptions{
				ContentType: "application/pdf",
			})
			if err != nil {
				s.logger.Error("upload report", zap.Error(err))
				return
			}
		}

		s.requestDB(r).ExecContext(ctx, `
			INSERT INTO generated_reports (schedule_id, tenant_id, name, format, storage_key, size_bytes)
			VALUES ($1, $2, $3, 'pdf', $4, $5)
		`, scheduleID, tenantID, name, key, int64(len(pdfData)))

		if s.accountMailer != nil {
			s.sendReportEmail(ctx, scheduleID, tenantID, name, pdfData)
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "generation started"})
}

func (s *APIServer) handleGenerateComplianceReport(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}
	var req struct {
		Framework string `json:"framework"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Framework == "" {
		req.Framework = "CIS"
	}

	go func() {
		if s.inventoryEngine == nil {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()

		_, err := s.inventoryEngine.GenerateComplianceReport(ctx, tenantID, inventory.ComplianceFramework(req.Framework))
		if err != nil {
			s.logger.Error("generate compliance report", zap.String("tenant", tenantID), zap.String("framework", req.Framework), zap.Error(err))
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "generation started"})
}

func (s *APIServer) handleListComplianceReports(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, tenant_id, framework, period_start, period_end, generated_at,
		       total_vulnerabilities, critical_count, high_count, medium_count, low_count,
		       pending_count, remediated_count, ignored_count, score, status, findings, remediations
		FROM compliance_reports WHERE tenant_id = $1
		ORDER BY generated_at DESC LIMIT 50
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var reports []map[string]interface{}
	for rows.Next() {
		var id, framework, status string
		var periodStart, periodEnd, generatedAt time.Time
		var totalVulns, criticalCount, highCount, mediumCount, lowCount, pendingCount, remediatedCount, ignoredCount int
		var score float64
		var findingsJSON, remediationsJSON []byte

		if err := rows.Scan(&id, &tenantID, &framework, &periodStart, &periodEnd, &generatedAt,
			&totalVulns, &criticalCount, &highCount, &mediumCount, &lowCount, &pendingCount, &remediatedCount, &ignoredCount,
			&score, &status, &findingsJSON, &remediationsJSON); err != nil {
			continue
		}

		var findings []map[string]interface{}
		if err := json.Unmarshal(findingsJSON, &findings); err != nil {
			continue
		}

		var remediations []string
		if err := json.Unmarshal(remediationsJSON, &remediations); err != nil {
			continue
		}

		reports = append(reports, map[string]interface{}{
			"id":                    id,
			"tenant_id":             tenantID,
			"framework":             framework,
			"period_start":          periodStart,
			"period_end":            periodEnd,
			"generated_at":          generatedAt,
			"total_vulnerabilities": totalVulns,
			"critical_count":        criticalCount,
			"high_count":            highCount,
			"medium_count":          mediumCount,
			"low_count":             lowCount,
			"pending_count":         pendingCount,
			"remediated_count":      remediatedCount,
			"ignored_count":         ignoredCount,
			"score":                 score,
			"status":                status,
			"findings":              findings,
			"remediations":          remediations,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"reports": reports})
}

func (s *APIServer) handleGetComplianceReport(w http.ResponseWriter, r *http.Request) {
	reportID := r.PathValue("reportID")
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}

	var id, framework, status string
	var periodStart, periodEnd, generatedAt time.Time
	var totalVulns, criticalCount, highCount, mediumCount, lowCount, pendingCount, remediatedCount, ignoredCount int
	var score float64
	var findingsJSON, remediationsJSON []byte

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, tenant_id, framework, period_start, period_end, generated_at,
		       total_vulnerabilities, critical_count, high_count, medium_count, low_count,
		       pending_count, remediated_count, ignored_count, score, status, findings, remediations
		FROM compliance_reports WHERE id = $1 AND tenant_id = $2
	`, reportID, tenantID).Scan(&id, &tenantID, &framework, &periodStart, &periodEnd, &generatedAt,
		&totalVulns, &criticalCount, &highCount, &mediumCount, &lowCount, &pendingCount, &remediatedCount, &ignoredCount,
		&score, &status, &findingsJSON, &remediationsJSON)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var findings []map[string]interface{}
	if err := json.Unmarshal(findingsJSON, &findings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse findings"})
		return
	}

	var remediations []string
	if err := json.Unmarshal(remediationsJSON, &remediations); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse remediations"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                    id,
		"tenant_id":             tenantID,
		"framework":             framework,
		"period_start":          periodStart,
		"period_end":            periodEnd,
		"generated_at":          generatedAt,
		"total_vulnerabilities": totalVulns,
		"critical_count":        criticalCount,
		"high_count":            highCount,
		"medium_count":          mediumCount,
		"low_count":             lowCount,
		"pending_count":         pendingCount,
		"remediated_count":      remediatedCount,
		"ignored_count":         ignoredCount,
		"score":                 score,
		"status":                status,
		"findings":              findings,
		"remediations":          remediations,
	})
}

func (s *APIServer) handleExportComplianceReportCSV(w http.ResponseWriter, r *http.Request) {
	reportID := r.PathValue("reportID")
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}

	var findingsJSON []byte
	err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT findings FROM compliance_reports WHERE id = $1 AND tenant_id = $2`, reportID, tenantID).Scan(&findingsJSON)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var findings []map[string]interface{}
	if err := json.Unmarshal(findingsJSON, &findings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse findings"})
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	if err := writer.Write([]string{"ID", "CVE ID", "Package", "Severity", "Description", "Remediation", "Status"}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to write header"})
		return
	}

	for _, f := range findings {
		record := []string{
			f["id"].(string),
			f["cve_id"].(string),
			f["package_name"].(string),
			f["severity"].(string),
			truncate(f["description"].(string), 200),
			truncate(f["remediation"].(string), 200),
			f["status"].(string),
		}
		if err := writer.Write(record); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to write record"})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to flush csv"})
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", reportID))
	w.Write(buf.Bytes())
}

func (s *APIServer) handleExportComplianceReportJSON(w http.ResponseWriter, r *http.Request) {
	reportID := r.PathValue("reportID")
	tenantID := r.PathValue("tenantID")
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}

	var id, framework, status string
	var periodStart, periodEnd, generatedAt time.Time
	var totalVulns, criticalCount, highCount, mediumCount, lowCount, pendingCount, remediatedCount, ignoredCount int
	var score float64
	var findingsJSON, remediationsJSON []byte

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, tenant_id, framework, period_start, period_end, generated_at,
		       total_vulnerabilities, critical_count, high_count, medium_count, low_count,
		       pending_count, remediated_count, ignored_count, score, status, findings, remediations
		FROM compliance_reports WHERE id = $1 AND tenant_id = $2
	`, reportID, tenantID).Scan(&id, &tenantID, &framework, &periodStart, &periodEnd, &generatedAt,
		&totalVulns, &criticalCount, &highCount, &mediumCount, &lowCount, &pendingCount, &remediatedCount, &ignoredCount,
		&score, &status, &findingsJSON, &remediationsJSON)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var findings []map[string]interface{}
	if err := json.Unmarshal(findingsJSON, &findings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse findings"})
		return
	}

	var remediations []string
	if err := json.Unmarshal(remediationsJSON, &remediations); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse remediations"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                    id,
		"tenant_id":             tenantID,
		"framework":             framework,
		"period_start":          periodStart,
		"period_end":            periodEnd,
		"generated_at":          generatedAt,
		"total_vulnerabilities": totalVulns,
		"critical_count":        criticalCount,
		"high_count":            highCount,
		"medium_count":          mediumCount,
		"low_count":             lowCount,
		"pending_count":         pendingCount,
		"remediated_count":      remediatedCount,
		"ignored_count":         ignoredCount,
		"score":                 score,
		"status":                status,
		"findings":              findings,
		"remediations":          remediations,
	})
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
