package alerting

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestGroupingEngineGetGroupsBySeverity(t *testing.T) {
	grouping := NewGroupingEngine()
	now := time.Now()

	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device2", "memory", SeverityWarning, "", now)
	grouping.GetOrCreateGroup("rule3", "tenant1", "device3", "disk", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule4", "tenant1", "device4", "network", SeverityInfo, "", now)

	criticalGroups := grouping.GetGroupsBySeverity(SeverityCritical)
	if len(criticalGroups) != 2 {
		t.Fatalf("expected 2 critical groups, got %d", len(criticalGroups))
	}
	for _, g := range criticalGroups {
		if g.Severity != SeverityCritical {
			t.Fatalf("expected critical severity, got %s", g.Severity)
		}
		if g.Status != GroupActive {
			t.Fatalf("expected active status, got %s", g.Status)
		}
	}

	warningGroups := grouping.GetGroupsBySeverity(SeverityWarning)
	if len(warningGroups) != 1 {
		t.Fatalf("expected 1 warning group, got %d", len(warningGroups))
	}

	infoGroups := grouping.GetGroupsBySeverity(SeverityInfo)
	if len(infoGroups) != 1 {
		t.Fatalf("expected 1 info group, got %d", len(infoGroups))
	}

	noneGroups := grouping.GetGroupsBySeverity("none")
	if len(noneGroups) != 0 {
		t.Fatalf("expected 0 none groups, got %d", len(noneGroups))
	}
}

func TestGroupingEngineGetActiveGroupsBySeverity(t *testing.T) {
	grouping := NewGroupingEngine()
	now := time.Now()

	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device2", "memory", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule3", "tenant1", "device3", "disk", SeverityWarning, "", now)

	grouping.ResolveGroup("rule1", "device1", now.Add(time.Minute))

	activeCritical := grouping.GetActiveGroupsBySeverity(SeverityCritical)
	if activeCritical != 1 {
		t.Fatalf("expected 1 active critical group, got %d", activeCritical)
	}

	activeWarning := grouping.GetActiveGroupsBySeverity(SeverityWarning)
	if activeWarning != 1 {
		t.Fatalf("expected 1 active warning group, got %d", activeWarning)
	}
}

func TestGroupingEngineResolveGroupsByDevice(t *testing.T) {
	grouping := NewGroupingEngine()
	now := time.Now()

	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device1", "memory", SeverityWarning, "", now)
	grouping.GetOrCreateGroup("rule3", "tenant1", "device2", "disk", SeverityCritical, "", now)

	resolved := grouping.ResolveGroupsByDevice("device1")
	if resolved != 2 {
		t.Fatalf("expected 2 groups resolved for device1, got %d", resolved)
	}

	remaining := grouping.GetGroupsByDevice("device1")
	if len(remaining) != 0 {
		t.Fatalf("expected 0 groups for device1 after resolve, got %d", len(remaining))
	}

	device2Groups := grouping.GetGroupsByDevice("device2")
	if len(device2Groups) != 1 {
		t.Fatalf("expected 1 group for device2, got %d", len(device2Groups))
	}
}

func TestGroupingEngineGetDeviceAlertCount(t *testing.T) {
	grouping := NewGroupingEngine()
	now := time.Now()

	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "memory", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "disk", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device2", "network", SeverityWarning, "", now)

	count := grouping.GetDeviceAlertCount("device1")
	if count != 3 {
		t.Fatalf("expected 3 alerts for device1, got %d", count)
	}

	count2 := grouping.GetDeviceAlertCount("device2")
	if count2 != 1 {
		t.Fatalf("expected 1 alert for device2, got %d", count2)
	}
}

func TestGroupingEngineResolveAll(t *testing.T) {
	grouping := NewGroupingEngine()
	now := time.Now()

	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device2", "memory", SeverityWarning, "", now)
	grouping.GetOrCreateGroup("rule3", "tenant1", "device3", "disk", SeverityInfo, "", now)

	resolved := grouping.ResolveAll()
	if resolved != 3 {
		t.Fatalf("expected 3 groups resolved, got %d", resolved)
	}

	allGroups := grouping.GetAllGroups()
	for _, g := range allGroups {
		if g.Status != GroupResolved {
			t.Fatalf("expected all groups resolved, got status %s", g.Status)
		}
	}
}

