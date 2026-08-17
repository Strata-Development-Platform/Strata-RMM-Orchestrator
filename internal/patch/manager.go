package patch

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

type Platform string

const (
	PlatformWindows Platform = "windows"
	PlatformLinux   Platform = "linux"
	PlatformMacOS   Platform = "macos"
)

type PatchSeverity string

const (
	SeverityCritical  PatchSeverity = "critical"
	SeverityImportant PatchSeverity = "important"
	SeverityModerate  PatchSeverity = "moderate"
	SeverityLow       PatchSeverity = "low"
)

type PatchStatus string

const (
	StatusPending   PatchStatus = "pending"
	StatusApproved  PatchStatus = "approved"
	StatusCanary    PatchStatus = "canary"
	StatusDeploying PatchStatus = "deploying"
	StatusInstalled PatchStatus = "installed"
	StatusCompleted PatchStatus = "completed"
	StatusFailed    PatchStatus = "failed"
	StatusRebootReq PatchStatus = "reboot_required"
	StatusCancelled PatchStatus = "cancelled"
)

type Patch struct {
	ID          string        `json:"id"`
	TenantID    string        `json:"tenant_id"`
	KB          string        `json:"kb"`
	Title       string        `json:"title"`
	Platform    Platform      `json:"platform"`
	Severity    PatchSeverity `json:"severity"`
	Description string        `json:"description"`
	CVE         []string      `json:"cve"`
	Published   time.Time     `json:"published"`
	CreatedAt   time.Time     `json:"created_at"`
}

type PatchPolicy struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	Name           string            `json:"name"`
	Enabled        bool              `json:"enabled"`
	Platforms      []Platform        `json:"platforms"`
	ApprovalMode   string            `json:"approval_mode"`
	Severity       PatchSeverity     `json:"severity"`
	MaintenanceWin string            `json:"maintenance_window"`
	DeviceFilter   map[string]string `json:"device_filter"`
	MaxRetries     int               `json:"max_retries"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type Deployment struct {
	ID           string      `json:"id"`
	PolicyID     string      `json:"policy_id"`
	TenantID     string      `json:"tenant_id"`
	Status       PatchStatus `json:"status"`
	DeviceCount  int         `json:"device_count"`
	Installed    int         `json:"installed"`
	Failed       int         `json:"failed"`
	Pending      int         `json:"pending"`
	ScheduledFor time.Time   `json:"scheduled_for"`
	StartedAt    *time.Time  `json:"started_at,omitempty"`
	CompletedAt  *time.Time  `json:"completed_at,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
}

