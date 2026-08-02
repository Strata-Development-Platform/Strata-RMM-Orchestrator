package timescale

import "fmt"

type Migration struct {
	Version int
	Name    string
	SQL     string
}

func (m Migration) String() string {
	return fmt.Sprintf("v%d: %s", m.Version, m.Name)
}

func AllMigrations() []Migration {
	return []Migration{
		{
			Version: 1,
			Name:    "initial_metrics_schema",
			SQL:     Migration001,
		},
		{
			Version: 2,
			Name:    "alerts_and_probes_schema",
			SQL:     Migration002,
		},
		{
			Version: 3,
			Name:    "backup_and_recovery_schema",
			SQL:     Migration003,
		},
		{
			Version: 4,
			Name:    "dynamic_retention_policies",
			SQL:     Migration004,
		},
	}
}

const Migration001 = `
-- TimescaleDB Metrics Schema
-- Requires: TimescaleDB 2.15+ extension

CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

-- Metrics hypertable
CREATE TABLE IF NOT EXISTS metrics (
    time        TIMESTAMPTZ       NOT NULL,
    tenant_id   UUID              NOT NULL,
    device_id   UUID              NOT NULL,
    metric_name TEXT              NOT NULL,
    value       DOUBLE PRECISION  NOT NULL,
    tags        JSONB             DEFAULT '{}'
);

-- Convert to hypertable (idempotent)
SELECT create_hypertable('metrics', 'time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

-- Compression policy
ALTER TABLE metrics SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'tenant_id, device_id, metric_name',
    timescaledb.compress_orderby = 'time DESC'
);
SELECT add_compression_policy('metrics', INTERVAL '7 days',
    if_not_exists => TRUE
);

-- Retention policy (optional, configurable per tenant)
SELECT add_retention_policy('metrics', INTERVAL '365 days',
    if_not_exists => TRUE
);

-- Continuous aggregates for downsampling
CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_1m
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', time) AS bucket,
    tenant_id,
    device_id,
    metric_name,
    AVG(value) AS avg,
    MIN(value) AS min,
    MAX(value) AS max,
    LAST(value, time) AS last,
    COUNT(*) AS sample_count
FROM metrics
GROUP BY bucket, tenant_id, device_id, metric_name
WITH NO DATA;

SELECT add_continuous_aggregate_policy('metrics_1m',
    start_offset => INTERVAL '3 days',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 minute',
    if_not_exists => TRUE
);

CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_1h
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', time) AS bucket,
    tenant_id,
    device_id,
    metric_name,
    AVG(value) AS avg,
    MIN(value) AS min,
    MAX(value) AS max,
    LAST(value, time) AS last,
    COUNT(*) AS sample_count
FROM metrics
GROUP BY bucket, tenant_id, device_id, metric_name
WITH NO DATA;

SELECT add_continuous_aggregate_policy('metrics_1h',
    start_offset => INTERVAL '7 days',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

-- Indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_metrics_tenant_time
    ON metrics (tenant_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_device_time
    ON metrics (tenant_id, device_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_metric_name
    ON metrics (tenant_id, device_id, metric_name, time DESC);

-- Events table (non-time-series, but time-stamped)
CREATE TABLE IF NOT EXISTS events (
    id          BIGSERIAL,
    time        TIMESTAMPTZ       NOT NULL DEFAULT NOW(),
    tenant_id   UUID              NOT NULL,
    device_id   UUID              NOT NULL,
    event_type  TEXT              NOT NULL,
    message     TEXT              NOT NULL DEFAULT '',
    tags        JSONB             DEFAULT '{}'
);

SELECT create_hypertable('events', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_events_tenant_type
    ON events (tenant_id, event_type, time DESC);

-- Heartbeat tracking
CREATE TABLE IF NOT EXISTS heartbeats (
    time        TIMESTAMPTZ       NOT NULL DEFAULT NOW(),
    tenant_id   UUID              NOT NULL,
    device_id   UUID              NOT NULL,
    status      TEXT              NOT NULL DEFAULT 'ok',
    agent_version TEXT            NOT NULL DEFAULT '',
    metadata    JSONB             DEFAULT '{}'
);

SELECT create_hypertable('heartbeats', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

-- Deduplication table for agent commands
CREATE TABLE IF NOT EXISTS command_queue (
    id          BIGSERIAL PRIMARY KEY,
    created_at  TIMESTAMPTZ       NOT NULL DEFAULT NOW(),
    tenant_id   UUID              NOT NULL,
    device_id   UUID              NOT NULL,
    command     TEXT              NOT NULL,
    payload     JSONB             DEFAULT '{}',
    status      TEXT              NOT NULL DEFAULT 'pending',
    result      JSONB,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_cmd_queue_status
    ON command_queue (tenant_id, device_id, status);
`