func TestGroupingEngineGetCascadeGroups(t *testing.T) {
	grouping := NewGroupingEngine()
	now := time.Now()

	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device1", "memory", SeverityWarning, "", now)
	grouping.GetOrCreateGroup("rule3", "tenant1", "device2", "disk", SeverityCritical, "", now)

	cascadeGroups := grouping.GetCascadeGroups("tenant1")
	if len(cascadeGroups) != 2 {
		t.Fatalf("expected 2 cascade groups, got %d", len(cascadeGroups))
	}

	device1Cascades := 0
	device2Cascades := 0
	for _, cg := range cascadeGroups {
		if cg.Key.DeviceID == "device1" {
			device1Cascades++
			if len(cg.Groups) != 2 {
				t.Fatalf("expected 2 groups for device1 cascade, got %d", len(cg.Groups))
			}
		}
		if cg.Key.DeviceID == "device2" {
			device2Cascades++
			if len(cg.Groups) != 1 {
				t.Fatalf("expected 1 group for device2 cascade, got %d", len(cg.Groups))
			}
		}
	}
	if device1Cascades != 1 {
		t.Fatalf("expected 1 device1 cascade, got %d", device1Cascades)
	}
	if device2Cascades != 1 {
		t.Fatalf("expected 1 device2 cascade, got %d", device2Cascades)
	}
}

func TestGroupingEngineGetTimeWindowGroups(t *testing.T) {
	grouping := NewGroupingEngine()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	grouping.now = func() time.Time { return now }

	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now.Add(-10*time.Minute))
	grouping.GetOrCreateGroup("rule2", "tenant1", "device2", "memory", SeverityWarning, "", now.Add(-45*time.Minute))
	grouping.GetOrCreateGroup("rule3", "tenant1", "device3", "disk", SeverityInfo, "", now.Add(-2*time.Hour))

	recentGroups := grouping.GetTimeWindowGroups(1 * time.Hour)
	if len(recentGroups) != 2 {
		t.Fatalf("expected 2 groups in 1h window, got %d", len(recentGroups))
	}

	oldGroups := grouping.GetTimeWindowGroups(30 * time.Minute)
	if len(oldGroups) != 1 {
		t.Fatalf("expected 1 group in 30m window, got %d", len(oldGroups))
	}
	if oldGroups[0].Key.DeviceID != "device1" {
		t.Fatalf("expected device1 in 30m window, got %s", oldGroups[0].Key.DeviceID)
	}
}

func TestGroupingEngineResolveCascadeGroup(t *testing.T) {
	grouping := NewGroupingEngine()
	now := time.Now()

	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device1", "memory", SeverityWarning, "", now)
	grouping.GetOrCreateGroup("rule3", "tenant1", "device2", "disk", SeverityCritical, "", now)

	resolved := grouping.ResolveCascadeGroup("device1")
	if resolved != 2 {
		t.Fatalf("expected 2 groups resolved for device1 cascade, got %d", resolved)
	}

	device1Groups := grouping.GetGroupsByDevice("device1")
	if len(device1Groups) != 0 {
		t.Fatalf("expected 0 groups for device1 after cascade resolve, got %d", len(device1Groups))
	}

	device2Groups := grouping.GetGroupsByDevice("device2")
	if len(device2Groups) != 1 {
		t.Fatalf("expected 1 group for device2, got %d", len(device2Groups))
	}
}

func TestGroupingEngineSeverityTracking(t *testing.T) {
	grouping := NewGroupingEngine()
	now := time.Now()

	group, created := grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	if !created {
		t.Fatal("first group should be created")
	}
	if group.Severity != SeverityCritical {
		t.Fatalf("expected SeverityCritical, got %s", group.Severity)
	}
	if group.TenantID != "tenant1" {
		t.Fatalf("expected tenant1, got %s", group.TenantID)
	}

	_, _ = grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "memory", SeverityWarning, "", now)
	group = grouping.GetGroup("rule1", "device1")
	if group.Severity != SeverityCritical {
		t.Fatalf("severity should remain critical after second metric, got %s", group.Severity)
	}
}

func TestGroupingEngineFormatGroupMessageWithSeverity(t *testing.T) {
	grouping := NewGroupingEngine()
	now := time.Now()

	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	group := grouping.GetGroup("rule1", "device1")

	msg := grouping.FormatGroupMessage(group)
	if !strings.Contains(msg, "rule1") {
		t.Fatalf("message should contain rule1, got: %s", msg)
	}
	if !strings.Contains(msg, "device1") {
		t.Fatalf("message should contain device1, got: %s", msg)
	}
	if !strings.Contains(msg, "cpu") {
		t.Fatalf("message should contain cpu, got: %s", msg)
	}
}

// API endpoint tests

type testAlertServer struct {
	store      *memoryAlertStore
	mStore     *memoryMaintenanceStore
	engine     *Engine
	httptest   *httptest.Server
	handler    http.Handler
}

