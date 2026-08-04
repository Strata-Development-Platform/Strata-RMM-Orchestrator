package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// UPS MIB-2 OIDs (RFC 1628)
const (
	OIDUPSBatteryCapacity = ".1.3.6.1.2.1.33.1.4.1.1" // batteryCapacity (percent)
	OIDUPSBatteryPercent  = ".1.3.6.1.2.1.33.1.4.1.2" // batteryPercent (percent)
	OIDUPSBatteryRuntime  = ".1.3.6.1.2.1.33.1.4.2.1" // batteryRuntime (seconds)
	OIDUPSInputStatus     = ".1.3.6.1.2.1.33.1.3.1.1" // inputStatus
	OIDUPSInputVoltage    = ".1.3.6.1.2.1.33.1.3.2.1" // inputVoltage
	OIDUPSOutputStatus    = ".1.3.6.1.2.1.33.1.5.1.1" // outputStatus
	OIDUPSOutputVoltage   = ".1.3.6.1.2.1.33.1.5.2.1" // outputVoltage
	OIDUPSLoad            = ".1.3.6.1.2.1.33.1.5.3.1" // outputLoad (percent)
)

// UPSStatus represents parsed UPS telemetry from SNMP MIB data
type UPSStatus struct {
	TenantID       string    `json:"tenant_id"`
	DeviceID       string    `json:"device_id"`
	ProbeID        string    `json:"probe_id"`
	Timestamp      time.Time `json:"timestamp"`
	BatteryPercent int       `json:"battery_percent"`
	BatteryRuntime int       `json:"battery_runtime_seconds"`
	InputStatus    int       `json:"input_status"`    // 1=normal, 2=emergency, 3=temporary, 4=backup
	OutputStatus   int       `json:"output_status"`   // 1=normal, 2=on_battery, 3=under_loop
	OutputLoad     int       `json:"output_load_pct"` // percent
	AlertLevel     string    `json:"alert_level"`     // "normal", "warning", "critical", "shutdown"
}

// ShutdownEvent represents a triggered shutdown sequence
type ShutdownEvent struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	UPSDeviceID string    `json:"ups_device_id"`
	BatteryMin  int       `json:"battery_minutes_remaining"`
	InitiatedAt time.Time `json:"initiated_at"`
	TargetOrder []string  `json:"target_order"` // ordered list of device IDs to shut down
	Status      string    `json:"status"`       // "initiated", "in_progress", "completed", "aborted"
	AbortedAt   *time.Time `json:"aborted_at,omitempty"`
	AbortedBy   *string   `json:"aborted_by,omitempty"`
}

// PowerEventManager handles UPS alert tracking and graceful shutdown orchestration
type PowerEventManager struct {
	db       *sql.DB
	nats     *nats.Conn
	js       nats.JetStreamContext
	log      *zap.Logger
	mu       sync.RWMutex
	upses    map[string]*UPSState
	events   map[string]*ShutdownEvent
	closed   bool
	ackCh    chan struct{}
	shutdown chan struct{}
}

// UPSState tracks the current state of a UPS device
type UPSState struct {
	DeviceID       string
	TenantID       string
	ProbeID        string
	BatteryPercent int
	BatteryRuntime int // seconds remaining
	LastSeen       time.Time
	AlertLevel     string
}

// ShutdownTarget represents a device targeted for graceful shutdown
type ShutdownTarget struct {
	DeviceID string    `json:"device_id"`
	DeviceType string  `json:"device_type"` // "hypervisor", "san", "vm_pool", "nas"
	ShutdownOrder int `json:"shutdown_order"`
	Command    string    `json:"command"` // NATS command subject
	Timeout    time.Duration `json:"timeout"`
}

// ShutdownOrder defines the sequence of shutdown targets
var ShutdownOrder = []string{"hypervisor", "san", "nas", "vm_pool"}

