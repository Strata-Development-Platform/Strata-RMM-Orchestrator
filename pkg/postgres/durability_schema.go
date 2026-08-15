package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

type durabilityMigration struct {
	ID   int
	Name string
	Up   string
}

// ApplyDurabilitySchema applies the relational extensions required by the
// durable patch/software job bridge. The repository also retains standalone
// SQL copies under migrations/ for packaging/audit purposes, but the runtime's
// authoritative historical schema registry is the hard-coded Migrations()
// list. These extensions intentionally use a separate ledger so they can be
// applied after that historical registry without making legacy schema-version
// validation believe the core migration list has advanced past its known max.
//
// The transaction takes the same advisory lock as SchemaManager.Apply, so a
// second process cannot race the core schema or another durability extension
// during startup.
func ApplyDurabilitySchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("durability schema database is required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin durability schema transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, GetLockID()); err != nil {
		return fmt.Errorf("acquire durability schema lock: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS durability_schema_migrations (
			id         INT PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create durability schema ledger: %w", err)
	}

	for _, migration := range durabilityMigrations() {
		var exists bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM durability_schema_migrations WHERE id = $1)`, migration.ID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check durability migration %d: %w", migration.ID, err)
		}
		if exists {
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.Up); err != nil {
			return fmt.Errorf("apply durability migration %d (%s): %w", migration.ID, migration.Name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO durability_schema_migrations (id, name) VALUES ($1, $2)`, migration.ID, migration.Name,
		); err != nil {
			return fmt.Errorf("record durability migration %d: %w", migration.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit durability schema: %w", err)
	}
	return nil
}

