BEGIN;

DROP TRIGGER IF EXISTS trg_refresh_job_terminal_aggregate_from_target ON job_targets;
DROP FUNCTION IF EXISTS refresh_job_terminal_aggregate_from_target();

DROP TRIGGER IF EXISTS trg_enforce_job_terminal_aggregate ON jobs;
DROP FUNCTION IF EXISTS enforce_job_terminal_aggregate();

COMMIT;
