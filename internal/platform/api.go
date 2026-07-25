package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/alerting"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/inventory"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/remote"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

type APIServer struct {
	addr           string
	db             *timescale.Client
	nats           *nats.Conn
	auth           *auth.EnrollmentManager
	totp           *auth.TOTPManager
	mfaStore       *auth.MFAStore
	logger         *zap.Logger
	server         *http.Server
	alertEngine    *alerting.Engine
	vulnEngine     *inventory.VulnerabilityEngine
	recordingStore *remote.RecordingStore
	storageBackend storage.Backend
}

func NewAPIServer(addr string, db *timescale.Client, nc *nats.Conn, logger *zap.Logger) *APIServer {
	em := auth.NewEnrollmentManager("strata-rmm-dev-secret")
	s := &APIServer{
		addr:   addr,
		db:     db,
		nats:   nc,
		auth:   em,
		totp:   auth.NewTOTPManager(),
		logger: logger,
	}
	if db != nil {
		s.mfaStore = auth.NewMFAStore(db.DB())
	}
	return s
}

func (s *APIServer) WithAlertEngine(e *alerting.Engine) *APIServer {
	s.alertEngine = e
	return s
}

func (s *APIServer) WithVulnEngine(e *inventory.VulnerabilityEngine) *APIServer {
	s.vulnEngine = e
	return s
}

func (s *APIServer) WithRecordingStore(rs *remote.RecordingStore) *APIServer {
	s.recordingStore = rs
	return s
}

func (s *APIServer) WithStorageBackend(sb storage.Backend) *APIServer {
	s.storageBackend = sb
	return s
}

func (s *APIServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/enroll", s.handleEnroll)

	mux.HandleFunc("GET /api/v1/metrics", s.handleQueryMetrics)
	mux.HandleFunc("GET /api/v1/devices/{tenantID}/{deviceID}/metrics/{metricName}", s.handleDeviceMetrics)
	mux.HandleFunc("GET /api/v1/heartbeat/{tenantID}/{deviceID}", s.handleGetHeartbeat)

	mux.HandleFunc("GET /api/v1/alerts/{tenantID}", s.handleListActiveAlerts)
	mux.HandleFunc("GET /api/v1/alerts/{tenantID}/history", s.handleAlertHistory)
	mux.HandleFunc("POST /api/v1/alerts/{alertID}/acknowledge", s.handleAcknowledgeAlert)

	mux.HandleFunc("POST /api/v1/rules/{tenantID}", s.handleCreateRule)
	mux.HandleFunc("GET /api/v1/rules/{tenantID}", s.handleListRules)
	mux.HandleFunc("DELETE /api/v1/rules/{tenantID}/{ruleID}", s.handleDeleteRule)

	mux.HandleFunc("GET /api/v1/vulnerabilities/device/{deviceID}", s.handleDeviceVulnerabilities)
	mux.HandleFunc("GET /api/v1/vulnerabilities/tenant/{tenantID}", s.handleTenantVulnerabilities)
	mux.HandleFunc("POST /api/v1/mfa/enroll/{userID}", s.handleMFAEnroll)
	mux.HandleFunc("POST /api/v1/mfa/verify/{userID}", s.handleMFAVerify)
	mux.HandleFunc("GET /api/v1/mfa/status/{userID}", s.handleMFAStatus)
	mux.HandleFunc("DELETE /api/v1/mfa/{userID}", s.handleMFADisable)

	mux.HandleFunc("GET /api/v1/recordings/{tenantID}", s.handleListRecordings)
	mux.HandleFunc("GET /api/v1/recordings/{id}/playback", s.handlePlaybackRecording)
	mux.HandleFunc("DELETE /api/v1/recordings/{id}", s.handleDeleteRecording)

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

// --- Alert API Handlers ---

func (s *APIServer) handleListActiveAlerts(w http.ResponseWriter, r *http.Request) {
	if s.alertEngine == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "alerting not enabled"})
		return
	}
	tenantID := r.PathValue("tenantID")
	alerts, err := s.alertEngine.GetActiveAlerts(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if alerts == nil {
		alerts = []*alerting.Alert{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"alerts": alerts, "count": len(alerts)})
}

func (s *APIServer) handleAlertHistory(w http.ResponseWriter, r *http.Request) {
	if s.alertEngine == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "alerting not enabled"})
		return
	}
	tenantID := r.PathValue("tenantID")
	limit := intQueryParam(r, "limit", 50)
	offset := intQueryParam(r, "offset", 0)
	alerts, err := s.alertEngine.GetAlertHistory(r.Context(), tenantID, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if alerts == nil {
		alerts = []*alerting.Alert{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"alerts": alerts, "count": len(alerts)})
}

func (s *APIServer) handleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	if s.alertEngine == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "alerting not enabled"})
		return
	}
	alertID := r.PathValue("alertID")
	if err := s.alertEngine.AcknowledgeAlert(r.Context(), alertID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "acknowledged"})
}

