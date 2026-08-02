package inventory

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ComplianceFramework represents a compliance standard
type ComplianceFramework string

const (
	CIS     ComplianceFramework = "CIS"
	HIPAA   ComplianceFramework = "HIPAA"
	PCI_DSS ComplianceFramework = "PCI-DSS"
	GDPR    ComplianceFramework = "GDPR"
	SOC2    ComplianceFramework = "SOC2"
	NIST    ComplianceFramework = "NIST"
)

// ComplianceReport represents a compliance report for a tenant
type ComplianceReport struct {
	ID                   string              `json:"id"`
	TenantID             string              `json:"tenant_id"`
	Framework            ComplianceFramework `json:"framework"`
	PeriodStart          time.Time           `json:"period_start"`
	PeriodEnd            time.Time           `json:"period_end"`
	GeneratedAt          time.Time           `json:"generated_at"`
	TotalVulnerabilities int                 `json:"total_vulnerabilities"`
	CriticalCount        int                 `json:"critical_count"`
	HighCount            int                 `json:"high_count"`
	MediumCount          int                 `json:"medium_count"`
	LowCount             int                 `json:"low_count"`
	PendingCount         int                 `json:"pending_count"`
	RemediatedCount      int                 `json:"remediated_count"`
	IgnoredCount         int                 `json:"ignored_count"`
	Score                float64             `json:"score"`
	Status               string              `json:"status"`
	Findings             []ComplianceFinding `json:"findings,omitempty"`
	Remediations         []string            `json:"remediations,omitempty"`
}

// ComplianceFinding represents a specific finding in a compliance report
type ComplianceFinding struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	PackageName string `json:"package_name"`
	CVEID       string `json:"cve_id"`
	Description string `json:"description"`
	Remediation string `json:"remediation"`
	Status      string `json:"status"`
}

// ReportingEngine generates compliance reports
type ReportingEngine struct {
	db       *sql.DB
	logger   *zap.Logger
	reporter *Reporter
}

// NewReportingEngine creates a new reporting engine
func NewReportingEngine(db *sql.DB, logger *zap.Logger) *ReportingEngine {
	return &ReportingEngine{
		db:       db,
		logger:   logger,
		reporter: NewReporter(),
	}
}

// GenerateComplianceReport generates a compliance report for a tenant
func (r *ReportingEngine) GenerateComplianceReport(ctx context.Context, tenantID string, framework ComplianceFramework) (*ComplianceReport, error) {
	now := time.Now()
	periodStart := now.AddDate(0, 0, -7) // Last 7 days
	periodEnd := now

	// Get vulnerability counts by severity
	criticalCount, err := r.getVulnCount(ctx, tenantID, "critical", "open")
	if err != nil {
		return nil, fmt.Errorf("get critical count: %w", err)
	}

	highCount, err := r.getVulnCount(ctx, tenantID, "high", "open")
	if err != nil {
		return nil, fmt.Errorf("get high count: %w", err)
	}

	mediumCount, err := r.getVulnCount(ctx, tenantID, "medium", "open")
	if err != nil {
		return nil, fmt.Errorf("get medium count: %w", err)
	}

	lowCount, err := r.getVulnCount(ctx, tenantID, "low", "open")
	if err != nil {
		return nil, fmt.Errorf("get low count: %w", err)
	}

	pendingCount, err := r.getVulnCount(ctx, tenantID, "", "open")
	if err != nil {
		return nil, fmt.Errorf("get pending count: %w", err)
	}

	remediatedCount, err := r.getVulnCount(ctx, tenantID, "", "patched")
	if err != nil {
		return nil, fmt.Errorf("get remediated count: %w", err)
	}

	ignoredCount, err := r.getVulnCount(ctx, tenantID, "", "ignored")
	if err != nil {
		return nil, fmt.Errorf("get ignored count: %w", err)
	}

	totalCount := criticalCount + highCount + mediumCount + lowCount

	// Calculate score based on compliance framework
	score := r.calculateScore(framework, criticalCount, highCount, mediumCount, lowCount)

	// Get findings
	findings, err := r.getFindings(ctx, tenantID, framework)
	if err != nil {
		return nil, fmt.Errorf("get findings: %w", err)
	}

	// Generate remediation recommendations
	remediations := r.generateRemediations(framework, criticalCount, highCount, mediumCount)

	report := &ComplianceReport{
		ID:                   generateID(),
		TenantID:             tenantID,
		Framework:            framework,
		PeriodStart:          periodStart,
		PeriodEnd:            periodEnd,
		GeneratedAt:          now,
		TotalVulnerabilities: totalCount,
		CriticalCount:        criticalCount,
		HighCount:            highCount,
		MediumCount:          mediumCount,
		LowCount:             lowCount,
		PendingCount:         pendingCount,
		RemediatedCount:      remediatedCount,
		IgnoredCount:         ignoredCount,
		Score:                score,
		Status:               r.getStatus(score),
		Findings:             findings,
		Remediations:         remediations,
	}

	// Save report to database
	if err := r.saveReport(ctx, report); err != nil {
		return nil, fmt.Errorf("save report: %w", err)
	}

	return report, nil
}

