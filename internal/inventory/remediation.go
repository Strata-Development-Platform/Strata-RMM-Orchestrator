package inventory

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// RemediationStatus represents the current status of a remediation attempt
type RemediationStatus string

const (
	RemediationPending  RemediationStatus = "pending"
	RemediationRunning  RemediationStatus = "running"
	RemediationSuccess  RemediationStatus = "success"
	RemediationFailed   RemediationStatus = "failed"
	RemediationRetrying RemediationStatus = "retrying"
	RemediationMaxRetry RemediationStatus = "max_retries_exceeded"
)

// RemediationAttempt tracks a single patch attempt for a vulnerability
type RemediationAttempt struct {
	ID              string            `json:"id"`
	VulnerabilityID string            `json:"vuln_id"`
	DeviceID        string            `json:"device_id"`
	TenantID        string            `json:"tenant_id"`
	AttemptNumber   int               `json:"attempt_number"`
	Status          RemediationStatus `json:"status"`
	Output          string            `json:"output,omitempty"`
	Error           string            `json:"error,omitempty"`
	RetryAt         *time.Time        `json:"retry_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// RemediationPolicy defines auto-remediation settings per tenant
type RemediationPolicy struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	Enabled                bool      `json:"enabled"`
	SeverityThreshold      string    `json:"severity_threshold"` // critical, high, medium, low
	AutoRemediate          bool      `json:"auto_remediate"`
	MaxRetries             int       `json:"max_retries"`
	RetryDelayHours        int       `json:"retry_delay_hours"`
	AutoApprove            bool      `json:"auto_approve"`
	RebootBehavior         string    `json:"reboot_behavior"`          // automatic, prompt, ignore
	MaintenanceWindowStart string    `json:"maintenance_window_start"` // HH:MM format
	MaintenanceWindowEnd   string    `json:"maintenance_window_end"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// RemediationEngine handles automatic vulnerability remediation
type RemediationEngine struct {
	db        *sql.DB
	logger    *zap.Logger
	patchExec *Executor
	policy    *RemediationPolicy
	mu        sync.RWMutex
}

// NewRemediationEngine creates a new remediation engine
func NewRemediationEngine(db *sql.DB, logger *zap.Logger) *RemediationEngine {
	return &RemediationEngine{
		db:        db,
		logger:    logger,
		patchExec: NewExecutor(),
		policy: &RemediationPolicy{
			Enabled:           false,
			SeverityThreshold: "critical",
			AutoRemediate:     false,
			MaxRetries:        3,
			RetryDelayHours:   1,
			AutoApprove:       true,
			RebootBehavior:    "automatic",
		},
	}
}

// WithPolicy sets the remediation policy for this engine
func (r *RemediationEngine) WithPolicy(policy *RemediationPolicy) *RemediationEngine {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policy = policy
	return r
}

// WithExecutor sets the patch executor to use
func (r *RemediationEngine) WithExecutor(exec *Executor) *RemediationEngine {
	r.patchExec = exec
	return r
}

// Start begins the auto-remediation process
func (r *RemediationEngine) Start(ctx context.Context) error {
	r.logger.Info("starting remediation engine")

	if !r.policy.Enabled {
		r.logger.Info("auto-remediation disabled")
		return nil
	}

	go r.remediationLoop(ctx)
	return nil
}

func (r *RemediationEngine) remediationLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processPendingRemediations(ctx)
		}
	}
}

func (r *RemediationEngine) processPendingRemediations(ctx context.Context) {
	r.mu.RLock()
	policy := r.policy
	r.mu.RUnlock()

	if !policy.Enabled || !policy.AutoRemediate {
		return
	}

	// Get vulnerabilities that need remediation based on severity threshold
	vulns, err := r.getVulnerabilitiesForRemediation(ctx, policy.SeverityThreshold)
	if err != nil {
		r.logger.Error("get vulnerabilities for remediation", zap.Error(err))
		return
	}

	for _, vuln := range vulns {
		if err := r.remediateVulnerability(ctx, vuln, policy); err != nil {
			r.logger.Error("remediate vulnerability", zap.String("vuln_id", vuln.ID), zap.Error(err))
		}
	}
}