func setupTestAlertServer(t *testing.T) *testAlertServer {
	t.Helper()
	store := newMemoryAlertStore()
	mStore := newMemoryMaintenanceStore()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	maintenance := NewMaintenanceEngine(mStore)
	engine := NewEngine(nil, nil, store, nil, zap.NewNop())
	engine.WithMaintenanceEngine(maintenance)
	engine.now = func() time.Time { return now }

	srv := &testAlertServer{
		store:  store,
		mStore: mStore,
		engine: engine,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/alerts/{tenantID}/groups", func(w http.ResponseWriter, r *http.Request) {
		groups := engine.GroupingEngine().GetAllGroups()
		tenantID := r.PathValue("tenantID")
		var filteredGroups []*AlertGroup
		for _, g := range groups {
			if tenantID == "" || g.TenantID == tenantID {
				filteredGroups = append(filteredGroups, g)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"groups": filteredGroups, "count": len(filteredGroups)})
	})
	mux.HandleFunc("GET /api/v1/alerts/{tenantID}/groups/severity/{severity}", func(w http.ResponseWriter, r *http.Request) {
		severity := r.PathValue("severity")
		groups := engine.GroupingEngine().GetGroupsBySeverity(Severity(severity))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"groups": groups, "count": len(groups), "severity": severity})
	})
	mux.HandleFunc("GET /api/v1/alerts/{tenantID}/groups/device/{deviceID}", func(w http.ResponseWriter, r *http.Request) {
		deviceID := r.PathValue("deviceID")
		groups := engine.GroupingEngine().GetGroupsByDevice(deviceID)
		alertCount := engine.GroupingEngine().GetDeviceAlertCount(deviceID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"groups": groups, "count": len(groups), "device_alert_count": alertCount, "device_id": deviceID})
	})
	mux.HandleFunc("POST /api/v1/alerts/{tenantID}/groups/device/{deviceID}/resolve", func(w http.ResponseWriter, r *http.Request) {
		deviceID := r.PathValue("deviceID")
		resolved := engine.GroupingEngine().ResolveGroupsByDevice(deviceID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "resolved", "resolved_count": fmt.Sprintf("%d", resolved), "device_id": deviceID})
	})
	mux.HandleFunc("POST /api/v1/alerts/{tenantID}/groups/resolve-all", func(w http.ResponseWriter, r *http.Request) {
		resolved := engine.GroupingEngine().ResolveAll()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "resolved", "resolved_count": fmt.Sprintf("%d", resolved)})
	})
	mux.HandleFunc("GET /api/v1/alerts/{tenantID}/groups/cascade", func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.PathValue("tenantID")
		cascadeGroups := engine.GroupingEngine().GetCascadeGroups(tenantID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"cascade_groups": cascadeGroups, "count": len(cascadeGroups)})
	})
	mux.HandleFunc("GET /api/v1/alerts/{tenantID}/groups/time-window/{duration}", func(w http.ResponseWriter, r *http.Request) {
		durationStr := r.PathValue("duration")
		duration, err := time.ParseDuration(durationStr)
		if err != nil || duration <= 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid duration"})
			return
		}
		groups := engine.GroupingEngine().GetTimeWindowGroups(duration)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"groups": groups, "count": len(groups), "duration": durationStr})
	})

	srv.handler = mux
	srv.httptest = httptest.NewServer(mux)
	return srv
}