type DevicePatchState struct {
	DeviceID     string      `json:"device_id"`
	DeploymentID string      `json:"deployment_id"`
	PatchID      string      `json:"patch_id"`
	Status       PatchStatus `json:"status"`
	Attempts     int         `json:"attempts"`
	Error        string      `json:"error,omitempty"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

type Manager struct {
	nats   *nats.Conn
	tsdb   *timescale.Client
	logger *zap.Logger
	store  *Store

	mu       sync.RWMutex
	policies map[string]*PatchPolicy
}

func NewManager(nc *nats.Conn, tsdb *timescale.Client, store *Store, logger *zap.Logger) *Manager {
	return &Manager{nats: nc, tsdb: tsdb, logger: logger, store: store, policies: make(map[string]*PatchPolicy)}
}

func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("starting patch manager")

	policies, err := m.store.LoadPolicies(ctx)
	if err != nil {
		return fmt.Errorf("load policies: %w", err)
	}
	m.mu.Lock()
	for _, p := range policies {
		m.policies[p.ID] = p
	}
	m.mu.Unlock()
	m.logger.Info("patch policies loaded", zap.Int("count", len(policies)))

	// Patch install commands and terminal results use the platform's generic
	// durable jobs/outbox protocol. This legacy inventory subject remains read-
	// only until patch scan inventory is moved onto the same structured result
	// contract.
	invSub, err := m.nats.Subscribe("tenant.*.agent.*.patch.inventory", m.handlePatchInventory)
	if err != nil {
		return fmt.Errorf("subscribe patch inventory: %w", err)
	}
	defer func() {
		if err := invSub.Unsubscribe(); err != nil {
			m.logger.Warn("unsubscribe patch inventory", zap.Error(err))
		}
	}()

	go m.deploymentScheduler(ctx)
	return nil
}

func (m *Manager) handlePatchInventory(msg *nats.Msg) {
	tenantID, deviceID, err := patchInventoryTransportIdentity(msg.Subject)
	if err != nil {
		m.logger.Warn("invalid patch inventory subject", zap.String("subject", msg.Subject), zap.Error(err))
		return
	}
	var inv struct {
		OS        string   `json:"os"`
		Installed []*Patch `json:"installed"`
		Missing   []*Patch `json:"missing"`
	}
	if err := json.Unmarshal(msg.Data, &inv); err != nil {
		m.logger.Warn("invalid patch inventory", zap.Error(err))
		return
	}
	if err := m.store.SaveInventory(context.Background(), tenantID, deviceID, inv.Installed, inv.Missing); err != nil {
		m.logger.Error("save patch inventory", zap.String("tenant_id", tenantID), zap.String("device_id", deviceID), zap.Error(err))
	}
}

func (m *Manager) deploymentScheduler(ctx context.Context) {
	// Reconcile once at startup so a restarted orchestrator does not wait for the
	// first minute tick before resuming a durable canary/broad rollout.
	m.processScheduledDeployments(ctx)
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.processScheduledDeployments(ctx)
		}
	}
}

func (m *Manager) processScheduledDeployments(ctx context.Context) {
	pending, err := m.store.GetPendingDeployments(ctx)
	if err != nil {
		m.logger.Error("get pending deployments", zap.Error(err))
		return
	}
	for _, d := range pending {
		if time.Now().Before(d.ScheduledFor) {
			continue
		}
		m.executeDeployment(ctx, d)
	}

	canaries, err := m.store.GetCanaryDeployments(ctx)
	if err != nil {
		m.logger.Error("get canary deployments", zap.Error(err))
		return
	}
	for _, d := range canaries {
		m.advanceCanaryDeployment(ctx, d)
	}

	deploying, err := m.store.GetDeployingDeployments(ctx)
	if err != nil {
		m.logger.Error("get deploying deployments", zap.Error(err))
		return
	}
	for _, d := range deploying {
		m.advanceDeployingDeployment(ctx, d)
	}
}

func (m *Manager) policyFor(id string) (*PatchPolicy, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	policy, ok := m.policies[id]
	return policy, ok
}

func (m *Manager) executeDeployment(ctx context.Context, dep *Deployment) {
	m.logger.Info("starting patch canary", zap.String("deployment_id", dep.ID), zap.Time("scheduled", dep.ScheduledFor))
	policy, ok := m.policyFor(dep.PolicyID)
	if !ok {
		m.logger.Error("policy not found", zap.String("policy_id", dep.PolicyID))
		return
	}
	if !maintenanceWindowAllows(time.Now(), policy.MaintenanceWin) {
		m.logger.Debug("patch deployment outside maintenance window", zap.String("deployment_id", dep.ID))
		return
	}
	if _, err := m.store.GetDeploymentPatchIDs(ctx, dep.ID); err != nil {
		m.logger.Error("patch deployment has no explicit approved patch selection", zap.String("deployment_id", dep.ID), zap.Error(err))
		return
	}
	if _, err := m.store.PrepareCanaryRollout(ctx, dep.ID, defaultCanaryPercent); err != nil {
		m.logger.Error("prepare canary rollout", zap.String("deployment_id", dep.ID), zap.Error(err))
		return
	}

	now := time.Now()
	dep.StartedAt = &now
	dep.Status = StatusCanary
	if err := m.store.UpdateDeployment(ctx, dep); err != nil {
		m.logger.Error("persist canary deployment phase", zap.Error(err))
		return
	}
	m.dispatchRolloutGroup(ctx, dep, policy, rolloutGroupCanary)
}

func (m *Manager) advanceCanaryDeployment(ctx context.Context, dep *Deployment) {
	policy, ok := m.policyFor(dep.PolicyID)
	if !ok {
		m.logger.Error("policy not found", zap.String("policy_id", dep.PolicyID))
		return
	}
	if maintenanceWindowAllows(time.Now(), policy.MaintenanceWin) {
		m.dispatchRolloutGroup(ctx, dep, policy, rolloutGroupCanary)
	}
	gate, err := m.store.GetRolloutJobGate(ctx, dep.ID, rolloutGroupCanary)
	if err != nil {
		m.logger.Error("evaluate canary durable-job gate", zap.String("deployment_id", dep.ID), zap.Error(err))
		return
	}
	if !gate.Ready() {
		return
	}
	dep.Installed = gate.Succeeded
	dep.Failed = gate.Failed
	dep.Pending = dep.DeviceCount - gate.Completed
	if !gate.Passes(defaultCanarySuccessThreshold) {
		now := time.Now()
		dep.Status = StatusFailed
		dep.CompletedAt = &now
		if err := m.store.UpdateDeployment(ctx, dep); err != nil {
			m.logger.Error("persist failed canary gate", zap.Error(err))
		}
		return
	}
	dep.Status = StatusDeploying
	if err := m.store.UpdateDeployment(ctx, dep); err != nil {
		m.logger.Error("persist broad rollout phase", zap.Error(err))
		return
	}
	if maintenanceWindowAllows(time.Now(), policy.MaintenanceWin) {
		m.dispatchRolloutGroup(ctx, dep, policy, rolloutGroupBroad)
	}
}

func (m *Manager) advanceDeployingDeployment(ctx context.Context, dep *Deployment) {
	policy, ok := m.policyFor(dep.PolicyID)
	if !ok {
		m.logger.Error("policy not found", zap.String("policy_id", dep.PolicyID))
		return
	}
	if maintenanceWindowAllows(time.Now(), policy.MaintenanceWin) {
		m.dispatchRolloutGroup(ctx, dep, policy, rolloutGroupBroad)
	}
	gate, err := m.store.GetDeploymentJobGate(ctx, dep.ID)
	if err != nil {
		m.logger.Error("evaluate deployment durable-job gate", zap.String("deployment_id", dep.ID), zap.Error(err))
		return
	}
	dep.Installed = gate.Succeeded
	dep.Failed = gate.Failed
	dep.Pending = gate.Total - gate.Completed
	if !gate.Ready() {
		if err := m.store.UpdateDeployment(ctx, dep); err != nil {
			m.logger.Error("persist deployment aggregate progress", zap.Error(err))
		}
		return
	}
	now := time.Now()
	dep.CompletedAt = &now
	if gate.Failed > 0 {
		dep.Status = StatusFailed
	} else {
		dep.Status = StatusCompleted
	}
	if err := m.store.UpdateDeployment(ctx, dep); err != nil {
		m.logger.Error("persist deployment completion", zap.Error(err))
	}
}

func (m *Manager) dispatchRolloutGroup(ctx context.Context, dep *Deployment, policy *PatchPolicy, rolloutGroup string) {
	devices, err := m.store.GetUndispatchedRolloutDevices(ctx, dep.ID, rolloutGroup)
	if err != nil {
		m.logger.Error("get undispatched patch targets", zap.String("deployment_id", dep.ID), zap.String("group", rolloutGroup), zap.Error(err))
		return
	}
	for _, deviceID := range devices {
		if err := m.store.QueuePatchRolloutDevice(ctx, dep.ID, deviceID, policy.MaxRetries); err != nil {
			m.logger.Error("queue durable patch job", zap.String("deployment_id", dep.ID), zap.String("device_id", deviceID), zap.Error(err))
		}
	}
}

func (m *Manager) CreatePolicy(ctx context.Context, policy *PatchPolicy) error {
	if err := m.store.SavePolicy(ctx, policy); err != nil {
		return err
	}
	m.mu.Lock()
	m.policies[policy.ID] = policy
	m.mu.Unlock()
	return nil
}

func (m *Manager) ListPolicies(ctx context.Context, tenantID string) ([]*PatchPolicy, error) {
	return m.store.ListPolicies(ctx, tenantID)
}

func (m *Manager) DeletePolicy(ctx context.Context, policyID string) error {
	if err := m.store.DeletePolicy(ctx, policyID); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.policies, policyID)
	m.mu.Unlock()
	return nil
}

// CreateDeployment remains for compatibility with older internal callers. New
// production callers must use CreateDeploymentWithPatches so a deployment has
// an explicit approved patch selection before it can leave pending state.
func (m *Manager) CreateDeployment(ctx context.Context, dep *Deployment, deviceIDs []string) error {
	return m.store.CreateDeployment(ctx, dep, deviceIDs)
}

func (m *Manager) CreateDeploymentWithPatches(ctx context.Context, dep *Deployment, deviceIDs, patchIDs []string) error {
	return m.store.CreateDeploymentWithPatches(ctx, dep, deviceIDs, patchIDs)
}

func (m *Manager) ListDeployments(ctx context.Context, tenantID string) ([]*Deployment, error) {
	return m.store.ListDeployments(ctx, tenantID)
}

func (m *Manager) GetPatchInventory(ctx context.Context, tenantID, deviceID string) ([]*Patch, []*Patch, error) {
	return m.store.GetInventory(ctx, tenantID, deviceID)
}
