package postgres

import (
	"database/sql"
	"fmt"
	"time"
)

type Migration struct {
	ID      int
	Name    string
	Up      string
	Down    string
}

func Migrations() []Migration {
	return []Migration{
		{
			ID:   1,
			Name: "create_tenants_table",
			Up: `
				CREATE TABLE IF NOT EXISTS tenants (
					id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					name          TEXT NOT NULL,
					slug          TEXT UNIQUE NOT NULL,
					plan          TEXT NOT NULL DEFAULT 'free',
					is_active     BOOLEAN NOT NULL DEFAULT true,
					settings      JSONB DEFAULT '{}',
					created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_tenants_slug ON tenants(slug);
			`,
			Down: `DROP TABLE IF EXISTS tenants CASCADE;`,
		},
		{
			ID:   2,
			Name: "create_users_table",
			Up: `
				CREATE TABLE IF NOT EXISTS users (
					id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					email         TEXT NOT NULL,
					password_hash TEXT NOT NULL,
					role          TEXT NOT NULL DEFAULT 'viewer' CHECK (role IN ('admin', 'technician', 'viewer')),
					is_active     BOOLEAN NOT NULL DEFAULT true,
					last_login    TIMESTAMPTZ,
					created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE(tenant_id, email)
				);
				CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(tenant_id);
				CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
			`,
			Down: `DROP TABLE IF EXISTS users CASCADE;`,
		},
		{
			ID:   3,
			Name: "create_devices_table",
			Up: `
				CREATE TABLE IF NOT EXISTS devices (
					id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					agent_id      UUID UNIQUE,
					hostname      TEXT NOT NULL,
					os            TEXT,
					os_version    TEXT,
					arch          TEXT,
					cpu_cores     INT,
					ram_total_mb  BIGINT,
					disk_total_mb BIGINT,
					public_ip     TEXT,
					private_ips   TEXT[] DEFAULT '{}',
					tags          JSONB DEFAULT '{}',
					last_heartbeat TIMESTAMPTZ,
					status        TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'online', 'offline', 'disabled')),
					enrolled_at   TIMESTAMPTZ,
					created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_devices_tenant ON devices(tenant_id);
				CREATE INDEX IF NOT EXISTS idx_devices_agent ON devices(agent_id);
				CREATE INDEX IF NOT EXISTS idx_devices_status ON devices(tenant_id, status);
				CREATE INDEX IF NOT EXISTS idx_devices_hostname ON devices(tenant_id, hostname);
			`,
			Down: `DROP TABLE IF EXISTS devices CASCADE;`,
		},
		{
			ID:   4,
			Name: "create_roles_permissions",
			Up: `
				CREATE TABLE IF NOT EXISTS permissions (
					id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					role     TEXT NOT NULL CHECK (role IN ('admin', 'technician', 'viewer')),
					resource TEXT NOT NULL,
					action   TEXT NOT NULL,
					UNIQUE(tenant_id, role, resource, action)
				);
				CREATE INDEX IF NOT EXISTS idx_permissions_tenant_role ON permissions(tenant_id, role);
			`,
			Down: `DROP TABLE IF EXISTS permissions CASCADE;`,
		},
		{
			ID:   5,
			Name: "enable_rls_and_create_policies",
			Up: `
				ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
				ALTER TABLE users ENABLE ROW LEVEL SECURITY;
				ALTER TABLE permissions ENABLE ROW LEVEL SECURITY;

				CREATE POLICY tenant_isolation_devices ON devices
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
				CREATE POLICY tenant_isolation_users ON users
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
				CREATE POLICY tenant_isolation_permissions ON permissions
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
			`,
			Down: `
				DROP POLICY IF EXISTS tenant_isolation_devices ON devices;
				DROP POLICY IF EXISTS tenant_isolation_users ON users;
				DROP POLICY IF EXISTS tenant_isolation_permissions ON permissions;
				ALTER TABLE devices DISABLE ROW LEVEL SECURITY;
				ALTER TABLE users DISABLE ROW LEVEL SECURITY;
				ALTER TABLE permissions DISABLE ROW LEVEL SECURITY;
			`,
		},
		{
			ID:   6,
			Name: "create_enrollment_tokens_table",
			Up: `
				CREATE TABLE IF NOT EXISTS enrollment_tokens (
					id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					token      TEXT UNIQUE NOT NULL,
					expires_at TIMESTAMPTZ NOT NULL,
					used_at    TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					created_by UUID REFERENCES users(id)
				);
				CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_token ON enrollment_tokens(token);
				CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_tenant ON enrollment_tokens(tenant_id);
			`,
			Down: `DROP TABLE IF EXISTS enrollment_tokens CASCADE;`,
		},
		{
			ID:   7,
			Name: "create_audit_log_table",
			Up: `
				CREATE TABLE IF NOT EXISTS audit_log (
					id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					user_id    UUID REFERENCES users(id),
					action     TEXT NOT NULL,
					resource   TEXT NOT NULL,
					details    JSONB DEFAULT '{}',
					ip_address TEXT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_audit_log_tenant_time ON audit_log(tenant_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(tenant_id, action);
			`,
			Down: `DROP TABLE IF EXISTS audit_log CASCADE;`,
		},
		{
			ID:   8,
			Name: "create_alert_rules_table",
			Up: `
				CREATE TABLE IF NOT EXISTS alert_rules (
					id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					name          TEXT NOT NULL,
					type          TEXT NOT NULL CHECK (type IN ('threshold', 'heartbeat')),
					enabled       BOOLEAN NOT NULL DEFAULT true,
					severity      TEXT NOT NULL DEFAULT 'warning' CHECK (severity IN ('critical', 'warning', 'info')),
					metric_name   TEXT,
					condition     TEXT CHECK (condition IN ('gt', 'gte', 'lt', 'lte', 'eq', 'neq')),
					threshold     DOUBLE PRECISION,
					timeout       INTERVAL DEFAULT '5 minutes',
					device_id     UUID,
					cooldown      INTERVAL DEFAULT '5 minutes',
					channels      JSONB DEFAULT '["slack"]',
					template      TEXT,
					created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_alert_rules_tenant ON alert_rules(tenant_id);
				ALTER TABLE alert_rules ENABLE ROW LEVEL SECURITY;
				CREATE POLICY tenant_isolation_alert_rules ON alert_rules
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
			`,
			Down: `DROP TABLE IF EXISTS alert_rules CASCADE;`,
		},
		{
			ID:   9,
			Name: "create_notification_channels_table",
			Up: `
				CREATE TABLE IF NOT EXISTS notification_channels (
					id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					name       TEXT NOT NULL,
					type       TEXT NOT NULL CHECK (type IN ('slack', 'teams', 'webhook', 'pagerduty', 'email')),
					config     JSONB NOT NULL DEFAULT '{}',
					enabled    BOOLEAN NOT NULL DEFAULT true,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_notification_channels_tenant ON notification_channels(tenant_id);
				ALTER TABLE notification_channels ENABLE ROW LEVEL SECURITY;
				CREATE POLICY tenant_isolation_notification_channels ON notification_channels
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
			`,
			Down: `DROP TABLE IF EXISTS notification_channels CASCADE;`,
		},
		{
			ID:   10,
			Name: "create_schedule_maintenance_tables",
			Up: `
				CREATE TABLE IF NOT EXISTS maintenance_windows (
					id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					name        TEXT NOT NULL,
					start_time  TIMESTAMPTZ NOT NULL,
					end_time    TIMESTAMPTZ NOT NULL,
					device_ids  UUID[] DEFAULT '{}',
					tags        JSONB DEFAULT '{}',
					created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					created_by  UUID REFERENCES users(id)
				);
				CREATE INDEX IF NOT EXISTS idx_maintenance_tenant_time ON maintenance_windows(tenant_id, start_time);
				ALTER TABLE maintenance_windows ENABLE ROW LEVEL SECURITY;
				CREATE POLICY tenant_isolation_maintenance ON maintenance_windows
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
			`,
			Down: `DROP TABLE IF EXISTS maintenance_windows CASCADE;`,
		},
	}
}

