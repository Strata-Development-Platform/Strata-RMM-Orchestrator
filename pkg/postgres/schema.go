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
		{
			ID:   11,
			Name: "create_patch_policies_table",
			Up: `
				CREATE TABLE IF NOT EXISTS patch_policies (
					id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id        UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					name             TEXT NOT NULL,
					enabled          BOOLEAN NOT NULL DEFAULT true,
					platforms        JSONB NOT NULL DEFAULT '["windows","linux"]',
					approval_mode    TEXT NOT NULL DEFAULT 'auto' CHECK (approval_mode IN ('auto', 'manual')),
					severity         TEXT NOT NULL DEFAULT 'critical' CHECK (severity IN ('critical', 'important', 'moderate', 'low')),
					maintenance_window TEXT DEFAULT 'outside_business_hours',
					device_filter    JSONB DEFAULT '{}',
					max_retries      INT DEFAULT 3,
					created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_patch_policies_tenant ON patch_policies(tenant_id);
				ALTER TABLE patch_policies ENABLE ROW LEVEL SECURITY;
				CREATE POLICY tenant_isolation_patch_policies ON patch_policies
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
			`,
			Down: `DROP TABLE IF EXISTS patch_policies CASCADE;`,
		},
		{
			ID:   12,
			Name: "create_patch_deployments_table",
			Up: `
				CREATE TABLE IF NOT EXISTS patch_deployments (
					id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					policy_id     UUID NOT NULL REFERENCES patch_policies(id) ON DELETE CASCADE,
					tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					status        TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'deploying', 'installed', 'failed')),
					device_count  INT DEFAULT 0,
					installed     INT DEFAULT 0,
					failed        INT DEFAULT 0,
					pending       INT DEFAULT 0,
					scheduled_for TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					started_at    TIMESTAMPTZ,
					completed_at  TIMESTAMPTZ,
					created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_patch_deployments_tenant ON patch_deployments(tenant_id);
				CREATE INDEX IF NOT EXISTS idx_patch_deployments_schedule ON patch_deployments(status, scheduled_for);
				ALTER TABLE patch_deployments ENABLE ROW LEVEL SECURITY;
				CREATE POLICY tenant_isolation_patch_deployments ON patch_deployments
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
			`,
			Down: `DROP TABLE IF EXISTS patch_deployments CASCADE;`,
		},
		{
			ID:   13,
			Name: "create_patch_device_tracking",
			Up: `
				CREATE TABLE IF NOT EXISTS patch_deployment_devices (
					deployment_id UUID NOT NULL REFERENCES patch_deployments(id) ON DELETE CASCADE,
					device_id     UUID NOT NULL,
					PRIMARY KEY (deployment_id, device_id)
				);
				CREATE TABLE IF NOT EXISTS patch_device_states (
					deployment_id UUID NOT NULL,
					device_id     UUID NOT NULL,
					patch_id      TEXT NOT NULL,
					status        TEXT NOT NULL DEFAULT 'pending',
					attempts      INT DEFAULT 0,
					error         TEXT DEFAULT '',
					updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					PRIMARY KEY (deployment_id, device_id, patch_id)
				);
				CREATE TABLE IF NOT EXISTS patch_inventory (
					id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					device_id  UUID NOT NULL,
					snapshot   JSONB NOT NULL DEFAULT '{}',
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_patch_inventory_lookup ON patch_inventory(tenant_id, device_id, created_at DESC);
				ALTER TABLE patch_inventory ENABLE ROW LEVEL SECURITY;
				CREATE POLICY tenant_isolation_patch_inventory ON patch_inventory
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
			`,
			Down: `DROP TABLE IF EXISTS patch_inventory CASCADE; DROP TABLE IF EXISTS patch_device_states CASCADE; DROP TABLE IF EXISTS patch_deployment_devices CASCADE;`,
		},
		{
			ID:   14,
			Name: "create_vulnerability_tables",
			Up: `
				CREATE TABLE IF NOT EXISTS cve_database (
					id            TEXT PRIMARY KEY,
					package_name  TEXT NOT NULL,
					severity      TEXT NOT NULL DEFAULT 'unknown',
					score         DOUBLE PRECISION DEFAULT 0,
					description   TEXT DEFAULT '',
					fixed_in      TEXT NOT NULL,
					published     TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_cve_package ON cve_database(package_name);

				CREATE TABLE IF NOT EXISTS device_vulnerabilities (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					cve_id          TEXT NOT NULL REFERENCES cve_database(id),
					package_name    TEXT NOT NULL,
					current_version TEXT NOT NULL,
					fixed_in        TEXT NOT NULL,
					severity        TEXT NOT NULL DEFAULT 'unknown',
					status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'patched', 'ignored')),
					detected_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					resolved_at     TIMESTAMPTZ,
					UNIQUE(device_id, cve_id)
				);
				CREATE INDEX IF NOT EXISTS idx_device_vulns_device ON device_vulnerabilities(device_id);
				CREATE INDEX IF NOT EXISTS idx_device_vulns_status ON device_vulnerabilities(device_id, status);
				CREATE INDEX IF NOT EXISTS idx_device_vulns_severity ON device_vulnerabilities(device_id, severity);
			`,
			Down: `DROP TABLE IF EXISTS device_vulnerabilities CASCADE; DROP TABLE IF EXISTS cve_database CASCADE;`,
		},
		{
			ID:   15,
			Name: "seed_base_cve_data",
			Up: `
				INSERT INTO cve_database (id, package_name, severity, score, description, fixed_in) VALUES
					('CVE-2024-6387', 'openssh', 'critical', 9.8, 'OpenSSH regreSSHion: RCE in sshd', '9.8p1'),
					('CVE-2024-3094', 'xz-utils', 'critical', 10.0, 'XZ Utils backdoor (liblzma)', '5.6.1'),
					('CVE-2024-21626', 'docker', 'high', 8.6, 'Docker / runc container escape via process.cwd', '25.0.2'),
					('CVE-2024-4437', 'glibc', 'high', 8.4, 'glibc LD_PRELOAD privilege escalation', '2.40'),
					('CVE-2024-2961', 'glibc', 'high', 8.1, 'glibc iconv buffer overflow', '2.40'),
					('CVE-2024-31497', 'openssl', 'high', 7.5, 'PuTTY/OpenSSL ECDSA bias attack', '3.3.0'),
					('CVE-2024-27316', 'httpd', 'medium', 6.5, 'Apache HTTPd HTTP/2 CONTINUATION flood', '2.4.59'),
					('CVE-2024-24786', 'protobuf', 'medium', 5.5, 'Protocol Buffers JSON parse DoS', '25.0'),
					('CVE-2024-24576', 'rust', 'medium', 5.0, 'Rust std::process::Command argument injection on Windows', '1.77.2'),
					('CVE-2024-27135', 'curl', 'medium', 5.3, 'cURL HSTS subdomain match bypass', '8.7.0')
				ON CONFLICT (id) DO NOTHING;
			`,
			Down: `DELETE FROM cve_database;`,
		},
		{
			ID:   16,
			Name: "create_mfa_secrets_table",
			Up: `
				CREATE TABLE IF NOT EXISTS mfa_secrets (
					user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
					tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					secret     TEXT NOT NULL,
					enabled    BOOLEAN NOT NULL DEFAULT false,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				ALTER TABLE mfa_secrets ENABLE ROW LEVEL SECURITY;
				CREATE POLICY tenant_isolation_mfa_secrets ON mfa_secrets
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
			`,
			Down: `DROP TABLE IF EXISTS mfa_secrets CASCADE;`,
		},
		{
			ID:   17,
			Name: "create_session_recordings_table",
			Up: `
				CREATE TABLE IF NOT EXISTS session_recordings (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					session_id      TEXT NOT NULL,
					tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					user_id         UUID REFERENCES users(id),
					storage_key     TEXT NOT NULL,
					size_bytes      BIGINT DEFAULT 0,
					duration_ms     BIGINT DEFAULT 0,
					format          TEXT NOT NULL DEFAULT 'mkv',
					checksum_sha256 TEXT NOT NULL DEFAULT '',
					storage_backend TEXT NOT NULL DEFAULT 'minio',
					expires_at      TIMESTAMPTZ,
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_session_recordings_tenant ON session_recordings(tenant_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_session_recordings_session ON session_recordings(session_id);
				CREATE INDEX IF NOT EXISTS idx_session_recordings_expires ON session_recordings(expires_at) WHERE expires_at IS NOT NULL;
				ALTER TABLE session_recordings ENABLE ROW LEVEL SECURITY;
				CREATE POLICY tenant_isolation_session_recordings ON session_recordings
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
			`,
			Down: `DROP TABLE IF EXISTS session_recordings CASCADE;`,
		},
		{
			ID:   18,
			Name: "extend_cve_database",
			Up: `
				ALTER TABLE cve_database ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual';
				ALTER TABLE cve_database ADD COLUMN IF NOT EXISTS fixed_in_versions TEXT[] DEFAULT '{}';

				CREATE TABLE IF NOT EXISTS cve_sync_state (
					id          TEXT PRIMARY KEY,
					source      TEXT NOT NULL,
					last_synced TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					status      TEXT NOT NULL DEFAULT 'idle',
					error       TEXT DEFAULT '',
					records_new INT DEFAULT 0,
					records_updated INT DEFAULT 0
				);

				CREATE TABLE IF NOT EXISTS cve_package_ecosystem (
					package_name TEXT NOT NULL,
					ecosystem    TEXT NOT NULL DEFAULT 'Debian',
					PRIMARY KEY (package_name, ecosystem)
				);
			`,
			Down: `
				ALTER TABLE cve_database DROP COLUMN IF EXISTS source;
				ALTER TABLE cve_database DROP COLUMN IF EXISTS fixed_in_versions;
				DROP TABLE IF EXISTS cve_sync_state;
				DROP TABLE IF EXISTS cve_package_ecosystem;
			`,
		},
		{
			ID:   19,
			Name: "create_tenant_encryption_keys_table",
			Up: `
				CREATE TABLE IF NOT EXISTS tenant_encryption_keys (
					id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id    UUID UNIQUE NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					key_alias    TEXT NOT NULL DEFAULT 'primary',
					kms_type     TEXT NOT NULL DEFAULT 'local' CHECK (kms_type IN ('local', 'aws_kms', 'gcp_kms', 'azure_kv')),
					kms_key_id   TEXT DEFAULT '',
					encryption   TEXT NOT NULL DEFAULT 'aes-256-gcm' CHECK (encryption IN ('aes-256-gcm', 'aes-256-cbc', 'sse-s3', 'sse-kms')),
					key_material BYTEA DEFAULT '',
					region       TEXT DEFAULT '',
					endpoint     TEXT DEFAULT '',
					status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'rotating', 'compromised', 'disabled')),
					rotated_at   TIMESTAMPTZ,
					expires_at   TIMESTAMPTZ,
					created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_tenant_encryption_keys_tenant ON tenant_encryption_keys(tenant_id);
				ALTER TABLE tenant_encryption_keys ENABLE ROW LEVEL SECURITY;
				CREATE POLICY tenant_isolation_encryption_keys ON tenant_encryption_keys
					USING (tenant_id = current_setting('app.tenant_id')::UUID);
			`,
			Down: `DROP TABLE IF EXISTS tenant_encryption_keys CASCADE;`,
		},
		{
			ID:   20,
			Name: "create_user_tenant_access_and_auth",
			Up: `
				ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash_updated TEXT DEFAULT '';

				CREATE TABLE IF NOT EXISTS user_tenant_access (
					user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					granted_by UUID REFERENCES users(id),
					PRIMARY KEY (user_id, tenant_id)
				);
				CREATE INDEX IF NOT EXISTS idx_user_tenant_access_user ON user_tenant_access(user_id);
				CREATE INDEX IF NOT EXISTS idx_user_tenant_access_tenant ON user_tenant_access(tenant_id);

				CREATE TABLE IF NOT EXISTS audit_auth (
					id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					user_id    UUID REFERENCES users(id),
					action     TEXT NOT NULL,
					ip_address TEXT DEFAULT '',
					success    BOOLEAN NOT NULL DEFAULT true,
					details    JSONB DEFAULT '{}',
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_audit_auth_user_time ON audit_auth(user_id, created_at DESC);
			`,
			Down: `
				DROP TABLE IF EXISTS user_tenant_access CASCADE;
				DROP TABLE IF EXISTS audit_auth CASCADE;
				ALTER TABLE users DROP COLUMN IF EXISTS password_hash_updated;
			`,
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
