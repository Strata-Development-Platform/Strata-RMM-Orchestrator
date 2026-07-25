package platform

import (
	"context"
	"database/sql"
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
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/encrypt"
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
	cveSync        *inventory.CVESyncEngine
	thirdParty     *inventory.ThirdPartyEngine
	updateMgr      *UpdateManager
	keyStore       *encrypt.KeyStore
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
		s.keyStore = encrypt.NewKeyStore(db.DB())
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

func (s *APIServer) WithCVESyncEngine(e *inventory.CVESyncEngine) *APIServer {
	s.cveSync = e
	return s
}

func (s *APIServer) WithThirdPartyEngine(e *inventory.ThirdPartyEngine) *APIServer {
	s.thirdParty = e
	return s
}

func (s *APIServer) WithUpdateManager(mgr *UpdateManager) *APIServer {
	s.updateMgr = mgr
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
	mux.HandleFunc("POST /api/v1/agent/register", s.handleAgentRegister)

	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("GET /api/v1/auth/me", s.handleMe)

	mux.HandleFunc("GET /api/v1/platform/overview", s.handlePlatformOverview)
	mux.HandleFunc("GET /api/v1/platform/customers", s.handlePlatformCustomers)
	mux.HandleFunc("GET /api/v1/platform/customers/{tenantID}/devices", s.handleTenantDevices)
	mux.HandleFunc("GET /api/v1/platform/customers/{tenantID}/devices/{deviceID}", s.handleDeviceInventory)

	mux.HandleFunc("GET /api/v1/admin/users", s.handleAdminUsers)
	mux.HandleFunc("POST /api/v1/admin/users", s.handleAdminCreateUser)
	mux.HandleFunc("PUT /api/v1/admin/users/{userID}/tenants", s.handleAdminUpdateUserTenants)
	mux.HandleFunc("POST /api/v1/admin/customers", s.handleAdminCreateCustomer)

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
	mux.HandleFunc("GET /api/v1/vulnerabilities/tenant/{tenantID}/summary", s.handleVulnerabilitySummary)
	mux.HandleFunc("POST /api/v1/vulnerabilities/{vulnID}/resolve", s.handleResolveVulnerability)
	mux.HandleFunc("POST /api/v1/vulnerabilities/{vulnID}/ignore", s.handleIgnoreVulnerability)

	mux.HandleFunc("GET /api/v1/cve/stats", s.handleCVEDBStats)
	mux.HandleFunc("POST /api/v1/cve/sync", s.handleCVESync)
	mux.HandleFunc("GET /api/v1/cve/packages", s.handleCVEPackages)
	mux.HandleFunc("POST /api/v1/cve/packages", s.handleCVEAddPackage)
	mux.HandleFunc("DELETE /api/v1/cve/packages/{name}/{ecosystem}", s.handleCVEDeletePackage)
	mux.HandleFunc("GET /api/v1/cve/sync/status", s.handleCVESyncStatus)
	mux.HandleFunc("GET /api/v1/cve/package/{name}", s.handleCVEPackage)

	mux.HandleFunc("GET /api/v1/thirdparty/apps", s.handleThirdPartyApps)
	mux.HandleFunc("GET /api/v1/thirdparty/packages", s.handleThirdPartyPackages)
	mux.HandleFunc("POST /api/v1/thirdparty/sync", s.handleThirdPartySync)
	mux.HandleFunc("POST /api/v1/thirdparty/sync/{app}", s.handleThirdPartySyncApp)

	mux.HandleFunc("GET /api/v1/reports/{tenantID}", s.handleListReports)
	mux.HandleFunc("POST /api/v1/reports/{tenantID}/schedules", s.handleCreateSchedule)
	mux.HandleFunc("GET /api/v1/reports/{tenantID}/schedules", s.handleListSchedules)
	mux.HandleFunc("DELETE /api/v1/reports/{tenantID}/schedules/{scheduleID}", s.handleDeleteSchedule)
	mux.HandleFunc("POST /api/v1/reports/{tenantID}/generate", s.handleGenerateReport)

	mux.HandleFunc("POST /api/v1/remote/{tenantID}/session", s.handleRemoteSessionStart)
	mux.HandleFunc("DELETE /api/v1/remote/{tenantID}/session/{sessionID}", s.handleRemoteSessionStop)

	mux.HandleFunc("GET /api/v1/admin/update/check", s.handleUpdateCheck)
	mux.HandleFunc("POST /api/v1/admin/update/apply", s.handleUpdateApply)

	mux.HandleFunc("POST /api/v1/keys/{tenantID}", s.handleCreateKey)
	mux.HandleFunc("GET /api/v1/keys/{tenantID}", s.handleListKeys)
	mux.HandleFunc("GET /api/v1/keys/{tenantID}/active", s.handleGetActiveKey)
	mux.HandleFunc("POST /api/v1/keys/{tenantID}/rotate", s.handleRotateKey)
	mux.HandleFunc("DELETE /api/v1/keys/{tenantID}/{keyID}", s.handleRevokeKey)

	mux.HandleFunc("GET /api/v1/access/audit/{tenantID}", s.handleAuditLog)
	mux.HandleFunc("GET /api/v1/access/users/{tenantID}", s.handleTenantUsers)
	mux.HandleFunc("GET /api/v1/access/permissions/{tenantID}", s.handleTenantPermissions)

	mux.HandleFunc("GET /api/v1/scripts/{tenantID}", s.handleListScripts)
	mux.HandleFunc("POST /api/v1/scripts/{tenantID}", s.handleCreateScript)
	mux.HandleFunc("GET /api/v1/scripts/{tenantID}/{scriptID}", s.handleGetScript)
	mux.HandleFunc("DELETE /api/v1/scripts/{tenantID}/{scriptID}", s.handleDeleteScript)
	mux.HandleFunc("POST /api/v1/scripts/{tenantID}/{scriptID}/run", s.handleRunScript)
	mux.HandleFunc("GET /api/v1/scripts/{tenantID}/executions", s.handleScriptExecutions)
	mux.HandleFunc("GET /api/v1/scripts/{tenantID}/executions/{execID}", s.handleGetExecution)

	mux.HandleFunc("GET /api/v1/software/packages/{tenantID}", s.handleListPackages)
	mux.HandleFunc("POST /api/v1/software/packages/{tenantID}", s.handleCreatePackage)
	mux.HandleFunc("DELETE /api/v1/software/packages/{tenantID}/{pkgID}", s.handleDeletePackage)
	mux.HandleFunc("POST /api/v1/software/deployments/{tenantID}", s.handleCreateDeployment)
	mux.HandleFunc("GET /api/v1/software/deployments/{tenantID}", s.handleListDeployments)
	mux.HandleFunc("GET /api/v1/software/deployments/{tenantID}/{deployID}", s.handleGetDeployment)

	mux.HandleFunc("POST /api/v1/mfa/enroll/{userID}", s.handleMFAEnroll)
	mux.HandleFunc("POST /api/v1/mfa/verify/{userID}", s.handleMFAVerify)
	mux.HandleFunc("GET /api/v1/mfa/status/{userID}", s.handleMFAStatus)
	mux.HandleFunc("DELETE /api/v1/mfa/{userID}", s.handleMFADisable)

	mux.HandleFunc("GET /api/v1/recordings/{tenantID}", s.handleListRecordings)
	mux.HandleFunc("GET /api/v1/recordings/{id}/playback", s.handlePlaybackRecording)
	mux.HandleFunc("DELETE /api/v1/recordings/{id}", s.handleDeleteRecording)

	rateLimiter := auth.NewRateLimiter(30, 60)

	handler := rateLimiter.Middleware(
		auth.SecurityHeaders(
			withLogging(mux, s.logger),
		),
	)

	if s.nats != nil {
		sub, err := s.nats.Subscribe("tenant.>.agent.>.script.result", s.handleScriptResultNATS)
		if err != nil {
			s.logger.Warn("subscribe script results", zap.Error(err))
		} else {
			s.logger.Info("subscribed to script results")
			defer sub.Unsubscribe()
		}

		swSub, err := s.nats.Subscribe("tenant.>.agent.>.software.result", s.handleSoftwareResultNATS)
		if err != nil {
			s.logger.Warn("subscribe software results", zap.Error(err))
		} else {
			s.logger.Info("subscribed to software results")
			defer swSub.Unsubscribe()
		}
	}

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      auth.MaxBodySize(10 << 20)(handler),
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

func (s *APIServer) handleVulnerabilitySummary(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if s.vulnEngine == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "vulnerability engine not enabled"})
		return
	}
	count, err := s.vulnEngine.GetOpenVulnerabilityCount(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	summary, err := s.vulnEngine.GetRemediationSummary(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"open_count": count,
		"summary":    summary,
	})
}

