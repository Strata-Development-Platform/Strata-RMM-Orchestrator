package update

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/config"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
	"go.uber.org/zap"
)

// DefaultUpgradeSnapshotDir is retained as a compatibility alias for callers
// created before row-level upgrade backups replaced metadata-only snapshots.
const DefaultUpgradeSnapshotDir = DefaultUpgradeBackupDir

// NewRuntimePreflight returns the single fail-closed runtime prerequisite policy
// shared by CLI and HTTP upgrade entrypoints.
func NewRuntimePreflight(db *sql.DB, logger *zap.Logger, backupDir string) PreflightFunc {
	if backupDir == "" {
		backupDir = DefaultUpgradeBackupDir
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return func(ctx context.Context, release *OrchestratorRelease) (PreflightResult, error) {
		result := PreflightResult{Pass: true, Timestamp: time.Now().UTC()}
		appendCheck := func(name string, pass bool, message string) {
			result.Checks = append(result.Checks, PreflightCheck{Name: name, Pass: pass, Message: message})
			if !pass {
				result.Pass = false
			}
		}

		if db == nil {
			appendCheck("database", false, "live database is unavailable")
			appendCheck("backup", false, "backup was not attempted because the database is unavailable")
			return result, nil
		}

		checker := postgres.NewPreflightChecker(db, nil, nil)
		dbCheck := checker.CheckDatabaseConnectivity()
		appendCheck("database", dbCheck.Status != postgres.StatusFail, dbCheck.Message)
		diskCheck := checker.CheckDiskSpace()
		appendCheck("temporary_disk", diskCheck.Status != postgres.StatusFail, diskCheck.Message)

		targetSchema, err := strconv.Atoi(release.SchemaCompatibility)
		if err != nil || targetSchema < 0 {
			appendCheck("schema", false, fmt.Sprintf("signed target schema %q is invalid", release.SchemaCompatibility))
		} else {
			var liveSchema int
			err = db.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM schema_migrations").Scan(&liveSchema)
			result.SourceSchemaVersion = liveSchema
			result.TargetSchemaVersion = targetSchema
			switch {
			case err != nil:
				appendCheck("schema", false, fmt.Sprintf("cannot read live schema version: %v", err))
			case liveSchema < 0:
				appendCheck("schema", false, fmt.Sprintf("live schema %d is invalid", liveSchema))
			case liveSchema > targetSchema:
				appendCheck("schema", false, fmt.Sprintf("live schema %d is newer than signed candidate schema %d; downgrade through the update path is forbidden", liveSchema, targetSchema))
			default:
				appendCheck("schema", true, fmt.Sprintf("signed forward schema transition accepted: %d -> %d", liveSchema, targetSchema))
			}
		}

		if !result.Pass {
			appendCheck("backup", false, "backup was not attempted because an earlier upgrade prerequisite failed")
			return result, nil
		}

		cfg, cfgErr := config.LoadOrchestratorConfig()
		if cfgErr != nil {
			appendCheck("backup", false, fmt.Sprintf("cannot load active database configuration for upgrade backup: %v", cfgErr))
			return result, nil
		}
		databaseBackup, backupErr := createUpgradeDatabaseBackup(ctx, db, cfg.DB.DSN, backupDir, DefaultUpgradeHandoffPath)
		if backupErr != nil {
			appendCheck("backup", false, fmt.Sprintf("pre-upgrade PostgreSQL data backup failed: %v", backupErr))
			return result, nil
		}
		appendCheck("backup", true, fmt.Sprintf("pre-upgrade PostgreSQL data backup created and bound to restart handoff: %d bytes sha256=%s", databaseBackup.Size, databaseBackup.SHA256))
		logger.Info("row-level PostgreSQL upgrade backup ready",
			zap.Int64("size_bytes", databaseBackup.Size),
			zap.String("sha256", databaseBackup.SHA256),
		)
		return result, nil
	}
}