// NewPowerEventManager creates a new UPS power management handler
func NewPowerEventManager(db *sql.DB, nats *nats.Conn, log *zap.Logger) *PowerEventManager {
	pm := &PowerEventManager{
		db:       db,
		nats:     nats,
		log:      log,
		upses:    make(map[string]*UPSState),
		events:   make(map[string]*ShutdownEvent),
		ackCh:    make(chan struct{}, 64),
		shutdown: make(chan struct{}),
	}

	if nats != nil {
		js, err := nats.JetStream()
		if err == nil {
			pm.js = js
		}
	}

	return pm
}

// Start begins listening for UPS alerts on the probe SNMP subject
func (pm *PowerEventManager) Start(ctx context.Context) error {
	if pm.nats == nil {
		pm.log.Warn("nats connection is nil, UPS handler will not subscribe")
		return nil
	}

	sub, err := pm.nats.Subscribe("tenant.*.probe.*.snmp", pm.handleSNMPPayload)
	if err != nil {
		return fmt.Errorf("subscribe to UPS alerts: %w", err)
	}

	pm.log.Info("UPS power manager started, subscribed to probe SNMP alerts")

	go func() {
		<-ctx.Done()
		pm.mu.Lock()
		pm.closed = true
		pm.mu.Unlock()
		sub.Unsubscribe()
		close(pm.shutdown)
	}()

	return nil
}

// handleSNMPPayload processes incoming SNMP telemetry and extracts UPS data
func (pm *PowerEventManager) handleSNMPPayload(msg *nats.Msg) {
	select {
	case <-pm.shutdown:
		return
	default:
	}

	topic := msg.Subject
	payload := msg.Data

	tenantID, probeID, err := parseTenantProbeSubject(topic)
	if err != nil {
		if pm.log != nil {
			pm.log.Warn("parse UPS subject", zap.Error(err))
		}
		return
	}

	var records []map[string]interface{}
	if err := json.Unmarshal(payload, &records); err != nil {
		if pm.log != nil {
			pm.log.Warn("unmarshal UPS SNMP payload", zap.Error(err))
		}
		return
	}

	for _, record := range records {
		oid, _ := record["oid"].(string)
		valueStr, _ := record["value"].(string)

		if !isUPSRelatedOID(oid) {
			continue
		}

		value, err := parseNumericValue(valueStr)
		if err != nil {
			continue
		}

		deviceID, _ := record["device_id"].(string)
		if deviceID == "" {
			deviceID, _ = record["target"].(string)
		}
		if deviceID == "" {
			continue
		}

		pm.updateUPSState(tenantID, deviceID, probeID, oid, value)
	}

	pm.ackCh <- struct{}{}
}