func (r *RemediationEngine) getVulnerabilitiesForRemediation(ctx context.Context, severityThreshold string) ([]DeviceVulnerability, error) {
	severityOrder := map[string]int{
		"critical": 4,
		"high":     3,
		"medium":   2,
		"low":      1,
	}

	minSeverity := severityOrder[severityThreshold]
	if minSeverity == 0 {
		minSeverity = 4 // default to critical
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT dv.id, dv.device_id, dv.tenant_id, dv.package_name, dv.current_version,
		       dv.fixed_in, dv.severity, dv.status, dv.detected_at, dv.resolved_at,
		       d.os, d.os_version
		FROM device_vulnerabilities dv
		JOIN devices d ON dv.device_id = d.id
		WHERE dv.status = 'open'
		ORDER BY severity DESC, dv.detected_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query vulnerabilities: %w", err)
	}
	defer rows.Close()

	var vulns []DeviceVulnerability
	for rows.Next() {
		var v DeviceVulnerability
		if err := rows.Scan(&v.ID, &v.DeviceID, &v.TenantID, &v.PackageName, &v.CurrentVersion,
			&v.FixedIn, &v.Severity, &v.Status, &v.DetectedAt, &v.ResolvedAt,
			&v.OS, &v.OSVersion); err != nil {
			continue
		}

		severityScore := severityOrder[v.Severity]
		if severityScore >= minSeverity {
			vulns = append(vulns, v)
		}
	}

	return vulns, nil
}

func (r *RemediationEngine) remediateVulnerability(ctx context.Context, vuln DeviceVulnerability, policy *RemediationPolicy) error {
	// Check if we should skip based on severity threshold
	severityOrder := map[string]int{
		"critical": 4,
		"high":     3,
		"medium":   2,
		"low":      1,
	}
	minSeverity := severityOrder[policy.SeverityThreshold]
	if minSeverity == 0 {
		minSeverity = 4
	}

	vulnSeverity := severityOrder[vuln.Severity]
	if vulnSeverity < minSeverity {
		return nil
	}

	// Check max retries
	attemptCount, err := r.getAttemptCount(ctx, vuln.ID)
	if err != nil {
		return fmt.Errorf("get attempt count: %w", err)
	}

	if attemptCount >= policy.MaxRetries {
		r.updateVulnerabilityStatus(ctx, vuln.ID, RemediationMaxRetry.String())
		r.logger.Info("max remediation retries exceeded",
			zap.String("vuln_id", vuln.ID),
			zap.Int("attempts", attemptCount))
		return nil
	}

	// Create remediation attempt
	attempt := &RemediationAttempt{
		VulnerabilityID: vuln.ID,
		DeviceID:        vuln.DeviceID,
		TenantID:        vuln.TenantID,
		AttemptNumber:   attemptCount + 1,
		Status:          RemediationRunning,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := r.createRemediationAttempt(ctx, attempt); err != nil {
		return fmt.Errorf("create attempt: %w", err)
	}

	r.logger.Info("starting remediation attempt",
		zap.String("vuln_id", vuln.ID),
		zap.String("device", vuln.DeviceID),
		zap.String("package", vuln.PackageName),
		zap.Int("attempt", attemptCount+1))

	// Execute patch
	result, err := r.patchExec.Install(ctx, []string{vuln.PackageName})
	if err != nil {
		r.updateRemediationAttempt(ctx, attempt.ID, RemediationFailed, err.Error(), "")
		r.updateVulnerabilityStatus(ctx, vuln.ID, "pending_remediation")
		return fmt.Errorf("install patch: %w", err)
	}

	// Update attempt based on result
	output := ""
	if result.Output != "" {
		output = truncate(result.Output, 1000)
	}

	var status RemediationStatus
	if result.Status == "installed" {
		status = RemediationSuccess
	} else if result.Status == "reboot_required" {
		status = RemediationSuccess
		output += "; reboot required"
	} else {
		status = RemediationFailed
	}

	r.updateRemediationAttempt(ctx, attempt.ID, status, result.Error, output)

	if status == RemediationSuccess {
		r.updateVulnerabilityStatus(ctx, vuln.ID, "patched")
		r.logger.Info("vulnerability remediated successfully",
			zap.String("vuln_id", vuln.ID),
			zap.String("device", vuln.DeviceID),
			zap.String("package", vuln.PackageName))
	} else {
		r.logger.Warn("remediation failed",
			zap.String("vuln_id", vuln.ID),
			zap.String("device", vuln.DeviceID),
			zap.String("package", vuln.PackageName),
			zap.String("error", result.Error))

		if attemptCount+1 < policy.MaxRetries {
			r.updateVulnerabilityStatus(ctx, vuln.ID, "pending_remediation")
		}
	}

	return nil
}

func (r *RemediationEngine) getAttemptCount(ctx context.Context, vulnID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM remediation_attempts WHERE vulnerability_id = $1
	`, vulnID).Scan(&count)
	return count, err
}

func (r *RemediationEngine) createRemediationAttempt(ctx context.Context, attempt *RemediationAttempt) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO remediation_attempts (id, vulnerability_id, device_id, tenant_id,
		                                  attempt_number, status, output, error, retry_at,
		                                  created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, attempt.ID, attempt.VulnerabilityID, attempt.DeviceID, attempt.TenantID,
		attempt.AttemptNumber, attempt.Status, attempt.Output, attempt.Error,
		attempt.RetryAt, attempt.CreatedAt, attempt.UpdatedAt)
	return err
}

func (r *RemediationEngine) updateRemediationAttempt(ctx context.Context, attemptID string, status RemediationStatus, errorMsg, output string) {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `
		UPDATE remediation_attempts
		SET status = $1, output = $2, error = $3, updated_at = $4
		WHERE id = $5
	`, status, truncate(output, 1000), truncate(errorMsg, 500), now, attemptID)
	if err != nil {
		r.logger.Error("update remediation attempt", zap.Error(err))
	}
}

func (r *RemediationEngine) updateVulnerabilityStatus(ctx context.Context, vulnID, status string) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE device_vulnerabilities
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`, status, vulnID)
	if err != nil {
		r.logger.Error("update vulnerability status", zap.Error(err))
	}
}

