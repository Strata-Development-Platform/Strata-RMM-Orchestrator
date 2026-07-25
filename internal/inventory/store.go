package inventory

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CreateTenant(name, slug, plan string) (*postgres.Tenant, error) {
	var t postgres.Tenant
	err := s.db.QueryRow(`
		INSERT INTO tenants (name, slug, plan)
		VALUES ($1, $2, $3)
		RETURNING id, name, slug, plan, is_active, created_at, updated_at
	`, name, slug, plan).Scan(
		&t.ID, &t.Name, &t.Slug, &t.Plan, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	return &t, nil
}

func (s *Store) GetTenantByID(id string) (*postgres.Tenant, error) {
	var t postgres.Tenant
	err := s.db.QueryRow(`
		SELECT id, name, slug, plan, is_active, created_at, updated_at
		FROM tenants WHERE id = $1
	`, id).Scan(
		&t.ID, &t.Name, &t.Slug, &t.Plan, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tenant not found: %s", id)
		}
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return &t, nil
}

func (s *Store) ListTenants() ([]*postgres.Tenant, error) {
	rows, err := s.db.Query(`
		SELECT id, name, slug, plan, is_active, created_at, updated_at
		FROM tenants ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*postgres.Tenant
	for rows.Next() {
		var t postgres.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Plan, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, &t)
	}
	return tenants, nil
}

func (s *Store) RegisterDevice(tenantID, hostname string) (*postgres.Device, error) {
	var d postgres.Device
	err := s.db.QueryRow(`
		INSERT INTO devices (tenant_id, hostname, status)
		VALUES ($1, $2, 'pending')
		RETURNING id, tenant_id, hostname, status, created_at, updated_at
	`, tenantID, hostname).Scan(
		&d.ID, &d.TenantID, &d.Hostname, &d.Status, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("register device: %w", err)
	}
	return &d, nil
}

func (s *Store) GetDevice(id string) (*postgres.Device, error) {
	var d postgres.Device
	err := s.db.QueryRow(`
		SELECT id, tenant_id, COALESCE(agent_id::text, ''), hostname,
		       COALESCE(os, ''), COALESCE(os_version, ''), COALESCE(arch, ''),
		       COALESCE(cpu_cores, 0), COALESCE(ram_total_mb, 0), COALESCE(disk_total_mb, 0),
		       COALESCE(public_ip, ''), status, last_heartbeat, enrolled_at,
		       created_at, updated_at
		FROM devices WHERE id = $1
	`, id).Scan(
		&d.ID, &d.TenantID, &d.AgentID, &d.Hostname,
		&d.OS, &d.OSVersion, &d.Arch,
		&d.CPUCores, &d.RAMTotalMB, &d.DiskTotalMB,
		&d.PublicIP, &d.Status, &d.LastHeartbeat, &d.EnrolledAt,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("device not found: %s", id)
		}
		return nil, fmt.Errorf("get device: %w", err)
	}
	return &d, nil
}

func (s *Store) ListDevices(tenantID string, limit, offset int) ([]*postgres.Device, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, tenant_id, COALESCE(agent_id::text, ''), hostname,
		       COALESCE(os, ''), COALESCE(os_version, ''), COALESCE(arch, ''),
		       COALESCE(cpu_cores, 0), COALESCE(ram_total_mb, 0), COALESCE(disk_total_mb, 0),
		       COALESCE(public_ip, ''), status, last_heartbeat, enrolled_at,
		       created_at, updated_at
		FROM devices WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	var devices []*postgres.Device
	for rows.Next() {
		var d postgres.Device
		if err := rows.Scan(
			&d.ID, &d.TenantID, &d.AgentID, &d.Hostname,
			&d.OS, &d.OSVersion, &d.Arch,
			&d.CPUCores, &d.RAMTotalMB, &d.DiskTotalMB,
			&d.PublicIP, &d.Status, &d.LastHeartbeat, &d.EnrolledAt,
			&d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, &d)
	}
	return devices, nil
}

func (s *Store) UpdateDeviceStatus(deviceID, status string) error {
	_, err := s.db.Exec(`UPDATE devices SET status = $1, updated_at = NOW() WHERE id = $2`, status, deviceID)
	return err
}

func (s *Store) UpdateDeviceHeartbeat(deviceID string) error {
	now := time.Now()
	_, err := s.db.Exec(`
		UPDATE devices SET last_heartbeat = $1, status = 'online', updated_at = $1
		WHERE id = $2
	`, now, deviceID)
	return err
}

func (s *Store) UpdateDeviceAgentID(deviceID, agentID string) error {
	_, err := s.db.Exec(`
		UPDATE devices SET agent_id = $1, enrolled_at = NOW(), updated_at = NOW()
		WHERE id = $2
	`, agentID, deviceID)
	return err
}

func (s *Store) LinkAgentToDevice(tenantID, agentID, hostname string) (*postgres.Device, error) {
	id := uuid.New().String()
	var d postgres.Device
	err := s.db.QueryRow(`
		INSERT INTO devices (id, tenant_id, agent_id, hostname, status, enrolled_at)
		VALUES ($1, $2, $3, $4, 'online', NOW())
		ON CONFLICT (agent_id) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			status = 'online',
			updated_at = NOW()
		RETURNING id, tenant_id, hostname, status, created_at, updated_at
	`, id, tenantID, agentID, hostname).Scan(
		&d.ID, &d.TenantID, &d.Hostname, &d.Status, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("link agent to device: %w", err)
	}
	return &d, nil
}