func (r *ReportingEngine) getVulnCount(ctx context.Context, tenantID, severity, status string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM device_vulnerabilities dv
		JOIN devices d ON dv.device_id = d.id
		WHERE d.tenant_id = $1
	`
	args := []interface{}{tenantID}

	if severity != "" {
		query += ` AND LOWER(dv.severity) = LOWER($` + fmt.Sprint(len(args)+1) + `)`
		args = append(args, severity)
	}

	if status != "" {
		query += ` AND dv.status = $` + fmt.Sprint(len(args)+1)
		args = append(args, status)
	}

	var count int
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func (r *ReportingEngine) getFindings(ctx context.Context, tenantID string, framework ComplianceFramework) ([]ComplianceFinding, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT dv.id, dv.cve_id, dv.package_name, dv.severity, dv.status, dv.detected_at
		FROM device_vulnerabilities dv
		JOIN devices d ON dv.device_id = d.id
		WHERE d.tenant_id = $1
		ORDER BY dv.severity DESC, dv.detected_at DESC
		LIMIT 100
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query findings: %w", err)
	}
	defer rows.Close()

	var findings []ComplianceFinding
	for rows.Next() {
		var f ComplianceFinding
		if err := rows.Scan(&f.ID, &f.CVEID, &f.PackageName, &f.Severity, &f.Status, nil); err != nil {
			continue
		}
		f.Description = r.getFindingDescription(framework, f.Severity)
		f.Remediation = r.getFindingRemediation(framework, f.PackageName, f.Severity)
		findings = append(findings, f)
	}

	return findings, nil
}

func (r *ReportingEngine) getFindingDescription(framework ComplianceFramework, severity string) string {
	switch framework {
	case HIPAA:
		return r.getHIPAASeverityDesc(severity)
	case PCI_DSS:
		return r.getPCIDSSSeverityDesc(severity)
	default:
		return r.getGenericSeverityDesc(severity)
	}
}

func (r *ReportingEngine) getFindingRemediation(framework ComplianceFramework, packageName, severity string) string {
	switch framework {
	case CIS:
		return fmt.Sprintf("Update %s to latest version per CIS benchmark", packageName)
	case HIPAA:
		return fmt.Sprintf("Address %s vulnerability to maintain HIPAA compliance", packageName)
	case PCI_DSS:
		return fmt.Sprintf("Patch %s to meet PCI-DSS security requirements", packageName)
	default:
		return fmt.Sprintf("Update %s to mitigate security vulnerability", packageName)
	}
}

func (r *ReportingEngine) getGenericSeverityDesc(severity string) string {
	switch severity {
	case "critical":
		return "Critical security vulnerability requiring immediate remediation"
	case "high":
		return "High severity vulnerability that should be addressed within 7 days"
	case "medium":
		return "Medium severity vulnerability that should be addressed within 30 days"
	case "low":
		return "Low severity vulnerability for routine maintenance"
	default:
		return "Security vulnerability requiring review"
	}
}

func (r *ReportingEngine) getHIPAASeverityDesc(severity string) string {
	switch severity {
	case "critical":
		return "Critical: Immediate action required to protect ePHI"
	case "high":
		return "High: Significant risk to ePHI security"
	case "medium":
		return "Medium: Moderate risk requiring scheduled remediation"
	case "low":
		return "Low: Minor security issue for routine review"
	default:
		return "Security issue requiring HIPAA compliance review"
	}
}

func (r *ReportingEngine) getPCIDSSSeverityDesc(severity string) string {
	switch severity {
	case "critical":
		return "Critical: Immediate remediation required per PCI-DSS requirement"
	case "high":
		return "High: Must be remediated within 30 days per PCI-DSS"
	case "medium":
		return "Medium: Remediation required within 90 days per PCI-DSS"
	case "low":
		return "Low: Address per PCI-DSS quarterly review process"
	default:
		return "Security finding requiring PCI-DSS compliance review"
	}
}

func (r *ReportingEngine) calculateScore(framework ComplianceFramework, critical, high, medium, low int) float64 {
	if critical+high+medium+low == 0 {
		return 100.0
	}

	// Different frameworks have different weightings
	var weights map[string]float64
	switch framework {
	case HIPAA:
		weights = map[string]float64{
			"critical": 40,
			"high":     30,
			"medium":   20,
			"low":      10,
		}
	case PCI_DSS:
		weights = map[string]float64{
			"critical": 35,
			"high":     30,
			"medium":   25,
			"low":      10,
		}
	default: // CIS, SOC2, NIST
		weights = map[string]float64{
			"critical": 30,
			"high":     25,
			"medium":   25,
			"low":      20,
		}
	}

	totalWeight := float64(critical*int(weights["critical"]) +
		high*int(weights["high"]) +
		medium*int(weights["medium"]) +
		low*int(weights["low"]))

	maxScore := float64((critical + high + medium + low) * 40)
	if maxScore == 0 {
		return 100.0
	}

	score := 100.0 - (totalWeight * 100.0 / maxScore)
	if score < 0 {
		score = 0
	}

	return score
}

func (r *ReportingEngine) getStatus(score float64) string {
	if score >= 90 {
		return "compliant"
	} else if score >= 70 {
		return "mostly_compliant"
	} else if score >= 50 {
		return "non_compliant"
	}
	return "critical_non_compliant"
}

func (r *ReportingEngine) generateRemediations(framework ComplianceFramework, critical, high, medium int) []string {
	var remediations []string

	if critical > 0 {
		remediations = append(remediations,
			fmt.Sprintf("URGENT: %d critical vulnerability(s) require immediate attention", critical))
	}

	if high > 0 {
		remediations = append(remediations,
			fmt.Sprintf("HIGH PRIORITY: %d high severity vulnerability(s) should be addressed within 7 days", high))
	}

	if medium > 0 {
		remediations = append(remediations,
			fmt.Sprintf("MEDIUM PRIORITY: %d medium severity vulnerability(s) should be addressed within 30 days", medium))
	}

	if critical == 0 && high == 0 && medium == 0 {
		remediations = append(remediations, "No immediate remediations required")
	}

	return remediations
}

func (r *ReportingEngine) saveReport(ctx context.Context, report *ComplianceReport) error {
	findingsJSON, _ := json.Marshal(report.Findings)
	remediationsJSON, _ := json.Marshal(report.Remediations)

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO compliance_reports (id, tenant_id, framework, period_start, period_end,
		                                generated_at, total_vulnerabilities, critical_count,
		                                high_count, medium_count, low_count, pending_count,
		                                remediated_count, ignored_count, score, status,
		                                findings, remediations)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`, report.ID, report.TenantID, report.Framework, report.PeriodStart, report.PeriodEnd,
		report.GeneratedAt, report.TotalVulnerabilities, report.CriticalCount,
		report.HighCount, report.MediumCount, report.LowCount, report.PendingCount,
		report.RemediatedCount, report.IgnoredCount, report.Score, report.Status,
		findingsJSON, remediationsJSON)
	return err
}

