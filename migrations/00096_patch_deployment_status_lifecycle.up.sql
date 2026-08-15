ALTER TABLE patch_deployments
    DROP CONSTRAINT IF EXISTS patch_deployments_status_check;

ALTER TABLE patch_deployments
    ADD CONSTRAINT patch_deployments_status_check
    CHECK (status IN (
        'pending', 'approved', 'canary', 'deploying', 'installed',
        'completed', 'failed', 'reboot_required', 'cancelled'
    ));
