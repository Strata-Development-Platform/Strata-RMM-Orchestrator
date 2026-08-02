package inventory

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

type CVESyncEngine struct {
	db         *sql.DB
	logger     *zap.Logger
	httpClient *http.Client
	nvdAPIKey  string
	interval   time.Duration
	mu         sync.Mutex
	running    bool
	remediateFn func(deviceID, cveID, severity string)
}

type OSVQuery struct {
	Package OSVPackage `json:"package"`
}

type OSVPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type OSVQueryBatch struct {
	Queries []OSVQuery `json:"queries"`
}

type OSVResponse struct {
	Vulns []OSVVuln `json:"vulns"`
}

type OSVVuln struct {
	ID         string          `json:"id"`
	Summary    string          `json:"summary"`
	Details    string          `json:"details"`
	Aliases    []string        `json:"aliases"`
	Published  string          `json:"published"`
	Modified   string          `json:"modified"`
	Severity   []OSVSeverity   `json:"severity"`
	Affected   []OSVAffected   `json:"affected"`
	References []OSVReference  `json:"references"`
}

type OSVSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

type OSVAffected struct {
	Package OSVPackage     `json:"package"`
	Ranges  []OSVRange     `json:"ranges"`
	Versions []string      `json:"versions"`
}

type OSVRange struct {
	Type   string        `json:"type"`
	Events []OSVEvent    `json:"events"`
}

type OSVEvent struct {
	Introduced string `json:"introduced"`
	Fixed      string `json:"fixed"`
}

type OSVReference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type NVDBucket struct {
	StartIndex int           `json:"startIndex"`
	Total      int           `json:"total"`
	Vulns      []NVDVuln     `json:"vulnerabilities"`
}

type NVDVuln struct {
	ID      string      `json:"id"`
	Metrics *NVDMetrics `json:"metrics"`
	Desc    []NVDDesc   `json:"descriptions"`
}

type NVDMetrics struct {
	CVSS31 []NVDCVSS `json:"cvssMetricV31"`
	CVSS30 []NVDCVSS `json:"cvssMetricV30"`
}

type NVDCVSS struct {
	Score     float64 `json:"baseScore"`
	Severity  string  `json:"baseSeverity"`
	Vector    string  `json:"vectorString"`
}

type NVDDesc struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type PackageEntry struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

var defaultPackages = []PackageEntry{
	{Name: "openssh", Ecosystem: "Debian"},
	{Name: "openssl", Ecosystem: "Debian"},
	{Name: "glibc", Ecosystem: "Debian"},
	{Name: "curl", Ecosystem: "Debian"},
	{Name: "docker", Ecosystem: "Debian"},
	{Name: "xz-utils", Ecosystem: "Debian"},
	{Name: "httpd", Ecosystem: "Debian"},
	{Name: "protobuf", Ecosystem: "Debian"},
	{Name: "rust", Ecosystem: "Debian"},
	{Name: "bash", Ecosystem: "Debian"},
	{Name: "systemd", Ecosystem: "Debian"},
	{Name: "zlib", Ecosystem: "Debian"},
	{Name: "libpam", Ecosystem: "Debian"},
	{Name: "nginx", Ecosystem: "Debian"},
	{Name: "libssl", Ecosystem: "Debian"},
	{Name: "wget", Ecosystem: "Debian"},
	{Name: "git", Ecosystem: "Debian"},
	{Name: "python3", Ecosystem: "PyPI"},
	{Name: "node", Ecosystem: "npm"},
}

var osvEndpoint = "https://api.osv.dev/v1/query"
var osvBatchEndpoint = "https://api.osv.dev/v1/querybatch"
var nvdEndpoint = "https://services.nvd.nist.gov/rest/json/cves/2.0"

func NewCVESyncEngine(db *sql.DB, logger *zap.Logger) *CVESyncEngine {
	return &CVESyncEngine{
		db:     db,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				DisableCompression:  false,
			},
		},
		interval: 6 * time.Hour,
	}
}

func (e *CVESyncEngine) WithNVDAPIKey(key string) *CVESyncEngine {
	e.nvdAPIKey = key
	return e
}

func (e *CVESyncEngine) WithInterval(d time.Duration) *CVESyncEngine {
	e.interval = d
	return e
}

func (e *CVESyncEngine) Start(ctx context.Context) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.mu.Unlock()

	go func() {
		e.logger.Info("CVE sync engine started", zap.Duration("interval", e.interval))

		e.runSync(ctx)

		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				e.runSync(ctx)
			case <-ctx.Done():
				e.logger.Info("CVE sync engine stopped")
				return
			}
		}
	}()
}

func (e *CVESyncEngine) Stop() {
	e.mu.Lock()
	e.running = false
	e.mu.Unlock()
}

func (e *CVESyncEngine) Sync(ctx context.Context) {
	e.runSync(ctx)
}

// SetRemediationEngine sets the remediation callback
func (e *CVESyncEngine) SetRemediationEngine(fn func(deviceID, cveID, severity string)) {
	e.remediateFn = fn
}

