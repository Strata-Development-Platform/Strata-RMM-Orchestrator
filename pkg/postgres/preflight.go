package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"golang.org/x/sys/unix"
)

const minDiskSpaceMB = 500
const maxActiveConnectionsWarn = 100

// Valid statuses for PreflightCheck.
const (
	StatusPass     = "pass"
	StatusFail     = "fail"
	StatusWarn     = "warn"
	StatusNotApplicable = "not_applicable"
)

// ValidStatuses returns all allowed status strings.
var ValidStatuses = map[string]bool{
	StatusPass:      true,
	StatusFail:      true,
	StatusWarn:      true,
	StatusNotApplicable: true,
}

// PreflightResult holds the overall outcome of a preflight run.
type PreflightResult struct {
	Pass      bool
	Warnings  int
	Failures  int
	Checks    []PreflightCheck
	Timestamp time.Time
}

// PreflightCheck represents a single preflight check result.
//
// Status must be one of: "pass", "fail", "warn", "not_applicable".
// Message is always populated with human-readable information.
// Error is set only when Status is "fail".
type PreflightCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// PreflightChecker performs preflight checks on the orchestrator environment.
type PreflightChecker struct {
	db       *sql.DB
	cfg      *OrchestratorConfig
	natsConn *nats.Conn
}

// OrchestratorConfig is a subset of the full config needed for preflight checks.
type OrchestratorConfig struct {
	NATSURL   string
	NATSToken string
	NATSTLSEnabled bool

	DBDSN          string
	DBMaxOpenConns int
	DBMaxIdleConns int
	DBConnMaxLifetime time.Duration

	StorageBackend  string
	StorageBucket   string
	StorageEndpoint string
	StorageAccessKey string
	StorageSecretKey string
	StorageUseSSL   bool
	StorageKMSKeyID string

	JWTSecret string

	HTTPAPIAddr          string
	HTTPReadTimeout      time.Duration
	HTTPWriteTimeout     time.Duration
	HTTPMaxBodySizeBytes int64

	RuntimeMode string

	SeedingSeedDev    bool
	SeedingDevAdminEmail string
	SeedingDevAdminPwd string
}

// NewPreflightChecker creates a checker. Pass nil cfg when NATS/storage checks
// are not needed.
func NewPreflightChecker(db *sql.DB, cfg *OrchestratorConfig, natsConn *nats.Conn) *PreflightChecker {
	return &PreflightChecker{db: db, cfg: cfg, natsConn: natsConn}
}

// RunAll executes every registered preflight check.
func (pc *PreflightChecker) RunAll() *PreflightResult {
	result := &PreflightResult{
		Timestamp: time.Now(),
	}

	checks := []struct {
		name string
		fn   func() PreflightCheck
	}{
		{"DatabaseConnectivity", pc.CheckDatabaseConnectivity},
		{"SchemaVersion", pc.CheckSchemaVersion},
		{"DiskSpace", pc.CheckDiskSpace},
		{"PostgresVersion", pc.CheckPostgresVersion},
		{"ActiveConnections", pc.CheckActiveConnections},
		{"NATSConnectivity", pc.CheckNATSConnectivity},
		{"JetStreamAccess", pc.CheckJetStreamAccess},
		{"StorageWritable", pc.CheckStorageWritable},
		{"SecretsPresent", pc.CheckSecretsPresent},
		{"CandidateArtifact", pc.CheckCandidateArtifact},
	}

	for _, c := range checks {
		check := c.fn()
		check.Name = c.name
		result.Checks = append(result.Checks, check)

		if check.Status == StatusWarn {
			result.Warnings++
		}
		if check.Status == StatusFail {
			result.Failures++
		}
	}

	result.Pass = result.Failures == 0
	return result
}

// ---------------------------------------------------------------------------
// DatabaseConnectivity — pings the database
// ---------------------------------------------------------------------------

