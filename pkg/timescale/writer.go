package timescale

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type Client struct {
	writeDB *sql.DB
	readDB  *sql.DB
}

type MetricRow struct {
	Time       time.Time
	TenantID   string
	DeviceID   string
	MetricName string
	Value      float64
	Tags       map[string]string
}

type EventRow struct {
	Time      time.Time
	TenantID  string
	DeviceID  string
	EventType string
	Message   string
	Tags      map[string]string
}

type HeartbeatRow struct {
	Time         time.Time
	TenantID     string
	DeviceID     string
	Status       string
	AgentVersion string
	Metadata     map[string]string
}

func NewClient(ctx context.Context, primaryDSN, replicaDSN string) (*Client, error) {
	writeDB, err := sql.Open("postgres", primaryDSN)
	if err != nil {
		return nil, fmt.Errorf("opening primary connection: %w", err)
	}

	writeDB.SetMaxOpenConns(20)
	writeDB.SetMaxIdleConns(5)
	writeDB.SetConnMaxLifetime(5 * time.Minute)

	if err := writeDB.PingContext(ctx); err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("pinging primary: %w", err)
	}

	var readDB *sql.DB
	if replicaDSN != "" {
		readDB, err = sql.Open("postgres", replicaDSN)
		if err != nil {
			_ = writeDB.Close()
			return nil, fmt.Errorf("opening replica connection: %w", err)
		}

		readDB.SetMaxOpenConns(25)
		readDB.SetMaxIdleConns(10)
		readDB.SetConnMaxLifetime(5 * time.Minute)

		if err := readDB.PingContext(ctx); err != nil {
			_ = readDB.Close()
			return nil, fmt.Errorf("pinging replica: %w", err)
		}
	}

	return &Client{writeDB: writeDB, readDB: readDB}, nil
}

func (c *Client) SetPoolConfig(maxOpen, maxIdle int, maxLifetime time.Duration) {
	if c.writeDB != nil {
		if maxOpen > 0 {
			c.writeDB.SetMaxOpenConns(maxOpen)
		}
		if maxIdle >= 0 {
			c.writeDB.SetMaxIdleConns(maxIdle)
		}
		if maxLifetime > 0 {
			c.writeDB.SetConnMaxLifetime(maxLifetime)
		}
	}
	if c.readDB != nil {
		if maxOpen > 0 {
			c.readDB.SetMaxOpenConns(maxOpen)
		}
		if maxIdle >= 0 {
			c.readDB.SetMaxIdleConns(maxIdle)
		}
		if maxLifetime > 0 {
			c.readDB.SetConnMaxLifetime(maxLifetime)
		}
	}
}

func (c *Client) DB() *sql.DB {
	return c.writeDB
}

func (c *Client) ReadDB() *sql.DB {
	return c.readDB
}

func (c *Client) Close() {
	if c.writeDB != nil {
		_ = c.writeDB.Close()
	}
	if c.readDB != nil {
		_ = c.readDB.Close()
	}
}

// DBFor reads from the replica if available, otherwise falls back to primary.
func (c *Client) DBFor(queryType string) *sql.DB {
	if queryType == "read" && c.readDB != nil {
		return c.readDB
	}
	return c.writeDB
}

func (c *Client) ApplyMigrations(ctx context.Context) error {
	for _, m := range AllMigrations() {
		if _, err := c.writeDB.ExecContext(ctx, m.SQL); err != nil {
			return fmt.Errorf("migration %s: %w", m, err)
		}
	}
	return nil
}

func (c *Client) InsertMetrics(ctx context.Context, metrics []MetricRow) error {
	if len(metrics) == 0 {
		return nil
	}

	tx, err := c.writeDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metrics (time, tenant_id, device_id, metric_name, value, tags)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, m := range metrics {
		tagsJSON := "{}"
		if len(m.Tags) > 0 {
			tagsJSON = mapToJSON(m.Tags)
		}
		if _, err := stmt.ExecContext(ctx, m.Time, m.TenantID, m.DeviceID, m.MetricName, m.Value, tagsJSON); err != nil {
			return fmt.Errorf("inserting metric: %w", err)
		}
	}

	return tx.Commit()
}