func (e *CVESyncEngine) TriggerRemediation(ctx context.Context, deviceID, cveID, severity string) {
	if e.remediateFn != nil {
		e.remediateFn(deviceID, cveID, severity)
	}
}

func (e *CVESyncEngine) runSync(ctx context.Context) {
	e.logger.Info("starting CVE sync")

	var osvErr, nvdErr error

	if err := e.syncOSV(ctx); err != nil {
		e.logger.Error("OSV sync failed", zap.Error(err))
		osvErr = err
	}
	if err := e.updateSyncState(ctx, "osv", osvErr); err != nil {
		e.logger.Error("update sync state", zap.Error(err))
	}

	if e.nvdAPIKey != "" {
		if err := e.syncNVD(ctx); err != nil {
			e.logger.Error("NVD sync failed", zap.Error(err))
			nvdErr = err
		}
		if err := e.updateSyncState(ctx, "nvd", nvdErr); err != nil {
			e.logger.Error("update sync state", zap.Error(err))
		}
	}

	e.logger.Info("CVE sync complete")
}

func (e *CVESyncEngine) syncOSV(ctx context.Context) error {
	packages := e.loadTrackedPackages(ctx)
	if len(packages) == 0 {
		packages = defaultPackages
	}

	batch := make([]OSVQuery, 0, len(packages))
	for _, p := range packages {
		batch = append(batch, OSVQuery{
			Package: OSVPackage{
				Name:      p.Name,
				Ecosystem: p.Ecosystem,
			},
		})
	}

	body, err := json.Marshal(OSVQueryBatch{Queries: batch})
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", osvBatchEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("osv request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("osv returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var results []struct {
		Vulns []OSVVuln `json:"vulns"`
	}
	if err := json.Unmarshal(respBody, &results); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	newCount := 0
	updatedCount := 0

	for _, result := range results {
		for _, vuln := range result.Vulns {
			created, err := e.upsertCVE(ctx, vuln)
			if err != nil {
				e.logger.Warn("upsert CVE", zap.String("cve", vuln.ID), zap.Error(err))
				continue
			}
			if created {
				newCount++
			} else {
				updatedCount++
			}
		}
	}

	e.logger.Info("OSV sync completed",
		zap.Int("new", newCount),
		zap.Int("updated", updatedCount),
	)
	return nil
}