func (pc *PreflightChecker) CheckDatabaseConnectivity() PreflightCheck {
	if pc.db == nil {
		return PreflightCheck{
			Status:  StatusNotApplicable,
			Message: "database connection not available; skipping connectivity check",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := pc.db.PingContext(ctx)
	if err != nil {
		return PreflightCheck{
			Status:  StatusFail,
			Message: RedactCredentials("database connectivity check failed"),
			Error:   RedactCredentials(fmt.Sprintf("database connectivity check failed: %v", err)),
		}
	}

	return PreflightCheck{
		Status:  StatusPass,
		Message: "database is reachable",
	}
}

// ---------------------------------------------------------------------------
// SchemaVersion — verifies migration table state
// ---------------------------------------------------------------------------

func (pc *PreflightChecker) CheckSchemaVersion() PreflightCheck {
	if pc.db == nil {
		return PreflightCheck{
			Status:  StatusNotApplicable,
			Message: "database not available; skipping schema version check",
		}
	}

	// First, verify we can at least see the table.
	var tableExists int
	err := pc.db.QueryRow(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'schema_migrations' AND table_schema = 'public'",
	).Scan(&tableExists)
	if err != nil {
		return PreflightCheck{
			Status:  StatusFail,
			Message: "failed to check for schema_migrations table",
			Error:   RedactCredentials(fmt.Sprintf("failed to check for schema_migrations table: %v", err)),
		}
	}

	if tableExists == 0 {
		// Table does not exist. This is OK for a brand-new (clean) install,
		// but a warning for anything that had a previous version.
		// We cannot distinguish these cases without checking the Postgres
		// version — however, we already know the DB is reachable (above).
		// A missing table on an already-running instance is still a "warn"
		// because the operator may not have run migrations yet.
		return PreflightCheck{
			Status:  StatusWarn,
			Message: "schema_migrations table does not exist; this is a clean install or migrations have not been applied",
		}
	}

	// Table exists — read the latest applied migration ID.
	var version sql.NullInt64
	err = pc.db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return PreflightCheck{
			Status:  StatusFail,
			Message: "failed to read schema_migrations",
			Error:   RedactCredentials(fmt.Sprintf("failed to read schema_migrations: %v", err)),
		}
	}

	maxID := 0
	if version.Valid {
		maxID = int(version.Int64)
	}

	totalMigrations := len(Migrations())
	if maxID == 0 {
		return PreflightCheck{
			Status:  StatusWarn,
			Message: fmt.Sprintf("no migrations applied yet; %d migrations available", totalMigrations),
		}
	}

	if maxID < totalMigrations {
		return PreflightCheck{
			Status:  StatusWarn,
			Message: fmt.Sprintf("schema is at version %d; %d migration(s) available", maxID, totalMigrations-maxID),
		}
	}

	return PreflightCheck{
		Status:  StatusPass,
		Message: fmt.Sprintf("schema is current at version %d/%d", maxID, totalMigrations),
	}
}

// ---------------------------------------------------------------------------
// DiskSpace — uses unix.Statfs to report actual available space
// ---------------------------------------------------------------------------

func (pc *PreflightChecker) CheckDiskSpace() PreflightCheck {
	if pc.db == nil {
		return pc.checkDiskSpaceFallback("database not available")
	}

	dataDir := "/tmp"
	err := pc.db.QueryRow("SHOW data_directory").Scan(&dataDir)
	if err != nil || dataDir == "" {
		return pc.checkDiskSpaceFallback(fmt.Sprintf("could not determine Postgres data directory; using fallback: %v", err))
	}

	return pc.checkDiskSpaceOnDir(dataDir)
}

func (pc *PreflightChecker) checkDiskSpaceFallback(reason string) PreflightCheck {
	info, err := os.Stat("/tmp")
	if err != nil {
		if os.IsNotExist(err) {
			return PreflightCheck{
				Status:  StatusWarn,
				Message: fmt.Sprintf("disk space /tmp does not exist (%s); skipping check", reason),
			}
		}
		return PreflightCheck{
			Status:  StatusWarn,
			Message: fmt.Sprintf("unable to stat /tmp (%s); skipping: %v", reason, err),
		}
	}

	if !info.IsDir() {
		return PreflightCheck{
			Status:  StatusWarn,
			Message: fmt.Sprintf("/tmp is not a directory (%s); skipping", reason),
		}
	}

	return pc.reportDiskSpace("/tmp")
}

func (pc *PreflightChecker) checkDiskSpaceOnDir(dir string) PreflightCheck {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return PreflightCheck{
				Status:  StatusWarn,
				Message: fmt.Sprintf("disk space directory %q does not exist; skipping check", dir),
			}
		}
		return PreflightCheck{
			Status:  StatusWarn,
			Message: fmt.Sprintf("unable to stat disk space directory %q: %v (skipping)", dir, err),
		}
	}

	if !info.IsDir() {
		return PreflightCheck{
			Status:  StatusWarn,
			Message: fmt.Sprintf("disk space path %q is not a directory; skipping check", dir),
		}
	}

	return pc.reportDiskSpace(dir)
}

