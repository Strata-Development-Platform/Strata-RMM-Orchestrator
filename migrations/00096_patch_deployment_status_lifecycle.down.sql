DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM patch_deployments
        WHERE status NOT IN ('pending', 'approved', 'deploying', 'installed', 'failed')
    ) THEN
        RAISE EXCEPTION 'cannot restore legacy patch_deployments status constraint while extended lifecycle states exist';
    END IF;
END $$;

ALTER TABLE patch_deployments
    DROP CONSTRAINT IF EXISTS patch_deployments_status_check;

ALTER TABLE patch_deployments
    ADD CONSTRAINT patch_deployments_status_check
    CHECK (status IN ('pending', 'approved', 'deploying', 'installed', 'failed'));