// ExportReport exports a compliance report to CSV format
func (r *ReportingEngine) ExportReport(ctx context.Context, reportID string) ([]byte, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT findings FROM compliance_reports WHERE id = $1
	`, reportID)
	if err != nil {
		return nil, fmt.Errorf("query report: %w", err)
	}
	defer rows.Close()

	var findingsJSON []byte
	if rows.Next() {
		rows.Scan(&findingsJSON)
	}

	var findings []ComplianceFinding
	if err := json.Unmarshal(findingsJSON, &findings); err != nil {
		return nil, fmt.Errorf("unmarshal findings: %w", err)
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	if err := writer.Write([]string{"ID", "CVE ID", "Package", "Severity", "Description", "Remediation", "Status"}); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}

	// Write findings
	for _, f := range findings {
		record := []string{
			f.ID,
			f.CVEID,
			f.PackageName,
			f.Severity,
			truncate(f.Description, 200),
			truncate(f.Remediation, 200),
			f.Status,
		}
		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("write record: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("flush csv: %w", err)
	}

	return buf.Bytes(), nil
}

// ExportReportJSON exports a compliance report to JSON format
func (r *ReportingEngine) ExportReportJSON(ctx context.Context, reportID string) ([]byte, error) {
	var report ComplianceReport
	var findingsJSON, remediationsJSON []byte

	err := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, framework, period_start, period_end, generated_at,
		       total_vulnerabilities, critical_count, high_count, medium_count, low_count,
		       pending_count, remediated_count, ignored_count, score, status,
		       findings, remediations
		FROM compliance_reports
		WHERE id = $1
	`, reportID).Scan(
		&report.ID, &report.TenantID, &report.Framework, &report.PeriodStart, &report.PeriodEnd,
		&report.GeneratedAt, &report.TotalVulnerabilities, &report.CriticalCount,
		&report.HighCount, &report.MediumCount, &report.LowCount, &report.PendingCount,
		&report.RemediatedCount, &report.IgnoredCount, &report.Score, &report.Status,
		&findingsJSON, &remediationsJSON)

	if err != nil {
		return nil, fmt.Errorf("query report: %w", err)
	}

	if err := json.Unmarshal(findingsJSON, &report.Findings); err != nil {
		return nil, fmt.Errorf("unmarshal findings: %w", err)
	}

	if err := json.Unmarshal(remediationsJSON, &report.Remediations); err != nil {
		return nil, fmt.Errorf("unmarshal remediations: %w", err)
	}

	return json.Marshal(report)
}