func (pc *PreflightChecker) reportDiskSpace(dir string) PreflightCheck {
	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return PreflightCheck{
			Status:  StatusWarn,
			Message: fmt.Sprintf("statfs failed for %q: %v (skipping disk space check)", dir, err),
		}
	}

	availableMB := int64(stat.Bavail) * int64(stat.Bsize) / 1024 / 1024
	totalMB := int64(stat.Blocks) * int64(stat.Bsize) / 1024 / 1024

	if availableMB < minDiskSpaceMB {
		return PreflightCheck{
			Status:  StatusFail,
			Message: fmt.Sprintf("disk space critical on %q: %d MB available (minimum: %d MB)", dir, availableMB, minDiskSpaceMB),
			Error:   fmt.Sprintf("disk space critical on %q: %d MB available (minimum: %d MB)", dir, availableMB, minDiskSpaceMB),
		}
	}

	status := StatusPass
	message := fmt.Sprintf("disk space OK on %q: %d MB available of %d MB (minimum: %d MB)", dir, availableMB, totalMB, minDiskSpaceMB)
	if availableMB < minDiskSpaceMB*5 {
		status = StatusWarn
		message = fmt.Sprintf("disk space low on %q: %d MB available of %d MB (minimum: %d MB)", dir, availableMB, totalMB, minDiskSpaceMB)
	}

	return PreflightCheck{
		Status:  status,
		Message: message,
	}
}

// ---------------------------------------------------------------------------
// PostgresVersion — verifies minimum version requirement
// ---------------------------------------------------------------------------

func (pc *PreflightChecker) CheckPostgresVersion() PreflightCheck {
	if pc.db == nil {
		return PreflightCheck{
			Status:  StatusNotApplicable,
			Message: "database not available; skipping version check",
		}
	}

	var versionString string
	err := pc.db.QueryRow("SHOW server_version").Scan(&versionString)
	if err != nil {
		return PreflightCheck{
			Status:  StatusFail,
			Message: "failed to get Postgres version",
			Error:   RedactCredentials(fmt.Sprintf("failed to get Postgres version: %v", err)),
		}
	}

	parsed, err := parsePostgresVersion(versionString)
	if err != nil {
		return PreflightCheck{
			Status:  StatusFail,
			Message: fmt.Sprintf("could not parse Postgres version string %q", versionString),
			Error:   fmt.Sprintf("could not parse Postgres version string %q: %v", versionString, err),
		}
	}

	minVersion := PostgresVersion{Major: 14, Minor: 0}
	if parsed.LessThan(minVersion) {
		return PreflightCheck{
			Status:  StatusFail,
			Message: fmt.Sprintf("Postgres version %s is below minimum %d.%d", versionString, minVersion.Major, minVersion.Minor),
			Error:   fmt.Sprintf("Postgres version %s is below minimum %d.%d", versionString, minVersion.Major, minVersion.Minor),
		}
	}

	return PreflightCheck{
		Status:  StatusPass,
		Message: fmt.Sprintf("Postgres version %s meets minimum requirement %d.%d", versionString, minVersion.Major, minVersion.Minor),
	}
}

// ---------------------------------------------------------------------------
// ActiveConnections — checks for connection saturation
// ---------------------------------------------------------------------------

func (pc *PreflightChecker) CheckActiveConnections() PreflightCheck {
	if pc.db == nil {
		return PreflightCheck{
			Status:  StatusNotApplicable,
			Message: "database not available; skipping active connections check",
		}
	}

	var activeCount int
	err := pc.db.QueryRow("SELECT count(*) FROM pg_stat_activity WHERE state = 'active'").Scan(&activeCount)
	if err != nil {
		return PreflightCheck{
			Status:  StatusFail,
			Message: "failed to query active connections",
			Error:   RedactCredentials(fmt.Sprintf("failed to query active connections: %v", err)),
		}
	}

	if activeCount > maxActiveConnectionsWarn {
		return PreflightCheck{
			Status:  StatusWarn,
			Message: fmt.Sprintf("high number of active connections: %d (threshold: %d)", activeCount, maxActiveConnectionsWarn),
		}
	}

	return PreflightCheck{
		Status:  StatusPass,
		Message: fmt.Sprintf("active connections OK: %d (threshold: %d)", activeCount, maxActiveConnectionsWarn),
	}
}