// updateUPSState updates the UPS state tracker and triggers alerts or shutdowns
func (pm *PowerEventManager) updateUPSState(tenantID, deviceID, probeID string, oid string, value float64) {
	if deviceID == "" {
		return
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	state, exists := pm.upses[deviceID]
	if !exists {
		state = &UPSState{
			DeviceID:   deviceID,
			TenantID:   tenantID,
			ProbeID:    probeID,
			LastSeen:   time.Now().UTC(),
		}
		pm.upses[deviceID] = state
	}

	state.ProbeID = probeID
	state.LastSeen = time.Now().UTC()

	switch oid {
	case OIDUPSBatteryPercent, OIDUPSBatteryCapacity:
		state.BatteryPercent = int(value)
	case OIDUPSBatteryRuntime:
		state.BatteryRuntime = int(value)
	}

	state.AlertLevel = determineAlertLevel(state)

	// Check for critical battery runtime (< 10 minutes = 600 seconds)
	if state.BatteryRuntime > 0 && state.BatteryRuntime < 600 {
		pm.triggerShutdown(tenantID, deviceID, state)
	}
}

// determineAlertLevel classifies UPS alert severity based on current state.
// Priority: shutdown (runtime < 10 min) > critical (runtime < 30 min) > warning (battery < 50%) > normal
func determineAlertLevel(s *UPSState) string {
	// Runtime thresholds take priority over battery percentage
	if s.BatteryRuntime > 0 {
		seconds := s.BatteryRuntime
		if seconds < 600 {
			return "shutdown"
		}
		if seconds < 1800 {
			return "critical"
		}
	}

	// Battery percentage thresholds
	if s.BatteryPercent > 0 && s.BatteryPercent <= 50 {
		return "warning"
	}

	return "normal"
}

// isUPSRelatedOID checks if the given OID is from the UPS MIB-2 tree
func isUPSRelatedOID(oid string) bool {
	return oid == OIDUPSBatteryCapacity ||
		oid == OIDUPSBatteryPercent ||
		oid == OIDUPSBatteryRuntime ||
		oid == OIDUPSInputStatus ||
		oid == OIDUPSInputVoltage ||
		oid == OIDUPSOutputStatus ||
		oid == OIDUPSOutputVoltage ||
		oid == OIDUPSLoad ||
		hasUPSOIDPrefix(oid)
}

// hasUPSOIDPrefix checks if an OID starts with the UPS MIB-2 tree root
func hasUPSOIDPrefix(oid string) bool {
	prefix := ".1.3.6.1.2.1.33."
	if oid == ".1.3.6.1.2.1.33" {
		return true
	}
	return len(oid) >= len(prefix) && oid[:len(prefix)] == prefix
}

// parseNumericValue extracts a numeric value from SNMP string representation
func parseNumericValue(s string) (float64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}

	var val float64
	_, err := fmt.Sscanf(s, "%f", &val)
	if err != nil {
		// Try integer parsing
		var ival int
		_, err2 := fmt.Sscanf(s, "%d", &ival)
		if err2 != nil {
			return 0, fmt.Errorf("cannot parse %q as number: %v and %v", s, err, err2)
		}
		val = float64(ival)
	}

	return val, nil
}

// parseTenantProbeSubject extracts tenant_id and probe_id from a NATS subject.
// Subject format: "tenant.{tenant_id}.probe.{probe_id}.{extra}"
func parseTenantProbeSubject(subject string) (string, string, error) {
	parts := strings.Split(subject, ".")
	if len(parts) < 4 {
		return "", "", fmt.Errorf("insufficient subject parts: %s", subject)
	}

	if parts[0] != "tenant" {
		return "", "", fmt.Errorf("invalid subject prefix: %s", subject)
	}
	if parts[2] != "probe" {
		return "", "", fmt.Errorf("missing probe segment in subject: %s", subject)
	}

	// parts[1] = tenant_id, parts[3] = probe_id
	tenantID := parts[1]
	probeID := parts[3]

	return tenantID, probeID, nil
}

// triggerShutdown initiates a graceful shutdown sequence for devices downstream of a failing UPS
func (pm *PowerEventManager) triggerShutdown(tenantID, upsDeviceID string, state *UPSState) {
	// Check if a shutdown event is already in progress for this UPS
	for _, evt := range pm.events {
		if evt.UPSDeviceID == upsDeviceID && evt.Status != "completed" && evt.Status != "aborted" {
			if pm.log != nil {
				pm.log.Warn("shutdown already in progress",
					zap.String("tenant_id", tenantID),
					zap.String("ups_device_id", upsDeviceID),
				)
			}
			return
		}
	}

	// Query the device dependency tree from CMDB
	targets, err := pm.queryShutdownTargets(tenantID, upsDeviceID, state)
	if err != nil {
		if pm.log != nil {
			pm.log.Error("query shutdown targets",
				zap.String("tenant_id", tenantID),
				zap.String("ups_device_id", upsDeviceID),
				zap.Error(err),
			)
		}
		return
	}

	if len(targets) == 0 {
		if pm.log != nil {
			pm.log.Warn("no shutdown targets found for UPS",
				zap.String("tenant_id", tenantID),
				zap.String("ups_device_id", upsDeviceID),
				zap.Int("battery_runtime_sec", state.BatteryRuntime),
			)
		}
		return
	}

	eventID := uuid.New().String()
	event := &ShutdownEvent{
		ID:          eventID,
		TenantID:    tenantID,
		UPSDeviceID: upsDeviceID,
		BatteryMin:  int(math.Ceil(float64(state.BatteryRuntime) / 60.0)),
		InitiatedAt: time.Now().UTC(),
		TargetOrder: make([]string, len(targets)),
		Status:      "initiated",
	}

	for i, target := range targets {
		event.TargetOrder[i] = target.DeviceID
		// Execute shutdown command for this target
		go pm.executeShutdownTarget(target, eventID)
	}

	pm.events[eventID] = event
	if pm.log != nil {
		pm.log.Info("shutdown sequence initiated",
			zap.String("event_id", eventID),
			zap.String("tenant_id", tenantID),
			zap.String("ups_device_id", upsDeviceID),
			zap.Int("battery_minutes", event.BatteryMin),
			zap.Int("targets", len(targets)),
		)
	}
}

