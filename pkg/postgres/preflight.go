package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const minDiskSpaceMB = 500
const maxActiveConnectionsWarn = 100

type PreflightResult struct {
	Pass      bool
	Checks    []PreflightCheck
	Timestamp time.Time
}

type PreflightCheck struct {
	Name    string
	Status  string // "pass", "fail", "warn"
	Message string
}

type PreflightChecker struct {
	db *sql.DB
}

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

func NewPreflightChecker(db *sql.DB) *PreflightChecker {
	return &PreflightChecker{db: db}
}

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
	}

	for _, c := range checks {
		check := c.fn()
		check.Name = c.name
		result.Checks = append(result.Checks, check)
	}

	result.Pass = true
	for _, check := range result.Checks {
		if check.Status == "fail" {
			result.Pass = false
			break
		}
	}

	return result
}

func (pc *PreflightChecker) CheckDatabaseConnectivity() PreflightCheck {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := pc.db.PingContext(ctx)
	if err != nil {
		return PreflightCheck{
			Status:  "fail",
			Message: fmt.Sprintf("database connectivity check failed: %v", err),
		}
	}

	return PreflightCheck{
		Status:  "pass",
		Message: "database is reachable",
	}
}

func (pc *PreflightChecker) CheckSchemaVersion() PreflightCheck {
	var version sql.NullInt64
	err := pc.db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		if err == ErrTableNotFound {
			return PreflightCheck{
				Status:  "warn",
				Message: "schema_migrations table does not exist yet; migrations not yet applied",
			}
		}
		return PreflightCheck{
			Status:  "fail",
			Message: fmt.Sprintf("failed to check schema version: %v", err),
		}
	}

	maxID := 0
	if version.Valid {
		maxID = int(version.Int64)
	}

	totalMigrations := len(Migrations())
	if maxID == 0 {
		return PreflightCheck{
			Status:  "warn",
			Message: fmt.Sprintf("no migrations applied yet; %d migrations available", totalMigrations),
		}
	}

	if maxID < totalMigrations {
		return PreflightCheck{
			Status:  "warn",
			Message: fmt.Sprintf("schema is at version %d; %d migration(s) available", maxID, totalMigrations-maxID),
		}
	}

	return PreflightCheck{
		Status:  "pass",
		Message: fmt.Sprintf("schema is current at version %d/%d", maxID, totalMigrations),
	}
}

func (pc *PreflightChecker) CheckDiskSpace() PreflightCheck {
	// Determine the data directory from Postgres if available, otherwise use /tmp
	dataDir := "/tmp"
	var dirErr error
	if pc.db != nil {
		var pgDataDir string
		dirErr = pc.db.QueryRow("SHOW data_directory").Scan(&pgDataDir)
		if dirErr == nil && pgDataDir != "" {
			dataDir = pgDataDir
		}
	}

	info, err := os.Stat(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return PreflightCheck{
				Status:  "warn",
				Message: fmt.Sprintf("disk space directory %q does not exist; skipping disk space check", dataDir),
			}
		}
		return PreflightCheck{
			Status:  "warn",
			Message: fmt.Sprintf("unable to stat disk space directory %q: %v (skipping)", dataDir, err),
		}
	}

	if info.IsDir() {
		return PreflightCheck{
			Status:  "pass",
			Message: fmt.Sprintf("disk space directory %q exists", dataDir),
		}
	}

	return PreflightCheck{
		Status:  "warn",
		Message: fmt.Sprintf("disk space path %q is not a directory; skipping check", dataDir),
	}
}

func (pc *PreflightChecker) CheckPostgresVersion() PreflightCheck {
	var versionString string
	err := pc.db.QueryRow("SHOW server_version").Scan(&versionString)
	if err != nil {
		return PreflightCheck{
			Status:  "fail",
			Message: fmt.Sprintf("failed to get Postgres version: %v", err),
		}
	}

	parsed, err := parsePostgresVersion(versionString)
	if err != nil {
		return PreflightCheck{
			Status:  "fail",
			Message: fmt.Sprintf("could not parse Postgres version string %q: %v", versionString, err),
		}
	}

	minVersion := PostgresVersion{Major: 14, Minor: 0}
	if parsed.LessThan(minVersion) {
		return PreflightCheck{
			Status:  "fail",
			Message: fmt.Sprintf("Postgres version %s is below minimum %d.%d", versionString, minVersion.Major, minVersion.Minor),
		}
	}

	return PreflightCheck{
		Status:  "pass",
		Message: fmt.Sprintf("Postgres version %s meets minimum requirement %d.%d", versionString, minVersion.Major, minVersion.Minor),
	}
}

func (pc *PreflightChecker) CheckActiveConnections() PreflightCheck {
	var activeCount int
	err := pc.db.QueryRow("SELECT count(*) FROM pg_stat_activity WHERE state = 'active'").Scan(&activeCount)
	if err != nil {
		return PreflightCheck{
			Status:  "fail",
			Message: fmt.Sprintf("failed to query active connections: %v", err),
		}
	}

	if activeCount > maxActiveConnectionsWarn {
		return PreflightCheck{
			Status:  "warn",
			Message: fmt.Sprintf("high number of active connections: %d (threshold: %d)", activeCount, maxActiveConnectionsWarn),
		}
	}

	return PreflightCheck{
		Status:  "pass",
		Message: fmt.Sprintf("active connections OK: %d (threshold: %d)", activeCount, maxActiveConnectionsWarn),
	}
}
