package probe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewRedfishClient verifies client creation.
func TestNewRedfishClient(t *testing.T) {
	target := RedfishTarget{
		Host:     "192.168.1.100",
		Port:     443,
		Username: "admin",
		Password: "secret",
		Protocol: "https",
		Timeout:  30 * time.Second,
	}

	client := NewRedfishClient(target)
	if client == nil {
		t.Fatal("NewRedfishClient returned nil")
	}
	if client.baseURL != "https://192.168.1.100:443" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "https://192.168.1.100:443")
	}
	if client.username != "admin" {
		t.Errorf("username = %q, want %q", client.username, "admin")
	}
}

// TestRedfishClientDefaults verifies default port and protocol.
func TestRedfishClientDefaults(t *testing.T) {
	target := RedfishTarget{
		Host: "192.168.1.100",
	}

	client := NewRedfishClient(target)
	if client.baseURL != "https://192.168.1.100:443" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "https://192.168.1.100:443")
	}
}

// TestRedfishClientHTTP verifies HTTP (not HTTPS) works.
func TestRedfishClientHTTP(t *testing.T) {
	target := RedfishTarget{
		Host:     "192.168.1.100",
		Port:     80,
		Username: "admin",
		Password: "secret",
		Protocol: "http",
	}

	client := NewRedfishClient(target)
	if client.baseURL != "http://192.168.1.100:80" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "http://192.168.1.100:80")
	}
}