// GetRemediationHistory returns remediation attempts for a vulnerability
func (r *RemediationEngine) GetRemediationHistory(ctx context.Context, vulnID string) ([]RemediationAttempt, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, vulnerability_id, device_id, tenant_id, attempt_number, status,
		       output, error, retry_at, created_at, updated_at
		FROM remediation_attempts
		WHERE vulnerability_id = $1
		ORDER BY attempt_number ASC
	`, vulnID)
	if err != nil {
		return nil, fmt.Errorf("query attempts: %w", err)
	}
	defer rows.Close()

	var attempts []RemediationAttempt
	for rows.Next() {
		var a RemediationAttempt
		if err := rows.Scan(&a.ID, &a.VulnerabilityID, &a.DeviceID, &a.TenantID,
			&a.AttemptNumber, &a.Status, &a.Output, &a.Error, &a.RetryAt,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			continue
		}
		attempts = append(attempts, a)
	}

	return attempts, nil
}

// GetRemediationSummary returns summary statistics for remediation
func (r *RemediationEngine) GetRemediationSummary(ctx context.Context, tenantID string) (map[string]int, error) {
	summary := make(map[string]int)

	rows, err := r.db.QueryContext(ctx, `
		SELECT status, COUNT(*)
		FROM remediation_attempts ra
		JOIN device_vulnerabilities dv ON ra.vulnerability_id = dv.id
		JOIN devices d ON ra.device_id = d.id
		WHERE d.tenant_id = $1
		GROUP BY ra.status
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query summary: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			continue
		}
		summary[status] = count
	}

	return summary, nil
}

// GetPolicy returns the current remediation policy
func (r *RemediationEngine) GetPolicy() *RemediationPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.policy
}