func durabilityMigrations() []durabilityMigration {
	return []durabilityMigration{
		{
			ID:   1,
			Name: "patch_rollout_durability",
			Up: `
ALTER TABLE patch_deployment_devices
    ADD COLUMN IF NOT EXISTS rollout_group TEXT NOT NULL DEFAULT 'broad',
    ADD COLUMN IF NOT EXISTS dispatched_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS dispatch_attempts INTEGER NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'patch_deployment_devices_rollout_group_check'
          AND conrelid = 'patch_deployment_devices'::regclass
    ) THEN
        ALTER TABLE patch_deployment_devices
            ADD CONSTRAINT patch_deployment_devices_rollout_group_check
            CHECK (rollout_group IN ('canary', 'broad'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'patch_deployment_devices_dispatch_attempts_check'
          AND conrelid = 'patch_deployment_devices'::regclass
    ) THEN
        ALTER TABLE patch_deployment_devices
            ADD CONSTRAINT patch_deployment_devices_dispatch_attempts_check
            CHECK (dispatch_attempts >= 0);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_patch_deployment_devices_rollout_dispatch
    ON patch_deployment_devices (deployment_id, rollout_group, dispatched_at);
`,
		},
		{
			ID:   2,
			Name: "patch_jobs_bridge",
			Up: `
CREATE TABLE IF NOT EXISTS patch_deployment_patches (
    deployment_id uuid NOT NULL REFERENCES patch_deployments(id) ON DELETE CASCADE,
    patch_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    PRIMARY KEY (deployment_id, patch_id),
    CONSTRAINT patch_deployment_patches_patch_id_nonempty CHECK (length(btrim(patch_id)) > 0)
);

ALTER TABLE patch_deployment_devices
    ADD COLUMN IF NOT EXISTS job_id uuid REFERENCES jobs(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS job_target_id uuid REFERENCES job_targets(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_patch_deployment_devices_job_target
    ON patch_deployment_devices(job_target_id)
    WHERE job_target_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_patch_deployment_patches_deployment
    ON patch_deployment_patches(deployment_id);

DO $$
BEGIN
    IF to_regprocedure('enforce_recovery_mutation_gate()') IS NOT NULL THEN
        DROP TRIGGER IF EXISTS recovery_mutation_gate_trigger ON patch_deployment_patches;
        CREATE TRIGGER recovery_mutation_gate_trigger
            BEFORE INSERT OR UPDATE OR DELETE ON patch_deployment_patches
            FOR EACH STATEMENT EXECUTE FUNCTION enforce_recovery_mutation_gate();
    END IF;
END $$;
`,
		},
		{
			ID:   3,
			Name: "software_jobs_bridge",
			Up: `
ALTER TABLE software_deployment_targets
    ADD COLUMN IF NOT EXISTS job_id uuid REFERENCES jobs(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS job_target_id uuid REFERENCES job_targets(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_software_deployment_targets_job_target
    ON software_deployment_targets(job_target_id)
    WHERE job_target_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_software_deployment_targets_job
    ON software_deployment_targets(job_id)
    WHERE job_id IS NOT NULL;

CREATE OR REPLACE FUNCTION sync_software_deployment_target_from_job_target()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    deployment_uuid uuid;
BEGIN
    IF NEW.status IS NOT DISTINCT FROM OLD.status THEN
        RETURN NEW;
    END IF;
    IF NEW.status NOT IN ('succeeded', 'failed', 'expired', 'cancelled') THEN
        RETURN NEW;
    END IF;

    UPDATE software_deployment_targets
    SET status = CASE WHEN NEW.status = 'succeeded' THEN 'success' ELSE 'failed' END,
        error_message = CASE
            WHEN NEW.status = 'succeeded' THEN ''
            ELSE left(COALESCE(NULLIF(NEW.error_message, ''), 'durable job ' || NEW.status), 4096)
        END,
        started_at = COALESCE(started_at, NEW.started_at),
        completed_at = COALESCE(NEW.completed_at, NOW())
    WHERE job_target_id = NEW.id
      AND status IN ('pending', 'deploying')
    RETURNING deployment_id INTO deployment_uuid;

    IF deployment_uuid IS NULL THEN
        RETURN NEW;
    END IF;

    UPDATE software_deployments AS d
    SET status = CASE
            WHEN EXISTS (
                SELECT 1 FROM software_deployment_targets t
                WHERE t.deployment_id = d.id AND t.status = 'failed'
            ) THEN 'failed'
            ELSE 'completed'
        END,
        completed_at = NOW()
    WHERE d.id = deployment_uuid
      AND NOT EXISTS (
          SELECT 1 FROM software_deployment_targets t
          WHERE t.deployment_id = d.id AND t.status IN ('pending', 'deploying')
      );

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_sync_software_deployment_target_from_job_target ON job_targets;
CREATE TRIGGER trg_sync_software_deployment_target_from_job_target
AFTER UPDATE OF status ON job_targets
FOR EACH ROW EXECUTE FUNCTION sync_software_deployment_target_from_job_target();
`,
		},
		{
			ID:   4,
			Name: "job_terminal_aggregation",
			Up: `
CREATE OR REPLACE FUNCTION enforce_job_terminal_aggregate()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    nonterminal_count integer;
    succeeded_count integer;
    failed_count integer;
    expired_count integer;
    cancelled_count integer;
BEGIN
    IF NEW.status NOT IN ('succeeded', 'failed', 'cancelled') THEN
        RETURN NEW;
    END IF;

    SELECT
        count(*) FILTER (WHERE status IN ('pending', 'queued', 'dispatched', 'running', 'waiting')),
        count(*) FILTER (WHERE status = 'succeeded'),
        count(*) FILTER (WHERE status = 'failed'),
        count(*) FILTER (WHERE status = 'expired'),
        count(*) FILTER (WHERE status = 'cancelled')
    INTO nonterminal_count, succeeded_count, failed_count, expired_count, cancelled_count
    FROM job_targets WHERE job_id = NEW.id;

    IF nonterminal_count <> 0 THEN
        RETURN NEW;
    END IF;
    NEW.completed_count := succeeded_count;
    NEW.failed_count := failed_count + expired_count;
    NEW.completed_at := COALESCE(NEW.completed_at, NOW());
    IF failed_count > 0 OR expired_count > 0 THEN
        NEW.status := 'failed';
    ELSIF cancelled_count > 0 THEN
        NEW.status := 'cancelled';
    ELSE
        NEW.status := 'succeeded';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_enforce_job_terminal_aggregate ON jobs;
CREATE TRIGGER trg_enforce_job_terminal_aggregate
BEFORE UPDATE OF status ON jobs
FOR EACH ROW EXECUTE FUNCTION enforce_job_terminal_aggregate();

CREATE OR REPLACE FUNCTION refresh_job_terminal_aggregate_from_target()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.status IS NOT DISTINCT FROM OLD.status THEN
        RETURN NEW;
    END IF;
    IF NEW.status NOT IN ('succeeded', 'failed', 'expired', 'cancelled') THEN
        RETURN NEW;
    END IF;

    UPDATE jobs AS j
    SET completed_count = counts.succeeded_count,
        failed_count = counts.failed_count + counts.expired_count,
        status = CASE
            WHEN counts.nonterminal_count > 0 THEN j.status
            WHEN counts.failed_count > 0 OR counts.expired_count > 0 THEN 'failed'
            WHEN counts.cancelled_count > 0 THEN 'cancelled'
            ELSE 'succeeded'
        END,
        completed_at = CASE
            WHEN counts.nonterminal_count = 0 THEN COALESCE(j.completed_at, NOW())
            ELSE j.completed_at
        END,
        updated_at = NOW()
    FROM (
        SELECT
            count(*) FILTER (WHERE status IN ('pending', 'queued', 'dispatched', 'running', 'waiting')) AS nonterminal_count,
            count(*) FILTER (WHERE status = 'succeeded') AS succeeded_count,
            count(*) FILTER (WHERE status = 'failed') AS failed_count,
            count(*) FILTER (WHERE status = 'expired') AS expired_count,
            count(*) FILTER (WHERE status = 'cancelled') AS cancelled_count
        FROM job_targets WHERE job_id = NEW.job_id
    ) AS counts
    WHERE j.id = NEW.job_id;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_refresh_job_terminal_aggregate_from_target ON job_targets;
CREATE TRIGGER trg_refresh_job_terminal_aggregate_from_target
AFTER UPDATE OF status ON job_targets
FOR EACH ROW EXECUTE FUNCTION refresh_job_terminal_aggregate_from_target();
`,
		},
		{
			ID:   5,
			Name: "patch_deployment_status_lifecycle",
			Up: `
ALTER TABLE patch_deployments
    DROP CONSTRAINT IF EXISTS patch_deployments_status_check;
ALTER TABLE patch_deployments
    ADD CONSTRAINT patch_deployments_status_check
    CHECK (status IN (
        'pending', 'approved', 'canary', 'deploying', 'installed',
        'completed', 'failed', 'reboot_required', 'cancelled'
    ));
`,
		},
	}
}
