package update

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/backup"
)

const (
	DefaultUpgradeBackupDir     = "/var/lib/strata-rmm/backups/upgrades"
	DefaultUpgradeHandoffPath   = "/var/lib/strata-rmm/updates/database-backup.env"
)

type upgradeDatabaseBackup struct {
	Path   string
	SHA256 string
	Size   int64
}

func createUpgradeDatabaseBackup(ctx context.Context, db *sql.DB, dsn, backupDir, handoffPath string) (upgradeDatabaseBackup, error) {
	if db == nil {
		return upgradeDatabaseBackup{}, fmt.Errorf("live database is required")
	}
	if strings.TrimSpace(dsn) == "" {
		return upgradeDatabaseBackup{}, fmt.Errorf("active database DSN is required")
	}
	if backupDir == "" {
		backupDir = DefaultUpgradeBackupDir
	}
	if handoffPath == "" {
		handoffPath = DefaultUpgradeHandoffPath
	}
	connection, err := verifiedUpgradeConnection(ctx, db, dsn)
	if err != nil {
		return upgradeDatabaseBackup{}, err
	}
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return upgradeDatabaseBackup{}, fmt.Errorf("create upgrade backup directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(handoffPath), 0700); err != nil {
		return upgradeDatabaseBackup{}, fmt.Errorf("create upgrade handoff directory: %w", err)
	}

	file, err := os.CreateTemp(backupDir, "postgres-upgrade-*.dump")
	if err != nil {
		return upgradeDatabaseBackup{}, fmt.Errorf("create upgrade database backup: %w", err)
	}
	path := file.Name()
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = os.Remove(path)
			_ = os.Remove(handoffPath)
		}
	}()
	if err := file.Chmod(0600); err != nil {
		return upgradeDatabaseBackup{}, fmt.Errorf("protect upgrade database backup: %w", err)
	}

	component, err := backup.NewPostgreSQLBackup(dsn)
	if err != nil {
		return upgradeDatabaseBackup{}, fmt.Errorf("configure PostgreSQL upgrade backup: %w", err)
	}
	manifest, err := component.Backup(ctx, file)
	if err != nil {
		return upgradeDatabaseBackup{}, fmt.Errorf("create PostgreSQL upgrade backup: %w", err)
	}
	if manifest.Size <= 0 || len(manifest.Digest) != 64 {
		return upgradeDatabaseBackup{}, fmt.Errorf("PostgreSQL upgrade backup returned invalid integrity metadata")
	}
	if err := file.Sync(); err != nil {
		return upgradeDatabaseBackup{}, fmt.Errorf("sync PostgreSQL upgrade backup: %w", err)
	}
	if err := file.Close(); err != nil {
		return upgradeDatabaseBackup{}, fmt.Errorf("close PostgreSQL upgrade backup: %w", err)
	}

	handoff := strings.Builder{}
	writeEnv := func(name, value string) {
		handoff.WriteString(name)
		handoff.WriteByte('=')
		handoff.WriteString(shellQuote(value))
		handoff.WriteByte('\n')
	}
	writeEnv("DB_BACKUP_PATH", path)
	writeEnv("DB_BACKUP_SHA256", manifest.Digest)
	writeEnv("PGHOST", connection.host)
	writeEnv("PGPORT", connection.port)
	writeEnv("PGUSER", connection.user)
	writeEnv("PGPASSWORD", connection.password)
	writeEnv("PGDATABASE", connection.database)
	writeEnv("PGSSLMODE", connection.sslmode)

	tempHandoff := handoffPath + ".tmp"
	if err := os.WriteFile(tempHandoff, []byte(handoff.String()), 0600); err != nil {
		return upgradeDatabaseBackup{}, fmt.Errorf("write upgrade database handoff: %w", err)
	}
	if err := os.Rename(tempHandoff, handoffPath); err != nil {
		_ = os.Remove(tempHandoff)
		return upgradeDatabaseBackup{}, fmt.Errorf("publish upgrade database handoff: %w", err)
	}
	cleanup = false
	return upgradeDatabaseBackup{Path: path, SHA256: manifest.Digest, Size: manifest.Size}, nil
}

type upgradeConnection struct {
	host, port, user, password, database, sslmode string
}

func verifiedUpgradeConnection(ctx context.Context, db *sql.DB, dsn string) (upgradeConnection, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return upgradeConnection{}, fmt.Errorf("upgrade database backup requires a PostgreSQL URL DSN")
	}
	password, _ := parsed.User.Password()
	connection := upgradeConnection{
		host: parsed.Hostname(), port: parsed.Port(), user: parsed.User.Username(), password: password,
		database: strings.TrimPrefix(parsed.Path, "/"), sslmode: parsed.Query().Get("sslmode"),
	}
	if connection.port == "" {
		connection.port = "5432"
	}
	if connection.sslmode == "" {
		connection.sslmode = "prefer"
	}
	if connection.host == "" || connection.user == "" || connection.database == "" {
		return upgradeConnection{}, fmt.Errorf("upgrade database DSN must include host, user, and database")
	}

	var liveDatabase, liveUser string
	var liveAddress sql.NullString
	var livePort sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT current_database(), current_user, inet_server_addr()::text, inet_server_port()").Scan(&liveDatabase, &liveUser, &liveAddress, &livePort); err != nil {
		return upgradeConnection{}, fmt.Errorf("verify active database identity: %w", err)
	}
	if liveDatabase != connection.database || liveUser != connection.user {
		return upgradeConnection{}, fmt.Errorf("configured database DSN does not match the active database identity")
	}
	if livePort.Valid && livePort.Int64 > 0 && fmt.Sprintf("%d", livePort.Int64) != connection.port {
		return upgradeConnection{}, fmt.Errorf("configured database port does not match the active database")
	}
	if liveAddress.Valid && liveAddress.String != "" {
		ips, err := net.LookupIP(connection.host)
		if err != nil {
			return upgradeConnection{}, fmt.Errorf("resolve configured database host for identity verification: %w", err)
		}
		matched := false
		for _, ip := range ips {
			if ip.String() == liveAddress.String {
				matched = true
				break
			}
		}
		if !matched {
			return upgradeConnection{}, fmt.Errorf("configured database host does not resolve to the active database server")
		}
	}
	return connection, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
