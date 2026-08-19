//go:build dbintegration

package observability

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/strata-rmm/strata-rmm-orchestrator/internal/postgresdriver"
)

func TestJobMetricsAgainstPostgreSQL(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `
		DROP TABLE IF EXISTS job_targets;
		DROP TABLE IF EXISTS jobs;
		CREATE TABLE jobs (id TEXT PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL);
		CREATE TABLE job_targets (
			id TEXT PRIMARY KEY,
			job_id TEXT NOT NULL REFERENCES jobs(id),
			status TEXT NOT NULL,
			retry_count INT NOT NULL DEFAULT 0
		);
		INSERT INTO jobs (id, created_at) VALUES ('job-1', NOW() - INTERVAL '20 minutes');
		INSERT INTO job_targets (id, job_id, status, retry_count)
		VALUES ('target-1', 'job-1', 'queued', 2), ('target-2', 'job-1', 'failed', 3);
	`); err != nil {
		t.Fatal(err)
	}

	registry := NewHTTPRegistry().WithJobDatabase(db)
	out := httptest.NewRecorder()
	registry.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := out.Body.String()
	for _, expected := range []string{
		`strata_job_targets{status="queued"} 1`,
		`strata_job_targets{status="failed"} 1`,
		`strata_job_target_retries{status="queued"} 2`,
		`strata_job_metrics_collection_success 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("missing %q in:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "strata_job_oldest_active_seconds 0") {
		t.Fatalf("active job age was not measured: %s", body)
	}
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
}
