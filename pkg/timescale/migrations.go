package timescale

import "fmt"

type Migration struct {
	Version int
	Name    string
	SQL     string
}

func AllMigrations() []Migration {
	return []Migration{
		{
			Version: 1,
			Name:    "initial_metrics_schema",
			SQL:     Migration001,
		},
	}
}

func (m Migration) String() string {
	return fmt.Sprintf("v%d: %s", m.Version, m.Name)
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