func (s *APIServer) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	if s.alertEngine == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "alerting not enabled"})
		return
	}
	tenantID := r.PathValue("tenantID")
	var rule alerting.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid rule"})
		return
	}
	rule.TenantID = tenantID
	if err := s.alertEngine.AddRule(r.Context(), &rule); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (s *APIServer) handleListRules(w http.ResponseWriter, r *http.Request) {
	if s.alertEngine == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "alerting not enabled"})
		return
	}
	tenantID := r.PathValue("tenantID")
	rules, err := s.alertEngine.ListRules(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if rules == nil {
		rules = []*alerting.Rule{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"rules": rules, "count": len(rules)})
}

func (s *APIServer) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	if s.alertEngine == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "alerting not enabled"})
		return
	}
	ruleID := r.PathValue("ruleID")
	if err := s.alertEngine.RemoveRule(r.Context(), ruleID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func intQueryParam(r *http.Request, name string, defaultVal int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return defaultVal
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return defaultVal
	}
	return n
}

// Vulnerability API handlers

func (s *APIServer) handleDeviceVulnerabilities(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	if s.vulnEngine == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "vulnerability engine not enabled"})
		return
	}
	vulns, err := s.vulnEngine.GetDeviceVulnerabilities(r.Context(), deviceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"vulnerabilities": vulns})
}

func (s *APIServer) handleTenantVulnerabilities(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if s.vulnEngine == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "vulnerability engine not enabled"})
		return
	}
	vulns, err := s.vulnEngine.GetTenantVulnerabilities(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"vulnerabilities": vulns})
}

func (s *APIServer) handleDeviceInventory(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	if s.db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not available"})
		return
	}
	inventoryStore := inventory.NewStore(s.db.DB())
	inv, err := inventoryStore.GetDevice(deviceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (s *APIServer) handleListRecordings(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if s.recordingStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recording store not available"})
		return
	}

	recordings, err := s.recordingStore.ListByTenant(tenantID, 50, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"recordings": recordings})
}

func (s *APIServer) handlePlaybackRecording(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.recordingStore == nil || s.storageBackend == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recording service not available"})
		return
	}

	rec, err := s.recordingStore.GetByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording not found"})
		return
	}

	// MFA gate: require X-MFA-Code header for playback
	if s.mfaStore != nil && rec.UserID != nil {
		mfaCode := r.Header.Get("X-MFA-Code")
		if mfaCode == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "MFA code required. Provide X-MFA-Code header.",
			})
			return
		}
		secret, err := s.mfaStore.GetByUserID(*rec.UserID)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "MFA not configured for user"})
			return
		}
		if !secret.Enabled {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "MFA not enabled for user"})
			return
		}
		valid, err := s.totp.ValidateCode(secret.Secret, mfaCode, time.Now())
		if err != nil || !valid {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid MFA code"})
			return
		}
	}

	url, err := s.storageBackend.PresignedURL(r.Context(), rec.StorageKey, storage.PresignedOptions{
		Method:      "GET",
		Expiry:      time.Hour,
		ContentType: "video/x-matroska",
		Disposition: "inline",
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "generate playback URL"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"recording":    rec,
		"playback_url": url,
	})
}

func (s *APIServer) handleMFAEnroll(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if s.mfaStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "MFA store not available"})
		return
	}

	secret, err := s.totp.GenerateSecret()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "generate secret"})
		return
	}

	uri := s.totp.ProvisioningURI(secret, userID+"@strata-rmm", "Strata RMM")

	if err := s.mfaStore.Create(userID, "", secret); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save secret"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"secret":            secret,
		"provisioning_uri":  uri,
		"qr_code_url":       "https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=" + url.QueryEscape(uri),
	})
}

func (s *APIServer) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if s.mfaStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "MFA store not available"})
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code required"})
		return
	}

	secret, err := s.mfaStore.GetByUserID(userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "MFA not configured"})
		return
	}

	valid, err := s.totp.ValidateCode(secret.Secret, req.Code, time.Now())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "validate code"})
		return
	}
	if !valid {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid code"})
		return
	}

	if !secret.Enabled {
		secret.Enabled = true
		s.mfaStore.Create(secret.UserID, secret.TenantID, secret.Secret)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"verified": true,
		"message":  "MFA enabled successfully",
	})
}

func (s *APIServer) handleMFAStatus(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if s.mfaStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "MFA store not available"})
		return
	}

	enabled, err := s.mfaStore.IsEnabled(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id": userID,
		"enabled": enabled,
	})
}

func (s *APIServer) handleMFADisable(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if s.mfaStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "MFA store not available"})
		return
	}

	if err := s.mfaStore.Disable(userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

func (s *APIServer) handleDeleteRecording(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.recordingStore == nil || s.storageBackend == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recording service not available"})
		return
	}

	rec, err := s.recordingStore.GetByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording not found"})
		return
	}

	if err := s.storageBackend.Delete(r.Context(), rec.StorageKey); err != nil {
		s.logger.Warn("failed to delete recording from storage", zap.String("key", rec.StorageKey), zap.Error(err))
	}

	if err := s.recordingStore.Delete(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete recording"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