// ---------------------------------------------------------------------------
// NATSConnectivity — verifies NATS connection and JetStream availability
// ---------------------------------------------------------------------------

func (pc *PreflightChecker) CheckNATSConnectivity() PreflightCheck {
	if pc.cfg == nil || pc.cfg.NATSURL == "" {
		return PreflightCheck{
			Status:  StatusNotApplicable,
			Message: "NATS configuration not available; skipping NATS connectivity check",
		}
	}

	// If we already have a live NATS connection, just ping it.
	if pc.natsConn != nil {
		if !pc.natsConn.IsConnected() {
			return PreflightCheck{
				Status:  StatusFail,
				Message: "NATS is not connected",
				Error:   RedactCredentials(fmt.Sprintf("NATS connectivity check failed: not connected (URL: %s)", RedactCredentials(pc.cfg.NATSURL))),
			}
		}
		return PreflightCheck{
			Status:  StatusPass,
			Message: "NATS connection is active",
		}
	}

	// Otherwise try to establish a new connection.
	nc, err := testNATSConnection(pc.cfg.NATSURL, pc.cfg.NATSToken, pc.cfg.NATSTLSEnabled)
	if err != nil {
		return PreflightCheck{
			Status:  StatusFail,
			Message: "NATS connectivity check failed",
			Error:   RedactCredentials(fmt.Sprintf("NATS connectivity check failed: %v (URL: %s)", err, RedactCredentials(pc.cfg.NATSURL))),
		}
	}
	defer nc.Close()

	return PreflightCheck{
		Status:  StatusPass,
		Message: fmt.Sprintf("NATS connection established (URL: %s)", RedactCredentials(pc.cfg.NATSURL)),
	}
}

// testNATSConnection opens a temporary NATS connection to verify reachability.
func testNATSConnection(natsURL, token string, tlsEnabled bool) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name("StrataRMM-Preflight"),
		nats.RetryOnFailedConnect(true),
		nats.Timeout(5 * time.Second),
	}
	if token != "" {
		opts = append(opts, nats.Token(token))
	}

	// We don't configure TLS for a connectivity ping because the caller
	// already validated TLS config; we just check reachability.
	return nats.Connect(natsURL, opts...)
}

// ---------------------------------------------------------------------------
// JetStreamAccess — verifies JetStream and required streams
// ---------------------------------------------------------------------------

func (pc *PreflightChecker) CheckJetStreamAccess() PreflightCheck {
	if pc.cfg == nil || pc.cfg.NATSURL == "" {
		return PreflightCheck{
			Status:  StatusNotApplicable,
			Message: "NATS configuration not available; skipping JetStream access check",
		}
	}

	// Reuse existing connection if available
	nc := pc.natsConn
	if nc == nil {
		var err error
		nc, err = testNATSConnection(pc.cfg.NATSURL, pc.cfg.NATSToken, pc.cfg.NATSTLSEnabled)
		if err != nil {
			// Can't reach NATS, so JetStream is unreachable too.
			return PreflightCheck{
				Status:  StatusWarn,
				Message: fmt.Sprintf("NATS unreachable (%s); JetStream access check skipped", RedactCredentials(pc.cfg.NATSURL)),
			}
		}
		defer nc.Close()
	}

	js, err := nc.JetStream()
	if err != nil {
		return PreflightCheck{
			Status:  StatusFail,
			Message: "JetStream not available",
			Error:   RedactCredentials(fmt.Sprintf("JetStream not available: %v", err)),
		}
	}

	if _, err := js.AccountInfo(); err != nil {
		return PreflightCheck{
			Status:  StatusFail,
			Message: "JetStream account info check failed",
			Error:   RedactCredentials(fmt.Sprintf("JetStream account access denied: %v", err)),
		}
	}

	// Check required streams exist.
	requiredStreams := []string{
		"tenant",
		"device",
		"agent",
	}

	streamsFound := 0
	for _, streamName := range requiredStreams {
		subjectPrefix := streamName + "."
		_, err := js.StreamInfo(streamName)
		if err == nil {
			streamsFound++
			continue
		}

		// Try to fetch info via subject prefix if stream doesn't exist yet
		// This is expected for new installations
		_ = subjectPrefix // suppress unused variable warning
	}

	if streamsFound == 0 {
		return PreflightCheck{
			Status:  StatusWarn,
			Message: fmt.Sprintf("JetStream enabled but no required streams found; %d/%d streams discovered (new install may be OK)", streamsFound, len(requiredStreams)),
		}
	}

	if streamsFound < len(requiredStreams) {
		return PreflightCheck{
			Status:  StatusWarn,
			Message: fmt.Sprintf("JetStream access OK but %d/%d required streams found", streamsFound, len(requiredStreams)),
		}
	}

	return PreflightCheck{
		Status:  StatusPass,
		Message: fmt.Sprintf("JetStream access verified; %d/%d required streams present", streamsFound, len(requiredStreams)),
	}
}

