BEGIN;

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
FOR EACH ROW
EXECUTE FUNCTION sync_software_deployment_target_from_job_target();

COMMIT;
