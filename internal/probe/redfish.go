package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RedfishResult represents a Redfish API query result.
type RedfishResult struct {
	Endpoint string                 `json:"endpoint"`
	Status   string                 `json:"status"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Error    string                 `json:"error,omitempty"`
}

// RedfishTarget is a Redfish management controller (iDRAC, iLO, XCC, etc.).
type RedfishTarget struct {
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	Protocol  string    `json:"protocol"` // https, http
	Timeout   time.Duration `json:"timeout"`
}

// RedfishClient communicates with Redfish management controllers.
type RedfishClient struct {
	client    *http.Client
	baseURL   string
	username  string
	password  string
}

// NewRedfishClient creates a new Redfish client.
func NewRedfishClient(target RedfishTarget) *RedfishClient {
	protocol := target.Protocol
	if protocol == "" {
		protocol = "https"
	}
	port := target.Port
	if port == 0 {
		port = 443
	}
	timeout := target.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &RedfishClient{
		client: &http.Client{
			Timeout: timeout,
		},
		baseURL:  fmt.Sprintf("%s://%s:%d", protocol, target.Host, port),
		username: target.Username,
		password: target.Password,
	}
}

// System retrieves system inventory from the Redfish controller.
func (c *RedfishClient) System(ctx context.Context) (*RedfishResult, error) {
	return c.get(ctx, "/redfish/v1/Systems/")
}

// Chassis retrieves chassis information.
func (c *RedfishClient) Chassis(ctx context.Context) (*RedfishResult, error) {
	return c.get(ctx, "/redfish/v1/Chassis/")
}

// Managers retrieves management controller information.
func (c *RedfishClient) Managers(ctx context.Context) (*RedfishResult, error) {
	return c.get(ctx, "/redfish/v1/Managers/")
}

// EventLog retrieves the system event log.
func (c *RedfishClient) EventLog(ctx context.Context) (*RedfishResult, error) {
	return c.get(ctx, "/redfish/v1/Systems/LogServices/")
}

// Health retrieves overall system health status.
func (c *RedfishClient) Health(ctx context.Context) (*RedfishResult, error) {
	return c.get(ctx, "/redfish/v1/Systems/")
}

// get performs a GET request to the Redfish API and returns parsed JSON.
func (c *RedfishClient) get(ctx context.Context, path string) (*RedfishResult, error) {
	url := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return &RedfishResult{
			Endpoint: url,
			Status:   "error",
			Error:    err.Error(),
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return &RedfishResult{
			Endpoint: url,
			Status:   "error",
			Error:    err.Error(),
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &RedfishResult{
			Endpoint: url,
			Status:   fmt.Sprintf("http_%d", resp.StatusCode),
			Error:    fmt.Sprintf("unexpected status: %d", resp.StatusCode),
		}, nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return &RedfishResult{
			Endpoint: url,
			Status:   "parse_error",
			Error:    err.Error(),
		}, nil
	}

	return &RedfishResult{
		Endpoint: url,
		Status:   "ok",
		Data:     data,
	}, nil
}

// ParseSystemResponse extracts key system information from a Redfish System response.
func ParseSystemResponse(data map[string]interface{}) SystemInfo {
	info := SystemInfo{
		Hostname:   getString(data, "HostName"),
		Model:      getString(data, "Model"),
		Manufacturer: getString(data, "Manufacturer"),
		Serial:     getString(data, "SerialNumber"),
		SKU:        getString(data, "SKU"),
		PowerState: getString(data, "PowerState"),
		Health:     getString(data, "Health"),
	}

	// Extract processor info
	if processors, ok := data["Processors"].([]interface{}); ok && len(processors) > 0 {
		if proc, ok := processors[0].(map[string]interface{}); ok {
			info.CPUModel = getString(proc, "Name")
			info.CPUCount = len(processors)
		}
	}

	// Extract memory info
	if memory, ok := data["Memory"].([]interface{}); ok {
		info.MemoryTotalMB = int64(len(memory)) // Approximation
	}

	// Extract network interfaces
	if netInterfaces, ok := data["EthernetInterfaces"].([]interface{}); ok {
		info.NetworkInterfaces = len(netInterfaces)
	}

	return info
}

// getString safely extracts a string value from a map.
func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

// SystemInfo represents extracted system information from Redfish.
type SystemInfo struct {
	Hostname           string `json:"hostname"`
	Model              string `json:"model"`
	Manufacturer       string `json:"manufacturer"`
	Serial             string `json:"serial"`
	SKU                string `json:"sku"`
	PowerState         string `json:"power_state"`
	Health             string `json:"health"`
	CPUModel           string `json:"cpu_model"`
	CPUCount           int    `json:"cpu_count"`
	MemoryTotalMB      int64  `json:"memory_total_mb"`
	NetworkInterfaces  int    `json:"network_interfaces"`
}

// ParseHealthResponse extracts health status from Redfish.
func ParseHealthResponse(data map[string]interface{}) HealthStatus {
	status := HealthStatus{
		OverallHealth: getString(data, "Health"),
		PowerState:    getString(data, "PowerState"),
	}

	// Check for sub-system health
	if members, ok := data["Members"].([]interface{}); ok {
		status.DeviceCount = len(members)
		for _, member := range members {
			if m, ok := member.(map[string]interface{}); ok {
				if health := getString(m, "Health"); health != "OK" && health != "" {
					status.Faults++
				}
			}
		}
	}

	return status
}

// HealthStatus represents the health status of a system.
type HealthStatus struct {
	OverallHealth string `json:"overall_health"`
	PowerState    string `json:"power_state"`
	DeviceCount   int    `json:"device_count"`
	Faults        int    `json:"faults"`
}