func (c *Client) InsertEvents(ctx context.Context, events []EventRow) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := c.writeDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (time, tenant_id, device_id, event_type, message, tags)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, e := range events {
		tagsJSON := "{}"
		if len(e.Tags) > 0 {
			tagsJSON = mapToJSON(e.Tags)
		}
		if _, err := stmt.ExecContext(ctx, e.Time, e.TenantID, e.DeviceID, e.EventType, e.Message, tagsJSON); err != nil {
			return fmt.Errorf("inserting event: %w", err)
		}
	}

	return tx.Commit()
}

func (c *Client) RecordHeartbeat(ctx context.Context, hb HeartbeatRow) error {
	metaJSON := "{}"
	if len(hb.Metadata) > 0 {
		metaJSON = mapToJSON(hb.Metadata)
	}

	_, err := c.writeDB.ExecContext(ctx, `
		INSERT INTO heartbeats (time, tenant_id, device_id, status, agent_version, metadata)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)`,
		hb.Time, hb.TenantID, hb.DeviceID, hb.Status, hb.AgentVersion, metaJSON,
	)
	return err
}

func (c *Client) GetLatestHeartbeat(ctx context.Context, tenantID, deviceID string) (*HeartbeatRow, error) {
	row := c.DBFor("read").QueryRowContext(ctx, `
		SELECT time, tenant_id, device_id, status, agent_version, metadata
		FROM heartbeats
		WHERE tenant_id = $1 AND device_id = $2
		ORDER BY time DESC
		LIMIT 1`, tenantID, deviceID)

	var hb HeartbeatRow
	var metaJSON []byte
	err := row.Scan(&hb.Time, &hb.TenantID, &hb.DeviceID, &hb.Status, &hb.AgentVersion, &metaJSON)
	if err != nil {
		return nil, err
	}

	if len(metaJSON) > 0 {
		hb.Metadata = jsonToMap(string(metaJSON))
	}

	return &hb, nil
}