func (e *CVESyncEngine) syncNVD(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", nvdEndpoint, nil)
	if err != nil {
		return fmt.Errorf("create nvd request: %w", err)
	}

	q := req.URL.Query()
	q.Set("pubStartDate", time.Now().Add(-7*24*time.Hour).UTC().Format("2006-01-02T15:04:05.000"))
	q.Set("pubEndDate", time.Now().UTC().Format("2006-01-02T15:04:05.000"))
	q.Set("resultsPerPage", "50")
	req.URL.RawQuery = q.Encode()

	req.Header.Set("apiKey", e.nvdAPIKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("nvd request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("nvd returned %d", resp.StatusCode)
	}

	var bucket NVDBucket
	if err := json.NewDecoder(resp.Body).Decode(&bucket); err != nil {
		return fmt.Errorf("decode nvd: %w", err)
	}

	e.logger.Info("NVD sync completed", zap.Int("cves", len(bucket.Vulns)))
	return nil
}

func (e *CVESyncEngine) upsertCVE(ctx context.Context, vuln OSVVuln) (bool, error) {
	severity := "unknown"
	score := 0.0

	for _, s := range vuln.Severity {
		switch s.Type {
		case "CVSS_V3", "CVSS_V3.1":
			fmt.Sscanf(s.Score, "%f", &score)
		}
		if s.Type == "CVSS_V3" || s.Type == "CVSS_V3.1" {
			sev := scoreToSeverity(score)
			if sev != "" {
				severity = sev
			}
		}
	}

	description := vuln.Summary
	if description == "" {
		description = vuln.Details
	}
	if description == "" {
		description = vuln.ID
	}
	description = truncate(description, 500)

	packageName := ""
	var fixedVersions []string

	for _, aff := range vuln.Affected {
		if packageName == "" && aff.Package.Name != "" {
			packageName = aff.Package.Name
		}
		for _, r := range aff.Ranges {
			if r.Type == "SEMVER" || r.Type == "ECOSYSTEM" {
				for _, ev := range r.Events {
					if ev.Fixed != "" {
						fixedVersions = append(fixedVersions, ev.Fixed)
					}
				}
			}
		}
	}

	if packageName == "" || len(fixedVersions) == 0 {
		return false, nil
	}

	var exists bool
	err := e.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cve_database WHERE id = $1)`, vuln.ID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check exists: %w", err)
	}

	fixedIn := strings.Join(fixedVersions, ",")
	id := vuln.ID
	for _, alias := range vuln.Aliases {
		if strings.HasPrefix(alias, "CVE-") {
			id = alias
			break
		}
	}

	published := parseTime(vuln.Published)

	if exists {
		_, err = e.db.ExecContext(ctx, `
			UPDATE cve_database
			SET package_name = $1, severity = $2, score = $3, description = $4,
			    fixed_in = $5, fixed_in_versions = $6, source = 'osv',
			    published = $7
			WHERE id = $8
		`, packageName, severity, score, description, fixedIn, toSlice(fixedVersions), published, id)
		if err != nil {
			return false, fmt.Errorf("update cve: %w", err)
		}
		return false, nil
	}

	_, err = e.db.ExecContext(ctx, `
		INSERT INTO cve_database (id, package_name, severity, score, description, fixed_in, source, published)
		VALUES ($1, $2, $3, $4, $5, $6, 'osv', $7)
		ON CONFLICT (id) DO UPDATE SET
			package_name = EXCLUDED.package_name,
			severity = EXCLUDED.severity,
			score = EXCLUDED.score,
			description = EXCLUDED.description,
			fixed_in = EXCLUDED.fixed_in,
			source = 'osv',
			published = EXCLUDED.published
	`, id, packageName, severity, score, description, fixedIn, published)
	if err != nil {
		return false, fmt.Errorf("insert cve: %w", err)
	}
	return true, nil
}

func (e *CVESyncEngine) updateSyncState(ctx context.Context, source string, lastErr error) error {
	status := "success"
	errMsg := ""
	if lastErr != nil {
		status = "failed"
		errMsg = lastErr.Error()
	}

	_, err := e.db.ExecContext(ctx, `
		INSERT INTO cve_sync_state (id, source, last_synced, status, error)
		VALUES ($1, $2, NOW(), $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			last_synced = NOW(),
			status = EXCLUDED.status,
			error = EXCLUDED.error
	`, source+"-sync", source, status, truncate(errMsg, 500))
	return err
}

func (e *CVESyncEngine) loadTrackedPackages(ctx context.Context) []PackageEntry {
	if e.db == nil {
		return nil
	}
	rows, err := e.db.QueryContext(ctx, `SELECT package_name, ecosystem FROM cve_package_ecosystem`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var packages []PackageEntry
	for rows.Next() {
		var p PackageEntry
		if err := rows.Scan(&p.Name, &p.Ecosystem); err != nil {
			continue
		}
		packages = append(packages, p)
	}
	return packages
}

func (e *CVESyncEngine) AddTrackedPackage(ctx context.Context, name, ecosystem string) error {
	_, err := e.db.ExecContext(ctx, `
		INSERT INTO cve_package_ecosystem (package_name, ecosystem)
		VALUES ($1, $2) ON CONFLICT DO NOTHING
	`, name, ecosystem)
	return err
}

func (e *CVESyncEngine) RemoveTrackedPackage(ctx context.Context, name, ecosystem string) error {
	_, err := e.db.ExecContext(ctx, `
		DELETE FROM cve_package_ecosystem WHERE package_name = $1 AND ecosystem = $2
	`, name, ecosystem)
	return err
}

func (e *CVESyncEngine) ListTrackedPackages(ctx context.Context) ([]PackageEntry, error) {
	return e.loadTrackedPackages(ctx), nil
}

func (e *CVESyncEngine) GetSyncState(ctx context.Context) ([]SyncState, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT id, source, last_synced, status, error, records_new, records_updated
		FROM cve_sync_state ORDER BY last_synced DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []SyncState
	for rows.Next() {
		var s SyncState
		if err := rows.Scan(&s.ID, &s.Source, &s.LastSynced, &s.Status, &s.Error, &s.NewRecords, &s.UpdatedRecords); err != nil {
			continue
		}
		states = append(states, s)
	}
	return states, nil
}

type SyncState struct {
	ID             string    `json:"id"`
	Source         string    `json:"source"`
	LastSynced     time.Time `json:"last_synced"`
	Status         string    `json:"status"`
	Error          string    `json:"error,omitempty"`
	NewRecords     int       `json:"records_new"`
	UpdatedRecords int       `json:"records_updated"`
}

func (e *CVESyncEngine) GetCVECount(ctx context.Context) (int, error) {
	var count int
	err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cve_database`).Scan(&count)
	return count, err
}

func (e *CVESyncEngine) GetCVEByPackage(ctx context.Context, packageName string) ([]CVERecord, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT id, package_name, severity, score, description, fixed_in, published
		FROM cve_database WHERE package_name = $1
		ORDER BY score DESC
	`, packageName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cves []CVERecord
	for rows.Next() {
		var c CVERecord
		if err := rows.Scan(&c.ID, &c.PackageName, &c.Severity, &c.Score, &c.Description, &c.FixedIn, &c.Published); err != nil {
			continue
		}
		cves = append(cves, c)
	}
	return cves, nil
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	}
	t, err = time.Parse("2006-01-02T15:04:05", s)
	if err == nil {
		return t
	}
	return time.Now()
}

func scoreToSeverity(score float64) string {
	switch {
	case score >= 9.0:
		return "critical"
	case score >= 7.0:
		return "high"
	case score >= 4.0:
		return "medium"
	case score > 0:
		return "low"
	default:
		return "unknown"
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func toSlice(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}