func TestAPIGetAlertGroups(t *testing.T) {
	srv := setupTestAlertServer(t)
	defer srv.httptest.Close()

	grouping := srv.engine.GroupingEngine()
	now := time.Now()
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device1", "memory", SeverityWarning, "", now)

	resp, err := http.Get(srv.httptest.URL + "/api/v1/alerts/tenant1/groups")
	if err != nil {
		t.Fatalf("failed to get groups: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["count"].(float64) != 2 {
		t.Fatalf("expected count 2, got %v", result["count"])
	}
}

func TestAPIGetGroupsBySeverity(t *testing.T) {
	srv := setupTestAlertServer(t)
	defer srv.httptest.Close()

	grouping := srv.engine.GroupingEngine()
	now := time.Now()
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device2", "memory", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule3", "tenant1", "device3", "disk", SeverityWarning, "", now)

	resp, err := http.Get(srv.httptest.URL + "/api/v1/alerts/tenant1/groups/severity/critical")
	if err != nil {
		t.Fatalf("failed to get groups by severity: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["count"].(float64) != 2 {
		t.Fatalf("expected count 2 for critical, got %v", result["count"])
	}
	if result["severity"] != "critical" {
		t.Fatalf("expected severity critical, got %v", result["severity"])
	}
}

func TestAPIGetGroupsByDevice(t *testing.T) {
	srv := setupTestAlertServer(t)
	defer srv.httptest.Close()

	grouping := srv.engine.GroupingEngine()
	now := time.Now()
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device1", "memory", SeverityWarning, "", now)
	grouping.GetOrCreateGroup("rule3", "tenant1", "device2", "disk", SeverityCritical, "", now)

	resp, err := http.Get(srv.httptest.URL + "/api/v1/alerts/tenant1/groups/device/device1")
	if err != nil {
		t.Fatalf("failed to get groups by device: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["count"].(float64) != 2 {
		t.Fatalf("expected count 2 for device1, got %v", result["count"])
	}
	if result["device_alert_count"].(float64) != 2 {
		t.Fatalf("expected 2 alert count for device1, got %v", result["device_alert_count"])
	}
}

func TestAPIResolveGroupsByDevice(t *testing.T) {
	srv := setupTestAlertServer(t)
	defer srv.httptest.Close()

	grouping := srv.engine.GroupingEngine()
	now := time.Now()
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device1", "memory", SeverityWarning, "", now)
	grouping.GetOrCreateGroup("rule3", "tenant1", "device2", "disk", SeverityCritical, "", now)

	req, _ := http.NewRequest("POST", srv.httptest.URL+"/api/v1/alerts/tenant1/groups/device/device1/resolve", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to resolve groups: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "resolved" {
		t.Fatalf("expected status resolved, got %s", result["status"])
	}
	if result["resolved_count"] != "2" {
		t.Fatalf("expected 2 resolved, got %s", result["resolved_count"])
	}
}

func TestAPIResolveAllGroups(t *testing.T) {
	srv := setupTestAlertServer(t)
	defer srv.httptest.Close()

	grouping := srv.engine.GroupingEngine()
	now := time.Now()
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device2", "memory", SeverityWarning, "", now)
	grouping.GetOrCreateGroup("rule3", "tenant1", "device3", "disk", SeverityInfo, "", now)

	req, _ := http.NewRequest("POST", srv.httptest.URL+"/api/v1/alerts/tenant1/groups/resolve-all", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to resolve all: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["resolved_count"] != "3" {
		t.Fatalf("expected 3 resolved, got %s", result["resolved_count"])
	}
}

func TestAPICascadeGroups(t *testing.T) {
	srv := setupTestAlertServer(t)
	defer srv.httptest.Close()

	grouping := srv.engine.GroupingEngine()
	now := time.Now()
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device1", "memory", SeverityWarning, "", now)
	grouping.GetOrCreateGroup("rule3", "tenant1", "device2", "disk", SeverityCritical, "", now)

	resp, err := http.Get(srv.httptest.URL + "/api/v1/alerts/tenant1/groups/cascade")
	if err != nil {
		t.Fatalf("failed to get cascade groups: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["count"].(float64) != 2 {
		t.Fatalf("expected 2 cascade groups, got %v", result["count"])
	}
}

func TestAPITimeWindowGroups(t *testing.T) {
	srv := setupTestAlertServer(t)
	defer srv.httptest.Close()

	grouping := srv.engine.GroupingEngine()
	now := time.Now()
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", now)
	grouping.GetOrCreateGroup("rule2", "tenant1", "device2", "memory", SeverityWarning, "", now.Add(-30*time.Minute))

	resp, err := http.Get(srv.httptest.URL + "/api/v1/alerts/tenant1/groups/time-window/1h")
	if err != nil {
		t.Fatalf("failed to get time window groups: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["count"].(float64) != 2 {
		t.Fatalf("expected 2 groups in 1h window, got %v", result["count"])
	}
}

func TestAPITimeWindowGroupsInvalidDuration(t *testing.T) {
	srv := setupTestAlertServer(t)
	defer srv.httptest.Close()

	resp, err := http.Get(srv.httptest.URL + "/api/v1/alerts/tenant1/groups/time-window/invalid")
	if err != nil {
		t.Fatalf("failed to request invalid duration: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid duration, got %d", resp.StatusCode)
	}
}

func TestGroupingEngineGetCascadeGroupsEmpty(t *testing.T) {
	grouping := NewGroupingEngine()
	cascadeGroups := grouping.GetCascadeGroups("tenant1")
	if len(cascadeGroups) != 0 {
		t.Fatalf("expected 0 cascade groups for empty engine, got %d", len(cascadeGroups))
	}
}

func TestGroupingEngineResolveAllOnEmpty(t *testing.T) {
	grouping := NewGroupingEngine()
	resolved := grouping.ResolveAll()
	if resolved != 0 {
		t.Fatalf("expected 0 resolved on empty engine, got %d", resolved)
	}
}

func TestGroupingEngineResolveGroupsByDeviceOnEmpty(t *testing.T) {
	grouping := NewGroupingEngine()
	resolved := grouping.ResolveGroupsByDevice("nonexistent")
	if resolved != 0 {
		t.Fatalf("expected 0 resolved for nonexistent device, got %d", resolved)
	}
}