// GetReportsForTenant returns all compliance reports for a tenant
func (r *ReportingEngine) GetReportsForTenant(ctx context.Context, tenantID string) ([]ComplianceReport, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, framework, period_start, period_end, generated_at,
		       total_vulnerabilities, critical_count, high_count, medium_count, low_count,
		       pending_count, remediated_count, ignored_count, score, status, findings, remediations
		FROM compliance_reports
		WHERE tenant_id = $1
		ORDER BY generated_at DESC
		LIMIT 50
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query reports: %w", err)
	}
	defer rows.Close()

	var reports []ComplianceReport
	for rows.Next() {
		var r ComplianceReport
		var findingsJSON, remediationsJSON []byte

		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.Framework, &r.PeriodStart, &r.PeriodEnd,
			&r.GeneratedAt, &r.TotalVulnerabilities, &r.CriticalCount,
			&r.HighCount, &r.MediumCount, &r.LowCount, &r.PendingCount,
			&r.RemediatedCount, &r.IgnoredCount, &r.Score, &r.Status,
			&findingsJSON, &remediationsJSON); err != nil {
			continue
		}

		json.Unmarshal(findingsJSON, &r.Findings)
		json.Unmarshal(remediationsJSON, &r.Remediations)

		reports = append(reports, r)
	}

	return reports, nil
}

// GetReport returns a specific compliance report
func (r *ReportingEngine) GetReport(ctx context.Context, reportID string) (*ComplianceReport, error) {
	var report ComplianceReport
	var findingsJSON, remediationsJSON []byte

	err := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, framework, period_start, period_end, generated_at,
		       total_vulnerabilities, critical_count, high_count, medium_count, low_count,
		       pending_count, remediated_count, ignored_count, score, status, findings, remediations
		FROM compliance_reports
		WHERE id = $1
	`, reportID).Scan(
		&report.ID, &report.TenantID, &report.Framework, &report.PeriodStart, &report.PeriodEnd,
		&report.GeneratedAt, &report.TotalVulnerabilities, &report.CriticalCount,
		&report.HighCount, &report.MediumCount, &report.LowCount, &report.PendingCount,
		&report.RemediatedCount, &report.IgnoredCount, &report.Score, &report.Status,
		&findingsJSON, &remediationsJSON)

	if err != nil {
		return nil, fmt.Errorf("query report: %w", err)
	}

	json.Unmarshal(findingsJSON, &report.Findings)
	json.Unmarshal(remediationsJSON, &report.Remediations)

	return &report, nil
}

// StartScheduler starts the scheduled report generation
func (r *ReportingEngine) StartScheduler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.generateScheduledReports(ctx)
			}
		}
	}()
}

func (r *ReportingEngine) generateScheduledReports(ctx context.Context) {
	// Get all tenants
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM tenants WHERE is_active = true`)
	if err != nil {
		r.logger.Error("get tenants", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			continue
		}

		// Generate reports for each framework
		for _, framework := range []ComplianceFramework{CIS, HIPAA, PCI_DSS} {
			if _, err := r.GenerateComplianceReport(ctx, tenantID, framework); err != nil {
				r.logger.Warn("generate report", zap.String("tenant", tenantID), zap.String("framework", string(framework)), zap.Error(err))
			}
		}
	}
}

// Reporter helper struct
type Reporter struct {
}

func NewReporter() *Reporter {
	return &Reporter{}
}

func generateID() string {
	// Simplified ID generation
	return fmt.Sprintf("rep-%d", time.Now().UnixNano())
}