const Migration002 = `
-- Phase 2: Alerts + Probes Timescale Schema

-- Alert history hypertable (for time-series analysis of alert frequency)
CREATE TABLE IF NOT EXISTS alerts_ts (
    time        TIMESTAMPTZ       NOT NULL,
    tenant_id   UUID              NOT NULL,
    device_id   UUID              NOT NULL,
    rule_id     TEXT              NOT NULL,
    severity    TEXT              NOT NULL,
    status      TEXT              NOT NULL
);

SELECT create_hypertable('alerts_ts', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

ALTER TABLE alerts_ts SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'tenant_id, rule_id',
    timescaledb.compress_orderby = 'time DESC'
);
SELECT add_compression_policy('alerts_ts', INTERVAL '30 days',
    if_not_exists => TRUE
);
SELECT add_retention_policy('alerts_ts', INTERVAL '365 days',
    if_not_exists => TRUE
);

-- SNMP poll results hypertable
CREATE TABLE IF NOT EXISTS snmp_polls (
    time        TIMESTAMPTZ       NOT NULL,
    tenant_id   UUID              NOT NULL,
    probe_id    TEXT              NOT NULL,
    target_ip   TEXT              NOT NULL,
    oid         TEXT              NOT NULL,
    value       TEXT              NOT NULL,
    type        TEXT              NOT NULL DEFAULT 'string',
    uptime      BIGINT            DEFAULT 0
);

SELECT create_hypertable('snmp_polls', 'time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

ALTER TABLE snmp_polls SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'tenant_id, target_ip',
    timescaledb.compress_orderby = 'time DESC'
);
SELECT add_compression_policy('snmp_polls', INTERVAL '7 days',
    if_not_exists => TRUE
);
SELECT add_retention_policy('snmp_polls', INTERVAL '90 days',
    if_not_exists => TRUE
);

-- Network flow records hypertable
CREATE TABLE IF NOT EXISTS flow_records (
    time        TIMESTAMPTZ       NOT NULL,
    tenant_id   UUID              NOT NULL,
    probe_id    TEXT              NOT NULL,
    src_ip      TEXT              NOT NULL,
    dst_ip      TEXT              NOT NULL,
    src_port    INT               DEFAULT 0,
    dst_port    INT               DEFAULT 0,
    protocol    TEXT              NOT NULL DEFAULT 'tcp',
    bytes       BIGINT            DEFAULT 0,
    packets     BIGINT            DEFAULT 0,
    duration_ms INT               DEFAULT 0
);

SELECT create_hypertable('flow_records', 'time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

ALTER TABLE flow_records SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'tenant_id, probe_id',
    timescaledb.compress_orderby = 'time DESC'
);
SELECT add_compression_policy('flow_records', INTERVAL '7 days',
    if_not_exists => TRUE
);
SELECT add_retention_policy('flow_records', INTERVAL '30 days',
    if_not_exists => TRUE
);

-- Topology edges table
CREATE TABLE IF NOT EXISTS topology_edges (
    time        TIMESTAMPTZ       NOT NULL,
    tenant_id   UUID              NOT NULL,
    probe_id    TEXT              NOT NULL,
    src_mac     TEXT              NOT NULL,
    dst_mac     TEXT              NOT NULL,
    src_ip      TEXT              NOT NULL DEFAULT '',
    dst_ip      TEXT              NOT NULL DEFAULT '',
    src_port    TEXT              NOT NULL DEFAULT '',
    dst_port    TEXT              NOT NULL DEFAULT '',
    protocol    TEXT              NOT NULL DEFAULT 'lldp'
);

SELECT create_hypertable('topology_edges', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

SELECT add_retention_policy('topology_edges', INTERVAL '90 days',
    if_not_exists => TRUE
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_snmp_polls_lookup
    ON snmp_polls (tenant_id, target_ip, oid, time DESC);
CREATE INDEX IF NOT EXISTS idx_flow_records_lookup
    ON flow_records (tenant_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_flow_records_conversation
    ON flow_records (tenant_id, src_ip, dst_ip, time DESC);
`