func (c *Client) QueryMetrics(ctx context.Context, tenantID, deviceID, metricName string, start, end time.Time) ([]MetricRow, error) {
	rows, err := c.DBFor("read").QueryContext(ctx, `
		SELECT time, tenant_id, device_id, metric_name, value, tags
		FROM metrics
		WHERE tenant_id = $1
			AND device_id = $2
			AND metric_name = $3
			AND time >= $4
			AND time <= $5
		ORDER BY time ASC`, tenantID, deviceID, metricName, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []MetricRow
	for rows.Next() {
		var m MetricRow
		var tagsJSON []byte
		if err := rows.Scan(&m.Time, &m.TenantID, &m.DeviceID, &m.MetricName, &m.Value, &tagsJSON); err != nil {
			return nil, err
		}
		if len(tagsJSON) > 0 {
			m.Tags = jsonToMap(string(tagsJSON))
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) QueryAggregated(ctx context.Context, tenantID, deviceID, metricName string, start, end time.Time, bucket string) ([]MetricRow, error) {
	if c.readDB == nil {
		return nil, fmt.Errorf("read replica not configured")
	}
	viewName := "metrics_1m"
	switch bucket {
	case "1m":
		viewName = "metrics_1m"
	case "1h":
		viewName = "metrics_1h"
	}

	query := fmt.Sprintf(`
		SELECT bucket, tenant_id, device_id, metric_name, avg, min, max, last, sample_count
		FROM %s
		WHERE tenant_id = $1
			AND device_id = $2
			AND metric_name = $3
			AND bucket >= $4
			AND bucket <= $5
		ORDER BY bucket ASC`, viewName)

	rows, err := c.readDB.QueryContext(ctx, query, tenantID, deviceID, metricName, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []MetricRow
	for rows.Next() {
		var m MetricRow
		var avg, min, max, last float64
		var count int
		if err := rows.Scan(&m.Time, &m.TenantID, &m.DeviceID, &m.MetricName, &avg, &min, &max, &last, &count); err != nil {
			return nil, err
		}
		m.Value = avg
		m.Tags = map[string]string{
			"avg": fmt.Sprintf("%f", avg), "min": fmt.Sprintf("%f", min),
			"max": fmt.Sprintf("%f", max), "last": fmt.Sprintf("%f", last),
			"count": fmt.Sprintf("%d", count),
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// RetentionPolicy represents a retention policy for a hypertable
type RetentionPolicy struct {
	HypertableName  string        `json:"hypertable_name"`
	RetentionPeriod time.Duration `json:"retention_period"`
	JobID           int           `json:"job_id"`
	LastRunAt       *time.Time    `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time    `json:"next_run_at,omitempty"`
}

// SetRetentionPolicy applies a retention policy to the metrics hypertable.
// It drops any existing policy first, then creates a new one.
func (c *Client) SetRetentionPolicy(ctx context.Context, retentionDays int) error {
	if retentionDays < 1 || retentionDays > 10000 {
		return fmt.Errorf("retention_days must be between 1 and 10000, got %d", retentionDays)
	}
	_, err := c.writeDB.ExecContext(ctx, `SELECT set_retention_policy('metrics', ($1 || ' days')::INTERVAL)`, retentionDays)
	return err
}

// SetHeartbeatsRetention applies a retention policy to the heartbeats hypertable.
func (c *Client) SetHeartbeatsRetention(ctx context.Context, retentionDays int) error {
	if retentionDays < 1 || retentionDays > 10000 {
		return fmt.Errorf("retention_days must be between 1 and 10000, got %d", retentionDays)
	}
	_, err := c.writeDB.ExecContext(ctx, `SELECT set_retention_policy('heartbeats', ($1 || ' days')::INTERVAL)`, retentionDays)
	return err
}

// SetAlertsRetention applies a retention policy to the alerts_ts hypertable.
func (c *Client) SetAlertsRetention(ctx context.Context, retentionDays int) error {
	if retentionDays < 1 || retentionDays > 10000 {
		return fmt.Errorf("retention_days must be between 1 and 10000, got %d", retentionDays)
	}
	_, err := c.writeDB.ExecContext(ctx, `SELECT set_retention_policy('alerts_ts', ($1 || ' days')::INTERVAL)`, retentionDays)
	return err
}

// SetSNMPPollsRetention applies a retention policy to the snmp_polls hypertable.
func (c *Client) SetSNMPPollsRetention(ctx context.Context, retentionDays int) error {
	if retentionDays < 1 || retentionDays > 10000 {
		return fmt.Errorf("retention_days must be between 1 and 10000, got %d", retentionDays)
	}
	_, err := c.writeDB.ExecContext(ctx, `SELECT set_retention_policy('snmp_polls', ($1 || ' days')::INTERVAL)`, retentionDays)
	return err
}

// SetFlowRecordsRetention applies a retention policy to the flow_records hypertable.
func (c *Client) SetFlowRecordsRetention(ctx context.Context, retentionDays int) error {
	if retentionDays < 1 || retentionDays > 10000 {
		return fmt.Errorf("retention_days must be between 1 and 10000, got %d", retentionDays)
	}
	_, err := c.writeDB.ExecContext(ctx, `SELECT set_retention_policy('flow_records', ($1 || ' days')::INTERVAL)`, retentionDays)
	return err
}

// SetTopologyEdgesRetention applies a retention policy to the topology_edges hypertable.
func (c *Client) SetTopologyEdgesRetention(ctx context.Context, retentionDays int) error {
	if retentionDays < 1 || retentionDays > 10000 {
		return fmt.Errorf("retention_days must be between 1 and 10000, got %d", retentionDays)
	}
	_, err := c.writeDB.ExecContext(ctx, `SELECT set_retention_policy('topology_edges', ($1 || ' days')::INTERVAL)`, retentionDays)
	return err
}

// GetRetentionPolicy returns the retention policy for the metrics hypertable.
func (c *Client) GetRetentionPolicy(ctx context.Context) (*RetentionPolicy, error) {
	row := c.writeDB.QueryRowContext(ctx, `SELECT * FROM get_retention_policy('metrics')`)
	return scanRetentionPolicy(row)
}

// GetHeartbeatsRetentionPolicy returns the retention policy for the heartbeats hypertable.
func (c *Client) GetHeartbeatsRetentionPolicy(ctx context.Context) (*RetentionPolicy, error) {
	row := c.writeDB.QueryRowContext(ctx, `SELECT * FROM get_retention_policy('heartbeats')`)
	return scanRetentionPolicy(row)
}

// GetAllRetentionPolicies returns retention policies for all hypertables.
func (c *Client) GetAllRetentionPolicies(ctx context.Context) ([]RetentionPolicy, error) {
	tables := []string{"metrics", "heartbeats", "alerts_ts", "snmp_polls", "flow_records", "topology_edges"}
	var results []RetentionPolicy
	for _, table := range tables {
		row := c.writeDB.QueryRowContext(ctx, `SELECT * FROM get_retention_policy($1)`, table)
		policy, err := scanRetentionPolicy(row)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, fmt.Errorf("getting retention policy for %s: %w", table, err)
		}
		if policy != nil {
			results = append(results, *policy)
		}
	}
	return results, nil
}

func scanRetentionPolicy(row scannable) (*RetentionPolicy, error) {
	var p RetentionPolicy
	var name, periodText, periodTextStr string
	var jobID sql.NullInt64
	var lastRun, nextRun sql.NullTime

	err := row.Scan(&name, &periodText, &periodTextStr, &jobID, &lastRun, &nextRun)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	var period time.Duration
	if periodText != "" {
		// Parse the interval text back to duration
		daysStr := strings.TrimSuffix(strings.TrimPrefix(periodTextStr, "("), ")")
		// Try to extract days from the string
		for _, unit := range []string{"day", "days"} {
			if strings.HasSuffix(daysStr, " "+unit) || strings.HasSuffix(daysStr, unit) {
				// Extract number
				var days int
				n, _ := fmt.Sscanf(daysStr, "%d", &days)
				if n == 1 {
					period = time.Duration(days) * 24 * time.Hour
				}
				break
			}
		}
	}

	p.HypertableName = name
	p.RetentionPeriod = period
	if jobID.Valid {
		p.JobID = int(jobID.Int64)
	}
	if lastRun.Valid {
		p.LastRunAt = &lastRun.Time
	}
	if nextRun.Valid {
		p.NextRunAt = &nextRun.Time
	}
	return &p, nil
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func mapToJSON(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	b := []byte("{")
	first := true
	for k, v := range m {
		if !first {
			b = append(b, ',')
		}
		first = false
		b = append(b, '"')
		b = append(b, []byte(escapeJSON(k))...)
		b = append(b, '"', ':')
		b = append(b, '"')
		b = append(b, []byte(escapeJSON(v))...)
		b = append(b, '"')
	}
	b = append(b, '}')
	return string(b)
}

func jsonToMap(s string) map[string]string {
	m := make(map[string]string)
	if len(s) <= 2 {
		return m
	}
	s = s[1 : len(s)-1]
	i := 0
	for i < len(s) {
		if s[i] == '"' {
			i++
			keyStart := i
			for i < len(s) && s[i] != '"' {
				i++
			}
			key := s[keyStart:i]
			i++
			if i < len(s) && s[i] == ':' {
				i++
			}
			if i < len(s) && s[i] == '"' {
				i++
				valStart := i
				for i < len(s) && s[i] != '"' {
					i++
				}
				m[key] = s[valStart:i]
				i++
			}
		}
		i++
	}
	return m
}

func escapeJSON(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' {
			result = append(result, '\\', c)
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}
