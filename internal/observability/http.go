package observability

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var latencyBounds = [...]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type requestKey struct {
	method string
	route  string
	status int
}

type requestValue struct {
	count   uint64
	sum     float64
	buckets [len(latencyBounds) + 1]uint64
}

// HTTPRegistry records bounded-cardinality RED and saturation metrics for the
// orchestrator API. Routes are taken from net/http's matched pattern rather
// than the raw request path so tenant and resource identifiers never become
// metric labels.
type HTTPRegistry struct {
	inFlight atomic.Int64
	mu       sync.RWMutex
	requests map[requestKey]requestValue
	jobDB    *sql.DB
}

func NewHTTPRegistry() *HTTPRegistry {
	return &HTTPRegistry{requests: make(map[requestKey]requestValue)}
}

func (r *HTTPRegistry) WithJobDatabase(db *sql.DB) *HTTPRegistry {
	r.jobDB = db
	return r
}

func (r *HTTPRegistry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.inFlight.Add(1)
		defer r.inFlight.Add(-1)

		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		next.ServeHTTP(recorder, req)

		route := req.Pattern
		if route == "" {
			route = "unmatched"
		}
		r.observe(req.Method, route, recorder.status, time.Since(started).Seconds())
	})
}

func (r *HTTPRegistry) observe(method, route string, status int, seconds float64) {
	key := requestKey{method: method, route: route, status: status}
	r.mu.Lock()
	defer r.mu.Unlock()
	value := r.requests[key]
	value.count++
	value.sum += seconds
	index := len(latencyBounds)
	for i, bound := range latencyBounds {
		if seconds <= bound {
			index = i
			break
		}
	}
	for i := index; i < len(value.buckets); i++ {
		value.buckets[i]++
	}
	r.requests[key] = value
}

func (r *HTTPRegistry) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	r.mu.RLock()
	_, _ = io.WriteString(w, "# HELP strata_http_requests_in_flight Current API requests.\n")
	_, _ = io.WriteString(w, "# TYPE strata_http_requests_in_flight gauge\n")
	_, _ = fmt.Fprintf(w, "strata_http_requests_in_flight %d\n", r.inFlight.Load())
	_, _ = io.WriteString(w, "# HELP strata_http_requests_total API requests by method, matched route, and status.\n")
	_, _ = io.WriteString(w, "# TYPE strata_http_requests_total counter\n")
	_, _ = io.WriteString(w, "# HELP strata_http_request_duration_seconds API request latency.\n")
	_, _ = io.WriteString(w, "# TYPE strata_http_request_duration_seconds histogram\n")
	for key, value := range r.requests {
		labels := `method="` + escapeLabel(key.method) + `",route="` + escapeLabel(key.route) +
			`",status="` + strconv.Itoa(key.status) + `"`
		_, _ = fmt.Fprintf(w, "strata_http_requests_total{%s} %d\n", labels, value.count)
		for i, bound := range latencyBounds {
			_, _ = fmt.Fprintf(w, "strata_http_request_duration_seconds_bucket{%s,le=\"%g\"} %d\n",
				labels, bound, value.buckets[i])
		}
		_, _ = fmt.Fprintf(w, "strata_http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n",
			labels, value.buckets[len(latencyBounds)])
		_, _ = fmt.Fprintf(w, "strata_http_request_duration_seconds_sum{%s} %g\n", labels, value.sum)
		_, _ = fmt.Fprintf(w, "strata_http_request_duration_seconds_count{%s} %d\n", labels, value.count)
	}
	r.mu.RUnlock()
	r.writeJobMetrics(w)
}

func (r *HTTPRegistry) writeJobMetrics(w io.Writer) {
	if r.jobDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rows, err := r.jobDB.QueryContext(ctx, `
		SELECT status, COUNT(*), COALESCE(SUM(retry_count), 0)
		FROM job_targets
		GROUP BY status
		ORDER BY status`)
	if err != nil {
		writeCollectorResult(w, false)
		return
	}
	defer func() { _ = rows.Close() }()

	type state struct {
		status  string
		count   int64
		retries int64
	}
	var states []state
	for rows.Next() {
		var item state
		if err := rows.Scan(&item.status, &item.count, &item.retries); err != nil {
			writeCollectorResult(w, false)
			return
		}
		states = append(states, item)
	}
	if err := rows.Err(); err != nil {
		writeCollectorResult(w, false)
		return
	}

	var oldestSeconds float64
	if err := r.jobDB.QueryRowContext(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(j.created_at))), 0)
		FROM job_targets jt
		JOIN jobs j ON j.id = jt.job_id
		WHERE jt.status IN ('pending','queued','waiting','dispatched','acknowledged','running')`,
	).Scan(&oldestSeconds); err != nil {
		writeCollectorResult(w, false)
		return
	}

	_, _ = io.WriteString(w, "# HELP strata_job_targets Current durable job targets by status.\n")
	_, _ = io.WriteString(w, "# TYPE strata_job_targets gauge\n")
	_, _ = io.WriteString(w, "# HELP strata_job_target_retries Retry attempts accumulated by current target status.\n")
	_, _ = io.WriteString(w, "# TYPE strata_job_target_retries gauge\n")
	for _, item := range states {
		label := `status="` + escapeLabel(item.status) + `"`
		_, _ = fmt.Fprintf(w, "strata_job_targets{%s} %d\n", label, item.count)
		_, _ = fmt.Fprintf(w, "strata_job_target_retries{%s} %d\n", label, item.retries)
	}
	_, _ = io.WriteString(w, "# HELP strata_job_oldest_active_seconds Age of the oldest unfinished durable job.\n")
	_, _ = io.WriteString(w, "# TYPE strata_job_oldest_active_seconds gauge\n")
	_, _ = fmt.Fprintf(w, "strata_job_oldest_active_seconds %g\n", oldestSeconds)
	writeCollectorResult(w, true)
}

func writeCollectorResult(w io.Writer, success bool) {
	value := 0
	if success {
		value = 1
	}
	_, _ = io.WriteString(w, "# HELP strata_job_metrics_collection_success Whether the last job metrics collection succeeded.\n")
	_, _ = io.WriteString(w, "# TYPE strata_job_metrics_collection_success gauge\n")
	_, _ = fmt.Fprintf(w, "strata_job_metrics_collection_success %d\n", value)
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Unwrap allows http.ResponseController to retain optional capabilities such
// as flushing and hijacking that remote-control handlers may require.
func (w *statusRecorder) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