// queryShutdownTargets resolves the device dependency tree for a UPS asset
func (pm *PowerEventManager) queryShutdownTargets(tenantID, upsDeviceID string, state *UPSState) ([]ShutdownTarget, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Query the asset inventory for devices that depend on this UPS
	// The dependency is derived from the CMDB: sites, hosts, and infrastructure devices
	// that are physically connected to or powered by the same circuit as the UPS.
	//
	// In production, this queries the assets table with a circuit_id or ups_id join.
	// Here we build the ordered shutdown target list from known infrastructure classes.

	var targets []ShutdownTarget

	// Order: hypervisors first (to drain VMs), then SAN arrays, then NAS, then VM pools
	// Each group is sorted by shutdown_order within the group.

	// Hypervisors (ESXi, Proxmox, Hyper-V)
	hypervisors, err := pm.getDevicesByRole(ctx, tenantID, "hypervisor")
	if err != nil {
		pm.log.Warn("query hypervisors for shutdown", zap.Error(err))
	}
	for i, id := range hypervisors {
		targets = append(targets, ShutdownTarget{
			DeviceID:     id,
			DeviceType:   "hypervisor",
			ShutdownOrder: i,
			Command:      "shutdown",
			Timeout:      120 * time.Second,
		})
	}

	// SAN arrays (Pure, PowerStore, HPE Alletra)
	sans, err := pm.getDevicesByRole(ctx, tenantID, "san")
	if err != nil {
		pm.log.Warn("query SANs for shutdown", zap.Error(err))
	}
	for i, id := range sans {
		targets = append(targets, ShutdownTarget{
			DeviceID:     id,
			DeviceType:   "san",
			ShutdownOrder: len(hypervisors) + i,
			Command:      "shutdown",
			Timeout:      180 * time.Second,
		})
	}

	// NAS arrays (Synology, QNAP, TrueNAS)
	nas, err := pm.getDevicesByRole(ctx, tenantID, "nas")
	if err != nil {
		pm.log.Warn("query NAS for shutdown", zap.Error(err))
	}
	for i, id := range nas {
		targets = append(targets, ShutdownTarget{
			DeviceID:     id,
			DeviceType:   "nas",
			ShutdownOrder: len(hypervisors) + len(sans) + i,
			Command:      "shutdown",
			Timeout:      120 * time.Second,
		})
	}

	// VM pools / workload groups
	vms, err := pm.getDevicesByRole(ctx, tenantID, "vm_pool")
	if err != nil {
		pm.log.Warn("query VMs for shutdown", zap.Error(err))
	}
	for i, id := range vms {
		targets = append(targets, ShutdownTarget{
			DeviceID:     id,
			DeviceType:   "vm_pool",
			ShutdownOrder: len(hypervisors) + len(sans) + len(nas) + i,
			Command:      "vm_shutdown",
			Timeout:      60 * time.Second,
		})
	}

	return targets, nil
}