const Migration003 = `
-- Backup and Recovery schema for Phase 8C

CREATE TYPE recovery_state_enum AS ENUM (
    'idle', 'discovery', 'preflight', 'quiesce',
    'backup_database', 'backup_jetstream', 'backup_object_storage',
    'verify_integrity', 'pre_restore_validation',
    'restore_database', 'restore_jetstream', 'restore_object_storage',
    'post_restore_validation', 'health_check', 'verification',
    'rpo_validation', 'rto_validation', 'rollback', 'cleanup', 'completed'
);

CREATE TABLE IF NOT EXISTS backup_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    backup_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    state recovery_state_enum NOT NULL DEFAULT 'idle',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS recovery_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recovery_id TEXT NOT NULL UNIQUE,
    backup_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    state recovery_state_enum NOT NULL DEFAULT 'idle',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    rpo_minutes INT,
    rto_minutes INT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS backup_audit_log (
    id BIGSERIAL PRIMARY KEY,
    backup_id TEXT NOT NULL,
    action TEXT NOT NULL,
    details JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_backup_records_state
    ON backup_records (state, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_recovery_operations_recovery_id
    ON recovery_operations (recovery_id);

CREATE INDEX IF NOT EXISTS idx_backup_audit_log_backup_id
    ON backup_audit_log (backup_id, created_at DESC);
`
const Migration004 = `
-- TimescaleDB Migration 4: Dynamic Retention Policies
-- Enables per-tenant hot/warm/cold tiered retention management

-- Function to drop an existing retention policy for a hypertable
CREATE OR REPLACE FUNCTION drop_retention_policy(p_hypertable_name TEXT, p_fail_on_none BOOLEAN DEFAULT TRUE)
RETURNS VOID AS $$
DECLARE
    policy_job_id INT;
BEGIN
    SELECT job_id INTO policy_job_id
    FROM timescaledb_information.jobs
    WHERE proc_name = 'run_job'
      AND config IS NOT NULL
      AND config->>'hypertable_name' = p_hypertable_name;

    IF policy_job_id IS NOT NULL THEN
        PERFORM cancel_job(policy_job_id);
        PERFORM delete_job(policy_job_id);
    ELSIF p_fail_on_none THEN
        RAISE EXCEPTION 'no retention policy found for hypertable %', p_hypertable_name;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Function to set retention policy for a hypertable
CREATE OR REPLACE FUNCTION set_retention_policy(
    p_hypertable_name TEXT,
    p_retention_period INTERVAL,
    p_inplace BOOLEAN DEFAULT FALSE
)
RETURNS INT AS $$
DECLARE
    job_id INT;
BEGIN
    -- Drop existing policy if any
    BEGIN
        PERFORM drop_retention_policy(p_hypertable_name, FALSE);
    EXCEPTION WHEN OTHERS THEN
        -- Ignore errors if no existing policy
    END;

    -- Add new retention policy
    SELECT add_retention_policy(p_hypertable_name, p_retention_period, inplace => p_inplace)
    INTO job_id;

    RETURN job_id;
END;
$$ LANGUAGE plpgsql;

-- Function to get current retention settings for a hypertable
CREATE OR REPLACE FUNCTION get_retention_policy(p_hypertable_name TEXT)
RETURNS TABLE(
    hypertable_name TEXT,
    retention_period INTERVAL,
    retention_period_text TEXT,
    job_id INT,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        j.config->>'hypertable_name'::TEXT,
        (j.config->>'data_retention_period')::INTERVAL,
        (j.config->>'data_retention_period')::TEXT,
        j.job_id,
        j.last_run_at,
        j.next_run_at
    FROM timescaledb_information.jobs j
    WHERE j.proc_name = 'run_job'
      AND j.config IS NOT NULL
      AND j.config->>'hypertable_name' = p_hypertable_name;
END;
$$ LANGUAGE plpgsql;
`