// TestRedfishClientSystem verifies the System() method with a mock server.
func TestRedfishClientSystem(t *testing.T) {
	// Create mock Redfish server
	mockData := map[string]interface{}{
		"Id":          "System.1",
		"HostName":    "server01.example.com",
		"Model":       "PowerEdge R750",
		"Manufacturer": "Dell Inc.",
		"SerialNumber": "ABC123",
		"PowerState":  "On",
		"Health":      "OK",
		"Processors": []interface{}{
			map[string]interface{}{
				"Name": "Intel Xeon Gold 6248R",
			},
		},
		"Memory": []interface{}{
			map[string]interface{}{"CapacityMiB": 131072},
			map[string]interface{}{"CapacityMiB": 131072},
		},
		"EthernetInterfaces": []interface{}{
			map[string]interface{}{"Name": "NIC.Installed.1"},
			map[string]interface{}{"Name": "NIC.Installed.2"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/Systems/" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockData)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Parse mock server URL to get host and port
	target := RedfishTarget{
		Host:     "127.0.0.1",
		Port:     0, // Will be set from server URL
		Username: "admin",
		Password: "secret",
		Protocol: "http",
		Timeout:  5 * time.Second,
	}

	// Extract port from mock server URL (format: http://127.0.0.1:PORT)
	ctx := context.Background()
	client := NewRedfishClient(target)
	client.baseURL = server.URL

	resp, err := client.System(ctx)
	if err != nil {
		t.Fatalf("System() error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
	if resp.Error != "" {
		t.Errorf("error = %q, want empty", resp.Error)
	}
}

// TestRedfishClientErrorResponse verifies handling of error responses.
func TestRedfishClientErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	target := RedfishTarget{
		Host:     "127.0.0.1",
		Port:     0,
		Username: "admin",
		Password: "wrong",
		Protocol: "http",
	}

	client := NewRedfishClient(target)
	client.baseURL = server.URL

	ctx := context.Background()
	resp, err := client.System(ctx)
	if err != nil {
		t.Fatalf("System() error: %v", err)
	}
	if resp.Status == "ok" {
		t.Error("expected error status, got ok")
	}
}

// TestParseSystemResponse verifies parsing of system info.
func TestParseSystemResponse(t *testing.T) {
	data := map[string]interface{}{
		"HostName":       "server01.example.com",
		"Model":          "PowerEdge R750",
		"Manufacturer":   "Dell Inc.",
		"SerialNumber":   "ABC123",
		"SKU":            "SKU-001",
		"PowerState":     "On",
		"Health":         "OK",
		"Processors": []interface{}{
			map[string]interface{}{"Name": "Intel Xeon Gold 6248R"},
			map[string]interface{}{"Name": "Intel Xeon Gold 6248R"},
		},
		"Memory": []interface{}{
			map[string]interface{}{"CapacityMiB": 131072},
			map[string]interface{}{"CapacityMiB": 131072},
			map[string]interface{}{"CapacityMiB": 131072},
		},
		"EthernetInterfaces": []interface{}{
			map[string]interface{}{"Name": "NIC.Installed.1"},
		},
	}

	info := ParseSystemResponse(data)

	if info.Hostname != "server01.example.com" {
		t.Errorf("Hostname = %q, want %q", info.Hostname, "server01.example.com")
	}
	if info.Model != "PowerEdge R750" {
		t.Errorf("Model = %q, want %q", info.Model, "PowerEdge R750")
	}
	if info.Manufacturer != "Dell Inc." {
		t.Errorf("Manufacturer = %q, want %q", info.Manufacturer, "Dell Inc.")
	}
	if info.Serial != "ABC123" {
		t.Errorf("Serial = %q, want %q", info.Serial, "ABC123")
	}
	if info.CPUCount != 2 {
		t.Errorf("CPUCount = %d, want 2", info.CPUCount)
	}
	if info.CPUModel != "Intel Xeon Gold 6248R" {
		t.Errorf("CPUModel = %q, want %q", info.CPUModel, "Intel Xeon Gold 6248R")
	}
	if info.NetworkInterfaces != 1 {
		t.Errorf("NetworkInterfaces = %d, want 1", info.NetworkInterfaces)
	}
}

// TestParseSystemResponseEmpty verifies empty data handling.
func TestParseSystemResponseEmpty(t *testing.T) {
	info := ParseSystemResponse(map[string]interface{}{})
	if info.Hostname != "" {
		t.Errorf("expected empty Hostname, got %q", info.Hostname)
	}
	if info.CPUCount != 0 {
		t.Errorf("expected CPUCount = 0, got %d", info.CPUCount)
	}
}

// TestParseHealthResponse verifies health status parsing.
func TestParseHealthResponse(t *testing.T) {
	data := map[string]interface{}{
		"Health":     "OK",
		"PowerState": "On",
		"Members": []interface{}{
			map[string]interface{}{"Health": "OK"},
			map[string]interface{}{"Health": "Critical"},
			map[string]interface{}{"Health": "OK"},
		},
	}

	status := ParseHealthResponse(data)
	if status.OverallHealth != "OK" {
		t.Errorf("OverallHealth = %q, want %q", status.OverallHealth, "OK")
	}
	if status.PowerState != "On" {
		t.Errorf("PowerState = %q, want %q", status.PowerState, "On")
	}
	if status.DeviceCount != 3 {
		t.Errorf("DeviceCount = %d, want 3", status.DeviceCount)
	}
	if status.Faults != 1 {
		t.Errorf("Faults = %d, want 1", status.Faults)
	}
}

// TestGetSystemInfoJSONRoundTrip verifies SystemInfo JSON serialization.
func TestGetSystemInfoJSONRoundTrip(t *testing.T) {
	info := SystemInfo{
		Hostname:          "server01",
		Model:             "R750",
		Manufacturer:      "Dell",
		Serial:            "ABC123",
		SKU:               "SKU-001",
		PowerState:        "On",
		Health:            "OK",
		CPUModel:          "Xeon Gold",
		CPUCount:          2,
		MemoryTotalMB:     262144,
		NetworkInterfaces: 2,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var parsed SystemInfo
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if parsed.Hostname != info.Hostname {
		t.Errorf("Hostname = %q, want %q", parsed.Hostname, info.Hostname)
	}
	if parsed.MemoryTotalMB != info.MemoryTotalMB {
		t.Errorf("MemoryTotalMB = %d, want %d", parsed.MemoryTotalMB, info.MemoryTotalMB)
	}
}

// TestHealthStatusJSONRoundTrip verifies HealthStatus JSON serialization.
func TestHealthStatusJSONRoundTrip(t *testing.T) {
	status := HealthStatus{
		OverallHealth: "Critical",
		PowerState:    "On",
		DeviceCount:   5,
		Faults:        2,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var parsed HealthStatus
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if parsed.OverallHealth != status.OverallHealth {
		t.Errorf("OverallHealth = %q, want %q", parsed.OverallHealth, status.OverallHealth)
	}
	if parsed.Faults != status.Faults {
		t.Errorf("Faults = %d, want %d", parsed.Faults, status.Faults)
	}
}

// TestRedfishClientTimeout verifies timeout configuration.
func TestRedfishClientTimeout(t *testing.T) {
	target := RedfishTarget{
		Host:    "192.168.1.100",
		Port:    443,
		Timeout: 10 * time.Second,
	}

	client := NewRedfishClient(target)
	if client.client.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want %v", client.client.Timeout, 10*time.Second)
	}
}

// TestRedfishClientContextCancellation verifies context cancellation.
func TestRedfishClientContextCancellation(t *testing.T) {
	target := RedfishTarget{
		Host:    "192.168.1.100",
		Port:    443,
		Timeout: 1 * time.Second,
	}

	client := NewRedfishClient(target)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	resp, err := client.System(ctx)
	if err != nil {
		t.Fatalf("System() error: %v", err)
	}
	// Response may still be returned (error captured in resp)
	if resp == nil {
		t.Fatal("response should not be nil")
	}
}
