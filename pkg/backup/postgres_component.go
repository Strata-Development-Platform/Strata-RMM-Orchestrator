package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

	_ "github.com/strata-rmm/strata-rmm-orchestrator/internal/postgresdriver"
)

// PostgreSQLRecovery performs a logical backup from one database and restores
// it into a distinct target database. Credentials are passed through the child
// process environment so they cannot appear in process arguments or errors.
type PostgreSQLRecovery struct {
	sourceDSN string
	targetDSN string
}

// NewPostgreSQLBackup creates a source-only component suitable for backup and
// verification. Restore will fail closed until an explicit target is supplied.
func NewPostgreSQLBackup(sourceDSN string) (*PostgreSQLRecovery, error) {
	if strings.TrimSpace(sourceDSN) == "" {
		return nil, errors.New("source PostgreSQL DSN is required")
	}
	return &PostgreSQLRecovery{sourceDSN: sourceDSN}, nil
}

// NewPostgreSQLRecovery validates that source and target are explicitly
// configured and different. A restore is never allowed in place.
func NewPostgreSQLRecovery(sourceDSN, targetDSN string) (*PostgreSQLRecovery, error) {
	if strings.TrimSpace(sourceDSN) == "" {
		return nil, errors.New("source PostgreSQL DSN is required")
	}
	if strings.TrimSpace(targetDSN) == "" {
		return nil, errors.New("target PostgreSQL DSN is required")
	}
	if canonicalDSN(sourceDSN) == canonicalDSN(targetDSN) {
		return nil, errors.New("source and target PostgreSQL DSNs must differ")
	}
	return &PostgreSQLRecovery{sourceDSN: sourceDSN, targetDSN: targetDSN}, nil
}

// Backup streams a pg_dump custom-format artifact to w.
func (r *PostgreSQLRecovery) Backup(ctx context.Context, w io.Writer) (ComponentManifest, error) {
	if w == nil {
		return ComponentManifest{}, errors.New("backup writer is required")
	}
	hasher := sha256.New()
	counting := &countingWriter{writer: io.MultiWriter(w, hasher)}
	stderr := &boundedBuffer{limit: 4096}
	cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--no-owner", "--no-acl")
	commandEnv, err := postgresCommandEnv(r.sourceDSN)
	if err != nil {
		return ComponentManifest{}, fmt.Errorf("prepare pg_dump connection: %w", err)
	}
	cmd.Env = commandEnv
	cmd.Stdout = counting
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return ComponentManifest{}, sanitizedCommandError("pg_dump", err, stderr.String(), r.sourceDSN)
	}
	return ComponentManifest{
		Type:   string(repositoryComponentDatabase),
		Count:  1,
		Size:   counting.count,
		Digest: hex.EncodeToString(hasher.Sum(nil)),
		Metadata: map[string]string{
			"format": "postgresql-custom",
		},
	}, nil
}

// Restore streams a custom-format artifact into the configured clean target.
func (r *PostgreSQLRecovery) Restore(ctx context.Context, source io.Reader) error {
	if source == nil {
		return errors.New("restore reader is required")
	}
	if strings.TrimSpace(r.targetDSN) == "" {
		return ErrTargetDSNReq
	}
	stderr := &boundedBuffer{limit: 4096}
	cmd := exec.CommandContext(ctx, "pg_restore", "--no-owner", "--no-acl", "--exit-on-error")
	commandEnv, err := postgresCommandEnv(r.targetDSN)
	if err != nil {
		return fmt.Errorf("prepare pg_restore connection: %w", err)
	}
	cmd.Env = commandEnv
	cmd.Args = append(cmd.Args, "--dbname="+environmentValue(commandEnv, "PGDATABASE"))
	cmd.Stdin = source
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return sanitizedCommandError("pg_restore", err, stderr.String(), r.targetDSN)
	}
	db, err := sql.Open("postgres", r.targetDSN)
	if err != nil {
		return fmt.Errorf("open restored PostgreSQL target for gate reset: %w", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, `
		UPDATE recovery_mutation_gate
		SET quiesced = FALSE, operation_id = NULL, updated_at = NOW()
		WHERE singleton = TRUE
	`); err != nil {
		return fmt.Errorf("reset restored recovery mutation gate: %w", err)
	}
	return nil
}

func environmentValue(environment []string, name string) string {
	prefix := name + "="
	for index := len(environment) - 1; index >= 0; index-- {
		if strings.HasPrefix(environment[index], prefix) {
			return strings.TrimPrefix(environment[index], prefix)
		}
	}
	return ""
}

