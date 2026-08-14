BEGIN;

DROP TRIGGER IF EXISTS trg_sync_software_deployment_target_from_job_target ON job_targets;
DROP FUNCTION IF EXISTS sync_software_deployment_target_from_job_target();

DROP INDEX IF EXISTS idx_software_deployment_targets_job_target;
DROP INDEX IF EXISTS idx_software_deployment_targets_job;

ALTER TABLE software_deployment_targets
    DROP COLUMN IF EXISTS job_target_id,
    DROP COLUMN IF EXISTS job_id;

COMMIT;
