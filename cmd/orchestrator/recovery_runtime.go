package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	_ "github.com/strata-rmm/strata-rmm-orchestrator/internal/postgresdriver"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/backup"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/config"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/recovery"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/repository"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
)

type recoveryRuntime struct {
	engine *backup.Engine
	repo   repository.Repository
	close  func()
}

func buildRecoveryRuntime(ctx context.Context, cfg *config.OrchestratorConfig, targetDSN string) (*recoveryRuntime, error) {
	if cfg.Backup.EnvironmentID == "" {
		return nil, errors.New("STRATA_BACKUP_ENVIRONMENT_ID is required")
	}
	if cfg.Backup.KeyProviderPath == "" {
		return nil, errors.New("STRATA_BACKUP_KEY_PROVIDER_PATH is required")
	}
	repo, err := buildArtifactRepository(ctx, cfg)
	if err != nil {
		return nil, err
	}
	keys, err := recovery.NewFileKeyProvider(cfg.Backup.KeyProviderPath)
	if err != nil {
		return nil, fmt.Errorf("initialize recovery key provider: %w", err)
	}
	if _, err := keys.CurrentKey(ctx); err != nil {
		return nil, fmt.Errorf("recovery key provider has no active key: %w", err)
	}

	controlDSN := cfg.DB.DSN
	if targetDSN != "" {
		controlDSN = targetDSN
	}
	db, err := sql.Open("postgres", controlDSN)
	if err != nil {
		return nil, fmt.Errorf("open recovery control database: %w", err)
	}
	closeFunctions := []func(){func() { _ = db.Close() }}
	fail := func(err error) (*recoveryRuntime, error) {
		for index := len(closeFunctions) - 1; index >= 0; index-- {
			closeFunctions[index]()
		}
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return fail(fmt.Errorf("connect recovery control database: %w", err))
	}
	if targetDSN == "" {
		var gateExists bool
		if err := db.QueryRowContext(ctx, `
			SELECT to_regclass('public.recovery_mutation_gate') IS NOT NULL
		`).Scan(&gateExists); err != nil || !gateExists {
			if err == nil {
				err = errors.New("migration 65 is not applied")
			}
			return fail(fmt.Errorf("recovery mutation gate unavailable: %w", err))
		}
	}

	lock, err := backup.NewPostgresOperationLock(db, postgres.GetRecoveryLockID())
	if err != nil {
		return fail(err)
	}
	var quiescer backup.Quiescer
	if targetDSN == "" {
		quiescer, err = backup.NewPostgresQuiescer(db, uuid.NewString())
	} else {
		quiescer, err = backup.NewOfflineTargetQuiescer(cfg.Backup.EnvironmentID + "-recovery")
	}
	if err != nil {
		return fail(err)
	}

	components := make(map[repository.ComponentType]backup.RecoveryComponent)
	if targetDSN == "" {
		components[repository.ComponentDatabase], err = backup.NewPostgreSQLBackup(cfg.DB.DSN)
	} else {
		if err := ensureDistinctPostgresTargets(ctx, cfg.DB.DSN, targetDSN); err != nil {
			return fail(err)
		}
		components[repository.ComponentDatabase], err = backup.NewPostgreSQLRecovery(cfg.DB.DSN, targetDSN)
	}
	if err != nil {
		return fail(err)
	}

	natsConfig := cfg
	if targetDSN != "" {
		if cfg.Backup.RecoveryNATSURL == "" {
			return fail(errors.New("STRATA_RECOVERY_NATS_URL is required for restore"))
		}
		if cfg.Backup.RecoveryNATSURL == cfg.NATS.URL {
			return fail(errors.New("recovery NATS target must differ from the source"))
		}
		targetConfig := *cfg
		targetConfig.NATS = cfg.NATS
		targetConfig.NATS.URL = cfg.Backup.RecoveryNATSURL
		targetConfig.NATS.Token = cfg.Backup.RecoveryNATSToken
		targetConfig.NATS.TLSCAFile = cfg.Backup.RecoveryNATSTLSCAFile
		targetConfig.NATS.TLSCertFile = cfg.Backup.RecoveryNATSTLSCertFile
		targetConfig.NATS.TLSKeyFile = cfg.Backup.RecoveryNATSTLSKeyFile
		targetConfig.NATS.TLSEnabled = targetConfig.NATS.TLSCAFile != ""
		natsConfig = &targetConfig
	}
	if err := validateRecoveryTransport(cfg.RuntimeMode, natsConfig.NATS, controlDSN); err != nil {
		return fail(err)
	}
	nc, err := connectNATS(natsConfig)
	if err != nil {
		return fail(fmt.Errorf("connect NATS for recovery: %w", err))
	}
	closeFunctions = append(closeFunctions, nc.Close)
	jetstream, err := backup.NewJetStreamRecovery(nc)
	if err != nil {
		return fail(err)
	}
	components[repository.ComponentJetStream] = jetstream

	if cfg.Storage.Backend != "none" && cfg.Storage.Backend != "" {
		var source, target storage.Backend
		if targetDSN != "" {
			if cfg.Backup.RecoveryStorageBackend == "" || cfg.Backup.RecoveryStorageBucket == "" {
				return fail(errors.New("recovery storage backend and distinct target bucket are required"))
			}
			if cfg.Backup.RecoveryStorageBackend == cfg.Storage.Backend &&
				cfg.Backup.RecoveryStorageBucket == cfg.Storage.Bucket &&
				cfg.Backup.RecoveryStorageEndpoint == cfg.Storage.Endpoint {
				return fail(errors.New("recovery object-storage target must differ from the source"))
			}
			target, err = storage.NewBackend(ctx, storage.Config{
				Type: cfg.Backup.RecoveryStorageBackend, Bucket: cfg.Backup.RecoveryStorageBucket,
				Region: cfg.Backup.RecoveryStorageRegion, Endpoint: cfg.Backup.RecoveryStorageEndpoint,
				AccessKey: cfg.Backup.RecoveryStorageAccessKey, SecretKey: cfg.Backup.RecoveryStorageSecretKey,
				UseSSL: cfg.Backup.RecoveryStorageUseSSL,
			})
			if err != nil {
				return fail(fmt.Errorf("initialize recovery object storage: %w", err))
			}
			closeFunctions = append(closeFunctions, func() { _ = target.Close() })
			source = target
		} else {
			source, err = storage.NewBackend(ctx, storage.Config{
				Type: cfg.Storage.Backend, Bucket: cfg.Storage.Bucket, Region: cfg.Storage.Region,
				Endpoint: cfg.Storage.Endpoint, AccessKey: cfg.Storage.AccessKey,
				SecretKey: cfg.Storage.SecretKey, UseSSL: cfg.Storage.UseSSL,
			})
			if err != nil {
				return fail(fmt.Errorf("initialize source object storage: %w", err))
			}
			closeFunctions = append(closeFunctions, func() { _ = source.Close() })
			target = source
		}
		objectRecovery, err := backup.NewObjectStorageRecovery(source, target)
		if err != nil {
			return fail(err)
		}
		components[repository.ComponentObjectStore] = objectRecovery
	}

	engine, err := backup.NewEngine(backup.EngineConfig{
		EnvironmentID: cfg.Backup.EnvironmentID,
		SourceCommit:  cfg.Commit,
		SourceRelease: cfg.Version,
		SchemaVersion: 65,
	}, repo, keys, quiescer, lock, components)
	if err != nil {
		return fail(err)
	}
	return &recoveryRuntime{
		engine: engine,
		repo:   repo,
		close: func() {
			for index := len(closeFunctions) - 1; index >= 0; index-- {
				closeFunctions[index]()
			}
		},
	}, nil
}