// SetPolicy updates the remediation policy
func (r *RemediationEngine) SetPolicy(policy *RemediationPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policy = policy
}

// SavePolicy persists the policy to database
func (r *RemediationEngine) SavePolicy(ctx context.Context, policy *RemediationPolicy) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO remediation_policies (id, tenant_id, enabled, severity_threshold,
		                                  auto_remediate, max_retries, retry_delay_hours,
		                                  auto_approve, reboot_behavior, maintenance_window_start,
		                                  maintenance_window_end, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
		ON CONFLICT (tenant_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			severity_threshold = EXCLUDED.severity_threshold,
			auto_remediate = EXCLUDED.auto_remediate,
			max_retries = EXCLUDED.max_retries,
			retry_delay_hours = EXCLUDED.retry_delay_hours,
			auto_approve = EXCLUDED.auto_approve,
			reboot_behavior = EXCLUDED.reboot_behavior,
			maintenance_window_start = EXCLUDED.maintenance_window_start,
			maintenance_window_end = EXCLUDED.maintenance_window_end,
			updated_at = NOW()
	`, policy.ID, policy.TenantID, policy.Enabled, policy.SeverityThreshold,
		policy.AutoRemediate, policy.MaxRetries, policy.RetryDelayHours,
		policy.AutoApprove, policy.RebootBehavior, policy.MaintenanceWindowStart,
		policy.MaintenanceWindowEnd)
	return err
}

// GetPolicyForTenant loads the remediation policy for a tenant
func (r *RemediationEngine) GetPolicyForTenant(ctx context.Context, tenantID string) (*RemediationPolicy, error) {
	var policy RemediationPolicy
	var rebootBehavior, maintStart, maintEnd sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, enabled, severity_threshold, auto_remediate,
		       max_retries, retry_delay_hours, auto_approve, reboot_behavior,
		       maintenance_window_start, maintenance_window_end, created_at, updated_at
		FROM remediation_policies
		WHERE tenant_id = $1
	`, tenantID).Scan(
		&policy.ID, &policy.TenantID, &policy.Enabled, &policy.SeverityThreshold,
		&policy.AutoRemediate, &policy.MaxRetries, &policy.RetryDelayHours,
		&policy.AutoApprove, &rebootBehavior, &maintStart, &maintEnd,
		&policy.CreatedAt, &policy.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("policy not found for tenant %s", tenantID)
	}

	if rebootBehavior.Valid {
		policy.RebootBehavior = rebootBehavior.String
	}
	if maintStart.Valid {
		policy.MaintenanceWindowStart = maintStart.String
	}
	if maintEnd.Valid {
		policy.MaintenanceWindowEnd = maintEnd.String
	}

	return &policy, nil
}

// String returns string representation of RemediationStatus
func (s RemediationStatus) String() string {
	return string(s)
}

// DeviceVulnerability extends VulnerabilityState with device details
type DeviceVulnerability struct {
	ID             string     `json:"id"`
	DeviceID       string     `json:"device_id"`
	TenantID       string     `json:"tenant_id"`
	PackageName    string     `json:"package_name"`
	CurrentVersion string     `json:"current_version"`
	FixedIn        string     `json:"fixed_in"`
	Severity       string     `json:"severity"`
	Status         string     `json:"status"`
	DetectedAt     time.Time  `json:"detected_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	OS             string     `json:"os,omitempty"`
	OSVersion      string     `json:"os_version,omitempty"`
}

// PatchStatus constants from patch package
const (
	StatusInstalled PatchStatus = "installed"
	StatusFailed    PatchStatus = "failed"
	StatusRebootReq PatchStatus = "reboot_required"
)

type PatchStatus string

// Executor from patch package
type Executor struct {
	Platform string
}

func NewExecutor() *Executor {
	return &Executor{Platform: "linux"}
}

func (e *Executor) Install(ctx context.Context, packages []string) (*struct {
	Status    PatchStatus
	Output    string
	Error     string
	RebootReq bool
}, error) {
	return nil, fmt.Errorf("not implemented")
}