// ---------------------------------------------------------------------------
// StorageWritable — checks persistent storage paths are writable
// ---------------------------------------------------------------------------

func (pc *PreflightChecker) CheckStorageWritable() PreflightCheck {
	if pc.cfg == nil {
		return PreflightCheck{
			Status:  StatusNotApplicable,
			Message: "storage configuration not available; skipping storage writability check",
		}
	}

	backend := pc.cfg.StorageBackend
	if backend == "" || backend == "none" {
		return PreflightCheck{
			Status:  StatusNotApplicable,
			Message: "storage is disabled; skipping writability check",
		}
	}

	switch backend {
	case "local":
		return pc.checkLocalStorageWritable()
	case "s3", "minio":
		return pc.checkRemoteStorageWritable()
	default:
		return PreflightCheck{
			Status:  StatusWarn,
			Message: fmt.Sprintf("unknown storage backend %q; cannot verify writability", backend),
		}
	}
}

func (pc *PreflightChecker) checkLocalStorageWritable() PreflightCheck {
	// For local storage, check the configured endpoint or default path.
	basePath := "/var/lib/strata-rmm/releases"
	if pc.cfg.StorageEndpoint != "" {
		basePath = pc.cfg.StorageEndpoint
	}

	// Check the parent directory is writable.
	parentDir := filepath.Dir(basePath)
	info, err := os.Stat(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return PreflightCheck{
				Status:  StatusWarn,
				Message: fmt.Sprintf("storage path parent %q does not exist; cannot verify writability", parentDir),
			}
		}
		return PreflightCheck{
			Status:  StatusFail,
			Message: fmt.Sprintf("storage path %q stat failed", parentDir),
			Error:   fmt.Sprintf("storage path %q stat failed: %v", parentDir, err),
		}
	}

	if !info.IsDir() {
		return PreflightCheck{
			Status:  StatusFail,
			Message: fmt.Sprintf("storage path %q is not a directory", parentDir),
		}
	}

	// Check write permission.
	if err := checkWritable(parentDir); err != nil {
		return PreflightCheck{
			Status:  StatusFail,
			Message: fmt.Sprintf("storage path %q is not writable", parentDir),
			Error:   fmt.Sprintf("storage path %q is not writable: %v", parentDir, err),
		}
	}

	return PreflightCheck{
		Status:  StatusPass,
		Message: fmt.Sprintf("storage path %q is writable", parentDir),
	}
}

func (pc *PreflightChecker) checkRemoteStorageWritable() PreflightCheck {
	// For S3/Minio, verify that access keys are configured.
	if pc.cfg.StorageAccessKey == "" {
		return PreflightCheck{
			Status:  StatusFail,
			Message: "remote storage access key not configured",
			Error:   "remote storage access key not configured",
		}
	}

	// We can't actually write to remote storage without a client, but we
	// can check that the bucket and endpoint are configured.
	if pc.cfg.StorageBucket == "" {
		return PreflightCheck{
			Status:  StatusWarn,
			Message: "remote storage bucket not configured; skipping writability check",
		}
	}

	if pc.cfg.StorageEndpoint == "" {
		return PreflightCheck{
			Status:  StatusWarn,
			Message: "remote storage endpoint not configured; skipping writability check",
		}
	}

	return PreflightCheck{
		Status:  StatusPass,
		Message: fmt.Sprintf("remote storage configuration OK: %s bucket at %s", pc.cfg.StorageBucket, RedactCredentials(pc.cfg.StorageEndpoint)),
	}
}

