BEGIN;

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

COMMIT;
