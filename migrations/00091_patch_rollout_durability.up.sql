ALTER TABLE patch_deployment_devices
    ADD COLUMN IF NOT EXISTS rollout_group TEXT NOT NULL DEFAULT 'broad',
    ADD COLUMN IF NOT EXISTS dispatched_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS dispatch_attempts INTEGER NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'patch_deployment_devices_rollout_group_check'
          AND conrelid = 'patch_deployment_devices'::regclass
    ) THEN
        ALTER TABLE patch_deployment_devices
            ADD CONSTRAINT patch_deployment_devices_rollout_group_check
            CHECK (rollout_group IN ('canary', 'broad'));
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
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
