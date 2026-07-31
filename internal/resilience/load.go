package resilience

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxResponseBytes = 1 << 20

type LoadConfig struct {
	BaseURL        string
	Path           string
	Label          string
	BearerToken    string
	Duration       time.Duration
	Rate           int
	Concurrency    int
	RequestTimeout time.Duration
	MaxErrorRate   float64
	MaxP95         time.Duration
}

type LoadReport struct {
	Target          string        `json:"target"`
	StartedAt       time.Time     `json:"started_at"`
	Duration        time.Duration `json:"duration"`
	Requests        int64         `json:"requests"`
	Successes       int64         `json:"successes"`
	Failures        int64         `json:"failures"`
	ErrorRate       float64       `json:"error_rate"`
	P50             time.Duration `json:"p50"`
	P95             time.Duration `json:"p95"`
	P99             time.Duration `json:"p99"`
	Max             time.Duration `json:"max"`
	ThresholdPassed bool          `json:"threshold_passed"`
}

type LoadRunner struct {
	config LoadConfig
	client *http.Client
	target string
}

var labelPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func NewLoadRunner(config LoadConfig) (*LoadRunner, error) {
	base, err := url.Parse(config.BaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("load-test base URL is invalid")
	}
	if base.Scheme != "https" && base.Hostname() != "localhost" && base.Hostname() != "127.0.0.1" {
		return nil, fmt.Errorf("remote load-test base URL must use HTTPS")
	}
	if base.User != nil || base.RawQuery != "" || (base.Path != "" && base.Path != "/") {
		return nil, fmt.Errorf("load-test base URL must not contain credentials, query parameters, or a path")
	}
	if config.Path == "" || !strings.HasPrefix(config.Path, "/") {
		return nil, fmt.Errorf("load-test path must begin with /")
	}
	if strings.ContainsAny(config.Path, "?#") {
		return nil, fmt.Errorf("load-test path must not contain a query or fragment")
	}
	if !labelPattern.MatchString(config.Label) {
		return nil, fmt.Errorf("load-test label must be a bounded lowercase identifier")
	}
	target, err := url.Parse(strings.TrimRight(config.BaseURL, "/") + config.Path)
	if err != nil || target.Host != base.Host || target.Scheme != base.Scheme {
		return nil, fmt.Errorf("load-test target is invalid")
	}
	if config.Duration <= 0 || config.Rate <= 0 || config.Concurrency <= 0 {
		return nil, fmt.Errorf("duration, rate, and concurrency must be positive")
	}
	if time.Second/time.Duration(config.Rate) <= 0 {
		return nil, fmt.Errorf("rate is too high")
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 10 * time.Second
	}
	if config.MaxErrorRate < 0 || config.MaxErrorRate > 1 {
		return nil, fmt.Errorf("maximum error rate must be between 0 and 1")
	}
	if config.MaxP95 <= 0 {
		return nil, fmt.Errorf("maximum p95 latency must be positive")
	}
	return &LoadRunner{
		config: config,
		target: target.String(),
		client: &http.Client{
			Timeout: config.RequestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (r *LoadRunner) Run(ctx context.Context) LoadReport {
	started := time.Now().UTC()
	runCtx, cancel := context.WithTimeout(ctx, r.config.Duration)
	defer cancel()

	jobs := make(chan struct{}, r.config.Concurrency)
	var successes atomic.Int64
	var failures atomic.Int64
	var durationsMu sync.Mutex
	estimatedSamples := int64(r.config.Rate) * int64(r.config.Duration/time.Second)
	if estimatedSamples < 0 || estimatedSamples > 1_000_000 {
		estimatedSamples = 1_000_000
	}
	durations := make([]time.Duration, 0, int(estimatedSamples))
	var workers sync.WaitGroup

	for i := 0; i < r.config.Concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range jobs {
				requestStarted := time.Now()
				ok := r.request(ctx)
				elapsed := time.Since(requestStarted)
				durationsMu.Lock()
				durations = append(durations, elapsed)
				durationsMu.Unlock()
				if ok {
					successes.Add(1)
				} else {
					failures.Add(1)
				}
			}
		}()
	}

	interval := time.Second / time.Duration(r.config.Rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
sendLoop:
	for {
		select {
		case <-runCtx.Done():
			break sendLoop
		case <-ticker.C:
			select {
			case jobs <- struct{}{}:
			default:
				failures.Add(1)
			}
		}
	}
	close(jobs)
	workers.Wait()

	report := summarize(started, time.Since(started), successes.Load(), failures.Load(), durations)
	report.Target = r.config.Label
	report.ThresholdPassed = report.Requests > 0 &&
		report.ErrorRate <= r.config.MaxErrorRate &&
		report.P95 <= r.config.MaxP95
	return report
}

func (r *LoadRunner) request(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.target, nil)
	if err != nil {
		return false
	}
	if r.config.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.config.BearerToken)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func summarize(started time.Time, duration time.Duration, successes, failures int64, values []time.Duration) LoadReport {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	requests := successes + failures
	errorRate := float64(0)
	if requests > 0 {
		errorRate = float64(failures) / float64(requests)
	}
	return LoadReport{
		StartedAt: started,
		Duration:  duration,
		Requests:  requests,
		Successes: successes,
		Failures:  failures,
		ErrorRate: errorRate,
		P50:       percentile(sorted, 0.50),
		P95:       percentile(sorted, 0.95),
		P99:       percentile(sorted, 0.99),
		Max:       percentile(sorted, 1),
	}
}

func percentile(sorted []time.Duration, quantile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted)-1) * quantile)
	return sorted[index]
}