func (s *APIServer) handleResolveVulnerability(w http.ResponseWriter, r *http.Request) {
	vulnID := r.PathValue("vulnID")
	if s.vulnEngine == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "vulnerability engine not enabled"})
		return
	}
	if err := s.vulnEngine.ResolveVulnerability(r.Context(), vulnID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

func (s *APIServer) handleIgnoreVulnerability(w http.ResponseWriter, r *http.Request) {
	vulnID := r.PathValue("vulnID")
	if s.vulnEngine == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "vulnerability engine not enabled"})
		return
	}
	if err := s.vulnEngine.IgnoreVulnerability(r.Context(), vulnID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
}

func (s *APIServer) handleTenantDevices(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if s.db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not available"})
		return
	}
	store := inventory.NewStore(s.db.DB())
	devices, err := store.ListDevices(tenantID, 100, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": devices})
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

func (s *APIServer) handleThirdPartyApps(w http.ResponseWriter, _ *http.Request) {
	if s.thirdParty == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "third-party engine not available"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"apps": s.thirdParty.ListApps()})
}

func (s *APIServer) handleThirdPartyPackages(w http.ResponseWriter, r *http.Request) {
	if s.thirdParty == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "third-party engine not available"})
		return
	}
	pkgs, err := s.thirdParty.GetPackages(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"packages": pkgs})
}

