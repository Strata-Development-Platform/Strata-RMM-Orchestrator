DROP INDEX IF EXISTS idx_patch_deployment_devices_rollout_dispatch;

ALTER TABLE patch_deployment_devices
    DROP CONSTRAINT IF EXISTS patch_deployment_devices_dispatch_attempts_check,
    DROP CONSTRAINT IF EXISTS patch_deployment_devices_rollout_group_check,
    DROP COLUMN IF EXISTS dispatch_attempts,
    DROP COLUMN IF EXISTS dispatched_at,
    DROP COLUMN IF EXISTS rollout_group;
