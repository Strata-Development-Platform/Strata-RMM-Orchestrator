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
	if cfg.Backup.S3Bucket == "" {
		return nil, errors.New("STRATA_BACKUP_S3_BUCKET is required")
	}
	if cfg.Backup.S3Region == "" {
		return nil, errors.New("STRATA_BACKUP_S3_REGION is required")
	}

	db, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, fmt.Errorf("open recovery database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping recovery database: %w", err)
	}

	pgConfig, err := parseRecoveryPostgresConfig(targetDSN)
	if err != nil {
		db.Close()
		return nil, err
	}

	awsOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Backup.S3Region),
	}
	if cfg.Backup.S3AccessKeyID != "" || cfg.Backup.S3SecretAccessKey != "" {
		if cfg.Backup.S3AccessKeyID == "" || cfg.Backup.S3SecretAccessKey == "" {
			db.Close()
			return nil, errors.New("both STRATA_BACKUP_S3_ACCESS_KEY_ID and STRATA_BACKUP_S3_SECRET_ACCESS_KEY are required when using static credentials")
		}
		awsOptions = append(awsOptions, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.Backup.S3AccessKeyID,
			cfg.Backup.S3SecretAccessKey,
			cfg.Backup.S3SessionToken,
		)))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsOptions...)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("load backup s3 configuration: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.Backup.S3Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.Backup.S3Endpoint)
			options.UsePathStyle = cfg.Backup.S3ForcePathStyle
		}
	})

	repo := repository.NewPostgresRepository(db)
	objectStore := storage.NewS3Store(s3Client, cfg.Backup.S3Bucket)
	component := backup.NewPostgresComponentWithToolRunner(
		db,
		pgConfig,
		backup.CommandPostgresToolRunner{},
		cfg.Backup.MaxBackupBytes,
		cfg.Backup.MaxRestoreBytes,
	)
	engine := backup.NewEngine(repo, objectStore, component, cfg.Backup.EnvironmentID)

	return &recoveryRuntime{
		engine: engine,
		repo:   repo,
		close: func() {
			db.Close()
		},
	}, nil
}

func parseRecoveryPostgresConfig(dsn string) (backup.PostgresConfig, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return backup.PostgresConfig{}, fmt.Errorf("parse postgres dsn: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return backup.PostgresConfig{}, errors.New("recovery database URL must use postgres or postgresql scheme")
	}

	config := backup.PostgresConfig{
		Host:     u.Hostname(),
		Port:     u.Port(),
		Database: strings.TrimPrefix(u.Path, "/"),
		SSLMode:  u.Query().Get("sslmode"),
	}
	if u.User != nil {
		config.User = u.User.Username()
		if password, ok := u.User.Password(); ok {
			config.Password = password
		}
	}
	if config.Host == "" || config.Database == "" || config.User == "" {
		return backup.PostgresConfig{}, errors.New("recovery database URL must include host, user, and database")
	}
	return config, nil
}

func (r *recoveryRuntime) verifyTarget(ctx context.Context, target recovery.Target) error {
	if target.EnvironmentID != "" && target.EnvironmentID != r.engine.EnvironmentID() {
		return fmt.Errorf("target environment %q does not match configured environment %q", target.EnvironmentID, r.engine.EnvironmentID())
	}
	if target.BackupID == "" {
		return errors.New("backup_id is required")
	}
	if _, err := uuid.Parse(target.BackupID); err != nil {
		return fmt.Errorf("invalid backup_id: %w", err)
	}
	return nil
}

func (r *recoveryRuntime) lookupBackup(ctx context.Context, backupID string) (*repository.Backup, error) {
	id, err := uuid.Parse(backupID)
	if err != nil {
		return nil, err
	}
	return r.repo.GetBackup(ctx, id)
}