func (s *APIServer) handleThirdPartySync(w http.ResponseWriter, r *http.Request) {
	if s.thirdParty == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "third-party engine not available"})
		return
	}
	go s.thirdParty.SyncAll(r.Context())
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync triggered"})
}

func (s *APIServer) handleThirdPartySyncApp(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	if s.thirdParty == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "third-party engine not available"})
		return
	}
	result, err := s.thirdParty.SyncApp(r.Context(), app)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"result": result})
}

func (s *APIServer) handleCVEDBStats(w http.ResponseWriter, r *http.Request) {
	if s.cveSync == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "CVE sync not available"})
		return
	}
	count, err := s.cveSync.GetCVECount(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"cve_count": count})
}

func (s *APIServer) handleCVESync(w http.ResponseWriter, r *http.Request) {
	if s.cveSync == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "CVE sync not available"})
		return
	}
	go s.cveSync.Sync(r.Context())
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync triggered"})
}

func (s *APIServer) handleCVEPackages(w http.ResponseWriter, r *http.Request) {
	if s.cveSync == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "CVE sync not available"})
		return
	}
	packages, err := s.cveSync.ListTrackedPackages(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"packages": packages})
}

func (s *APIServer) handleCVEAddPackage(w http.ResponseWriter, r *http.Request) {
	if s.cveSync == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "CVE sync not available"})
		return
	}
	var req struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	if req.Ecosystem == "" {
		req.Ecosystem = "Debian"
	}
	if err := s.cveSync.AddTrackedPackage(r.Context(), req.Name, req.Ecosystem); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

func (s *APIServer) handleCVEDeletePackage(w http.ResponseWriter, r *http.Request) {
	if s.cveSync == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "CVE sync not available"})
		return
	}
	name := r.PathValue("name")
	ecosystem := r.PathValue("ecosystem")
	if err := s.cveSync.RemoveTrackedPackage(r.Context(), name, ecosystem); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *APIServer) handleCVESyncStatus(w http.ResponseWriter, r *http.Request) {
	if s.cveSync == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "CVE sync not available"})
		return
	}
	states, err := s.cveSync.GetSyncState(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sync_states": states})
}

func (s *APIServer) handleCVEPackage(w http.ResponseWriter, r *http.Request) {
	if s.cveSync == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "CVE sync not available"})
		return
	}
	name := r.PathValue("name")
	cves, err := s.cveSync.GetCVEByPackage(r.Context(), name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"cves": cves})
}