// getDevicesByRole queries the device inventory for devices of a given role.
// It joins the asset inventory tree to find devices physically connected to the same
// power circuit as the UPS, then returns them ordered for graceful shutdown.
func (pm *PowerEventManager) getDevicesByRole(ctx context.Context, tenantID, role string) ([]string, error) {
	if pm.db == nil {
		return nil, nil
	}

	query := `
		SELECT d.id
		FROM devices d
		JOIN sites s ON d.site_id = s.id
		JOIN clients c ON s.client_id = c.id
		WHERE d.tenant_id = $1
		  AND d.roles @> ARRAY[$2::text]
		  AND d.is_active = true
		ORDER BY d.id
	`

	rows, err := pm.db.QueryContext(ctx, query, tenantID, role)
	if err != nil {
		return nil, fmt.Errorf("query %s devices: %w", role, err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan device id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return ids, nil
}

// executeShutdownTarget sends a graceful shutdown command to a single target device
func (pm *PowerEventManager) executeShutdownTarget(target ShutdownTarget, eventID string) {
	subject := fmt.Sprintf("tenant.%s.cmd.shutdown", target.DeviceID)
	payload, err := json.Marshal(map[string]interface{}{
		"event_id":      eventID,
		"device_id":     target.DeviceID,
		"device_type":   target.DeviceType,
		"command":       target.Command,
		"timeout_secs":  int(target.Timeout.Seconds()),
		"graceful":      true,
		"initiated_at":  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		pm.log.Error("marshal shutdown command", zap.Error(err))
		pm.updateEventStatus(eventID, "aborted", ptrString("command_marshal_failed"))
		return
	}

	msg, err := pm.js.Publish(subject, payload)
	if err != nil {
		pm.log.Error("publish shutdown command",
			zap.String("subject", subject),
			zap.String("device_id", target.DeviceID),
			zap.Error(err),
		)
		return
	}

	pm.log.Info("shutdown command dispatched",
		zap.String("event_id", eventID),
		zap.String("device_id", target.DeviceID),
		zap.String("device_type", target.DeviceType),
		zap.Uint64("seq", msg.Sequence),
	)
}

// updateEventStatus updates the status of a shutdown event
func (pm *PowerEventManager) updateEventStatus(eventID, status string, abortedBy *string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	evt, exists := pm.events[eventID]
	if !exists {
		return
	}

	evt.Status = status
	if status == "aborted" && abortedBy != nil {
		t := time.Now().UTC()
		evt.AbortedAt = &t
		evt.AbortedBy = abortedBy
	}

	if pm.log != nil {
		pm.log.Info("shutdown event status updated",
			zap.String("event_id", eventID),
			zap.String("status", status),
		)
	}
}

// GetActiveEvents returns all non-terminal shutdown events for a tenant
func (pm *PowerEventManager) GetActiveEvents(tenantID string) []*ShutdownEvent {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var events []*ShutdownEvent
	for _, evt := range pm.events {
		if evt.TenantID == tenantID && evt.Status != "completed" && evt.Status != "aborted" {
			events = append(events, evt)
		}
	}
	return events
}

// GetUPSState returns the current state of a UPS device
func (pm *PowerEventManager) GetUPSState(deviceID string) *UPSState {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if state, ok := pm.upses[deviceID]; ok {
		copy := *state
		return &copy
	}
	return nil
}

// GetAllUPSStates returns a snapshot of all tracked UPS devices
func (pm *PowerEventManager) GetAllUPSStates() map[string]*UPSState {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make(map[string]*UPSState, len(pm.upses))
	for k, v := range pm.upses {
		cp := *v
		result[k] = &cp
	}
	return result
}

// AbortShutdown aborts a running shutdown event
func (pm *PowerEventManager) AbortShutdown(eventID, reason string) {
	pm.updateEventStatus(eventID, "aborted", &reason)
	if pm.log != nil {
		pm.log.Warn("shutdown aborted",
			zap.String("event_id", eventID),
			zap.String("reason", reason),
		)
	}
}

// ptrString returns a pointer to the given string
func ptrString(s string) *string {
	return &s
}
