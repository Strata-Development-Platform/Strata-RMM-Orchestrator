package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

type APIServer struct {
	addr    string
	db      *timescale.Client
	nats    *nats.Conn
	auth    *auth.EnrollmentManager
	logger  *zap.Logger
	server  *http.Server
}

func NewAPIServer(addr string, db *timescale.Client, nc *nats.Conn, logger *zap.Logger) *APIServer {
	em := auth.NewEnrollmentManager("strata-rmm-dev-secret")
	return &APIServer{
		addr:   addr,
		db:     db,
		nats:   nc,
		auth:   em,
		logger: logger,
	}
}

func (s *APIServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/enroll", s.handleEnroll)

	mux.HandleFunc("GET /api/v1/metrics", s.handleQueryMetrics)
	mux.HandleFunc("GET /api/v1/devices/{tenantID}/{deviceID}/metrics/{metricName}", s.handleDeviceMetrics)
	mux.HandleFunc("GET /api/v1/heartbeat/{tenantID}/{deviceID}", s.handleGetHeartbeat)

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      withLogging(mux, s.logger),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("API server starting", zap.String("addr", s.addr))

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Fatal("API server error", zap.Error(err))
		}
	}()

	return nil
}

func (s *APIServer) Stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleEnroll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID string `json:"tenant_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	token, err := s.auth.CreateEnrollmentToken(req.TenantID, 24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating token"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enrollment_token": token.Token,
		"tenant_id":        req.TenantID,
		"expires_at":       token.ExpiresAt,
		"nats_urls":        []string{s.nats.ConnectedUrl()},
	})
}

func (s *APIServer) handleQueryMetrics(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	deviceID := r.URL.Query().Get("device_id")
	metricName := r.URL.Query().Get("metric_name")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	bucket := r.URL.Query().Get("bucket")

	if tenantID == "" || metricName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant_id and metric_name required"})
		return
	}

	start := parseTime(startStr, time.Now().Add(-1*time.Hour))
	end := parseTime(endStr, time.Now())

	var metrics []timescale.MetricRow
	var err error

	if bucket != "" {
		metrics, err = s.db.QueryAggregated(r.Context(), tenantID, deviceID, metricName, start, end, bucket)
	} else {
		metrics, err = s.db.QueryMetrics(r.Context(), tenantID, deviceID, metricName, start, end)
	}

	if err != nil {
		s.logger.Error("querying metrics", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"metrics": metrics,
		"count":   len(metrics),
	})
}

func (s *APIServer) handleDeviceMetrics(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	deviceID := r.PathValue("deviceID")
	metricName := r.PathValue("metricName")

	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	bucket := r.URL.Query().Get("bucket")

	start := parseTime(startStr, time.Now().Add(-1*time.Hour))
	end := parseTime(endStr, time.Now())

	var metrics []timescale.MetricRow
	var err error

	if bucket != "" {
		metrics, err = s.db.QueryAggregated(r.Context(), tenantID, deviceID, metricName, start, end, bucket)
	} else {
		metrics, err = s.db.QueryMetrics(r.Context(), tenantID, deviceID, metricName, start, end)
	}

	if err != nil {
		s.logger.Error("querying device metrics", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":   deviceID,
		"metric_name": metricName,
		"metrics":     metrics,
		"count":       len(metrics),
	})
}

func (s *APIServer) handleGetHeartbeat(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	deviceID := r.PathValue("deviceID")

	hb, err := s.db.GetLatestHeartbeat(r.Context(), tenantID, deviceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no heartbeat found"})
		return
	}

	writeJSON(w, http.StatusOK, hb)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func parseTime(s string, defaultVal time.Time) time.Time {
	if s == "" {
		return defaultVal
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return defaultVal
	}
	return t
}

func withLogging(next http.Handler, logger *zap.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("api request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote", r.RemoteAddr),
			zap.Duration("duration", time.Since(start)),
		)
	})
}

func (s *APIServer) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token != "" {
			gen := auth.NewTokenGenerator("strata-rmm-dev-secret")
			claims, err := gen.Validate(token)
			if err == nil && claims != nil {
				r.Header.Set("X-Tenant-ID", claims.TenantID)
				r.Header.Set("X-Agent-ID", claims.AgentID)
			}
		}

		next.ServeHTTP(w, r)
	})
}