func (s *APIServer) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if s.keyStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key store not available"})
		return
	}
	var req struct {
		KeyAlias    string `json:"key_alias"`
		KMSProvider string `json:"kms_type"`
		Encryption  string `json:"encryption"`
		Region      string `json:"region"`
		Endpoint    string `json:"endpoint"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	key, err := s.keyStore.CreateKey(r.Context(), tenantID, encrypt.CreateKeyOptions{
		KeyAlias:    req.KeyAlias,
		KMSProvider: encrypt.KMSProvider(req.KMSProvider),
		Encryption:  encrypt.EncryptionScheme(req.Encryption),
		Region:      req.Region,
		Endpoint:    req.Endpoint,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, key)
}

func (s *APIServer) handleListKeys(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if s.keyStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key store not available"})
		return
	}
	keys, err := s.keyStore.ListKeys(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"keys": keys})
}

func (s *APIServer) handleGetActiveKey(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if s.keyStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key store not available"})
		return
	}
	key, err := s.keyStore.GetActiveKey(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	key.KeyMaterial = nil
	writeJSON(w, http.StatusOK, key)
}

func (s *APIServer) handleRotateKey(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if s.keyStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key store not available"})
		return
	}
	key, err := s.keyStore.RotateKey(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, key)
}

func (s *APIServer) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	keyID := r.PathValue("keyID")
	if s.keyStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key store not available"})
		return
	}
	if err := s.keyStore.RevokeKey(r.Context(), keyID, tenantID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *APIServer) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if s.db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not available"})
		return
	}

	limit := 50
	offset := 0

	rows, err := s.db.DB().QueryContext(r.Context(), `
		SELECT id, tenant_id, user_id, action, resource, details, ip_address, created_at
		FROM audit_log
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type AuditEntry struct {
		ID        string          `json:"id"`
		TenantID  string          `json:"tenant_id"`
		UserID    *string         `json:"user_id,omitempty"`
		Action    string          `json:"action"`
		Resource  string          `json:"resource"`
		Details   json.RawMessage `json:"details,omitempty"`
		IPAddress string          `json:"ip_address,omitempty"`
		CreatedAt time.Time       `json:"created_at"`
	}

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var userID sql.NullString
		var ipAddr sql.NullString
		var details sql.NullString

		if err := rows.Scan(&e.ID, &e.TenantID, &userID, &e.Action, &e.Resource, &details, &ipAddr, &e.CreatedAt); err != nil {
			continue
		}
		if userID.Valid {
			e.UserID = &userID.String
		}
		if ipAddr.Valid {
			e.IPAddress = ipAddr.String
		}
		if details.Valid {
			e.Details = json.RawMessage(details.String)
		}
		entries = append(entries, e)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"audit_entries": entries})
}

func (s *APIServer) handleTenantUsers(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if s.db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not available"})
		return
	}

	rows, err := s.db.DB().QueryContext(r.Context(), `
		SELECT id, email, role, is_active, last_login, created_at
		FROM users WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var id, email, role string
		var isActive bool
		var createdAt time.Time
		var lastLoginNull sql.NullTime

		if err := rows.Scan(&id, &email, &role, &isActive, &lastLoginNull, &createdAt); err != nil {
			continue
		}
		user := map[string]interface{}{
			"id":         id,
			"email":      email,
			"role":       role,
			"is_active":  isActive,
			"created_at": createdAt,
		}
		if lastLoginNull.Valid {
			user["last_login"] = lastLoginNull.Time
		}
		users = append(users, user)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": users})
}

func (s *APIServer) handleTenantPermissions(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if s.db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not available"})
		return
	}

	rows, err := s.db.DB().QueryContext(r.Context(), `
		SELECT id, tenant_id, role, resource, action
		FROM permissions
		WHERE tenant_id = $1
		ORDER BY role, resource
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var perms []map[string]interface{}
	for rows.Next() {
		var id, tenantIDStr, role, resource, action string
		if err := rows.Scan(&id, &tenantIDStr, &role, &resource, &action); err != nil {
			continue
		}
		perms = append(perms, map[string]interface{}{
			"id":       id,
			"role":     role,
			"resource": resource,
			"action":   action,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"permissions": perms})
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
