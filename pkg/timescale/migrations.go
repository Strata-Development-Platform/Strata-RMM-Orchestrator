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
		{
			Version: 2,
			Name:    "alerts_and_probes_schema",
			SQL:     Migration002,
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
