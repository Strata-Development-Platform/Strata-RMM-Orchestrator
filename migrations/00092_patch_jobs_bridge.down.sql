BEGIN;

DROP INDEX IF EXISTS idx_patch_deployment_devices_job_target;
ALTER TABLE patch_deployment_devices
    DROP COLUMN IF EXISTS job_target_id,
    DROP COLUMN IF EXISTS job_id;
DROP TABLE IF EXISTS patch_deployment_patches;

COMMIT;
