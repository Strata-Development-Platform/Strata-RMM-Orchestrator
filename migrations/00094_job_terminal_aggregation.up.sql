BEGIN;

CREATE OR REPLACE FUNCTION enforce_job_terminal_aggregate()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    nonterminal_count integer;
    succeeded_count integer;
    failed_count integer;
    expired_count integer;
    cancelled_count integer;
BEGIN
    IF NEW.status NOT IN ('succeeded', 'failed', 'cancelled') THEN
        RETURN NEW;
    END IF;

    SELECT
        count(*) FILTER (WHERE status IN ('pending', 'queued', 'dispatched', 'running', 'waiting')),
        count(*) FILTER (WHERE status = 'succeeded'),
        count(*) FILTER (WHERE status = 'failed'),
        count(*) FILTER (WHERE status = 'expired'),
        count(*) FILTER (WHERE status = 'cancelled')
    INTO nonterminal_count, succeeded_count, failed_count, expired_count, cancelled_count
    FROM job_targets
    WHERE job_id = NEW.id;

    IF nonterminal_count <> 0 THEN
        RETURN NEW;
    END IF;

    NEW.completed_count := succeeded_count;
    NEW.failed_count := failed_count + expired_count;
    NEW.completed_at := COALESCE(NEW.completed_at, NOW());

    IF failed_count > 0 OR expired_count > 0 THEN
        NEW.status := 'failed';
    ELSIF cancelled_count > 0 THEN
        NEW.status := 'cancelled';
    ELSE
        NEW.status := 'succeeded';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_enforce_job_terminal_aggregate ON jobs;
CREATE TRIGGER trg_enforce_job_terminal_aggregate
BEFORE UPDATE OF status ON jobs
FOR EACH ROW
EXECUTE FUNCTION enforce_job_terminal_aggregate();

CREATE OR REPLACE FUNCTION refresh_job_terminal_aggregate_from_target()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.status IS NOT DISTINCT FROM OLD.status THEN
        RETURN NEW;
    END IF;

    IF NEW.status NOT IN ('succeeded', 'failed', 'expired', 'cancelled') THEN
        RETURN NEW;
    END IF;

    UPDATE jobs AS j
    SET completed_count = counts.succeeded_count,
        failed_count = counts.failed_count + counts.expired_count,
        status = CASE
            WHEN counts.nonterminal_count > 0 THEN j.status
            WHEN counts.failed_count > 0 OR counts.expired_count > 0 THEN 'failed'
            WHEN counts.cancelled_count > 0 THEN 'cancelled'
            ELSE 'succeeded'
        END,
        completed_at = CASE
            WHEN counts.nonterminal_count = 0 THEN COALESCE(j.completed_at, NOW())
            ELSE j.completed_at
        END,
        updated_at = NOW()
    FROM (
        SELECT
            count(*) FILTER (WHERE status IN ('pending', 'queued', 'dispatched', 'running', 'waiting')) AS nonterminal_count,
            count(*) FILTER (WHERE status = 'succeeded') AS succeeded_count,
            count(*) FILTER (WHERE status = 'failed') AS failed_count,
            count(*) FILTER (WHERE status = 'expired') AS expired_count,
            count(*) FILTER (WHERE status = 'cancelled') AS cancelled_count
        FROM job_targets
        WHERE job_id = NEW.job_id
    ) AS counts
    WHERE j.id = NEW.job_id;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_refresh_job_terminal_aggregate_from_target ON job_targets;
CREATE TRIGGER trg_refresh_job_terminal_aggregate_from_target
AFTER UPDATE OF status ON job_targets
FOR EACH ROW
EXECUTE FUNCTION refresh_job_terminal_aggregate_from_target();

COMMIT;