func validateRecoveryTransport(mode config.RuntimeMode, natsConfig config.NATSConfig, dsn string) error {
	if mode != config.ModeProduction {
		return nil
	}
	if !natsConfig.TLSEnabled || natsConfig.TLSCAFile == "" {
		return errors.New("production recovery NATS requires TLS and a CA file")
	}
	parsed, err := url.Parse(natsConfig.URL)
	if err != nil || (parsed.Scheme != "tls" && parsed.Scheme != "nats+tls") {
		return errors.New("production recovery NATS URL must use tls:// or nats+tls://")
	}
	if (natsConfig.TLSCertFile == "") != (natsConfig.TLSKeyFile == "") {
		return errors.New("production recovery NATS client certificate and key must be configured together")
	}
	if natsConfig.Token == "" && natsConfig.TLSCertFile == "" {
		return errors.New("production recovery NATS requires token authentication or mutual TLS")
	}
	if strings.Contains(strings.ToLower(dsn), "sslmode=disable") {
		return errors.New("production recovery database must not disable TLS")
	}
	return nil
}

func ensureDistinctPostgresTargets(ctx context.Context, sourceDSN, targetDSN string) error {
	if sourceDSN == targetDSN {
		return errors.New("recovery target DSN equals the source DSN; in-place restore is forbidden")
	}
	target, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return fmt.Errorf("open recovery target database: %w", err)
	}
	defer func() { _ = target.Close() }()
	type identity struct {
		database string
		address  string
		port     int
	}
	readIdentity := func(db *sql.DB) (identity, error) {
		var value identity
		err := db.QueryRowContext(ctx, `
			SELECT current_database(), COALESCE(inet_server_addr()::text, 'local'), inet_server_port()
		`).Scan(&value.database, &value.address, &value.port)
		return value, err
	}
	targetIdentity, err := readIdentity(target)
	if err != nil {
		return fmt.Errorf("identify recovery target database: %w", err)
	}
	var targetTables int
	if err := target.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pg_tables WHERE schemaname = 'public'
	`).Scan(&targetTables); err != nil {
		return fmt.Errorf("inspect recovery target database: %w", err)
	}
	if targetTables != 0 {
		return errors.New("recovery target database must be empty")
	}
	source, err := sql.Open("postgres", sourceDSN)
	if err == nil {
		defer func() { _ = source.Close() }()
		sourceIdentity, sourceErr := readIdentity(source)
		if sourceErr == nil && sourceIdentity == targetIdentity {
			return errors.New("recovery target resolves to the source database; in-place restore is forbidden")
		}
	}
	return nil
}

func buildArtifactRepository(ctx context.Context, cfg *config.OrchestratorConfig) (repository.Repository, error) {
	switch cfg.Backup.RepositoryType {
	case "filesystem":
		if cfg.Backup.BackupDirectory == "" {
			return nil, errors.New("STRATA_BACKUP_DIRECTORY is required for filesystem repository")
		}
		return repository.NewFilesystemRepository(cfg.Backup.BackupDirectory)
	case "s3":
		if cfg.Backup.ExternalBackupBucket == "" || cfg.Backup.ExternalBackupRegion == "" ||
			cfg.Backup.ExternalBackupAccessKey == "" || cfg.Backup.ExternalBackupSecretKey == "" {
			return nil, errors.New("S3 backup repository bucket, region, access key, and secret key are required")
		}
		options := []func(*awsconfig.LoadOptions) error{
			awsconfig.WithRegion(cfg.Backup.ExternalBackupRegion),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				cfg.Backup.ExternalBackupAccessKey,
				cfg.Backup.ExternalBackupSecretKey,
				"",
			)),
		}
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, options...)
		if err != nil {
			return nil, fmt.Errorf("load S3 backup repository configuration: %w", err)
		}
		client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
			if cfg.Backup.ExternalBackupEndpoint != "" {
				options.BaseEndpoint = aws.String(cfg.Backup.ExternalBackupEndpoint)
				options.UsePathStyle = true
			}
		})
		return repository.NewS3Repository(client, cfg.Backup.ExternalBackupBucket)
	default:
		return nil, fmt.Errorf("unsupported backup repository type %q", cfg.Backup.RepositoryType)
	}
}