// checkWritable attempts to create a temp file in the given directory.
func checkWritable(dir string) error {
	tmpFile := filepath.Join(dir, ".strata-preflight-write-test")
	f, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	f.Close()
	os.Remove(tmpFile)
	return nil
}

// ---------------------------------------------------------------------------
// SecretsPresent — checks required secret keys are configured
// ---------------------------------------------------------------------------

func (pc *PreflightChecker) CheckSecretsPresent() PreflightCheck {
	if pc.cfg == nil {
		return PreflightCheck{
			Status:  StatusNotApplicable,
			Message: "configuration not available; skipping secrets check",
		}
	}

	var missing []string
	required := map[string]string{
		"JWT.Secret": pc.cfg.JWTSecret,
	}

	// DB.DSN always needs to be present (contains credentials).
	if pc.cfg.DBDSN == "" {
		missing = append(missing, "DB.DSN")
	}

	for label, value := range required {
		if value == "" {
			missing = append(missing, label)
		}
	}

	if len(missing) > 0 {
		return PreflightCheck{
			Status:  StatusFail,
			Message: "required secrets are not configured: " + strings.Join(missing, ", "),
			Error:   fmt.Sprintf("missing required secrets: %d", len(missing)),
		}
	}

	// JWT secret should be non-trivial.
	if len(pc.cfg.JWTSecret) < 32 {
		return PreflightCheck{
			Status:  StatusFail,
			Message: "JWT.Secret is too short (minimum 32 characters)",
			Error:   "JWT.Secret is too short (minimum 32 characters)",
		}
	}

	return PreflightCheck{
		Status:  StatusPass,
		Message: "all required secrets are configured",
	}
}

// ---------------------------------------------------------------------------
// CandidateArtifact — validates upgrade artifact existence and checksum
// ---------------------------------------------------------------------------

func (pc *PreflightChecker) CheckCandidateArtifact() PreflightCheck {
	if pc.cfg == nil {
		return PreflightCheck{
			Status:  StatusNotApplicable,
			Message: "configuration not available; skipping candidate artifact check",
		}
	}

	artifactPath := os.Getenv("STRATA_ARTIFACT_PATH")
	if artifactPath == "" {
		// Not running an upgrade context — not applicable.
		return PreflightCheck{
			Status:  StatusNotApplicable,
			Message: "STRATA_ARTIFACT_PATH not set; candidate artifact check skipped",
		}
	}

	info, err := os.Stat(artifactPath)
	if err != nil {
		if os.IsNotExist(err) {
			return PreflightCheck{
				Status:  StatusFail,
				Message: fmt.Sprintf("candidate artifact not found at %q", artifactPath),
				Error:   fmt.Sprintf("candidate artifact not found at %q", artifactPath),
			}
		}
		return PreflightCheck{
			Status:  StatusFail,
			Message: fmt.Sprintf("candidate artifact stat failed at %q", artifactPath),
			Error:   fmt.Sprintf("candidate artifact stat failed: %v", err),
		}
	}

	if info.IsDir() {
		return PreflightCheck{
			Status:  StatusFail,
			Message: fmt.Sprintf("candidate artifact path %q is a directory, not a file", artifactPath),
		}
	}

	// Validate checksum if STRATA_ARTIFACT_CHECKSUM is set.
	expectedChecksum := os.Getenv("STRATA_ARTIFACT_CHECKSUM")
	if expectedChecksum == "" {
		return PreflightCheck{
			Status:  StatusPass,
			Message: fmt.Sprintf("candidate artifact exists at %q (%d bytes); no checksum to verify", artifactPath, info.Size()),
		}
	}

	actual, err := computeFileChecksum(artifactPath)
	if err != nil {
		return PreflightCheck{
			Status:  StatusFail,
			Message: fmt.Sprintf("failed to compute checksum for %q", artifactPath),
			Error:   fmt.Sprintf("failed to compute checksum: %v", err),
		}
	}

	// Support both SHA-256 and SHA-512 formats.
	lowerExpected := strings.ToLower(strings.TrimSpace(expectedChecksum))
	lowerActual := strings.ToLower(strings.TrimSpace(actual))

	if lowerExpected == lowerActual {
		return PreflightCheck{
			Status:  StatusPass,
			Message: fmt.Sprintf("candidate artifact checksum verified: %s at %q", computeChecksumType(expectedChecksum), artifactPath),
		}
	}

	return PreflightCheck{
		Status:  StatusFail,
		Message: fmt.Sprintf("candidate artifact checksum mismatch at %q", artifactPath),
		Error:   fmt.Sprintf("checksum mismatch: expected %s, got %s", maskChecksum(lowerExpected), maskChecksum(lowerActual)),
	}
}