type SchemaManager struct {
	db *sql.DB
}

func NewSchemaManager(db *sql.DB) *SchemaManager {
	return &SchemaManager{db: db}
}

func (m *SchemaManager) Apply() error {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id         INT PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	for _, mig := range Migrations() {
		var exists bool
		err := m.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE id = $1)`, mig.ID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", mig.ID, err)
		}
		if exists {
			continue
		}

		if _, err := m.db.Exec(mig.Up); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", mig.ID, mig.Name, err)
		}

		if _, err := m.db.Exec(
			`INSERT INTO schema_migrations (id, name) VALUES ($1, $2)`,
			mig.ID, mig.Name,
		); err != nil {
			return fmt.Errorf("record migration %d: %w", mig.ID, err)
		}
	}

	return nil
}

func (m *SchemaManager) Unapply(target int) error {
	for _, mig := range Migrations() {
		if mig.ID <= target {
			continue
		}
		var exists bool
		err := m.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE id = $1)`, mig.ID).Scan(&exists)
		if err != nil || !exists {
			continue
		}
		if mig.Down == "" {
			continue
		}
		if _, err := m.db.Exec(mig.Down); err != nil {
			return fmt.Errorf("rollback migration %d (%s): %w", mig.ID, mig.Name, err)
		}
		if _, err := m.db.Exec(`DELETE FROM schema_migrations WHERE id = $1`, mig.ID); err != nil {
			return fmt.Errorf("unrecord migration %d: %w", mig.ID, err)
		}
	}
	return nil
}

type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Plan      string    `json:"plan"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Device struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	AgentID       string     `json:"agent_id,omitempty"`
	Hostname      string     `json:"hostname"`
	OS            string     `json:"os,omitempty"`
	OSVersion     string     `json:"os_version,omitempty"`
	Arch          string     `json:"arch,omitempty"`
	CPUCores      int        `json:"cpu_cores,omitempty"`
	RAMTotalMB    int64      `json:"ram_total_mb,omitempty"`
	DiskTotalMB   int64      `json:"disk_total_mb,omitempty"`
	PublicIP      string     `json:"public_ip,omitempty"`
	Status        string     `json:"status"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	EnrolledAt    *time.Time `json:"enrolled_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func SeedDevTenant(db *sql.DB) error {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM tenants WHERE slug = 'dev')`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check dev tenant: %w", err)
	}
	if exists {
		return nil
	}

	_, err = db.Exec(`
		INSERT INTO tenants (id, name, slug, plan)
		VALUES ('00000000-0000-0000-0000-000000000001', 'Development Tenant', 'dev', 'enterprise')
		ON CONFLICT (slug) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("seed dev tenant: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO users (id, tenant_id, email, password_hash, role)
		VALUES (
			'00000000-0000-0000-0000-000000000010',
			'00000000-0000-0000-0000-000000000001',
			'admin@dev.local',
			'$2a$10$placeholder', -- change-me: admin123
			'admin'
		) ON CONFLICT DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("seed dev user: %w", err)
	}

	return nil
}
