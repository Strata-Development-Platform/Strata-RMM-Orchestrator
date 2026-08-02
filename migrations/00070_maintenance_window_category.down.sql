ALTER TABLE policies DROP COLUMN IF EXISTS maintenance_timezone;
ALTER TABLE policies DROP COLUMN IF EXISTS maintenance_days;
ALTER TABLE policies DROP COLUMN IF EXISTS maintenance_end;
ALTER TABLE policies DROP COLUMN IF EXISTS maintenance_start;

ALTER TABLE policy_revisions DROP COLUMN IF EXISTS maintenance_timezone;
ALTER TABLE policy_revisions DROP COLUMN IF EXISTS maintenance_days;
ALTER TABLE policy_revisions DROP COLUMN IF EXISTS maintenance_end;
ALTER TABLE policy_revisions DROP COLUMN IF EXISTS maintenance_start;

DROP VIEW IF EXISTS policy_effective_config;