// Verify confirms that the target accepts queries after restore.
func (r *PostgreSQLRecovery) Verify(ctx context.Context, manifest ComponentManifest) error {
	if manifest.Type != string(repositoryComponentDatabase) {
		return fmt.Errorf("unexpected component type %q", manifest.Type)
	}
	if strings.TrimSpace(r.targetDSN) == "" {
		return ErrTargetDSNReq
	}
	db, err := sql.Open("postgres", r.targetDSN)
	if err != nil {
		return fmt.Errorf("open restored PostgreSQL target: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("query restored PostgreSQL target: %w", err)
	}
	var quiesced bool
	if err := db.QueryRowContext(ctx, `
		SELECT quiesced FROM recovery_mutation_gate WHERE singleton = TRUE
	`).Scan(&quiesced); err != nil {
		return fmt.Errorf("verify restored recovery mutation gate: %w", err)
	}
	if quiesced {
		return errors.New("restored recovery mutation gate remains closed")
	}
	return nil
}

func postgresCommandEnv(dsn string) ([]string, error) {
	parsed, err := url.Parse(dsn)
	values := map[string]string{}
	queryNames := map[string]string{
		"sslmode": "PGSSLMODE", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY",
		"sslrootcert": "PGSSLROOTCERT", "connect_timeout": "PGCONNECT_TIMEOUT",
		"application_name": "PGAPPNAME",
	}
	if err == nil && (parsed.Scheme == "postgres" || parsed.Scheme == "postgresql") {
		password, _ := parsed.User.Password()
		values["PGHOST"] = parsed.Hostname()
		values["PGPORT"] = parsed.Port()
		values["PGUSER"] = parsed.User.Username()
		values["PGPASSWORD"] = password
		values["PGDATABASE"] = strings.TrimPrefix(parsed.Path, "/")
		for queryName, environmentName := range queryNames {
			values[environmentName] = parsed.Query().Get(queryName)
		}
	} else {
		keywords, parseErr := parseKeywordDSN(dsn)
		if parseErr != nil {
			return nil, errors.New("invalid PostgreSQL keyword DSN")
		}
		names := map[string]string{
			"host": "PGHOST", "port": "PGPORT", "user": "PGUSER", "password": "PGPASSWORD",
			"dbname": "PGDATABASE", "sslmode": "PGSSLMODE", "sslcert": "PGSSLCERT",
			"sslkey": "PGSSLKEY", "sslrootcert": "PGSSLROOTCERT",
			"connect_timeout": "PGCONNECT_TIMEOUT", "application_name": "PGAPPNAME",
		}
		for keyword, environmentName := range names {
			values[environmentName] = keywords[keyword]
		}
	}
	if values["PGHOST"] == "" || values["PGDATABASE"] == "" {
		return nil, errors.New("PostgreSQL backup DSN requires host and database")
	}
	environment := os.Environ()
	for key, value := range values {
		if value != "" {
			environment = append(environment, key+"="+value)
		}
	}
	return environment, nil
}

func parseKeywordDSN(dsn string) (map[string]string, error) {
	result := map[string]string{}
	for position := 0; position < len(dsn); {
		for position < len(dsn) && (dsn[position] == ' ' || dsn[position] == '\t' || dsn[position] == '\n') {
			position++
		}
		if position == len(dsn) {
			break
		}
		keyStart := position
		for position < len(dsn) && dsn[position] != '=' && dsn[position] != ' ' && dsn[position] != '\t' {
			position++
		}
		if position == keyStart || position == len(dsn) || dsn[position] != '=' {
			return nil, errors.New("invalid keyword")
		}
		key := strings.ToLower(dsn[keyStart:position])
		position++
		var value strings.Builder
		quoted := position < len(dsn) && dsn[position] == '\''
		if quoted {
			position++
		}
		for position < len(dsn) {
			if quoted && dsn[position] == '\'' {
				position++
				break
			}
			if !quoted && (dsn[position] == ' ' || dsn[position] == '\t' || dsn[position] == '\n') {
				break
			}
			if dsn[position] == '\\' {
				position++
				if position == len(dsn) {
					return nil, errors.New("trailing escape")
				}
			}
			value.WriteByte(dsn[position])
			position++
		}
		if quoted && (position == 0 || dsn[position-1] != '\'') {
			return nil, errors.New("unterminated quote")
		}
		result[key] = value.String()
	}
	return result, nil
}

type boundedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.buf.Write(p)
	}
	return original, nil
}

func (b *boundedBuffer) String() string {
	return b.buf.String()
}

func sanitizedCommandError(command string, commandErr error, stderr string, secrets ...string) error {
	message := stderr
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	message = redactConnectionSecrets(message)
	if message == "" {
		return fmt.Errorf("%s failed: %w", command, commandErr)
	}
	return fmt.Errorf("%s failed: %w: %s", command, commandErr, strings.TrimSpace(message))
}

func redactConnectionSecrets(value string) string {
	fields := strings.Fields(value)
	for index, field := range fields {
		lower := strings.ToLower(field)
		if strings.HasPrefix(lower, "password=") {
			fields[index] = "password=[REDACTED]"
		}
		if strings.Contains(field, "://") && strings.Contains(field, "@") {
			schemeEnd := strings.Index(field, "://") + 3
			at := strings.LastIndex(field, "@")
			userInfo := field[schemeEnd:at]
			if colon := strings.Index(userInfo, ":"); colon >= 0 {
				fields[index] = field[:schemeEnd] + userInfo[:colon] + ":[REDACTED]" + field[at:]
			}
		}
	}
	return strings.Join(fields, " ")
}

func canonicalDSN(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

const repositoryComponentDatabase componentType = "postgresql"