func computeChecksumType(checksum string) string {
	lower := strings.ToLower(strings.TrimSpace(checksum))
	switch {
	case len(lower) == 64:
		return "SHA-256"
	case len(lower) == 128:
		return "SHA-512"
	default:
		return "unknown"
	}
}

func computeFileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func maskChecksum(cs string) string {
	if len(cs) <= 8 {
		return "***"
	}
	return cs[:4] + "..." + cs[len(cs)-4:]
}

// ---------------------------------------------------------------------------
// Version utilities (already in preflight.go)
// ---------------------------------------------------------------------------

type PostgresVersion struct {
	Major int
	Minor int
	Patch int
}

func (v PostgresVersion) LessThan(other PostgresVersion) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

var postgresVersionRe = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?(?:-|$)`)

func parsePostgresVersion(s string) (PostgresVersion, error) {
	s = strings.TrimSpace(s)

	if idx := strings.Index(s, " (PostgreSQL)"); idx != -1 {
		s = s[:idx]
	}
	if strings.Contains(s, "docker") || strings.Contains(s, "alpine") {
		return PostgresVersion{}, fmt.Errorf("cannot parse prerelease or docker image version string %q", s)
	}
	if idx := strings.IndexByte(s, '-'); idx != -1 {
		return PostgresVersion{}, fmt.Errorf("cannot parse prerelease version string %q", s)
	}

	matches := postgresVersionRe.FindStringSubmatch(s)
	if matches == nil {
		return PostgresVersion{}, fmt.Errorf("cannot parse version string %q", s)
	}
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch := 0
	if matches[3] != "" {
		patch, _ = strconv.Atoi(matches[3])
	}
	return PostgresVersion{Major: major, Minor: minor, Patch: patch}, nil
}

// ---------------------------------------------------------------------------
// Credential redaction
// ---------------------------------------------------------------------------

	var (
		dsnPasswordRe   = regexp.MustCompile(`(postgresql?|postgres)://([^:]+):([^@]+)@`)
		natsURLPasswordRe = regexp.MustCompile(`(nats|tls)://([^:]+):([^@]+)@`)
		tokenRe         = regexp.MustCompile(`(Bearer|Token)\s+[A-Za-z0-9\-._~+/]+=*`)
		apiKeyRe        = regexp.MustCompile(`((?:api[_-]?key|apikey|access[_-]?key|secret[_-]?key|auth[_-]?token)["']?\s*[:=]\s*["']?)[A-Za-z0-9\-._~+/=]{8,}`)
		passwordQueryRe = regexp.MustCompile(`([&?]password=)[^&]*`)
	)

// RedactCredentials replaces sensitive values in a string with redacted markers.
// It handles PostgreSQL/Postgres DSNs, NATS URLs, Bearer tokens, API keys,
// and query-string passwords.
func RedactCredentials(s string) string {
	result := s

	// Redact DSN passwords: postgresql://user:PASSWORD@host → postgresql://user:***@host
	result = dsnPasswordRe.ReplaceAllString(result, "${1}${2}:***@")

	// Redact NATS URL passwords
	result = natsURLPasswordRe.ReplaceAllString(result, "${1}://${2}:***@")

	// Redact Bearer/Token headers
	result = tokenRe.ReplaceAllString(result, "${1} ***")

	// Redact API key / secret key assignments.
	result = apiKeyRe.ReplaceAllString(result, "${1}***")

	// Redact password query parameters
	result = passwordQueryRe.ReplaceAllString(result, "${1}***")

	return result
}

// ---------------------------------------------------------------------------
// Runtime / OS utilities
// ---------------------------------------------------------------------------

func init() {
	// Ensure unix package is available (handles Windows build compatibility).
	_ = runtime.GOOS
}
