package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

const DefaultLockID = int64(0x535452415441524D) // "STRATARM" as safe positive int64
const DefaultRecoveryLockID = DefaultLockID + 1

func GetLockID() int64 {
	if env := os.Getenv("STRATA_MIGRATION_LOCK_ID"); env != "" {
		var id int64
		n, err := fmt.Sscanf(env, "%x", &id)
		if err == nil && n == 1 && id >= 0 {
			return id
		}
	}
	return DefaultLockID
}

func GetRecoveryLockID() int64 {
	return DefaultRecoveryLockID
}

type Migration struct {
	ID   int
	Name string
	Up   string
	Down string
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
		{
			ID:   21,
			Name: "add_deployment_id_to_tenants",
			Up: `
				ALTER TABLE tenants ADD COLUMN IF NOT EXISTS deployment_id TEXT UNIQUE;
				CREATE INDEX IF NOT EXISTS idx_tenants_deployment_id ON tenants(deployment_id);

				CREATE TABLE IF NOT EXISTS agent_registrations (
					id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					deployment_id TEXT NOT NULL,
					device_id     UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					agent_id      TEXT NOT NULL,
					public_key    BYTEA NOT NULL,
					hostname      TEXT NOT NULL,
					os            TEXT DEFAULT '',
					arch          TEXT DEFAULT '',
					ip_address    TEXT DEFAULT '',
					approved      BOOLEAN NOT NULL DEFAULT false,
					registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					approved_at   TIMESTAMPTZ
				);
				CREATE INDEX IF NOT EXISTS idx_agent_registrations_deployment ON agent_registrations(deployment_id);
			`,
			Down: `
				DROP TABLE IF EXISTS agent_registrations CASCADE;
				ALTER TABLE tenants DROP COLUMN IF EXISTS deployment_id;
			`,
		},
		{
			ID:   22,
			Name: "create_scripting_tables",
			Up: `
				CREATE TABLE IF NOT EXISTS scripts (
					id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id   UUID REFERENCES tenants(id) ON DELETE CASCADE,
					name        TEXT NOT NULL,
					description TEXT DEFAULT '',
					language    TEXT NOT NULL CHECK (language IN ('powershell', 'bash', 'python', 'batch')),
					content     TEXT NOT NULL,
					parameters  JSONB DEFAULT '[]',
					timeout_sec INT DEFAULT 300,
					is_public   BOOLEAN NOT NULL DEFAULT false,
					created_by  UUID REFERENCES users(id),
					created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_scripts_tenant ON scripts(tenant_id);

				CREATE TABLE IF NOT EXISTS script_executions (
					id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					script_id    UUID REFERENCES scripts(id) ON DELETE SET NULL,
					tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					device_id    UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					triggered_by UUID REFERENCES users(id),
					status       TEXT NOT NULL DEFAULT 'pending'
					             CHECK (status IN ('pending', 'running', 'success', 'failed', 'timeout', 'cancelled')),
					stdout       TEXT DEFAULT '',
					stderr       TEXT DEFAULT '',
					exit_code    INT,
					duration_ms  INT,
					parameters   JSONB DEFAULT '{}',
					started_at   TIMESTAMPTZ,
					completed_at TIMESTAMPTZ,
					created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_script_executions_tenant ON script_executions(tenant_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_script_executions_device ON script_executions(device_id);
				CREATE INDEX IF NOT EXISTS idx_script_executions_status ON script_executions(status);
			`,
			Down: `
				DROP TABLE IF EXISTS script_executions CASCADE;
				DROP TABLE IF EXISTS scripts CASCADE;
			`,
		},
		{
			ID:   23,
			Name: "create_software_deployment_tables",
			Up: `
				CREATE TABLE IF NOT EXISTS software_packages (
					id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id   UUID REFERENCES tenants(id) ON DELETE CASCADE,
					name        TEXT NOT NULL,
					version     TEXT NOT NULL,
					description TEXT DEFAULT '',
					platform    TEXT NOT NULL CHECK (platform IN ('windows', 'linux', 'macos', 'all')),
					package_type TEXT NOT NULL CHECK (package_type IN ('msi', 'exe', 'deb', 'rpm', 'appimage', 'script', 'other')),
					source_url  TEXT NOT NULL,
					checksum    TEXT DEFAULT '',
					checksum_type TEXT DEFAULT 'sha256',
					install_args TEXT DEFAULT '',
					uninstall_args TEXT DEFAULT '',
					detect_command TEXT DEFAULT '',
					is_third_party BOOLEAN NOT NULL DEFAULT false,
					created_by  UUID REFERENCES users(id),
					created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_software_packages_tenant ON software_packages(tenant_id);

				CREATE TABLE IF NOT EXISTS software_deployments (
					id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					package_id     UUID NOT NULL REFERENCES software_packages(id) ON DELETE CASCADE,
					tenant_id      UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					name           TEXT NOT NULL,
					status         TEXT NOT NULL DEFAULT 'pending'
					               CHECK (status IN ('pending', 'approved', 'deploying', 'completed', 'failed', 'cancelled')),
					schedule_type  TEXT NOT NULL DEFAULT 'now' CHECK (schedule_type IN ('now', 'maintenance_window', 'scheduled')),
					scheduled_for  TIMESTAMPTZ,
					created_by     UUID REFERENCES users(id),
					created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					completed_at   TIMESTAMPTZ
				);
				CREATE INDEX IF NOT EXISTS idx_software_deployments_tenant ON software_deployments(tenant_id);

				CREATE TABLE IF NOT EXISTS software_deployment_targets (
					deployment_id UUID NOT NULL REFERENCES software_deployments(id) ON DELETE CASCADE,
					device_id     UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					status        TEXT NOT NULL DEFAULT 'pending'
					              CHECK (status IN ('pending', 'downloading', 'installing', 'success', 'failed', 'skipped')),
					error_message TEXT DEFAULT '',
					duration_ms   INT DEFAULT 0,
					started_at    TIMESTAMPTZ,
					completed_at  TIMESTAMPTZ,
					PRIMARY KEY (deployment_id, device_id)
				);
			`,
			Down: `
				DROP TABLE IF EXISTS software_deployment_targets CASCADE;
				DROP TABLE IF EXISTS software_deployments CASCADE;
				DROP TABLE IF EXISTS software_packages CASCADE;
			`,
		},
		{
			ID:   24,
			Name: "create_reporting_tables",
			Up: `
				CREATE TABLE IF NOT EXISTS report_schedules (
					id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					name        TEXT NOT NULL,
					frequency   TEXT NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly')),
					day_of_week INT DEFAULT 1,
					day_of_month INT DEFAULT 1,
					format      TEXT NOT NULL DEFAULT 'pdf',
					sections    JSONB NOT NULL DEFAULT '["summary","alerts","cves","patches"]',
					recipients  TEXT[] DEFAULT '{}',
					enabled     BOOLEAN NOT NULL DEFAULT true,
					last_sent   TIMESTAMPTZ,
					created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_report_schedules_tenant ON report_schedules(tenant_id);

				CREATE TABLE IF NOT EXISTS generated_reports (
					id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					schedule_id UUID REFERENCES report_schedules(id) ON DELETE SET NULL,
					tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					name        TEXT NOT NULL,
					format      TEXT NOT NULL DEFAULT 'pdf',
					storage_key TEXT NOT NULL,
					size_bytes  BIGINT DEFAULT 0,
					sections    JSONB DEFAULT '[]',
					generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_generated_reports_tenant ON generated_reports(tenant_id, generated_at DESC);
			`,
			Down: `
				DROP TABLE IF EXISTS generated_reports CASCADE;
				DROP TABLE IF EXISTS report_schedules CASCADE;
			`,
		},
		{
			ID:   73,
			Name: "create_compliance_reports_table",
			Up: `
				CREATE TABLE IF NOT EXISTS compliance_reports (
					id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id         UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					framework         TEXT NOT NULL CHECK (framework IN ('CIS', 'HIPAA', 'PCI-DSS', 'GDPR', 'SOC2', 'NIST')),
					period_start      TIMESTAMPTZ NOT NULL,
					period_end        TIMESTAMPTZ NOT NULL,
					generated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					total_vulnerabilities INT NOT NULL DEFAULT 0,
					critical_count    INT NOT NULL DEFAULT 0,
					high_count        INT NOT NULL DEFAULT 0,
					medium_count      INT NOT NULL DEFAULT 0,
					low_count         INT NOT NULL DEFAULT 0,
					pending_count     INT NOT NULL DEFAULT 0,
					remediated_count  INT NOT NULL DEFAULT 0,
					ignored_count     INT NOT NULL DEFAULT 0,
					score             REAL NOT NULL DEFAULT 0,
					status            TEXT NOT NULL,
					findings          JSONB NOT NULL DEFAULT '[]',
					remediations      JSONB NOT NULL DEFAULT '[]'
				);
				CREATE INDEX IF NOT EXISTS idx_compliance_reports_tenant ON compliance_reports(tenant_id);
				CREATE INDEX IF NOT EXISTS idx_compliance_reports_framework ON compliance_reports(framework);
				CREATE INDEX IF NOT EXISTS idx_compliance_reports_generated_at ON compliance_reports(generated_at DESC);
			`,
			Down: `
				DROP TABLE IF EXISTS compliance_reports CASCADE;
			`,
		},
		{
			ID:   25,
			Name: "add_agent_version_and_update_source",
			Up: `
				ALTER TABLE devices ADD COLUMN IF NOT EXISTS agent_version TEXT DEFAULT '';
				ALTER TABLE tenants ADD COLUMN IF NOT EXISTS update_source TEXT NOT NULL DEFAULT 'server'
					CHECK (update_source IN ('github', 'server'));
				ALTER TABLE tenants ADD COLUMN IF NOT EXISTS update_channel TEXT NOT NULL DEFAULT 'stable'
					CHECK (update_channel IN ('stable', 'beta', 'alpha'));
				CREATE INDEX IF NOT EXISTS idx_devices_agent_version ON devices(agent_version);
			`,
			Down: `
				ALTER TABLE devices DROP COLUMN IF EXISTS agent_version;
				ALTER TABLE tenants DROP COLUMN IF EXISTS update_source;
				ALTER TABLE tenants DROP COLUMN IF EXISTS update_channel;
			`,
		},
		{
			ID:   26,
			Name: "create_alerts_table",
			Up: `
				CREATE TABLE IF NOT EXISTS alerts (
					id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					rule_id       UUID REFERENCES alert_rules(id) ON DELETE SET NULL,
					tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					device_id     TEXT,
					metric_name   TEXT,
					value         DOUBLE PRECISION,
					severity      TEXT NOT NULL,
					message       TEXT,
					status        TEXT NOT NULL DEFAULT 'firing',
					fired_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					resolved_at   TIMESTAMPTZ,
					acknowledged_at TIMESTAMPTZ,
					created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_alerts_tenant_status ON alerts(tenant_id, status);
				CREATE INDEX IF NOT EXISTS idx_alerts_rule_id ON alerts(rule_id);
			`,
			Down: `DROP TABLE IF EXISTS alerts CASCADE;`,
		},
		{
			ID:   27,
			Name: "create_msp_tenants",
			Up: `
				CREATE TABLE IF NOT EXISTS msp_tenants (
					id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					name           TEXT NOT NULL,
					slug           TEXT UNIQUE NOT NULL,
					plan           TEXT NOT NULL DEFAULT 'free',
					is_active      BOOLEAN NOT NULL DEFAULT true,
					settings       JSONB DEFAULT '{}',
					billing_email  TEXT,
					created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_msp_tenants_slug ON msp_tenants(slug);
			`,
			Down: `DROP TABLE IF EXISTS msp_tenants CASCADE;`,
		},
		{
			ID:   28,
			Name: "create_client_organizations",
			Up: `
				CREATE TABLE IF NOT EXISTS client_organizations (
					id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id         UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					name           TEXT NOT NULL,
					slug           TEXT NOT NULL,
					is_active      BOOLEAN NOT NULL DEFAULT true,
					settings       JSONB DEFAULT '{}',
					contact_email  TEXT,
					created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE(msp_id, slug)
				);
				CREATE INDEX IF NOT EXISTS idx_client_orgs_msp ON client_organizations(msp_id);
			`,
			Down: `DROP TABLE IF EXISTS client_organizations CASCADE;`,
		},
		{
			ID:   29,
			Name: "create_sites",
			Up: `
				CREATE TABLE IF NOT EXISTS sites (
					id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					client_id        UUID NOT NULL REFERENCES client_organizations(id) ON DELETE CASCADE,
					name             TEXT NOT NULL,
					slug             TEXT NOT NULL,
					address          TEXT,
					city             TEXT,
					state            TEXT,
					country          TEXT DEFAULT 'US',
					is_active        BOOLEAN NOT NULL DEFAULT true,
					contact_name     TEXT,
					contact_phone    TEXT,
					created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE(client_id, slug)
				);
				CREATE INDEX IF NOT EXISTS idx_sites_client ON sites(client_id);
			`,
			Down: `DROP TABLE IF EXISTS sites CASCADE;`,
		},
		{
			ID:   30,
			Name: "seed_default_msp",
			Up: `
				INSERT INTO msp_tenants (id, name, slug, plan)
				VALUES ('00000000-0000-0000-0000-000000000001', 'Strata Platform', 'strata', 'enterprise')
				ON CONFLICT (id) DO NOTHING;
				INSERT INTO client_organizations (id, msp_id, name, slug)
				SELECT id, '00000000-0000-0000-0000-000000000001', name, slug
				FROM tenants
				ON CONFLICT DO NOTHING;
			`,
			Down: `
				DELETE FROM client_organizations WHERE msp_id = '00000000-0000-0000-0000-000000000001';
				DELETE FROM msp_tenants WHERE id = '00000000-0000-0000-0000-000000000001';
			`,
		},
		{
			ID:   31,
			Name: "create_branding_profiles",
			Up: `
				CREATE TABLE IF NOT EXISTS branding_profiles (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id          UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					display_name    TEXT NOT NULL DEFAULT '',
					logo_light      TEXT,
					logo_dark       TEXT,
					favicon         TEXT,
					primary_color   TEXT NOT NULL DEFAULT '#2563eb',
					accent_color    TEXT NOT NULL DEFAULT '#6366f1',
					sidebar_bg      TEXT NOT NULL DEFAULT '#0f172a',
					header_bg       TEXT NOT NULL DEFAULT '#1e293b',
					login_bg        TEXT NOT NULL DEFAULT '#0f172a',
					portal_title    TEXT NOT NULL DEFAULT 'Strata RMM',
					welcome_text    TEXT NOT NULL DEFAULT 'Platform Management Console',
					support_email   TEXT,
					support_phone   TEXT,
					terms_url       TEXT,
					privacy_url     TEXT,
					is_default      BOOLEAN NOT NULL DEFAULT false,
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE(msp_id)
				);
				INSERT INTO branding_profiles (msp_id, display_name, is_default)
				SELECT id, name, true FROM msp_tenants ON CONFLICT (msp_id) DO NOTHING;
			`,
			Down: `DROP TABLE IF EXISTS branding_profiles CASCADE;`,
		},
		{
			ID:   32,
			Name: "create_custom_domains",
			Up: `
				CREATE TABLE IF NOT EXISTS custom_domains (
					id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id              UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					hostname            TEXT NOT NULL,
					domain_type         TEXT NOT NULL DEFAULT 'custom' CHECK (domain_type IN ('default', 'custom', 'portal')),
					verification_token  TEXT NOT NULL DEFAULT '',
					verification_status TEXT NOT NULL DEFAULT 'pending' CHECK (verification_status IN ('pending', 'verified', 'active', 'failed', 'suspended')),
					certificate_status  TEXT NOT NULL DEFAULT 'none' CHECK (certificate_status IN ('none', 'requested', 'issued', 'expired', 'failed')),
					is_primary          BOOLEAN NOT NULL DEFAULT false,
					branding_profile_id UUID REFERENCES branding_profiles(id) ON DELETE SET NULL,
					verified_at         TIMESTAMPTZ,
					last_check_at       TIMESTAMPTZ,
					created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE(hostname)
				);
				CREATE INDEX IF NOT EXISTS idx_custom_domains_msp ON custom_domains(msp_id);
				CREATE INDEX IF NOT EXISTS idx_custom_domains_status ON custom_domains(verification_status);
			`,
			Down: `DROP TABLE IF EXISTS custom_domains CASCADE;`,
		},
		{
			ID:   33,
			Name: "create_enrollment_tokens_v2",
			Up: `
				CREATE TABLE IF NOT EXISTS enrollment_tokens_v2 (
					id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id            UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					client_id         UUID NOT NULL REFERENCES client_organizations(id) ON DELETE CASCADE,
					site_id           UUID REFERENCES sites(id) ON DELETE CASCADE,
					token_hash        TEXT NOT NULL,
					created_by        TEXT NOT NULL DEFAULT '',
					max_uses          INT NOT NULL DEFAULT 1,
					use_count         INT NOT NULL DEFAULT 0,
					expires_at        TIMESTAMPTZ NOT NULL,
					is_revoked        BOOLEAN NOT NULL DEFAULT false,
					created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_enroll_tokens_hash ON enrollment_tokens_v2(token_hash);
				CREATE INDEX IF NOT EXISTS idx_enroll_tokens_msp ON enrollment_tokens_v2(msp_id);
			`,
			Down: `DROP TABLE IF EXISTS enrollment_tokens_v2 CASCADE;`,
		},
		{
			ID:   36,
			Name: "create_device_groups",
			Up: `
				CREATE TABLE IF NOT EXISTS device_groups (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id          UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					client_id       UUID NOT NULL REFERENCES client_organizations(id) ON DELETE CASCADE,
					site_id         UUID REFERENCES sites(id) ON DELETE CASCADE,
					name            TEXT NOT NULL,
					description     TEXT NOT NULL DEFAULT '',
					filter_tags     JSONB DEFAULT '{}',
					member_ids      TEXT[] NOT NULL DEFAULT '{}',
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_device_groups_msp ON device_groups(msp_id, client_id);
			`,
			Down: `DROP TABLE IF EXISTS device_groups CASCADE;`,
		},
		{
			ID:   34,
			Name: "create_jobs_table",
			Up: `
				CREATE TABLE IF NOT EXISTS jobs (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id          UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					client_id       UUID NOT NULL REFERENCES client_organizations(id) ON DELETE CASCADE,
					site_id         UUID REFERENCES sites(id) ON DELETE CASCADE,
					created_by      TEXT NOT NULL DEFAULT '',
					type            TEXT NOT NULL,
					status          TEXT NOT NULL DEFAULT 'pending'
						CHECK (status IN ('pending','queued','dispatched','running','succeeded','failed','cancelled','expired')),
					priority        INT NOT NULL DEFAULT 0,
					payload         JSONB NOT NULL DEFAULT '{}',
					result          JSONB,
					idempotency_key TEXT,
					max_retries     INT NOT NULL DEFAULT 3,
					retry_count     INT NOT NULL DEFAULT 0,
					max_devices     INT NOT NULL DEFAULT 0,
					completed_count INT NOT NULL DEFAULT 0,
					failed_count    INT NOT NULL DEFAULT 0,
					expires_at      TIMESTAMPTZ,
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					started_at      TIMESTAMPTZ,
					completed_at    TIMESTAMPTZ
				);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_idempotency ON jobs(idempotency_key) WHERE idempotency_key IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_jobs_msp_status ON jobs(msp_id, status);
				CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at DESC);
			`,
			Down: `DROP TABLE IF EXISTS jobs CASCADE;`,
		},
		{
			ID:   35,
			Name: "create_job_targets_table",
			Up: `
				CREATE TABLE IF NOT EXISTS job_targets (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					job_id          UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
					device_id       TEXT NOT NULL,
					status          TEXT NOT NULL DEFAULT 'pending'
						CHECK (status IN ('pending','queued','dispatched','acknowledged','running','succeeded','failed','cancelled','expired')),
					result          JSONB,
					error_message   TEXT,
					started_at      TIMESTAMPTZ,
					completed_at    TIMESTAMPTZ,
					retry_count     INT NOT NULL DEFAULT 0,
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_job_targets_job ON job_targets(job_id);
				CREATE INDEX IF NOT EXISTS idx_job_targets_device ON job_targets(device_id);
			`,
			Down: `DROP TABLE IF EXISTS job_targets CASCADE;`,
		},
		{
			ID:   37,
			Name: "create_policies_table",
			Up: `
				CREATE TABLE IF NOT EXISTS policies (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id          UUID REFERENCES msp_tenants(id) ON DELETE CASCADE,
					client_id       UUID REFERENCES client_organizations(id) ON DELETE CASCADE,
					site_id         UUID REFERENCES sites(id) ON DELETE CASCADE,
					name            TEXT NOT NULL,
					category        TEXT NOT NULL DEFAULT 'agent',
					description     TEXT NOT NULL DEFAULT '',
					config          JSONB NOT NULL DEFAULT '{}',
					version         INT NOT NULL DEFAULT 1,
					status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','archived')),
					is_default      BOOLEAN NOT NULL DEFAULT false,
					parent_id       UUID REFERENCES policies(id) ON DELETE SET NULL,
					scope_level     TEXT NOT NULL DEFAULT 'msp' CHECK (scope_level IN ('platform','msp','client','site','device')),
					created_by      TEXT NOT NULL DEFAULT '',
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_policies_msp ON policies(msp_id);
				CREATE INDEX IF NOT EXISTS idx_policies_scope ON policies(category, scope_level);
			`,
			Down: `DROP TABLE IF EXISTS policies CASCADE;`,
		},
		{
			ID:   38,
			Name: "create_policy_assignments",
			Up: `
				CREATE TABLE IF NOT EXISTS policy_assignments (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					policy_id       UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
					device_id       TEXT NOT NULL,
					effective_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_policy_assignments_device ON policy_assignments(device_id);
				CREATE INDEX IF NOT EXISTS idx_policy_assignments_policy ON policy_assignments(policy_id);
			`,
			Down: `DROP TABLE IF EXISTS policy_assignments CASCADE;`,
		},
		{
			ID:   39,
			Name: "create_platform_memberships",
			Up: `
				CREATE TABLE IF NOT EXISTS platforms (
					id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					name       TEXT NOT NULL DEFAULT 'Strata Platform',
					slug       TEXT UNIQUE NOT NULL DEFAULT 'strata',
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE TABLE IF NOT EXISTS memberships (
					id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					user_id    TEXT NOT NULL,
					role       TEXT NOT NULL DEFAULT 'viewer',
					scope_type TEXT NOT NULL CHECK (scope_type IN ('platform','msp','client','site')),
					scope_id   TEXT NOT NULL,
					created_by TEXT NOT NULL DEFAULT '',
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					expires_at TIMESTAMPTZ
				);
				CREATE INDEX IF NOT EXISTS idx_memberships_user ON memberships(user_id);
				CREATE INDEX IF NOT EXISTS idx_memberships_scope ON memberships(scope_type, scope_id);

				INSERT INTO platforms (id, name, slug)
				VALUES ('00000000-0000-0000-0000-000000000001', 'Strata Platform', 'strata')
				ON CONFLICT (id) DO NOTHING;
			`,
			Down: `DROP TABLE IF EXISTS memberships CASCADE; DROP TABLE IF EXISTS platforms CASCADE;`,
		},
		{
			ID:   40,
			Name: "create_support_grants",
			Up: `
				CREATE TABLE IF NOT EXISTS support_access_grants (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					platform_user_id TEXT NOT NULL,
					msp_id          UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					client_id       UUID REFERENCES client_organizations(id) ON DELETE CASCADE,
					site_id         UUID REFERENCES sites(id) ON DELETE CASCADE,
					reason          TEXT NOT NULL DEFAULT '',
					ticket_ref      TEXT NOT NULL DEFAULT '',
					approved_by     TEXT NOT NULL DEFAULT '',
					approved_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					expires_at      TIMESTAMPTZ NOT NULL,
					revoked_at      TIMESTAMPTZ,
					status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','expired','revoked'))
				);
				CREATE INDEX IF NOT EXISTS idx_support_grants_msp ON support_access_grants(msp_id, status);
				CREATE INDEX IF NOT EXISTS idx_support_grants_user ON support_access_grants(platform_user_id);
			`,
			Down: `DROP TABLE IF EXISTS support_access_grants CASCADE;`,
		},
		{
			ID:   41,
			Name: "add_site_id_to_devices",
			Up: `
				ALTER TABLE devices ADD COLUMN IF NOT EXISTS site_id UUID REFERENCES sites(id) ON DELETE SET NULL;
				ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_id UUID REFERENCES client_organizations(id) ON DELETE SET NULL;
				ALTER TABLE devices ADD COLUMN IF NOT EXISTS msp_id UUID REFERENCES msp_tenants(id) ON DELETE SET NULL;
				CREATE INDEX IF NOT EXISTS idx_devices_site ON devices(site_id);
				CREATE INDEX IF NOT EXISTS idx_devices_client ON devices(client_id);
				CREATE INDEX IF NOT EXISTS idx_devices_msp ON devices(msp_id);
			`,
			Down: `
				ALTER TABLE devices DROP COLUMN IF EXISTS site_id;
				ALTER TABLE devices DROP COLUMN IF EXISTS client_id;
				ALTER TABLE devices DROP COLUMN IF EXISTS msp_id;
			`,
		},
		{
			ID:   42,
			Name: "backfill_device_ownership",
			Up: `
				UPDATE devices d SET client_id = co.id
				FROM client_organizations co
				WHERE d.client_id IS NULL AND co.slug = 'dev';

				UPDATE devices d SET msp_id = co.msp_id
				FROM client_organizations co
				WHERE d.msp_id IS NULL AND d.client_id = co.id;

				INSERT INTO sites (id, client_id, name, slug)
				SELECT gen_random_uuid(), co.id, 'Default Site', 'default'
				FROM client_organizations co
				WHERE NOT EXISTS (SELECT 1 FROM sites s WHERE s.client_id = co.id AND s.slug = 'default');

				UPDATE devices d SET site_id = s.id
				FROM sites s
				WHERE d.site_id IS NULL AND d.client_id = s.client_id AND s.slug = 'default';
			`,
			Down: `
				UPDATE devices SET site_id = NULL, client_id = NULL, msp_id = NULL;
				DELETE FROM sites WHERE slug = 'default';
			`,
		},
		{
			ID:   43,
			Name: "repair_device_ownership_and_rls",
			Up: `
				-- Repair device ownership: map via legacy tenant_id
				UPDATE devices d SET client_id = co.id
				FROM client_organizations co
				WHERE d.client_id IS NULL AND d.tenant_id = co.id;

				UPDATE devices d SET msp_id = co.msp_id
				FROM client_organizations co
				WHERE d.msp_id IS NULL AND d.client_id = co.id;

				-- Create default sites for any client missing one
				INSERT INTO sites (id, client_id, name, slug)
				SELECT gen_random_uuid(), co.id, 'Default Site', 'default'
				FROM client_organizations co
				WHERE NOT EXISTS (SELECT 1 FROM sites s WHERE s.client_id = co.id AND s.slug = 'default');

				UPDATE devices d SET site_id = s.id
				FROM sites s
				WHERE d.site_id IS NULL AND d.client_id = s.client_id AND s.slug = 'default';

				-- Report validation
				SELECT 'orphan devices' as check_name, COUNT(*) FROM devices WHERE client_id IS NULL;
				SELECT 'orphan sites' as check_name, COUNT(*) FROM sites WHERE client_id IS NULL;
			`,
			Down: `SELECT 1; -- no-op; repair is idempotent`,
		},
		{
			ID:   44,
			Name: "ownership_constraints_and_rls",
			Up: `
				-- Validate ownership before adding constraints
				DO $$ BEGIN
					-- Check for orphans
					IF EXISTS (SELECT 1 FROM devices WHERE client_id IS NULL OR msp_id IS NULL OR site_id IS NULL) THEN
						RAISE WARNING 'devices with missing ownership exist; run validation';
					END IF;
					IF EXISTS (SELECT 1 FROM client_organizations WHERE msp_id IS NULL) THEN
						RAISE WARNING 'client_organizations with missing msp_id exist';
					END IF;
				END $$;

				-- Enable RLS on tenant-owned tables
				ALTER TABLE IF EXISTS client_organizations ENABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS sites ENABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS devices ENABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS memberships ENABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS support_access_grants ENABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS jobs ENABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS job_targets ENABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS device_groups ENABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS enrollment_tokens_v2 ENABLE ROW LEVEL SECURITY;

				-- Drop existing policies if any to allow idempotent re-apply
				DROP POLICY IF EXISTS msp_isolation_client_orgs ON client_organizations;
				DROP POLICY IF EXISTS msp_isolation_sites ON sites;
				DROP POLICY IF EXISTS msp_isolation_devices ON devices;

				-- MSP isolation policies
				CREATE POLICY msp_isolation_client_orgs ON client_organizations
					USING (msp_id = current_setting('app.msp_id')::uuid OR current_setting('app.role') = 'platform_admin');

				CREATE POLICY msp_isolation_sites ON sites
					USING (client_id IN (SELECT id FROM client_organizations WHERE msp_id = current_setting('app.msp_id')::uuid)
					       OR current_setting('app.role') = 'platform_admin');

				CREATE POLICY msp_isolation_devices ON devices
					USING (msp_id = current_setting('app.msp_id')::uuid OR current_setting('app.role') = 'platform_admin');

				CREATE POLICY msp_isolation_memberships ON memberships
					USING (scope_id = current_setting('app.msp_id')::text OR current_setting('app.role') = 'platform_admin');
			`,
			Down: `
				ALTER TABLE IF EXISTS client_organizations DISABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS sites DISABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS devices DISABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS memberships DISABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS support_access_grants DISABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS jobs DISABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS job_targets DISABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS device_groups DISABLE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS enrollment_tokens_v2 DISABLE ROW LEVEL SECURITY;
			`,
		},
		{
			ID:   45,
			Name: "add_missing_rls_policies_and_force_rls",
			Up: `
				-- Add WITH CHECK policies for INSERT/UPDATE on core tenant tables
				DROP POLICY IF EXISTS msp_isolation_client_orgs_insert ON client_organizations;
				CREATE POLICY msp_isolation_client_orgs_insert ON client_organizations
					FOR INSERT WITH CHECK (msp_id = current_setting('app.msp_id')::uuid);
				DROP POLICY IF EXISTS msp_isolation_client_orgs_update ON client_organizations;
				CREATE POLICY msp_isolation_client_orgs_update ON client_organizations
					FOR UPDATE USING (msp_id = current_setting('app.msp_id')::uuid);

				DROP POLICY IF EXISTS msp_isolation_devices_insert ON devices;
				CREATE POLICY msp_isolation_devices_insert ON devices
					FOR INSERT WITH CHECK (msp_id = current_setting('app.msp_id')::uuid);
				DROP POLICY IF EXISTS msp_isolation_devices_update ON devices;
				CREATE POLICY msp_isolation_devices_update ON devices
					FOR UPDATE USING (msp_id = current_setting('app.msp_id')::uuid);

				-- Safe missing-setting helper function
				CREATE OR REPLACE FUNCTION safe_msp_id() RETURNS uuid AS $$
				BEGIN
					BEGIN
						RETURN current_setting('app.msp_id')::uuid;
					EXCEPTION WHEN OTHERS THEN
						RETURN NULL;
					END;
				END;
				$$ LANGUAGE plpgsql IMMUTABLE;

				-- Force RLS on key tables (only for owner role)
				ALTER TABLE IF EXISTS client_organizations FORCE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS devices FORCE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS sites FORCE ROW LEVEL SECURITY;
			`,
			Down: `
				ALTER TABLE IF EXISTS client_organizations NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS devices NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE IF EXISTS sites NO FORCE ROW LEVEL SECURITY;
				DROP FUNCTION IF EXISTS safe_msp_id();
			`,
		},
		{
			ID:   46,
			Name: "harden_memberships_and_support_grants",
			Up: `
				ALTER TABLE memberships
					ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
				ALTER TABLE memberships
					DROP CONSTRAINT IF EXISTS memberships_status_check;
				ALTER TABLE memberships
					ADD CONSTRAINT memberships_status_check
					CHECK (status IN ('active', 'revoked'));
				CREATE UNIQUE INDEX IF NOT EXISTS idx_memberships_active_unique
					ON memberships(user_id, scope_type, scope_id, role)
					WHERE status = 'active';

				ALTER TABLE support_access_grants
					ADD COLUMN IF NOT EXISTS requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
				ALTER TABLE support_access_grants
					ADD COLUMN IF NOT EXISTS revoked_by TEXT NOT NULL DEFAULT '';
				ALTER TABLE support_access_grants
					ADD COLUMN IF NOT EXISTS permissions TEXT[] NOT NULL DEFAULT ARRAY['read']::TEXT[];

				CREATE OR REPLACE FUNCTION safe_msp_id() RETURNS uuid AS $$
					SELECT NULLIF(current_setting('app.msp_id', true), '')::uuid
				$$ LANGUAGE SQL STABLE;
			`,
			Down: `
				DROP INDEX IF EXISTS idx_memberships_active_unique;
				ALTER TABLE memberships DROP COLUMN IF EXISTS status;
				ALTER TABLE support_access_grants DROP COLUMN IF EXISTS requested_at;
				ALTER TABLE support_access_grants DROP COLUMN IF EXISTS revoked_by;
				ALTER TABLE support_access_grants DROP COLUMN IF EXISTS permissions;
			`,
		},
		{
			ID:   47,
			Name: "enforce_fail_closed_tenant_rls",
			Up: `
				-- Migration 45 forced sites before request-scoped settings existed.
				-- Temporarily restore owner access while normalizing legacy ownership;
				-- this migration re-enables FORCE RLS below before committing.
				ALTER TABLE client_organizations NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE sites NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE devices NO FORCE ROW LEVEL SECURITY;

				INSERT INTO tenants (id, name, slug, plan, is_active)
				SELECT c.id, c.name, c.msp_id::text || '-' || c.slug, 'managed', c.is_active
				FROM client_organizations c
				WHERE NOT EXISTS (SELECT 1 FROM tenants t WHERE t.id = c.id)
				ON CONFLICT (id) DO NOTHING;

				INSERT INTO sites (id, client_id, name, slug)
				SELECT gen_random_uuid(), c.id, 'Default Site', 'default'
				FROM client_organizations c
				WHERE NOT EXISTS (
					SELECT 1 FROM sites s WHERE s.client_id = c.id AND s.is_active = true
				);

				DO $$
				BEGIN
					IF EXISTS (
						SELECT 1 FROM devices
						WHERE msp_id IS NULL OR client_id IS NULL OR site_id IS NULL
					) THEN
						RAISE EXCEPTION 'cannot enforce tenant RLS: devices with incomplete ownership exist';
					END IF;
					IF EXISTS (
						SELECT 1 FROM sites s
						LEFT JOIN client_organizations c ON c.id = s.client_id
						WHERE c.id IS NULL
					) THEN
						RAISE EXCEPTION 'cannot enforce tenant RLS: orphan sites exist';
					END IF;
				END $$;

				CREATE OR REPLACE FUNCTION safe_app_setting(setting_name text)
				RETURNS text
				LANGUAGE SQL STABLE
				AS $$
					SELECT NULLIF(current_setting(setting_name, true), '')
				$$;

				CREATE OR REPLACE FUNCTION safe_msp_id()
				RETURNS uuid
				LANGUAGE SQL STABLE
				AS $$
					SELECT safe_app_setting('app.msp_id')::uuid
				$$;

				CREATE OR REPLACE FUNCTION app_is_platform_admin()
				RETURNS boolean
				LANGUAGE SQL STABLE
				AS $$
					SELECT COALESCE(safe_app_setting('app.role') = 'platform_admin', false)
				$$;

				CREATE OR REPLACE FUNCTION support_access_allowed(target_msp uuid)
				RETURNS boolean
				LANGUAGE SQL STABLE SECURITY DEFINER
				SET search_path = pg_catalog, public
				AS $$
					SELECT EXISTS (
						SELECT 1
						FROM public.support_access_grants g
						JOIN public.msp_tenants m ON m.id = g.msp_id
						WHERE g.id::text = public.safe_app_setting('app.support_grant_id')
						  AND g.platform_user_id = public.safe_app_setting('app.user_id')
						  AND g.msp_id = target_msp
						  AND g.status = 'active'
						  AND g.revoked_at IS NULL
						  AND g.expires_at > statement_timestamp()
						  AND m.is_active = true
						  AND COALESCE(public.safe_app_setting('app.permission'), 'read') = ANY(g.permissions)
					)
				$$;

				-- Remove every superseded policy, including the legacy tenant policy
				-- that reads app.tenant_id without missing-setting protection.
				DROP POLICY IF EXISTS tenant_isolation_devices ON devices;
				DROP POLICY IF EXISTS msp_isolation_client_orgs ON client_organizations;
				DROP POLICY IF EXISTS msp_isolation_client_orgs_insert ON client_organizations;
				DROP POLICY IF EXISTS msp_isolation_client_orgs_update ON client_organizations;
				DROP POLICY IF EXISTS msp_isolation_sites ON sites;
				DROP POLICY IF EXISTS msp_isolation_devices ON devices;
				DROP POLICY IF EXISTS msp_isolation_devices_insert ON devices;
				DROP POLICY IF EXISTS msp_isolation_devices_update ON devices;
				DROP POLICY IF EXISTS msp_isolation_memberships ON memberships;
				DROP POLICY IF EXISTS tenant_scope ON support_access_grants;
				DROP POLICY IF EXISTS tenant_scope ON jobs;
				DROP POLICY IF EXISTS tenant_scope ON job_targets;
				DROP POLICY IF EXISTS tenant_scope ON device_groups;
				DROP POLICY IF EXISTS tenant_scope ON enrollment_tokens_v2;

				CREATE POLICY tenant_scope ON client_organizations
					USING (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
						OR EXISTS (
							SELECT 1 FROM enrollment_tokens_v2 et
							WHERE et.client_id = client_organizations.id
							  AND et.token_hash = safe_app_setting('app.enrollment_hash')
						)
						OR EXISTS (
							SELECT 1
							FROM devices d
							JOIN agent_registrations ar ON ar.device_id = d.id
							WHERE d.client_id = client_organizations.id
							  AND ar.agent_id = safe_app_setting('app.user_id')
							  AND ar.approved = true
						)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
					);

				CREATE POLICY tenant_scope ON sites
					USING (
						app_is_platform_admin()
						OR EXISTS (
							SELECT 1 FROM client_organizations c
							WHERE c.id = sites.client_id
							  AND (c.msp_id = safe_msp_id() OR support_access_allowed(c.msp_id))
						)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR EXISTS (
							SELECT 1 FROM client_organizations c
							WHERE c.id = sites.client_id
							  AND (c.msp_id = safe_msp_id() OR support_access_allowed(c.msp_id))
						)
					);

				CREATE POLICY tenant_scope ON devices
					USING (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
						OR EXISTS (
							SELECT 1 FROM agent_registrations ar
							WHERE ar.device_id = devices.id
							  AND ar.agent_id = safe_app_setting('app.user_id')
							  AND ar.approved = true
						)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
					);

				CREATE POLICY tenant_scope ON memberships
					USING (
						app_is_platform_admin()
						OR user_id = safe_app_setting('app.user_id')
						OR (scope_type = 'msp' AND scope_id = safe_msp_id()::text)
						OR (
							scope_type = 'client'
							AND EXISTS (
								SELECT 1 FROM client_organizations c
								WHERE c.id::text = memberships.scope_id
								  AND (c.msp_id = safe_msp_id() OR support_access_allowed(c.msp_id))
							)
						)
						OR (
							scope_type = 'site'
							AND EXISTS (
								SELECT 1 FROM sites s
								JOIN client_organizations c ON c.id = s.client_id
								WHERE s.id::text = memberships.scope_id
								  AND (c.msp_id = safe_msp_id() OR support_access_allowed(c.msp_id))
							)
						)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR (scope_type = 'msp' AND scope_id = safe_msp_id()::text)
						OR (
							scope_type = 'client'
							AND EXISTS (
								SELECT 1 FROM client_organizations c
								WHERE c.id::text = memberships.scope_id
								  AND c.msp_id = safe_msp_id()
							)
						)
						OR (
							scope_type = 'site'
							AND EXISTS (
								SELECT 1 FROM sites s
								JOIN client_organizations c ON c.id = s.client_id
								WHERE s.id::text = memberships.scope_id
								  AND c.msp_id = safe_msp_id()
							)
						)
					);

				CREATE POLICY tenant_scope ON support_access_grants
					USING (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR (
							id::text = safe_app_setting('app.support_grant_id')
							AND platform_user_id = safe_app_setting('app.user_id')
							AND status = 'active'
							AND revoked_at IS NULL
							AND expires_at > statement_timestamp()
							AND COALESCE(safe_app_setting('app.permission'), 'read') = ANY(permissions)
						)
					)
					WITH CHECK (app_is_platform_admin() OR msp_id = safe_msp_id());

				CREATE POLICY tenant_scope ON jobs
					USING (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
					);

				CREATE POLICY tenant_scope ON job_targets
					USING (
						app_is_platform_admin()
						OR EXISTS (
							SELECT 1 FROM jobs j
							WHERE j.id = job_targets.job_id
							  AND (j.msp_id = safe_msp_id() OR support_access_allowed(j.msp_id))
						)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR EXISTS (
							SELECT 1 FROM jobs j
							WHERE j.id = job_targets.job_id
							  AND (j.msp_id = safe_msp_id() OR support_access_allowed(j.msp_id))
						)
					);

				CREATE POLICY tenant_scope ON device_groups
					USING (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
					);

				CREATE POLICY tenant_scope ON enrollment_tokens_v2
					USING (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
						OR token_hash = safe_app_setting('app.enrollment_hash')
					)
					WITH CHECK (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
						OR token_hash = safe_app_setting('app.enrollment_hash')
					);

				ALTER TABLE client_organizations ENABLE ROW LEVEL SECURITY;
				ALTER TABLE sites ENABLE ROW LEVEL SECURITY;
				ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
				ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
				ALTER TABLE support_access_grants ENABLE ROW LEVEL SECURITY;
				ALTER TABLE jobs ENABLE ROW LEVEL SECURITY;
				ALTER TABLE job_targets ENABLE ROW LEVEL SECURITY;
				ALTER TABLE device_groups ENABLE ROW LEVEL SECURITY;
				ALTER TABLE enrollment_tokens_v2 ENABLE ROW LEVEL SECURITY;

				ALTER TABLE client_organizations FORCE ROW LEVEL SECURITY;
				ALTER TABLE sites FORCE ROW LEVEL SECURITY;
				ALTER TABLE devices FORCE ROW LEVEL SECURITY;
				ALTER TABLE memberships FORCE ROW LEVEL SECURITY;
				ALTER TABLE support_access_grants FORCE ROW LEVEL SECURITY;
				ALTER TABLE jobs FORCE ROW LEVEL SECURITY;
				ALTER TABLE job_targets FORCE ROW LEVEL SECURITY;
				ALTER TABLE device_groups FORCE ROW LEVEL SECURITY;
				ALTER TABLE enrollment_tokens_v2 FORCE ROW LEVEL SECURITY;
			`,
			Down: `
				ALTER TABLE client_organizations NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE sites NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE devices NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE memberships NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE support_access_grants NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE jobs NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE job_targets NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE device_groups NO FORCE ROW LEVEL SECURITY;
				ALTER TABLE enrollment_tokens_v2 NO FORCE ROW LEVEL SECURITY;
				DROP FUNCTION IF EXISTS support_access_allowed(uuid);
				DROP FUNCTION IF EXISTS app_is_platform_admin();
				DROP FUNCTION IF EXISTS safe_app_setting(text);
			`,
		},
		{
			ID:   48,
			Name: "synchronize_client_device_ownership",
			Up: `
				INSERT INTO tenants (id, name, slug, plan, is_active)
				SELECT c.id, c.name, c.msp_id::text || '-' || c.slug, 'managed', c.is_active
				FROM client_organizations c
				WHERE NOT EXISTS (SELECT 1 FROM tenants t WHERE t.id = c.id)
				ON CONFLICT (id) DO NOTHING;

				INSERT INTO sites (id, client_id, name, slug)
				SELECT gen_random_uuid(), c.id, 'Default Site', 'default'
				FROM client_organizations c
				WHERE NOT EXISTS (
					SELECT 1 FROM sites s WHERE s.client_id = c.id AND s.is_active = true
				);
			`,
			Down: `SELECT 1; -- ownership synchronization is intentionally retained`,
		},
		{
			ID:   49,
			Name: "create_job_outbox",
			Up: `
				CREATE TABLE IF NOT EXISTS job_outbox (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id          UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					aggregate_type  TEXT NOT NULL DEFAULT 'job',
					aggregate_id    UUID NOT NULL,
					event_type      TEXT NOT NULL,
					schema_version  INT NOT NULL DEFAULT 1,
					payload         JSONB NOT NULL DEFAULT '{}',
					available_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					attempt_count   INT NOT NULL DEFAULT 0,
					lease_owner     TEXT,
					lease_expires   TIMESTAMPTZ,
					published_at    TIMESTAMPTZ,
					last_error      TEXT,
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_job_outbox_available ON job_outbox(available_at, published_at) WHERE published_at IS NULL;
				CREATE INDEX IF NOT EXISTS idx_job_outbox_msp ON job_outbox(msp_id);
			`,
			Down: `DROP TABLE IF EXISTS job_outbox CASCADE;`,
		},
		{
			ID:   50,
			Name: "create_job_inbox",
			Up: `
				CREATE TABLE IF NOT EXISTS job_inbox (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id          UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					message_id      TEXT NOT NULL,
					job_id          UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
					target_id       UUID REFERENCES job_targets(id) ON DELETE CASCADE,
					event_type      TEXT NOT NULL,
					payload         JSONB NOT NULL DEFAULT '{}',
					processed_at    TIMESTAMPTZ,
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_job_inbox_message ON job_inbox(msp_id, message_id);
				CREATE INDEX IF NOT EXISTS idx_job_inbox_job ON job_inbox(job_id);
			`,
			Down: `DROP TABLE IF EXISTS job_inbox CASCADE;`,
		},
		{
			ID:   51,
			Name: "enhance_jobs_for_durable_dispatch",
			Up: `
				ALTER TABLE jobs ADD COLUMN IF NOT EXISTS correlation_id TEXT;
				ALTER TABLE jobs ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;
				ALTER TABLE jobs ADD COLUMN IF NOT EXISTS scheduled_for TIMESTAMPTZ;
				ALTER TABLE jobs ADD COLUMN IF NOT EXISTS cancelled_by TEXT;
				ALTER TABLE jobs ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;
				ALTER TABLE jobs ADD COLUMN IF NOT EXISTS cancel_reason TEXT;
				ALTER TABLE jobs ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;
				ALTER TABLE jobs ADD COLUMN IF NOT EXISTS dispatch_count INT NOT NULL DEFAULT 0;

				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS agent_id TEXT;
				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS attempt INT NOT NULL DEFAULT 0;
				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS lease_owner TEXT;
				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS lease_expires TIMESTAMPTZ;
				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ;
				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS dispatched_at TIMESTAMPTZ;
				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMPTZ;
				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS exit_code INT;
				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS msp_id UUID REFERENCES msp_tenants(id) ON DELETE CASCADE;

				UPDATE job_targets jt SET msp_id = j.msp_id FROM jobs j WHERE jt.job_id = j.id AND jt.msp_id IS NULL;

				ALTER TABLE jobs ALTER COLUMN correlation_id SET DEFAULT gen_random_uuid()::text;

				DROP INDEX IF EXISTS idx_jobs_idempotency;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_idempotency_msp ON jobs(msp_id, idempotency_key) WHERE idempotency_key IS NOT NULL;

				CREATE INDEX IF NOT EXISTS idx_jobs_correlation ON jobs(correlation_id);
				CREATE INDEX IF NOT EXISTS idx_job_targets_lease ON job_targets(status, lease_expires) WHERE lease_owner IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_job_targets_retry ON job_targets(status, next_retry_at) WHERE next_retry_at IS NOT NULL;
			`,
			Down: `
				ALTER TABLE jobs DROP COLUMN IF EXISTS correlation_id;
				ALTER TABLE jobs DROP COLUMN IF EXISTS version;
				ALTER TABLE jobs DROP COLUMN IF EXISTS scheduled_for;
				ALTER TABLE jobs DROP COLUMN IF EXISTS cancelled_by;
				ALTER TABLE jobs DROP COLUMN IF EXISTS cancelled_at;
				ALTER TABLE jobs DROP COLUMN IF EXISTS cancel_reason;
				ALTER TABLE jobs DROP COLUMN IF EXISTS updated_at;
				ALTER TABLE jobs DROP COLUMN IF EXISTS dispatch_count;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS agent_id;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS attempt;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS lease_owner;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS lease_expires;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS next_retry_at;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS dispatched_at;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS acknowledged_at;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS exit_code;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS msp_id;
			`,
		},
		{
			ID:   52,
			Name: "harden_durable_job_orchestration",
			Up: `
				ALTER TABLE jobs ADD COLUMN IF NOT EXISTS request_hash TEXT;
				CREATE INDEX IF NOT EXISTS idx_job_targets_agent ON job_targets(msp_id, agent_id);

			`,
			Down: `
				DROP INDEX IF EXISTS idx_job_targets_agent;
				ALTER TABLE jobs DROP COLUMN IF EXISTS request_hash;
			`,
		},
		{
			ID:   53,
			Name: "create_plans_and_entitlements",
			Up: `
				CREATE TABLE IF NOT EXISTS plans (
					id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					name        TEXT NOT NULL,
					slug        TEXT UNIQUE NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					is_active   BOOLEAN NOT NULL DEFAULT true,
					features    JSONB NOT NULL DEFAULT '{}',
					max_devices INT NOT NULL DEFAULT 0,
					max_users   INT NOT NULL DEFAULT 0,
					created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE TABLE IF NOT EXISTS plan_entitlements (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id          UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					plan_id         UUID NOT NULL REFERENCES plans(id) ON DELETE RESTRICT,
					status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','past_due','suspended','cancelled')),
					device_count    INT NOT NULL DEFAULT 0,
					user_count      INT NOT NULL DEFAULT 0,
					started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					expires_at      TIMESTAMPTZ,
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					CHECK (device_count >= 0),
					CHECK (user_count >= 0),
					UNIQUE(msp_id)
				);
				CREATE INDEX IF NOT EXISTS idx_entitlements_msp ON plan_entitlements(msp_id);
				CREATE INDEX IF NOT EXISTS idx_entitlements_plan ON plan_entitlements(plan_id);

				-- Seed default plans
				INSERT INTO plans (id, name, slug, description, max_devices, max_users, features)
				VALUES
					('00000000-0000-0000-0000-000000000001', 'Free', 'free', 'Up to 5 devices', 5, 2, '{"scripting":true,"patching":false,"remote":false,"reporting":false}'::jsonb),
					('00000000-0000-0000-0000-000000000002', 'Starter', 'starter', 'Up to 25 devices', 25, 5, '{"scripting":true,"patching":true,"remote":false,"reporting":true}'::jsonb),
					('00000000-0000-0000-0000-000000000003', 'Professional', 'professional', 'Up to 100 devices', 100, 15, '{"scripting":true,"patching":true,"remote":true,"reporting":true}'::jsonb)
				ON CONFLICT (id) DO NOTHING;

				-- Assign default Free plan to existing MSPs
				INSERT INTO plan_entitlements (msp_id, plan_id, device_count, user_count)
				SELECT id, '00000000-0000-0000-0000-000000000001', 0, 0
				FROM msp_tenants
				ON CONFLICT (msp_id) DO NOTHING;

				ALTER TABLE plan_entitlements ENABLE ROW LEVEL SECURITY;

				DROP POLICY IF EXISTS tenant_scope ON plan_entitlements;
				CREATE POLICY tenant_scope ON plan_entitlements
					USING (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR msp_id = safe_msp_id()
						OR support_access_allowed(msp_id)
					);

				ALTER TABLE plan_entitlements FORCE ROW LEVEL SECURITY;
			`,
			Down: `DROP TABLE IF EXISTS plan_entitlements CASCADE; DROP TABLE IF EXISTS plans CASCADE;`,
		},
		{
			ID:   54,
			Name: "complete_saas_control_plane",
			Up: `
				CREATE TABLE IF NOT EXISTS usage_snapshots (
					id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id       UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					device_count INT NOT NULL DEFAULT 0 CHECK (device_count >= 0),
					user_count   INT NOT NULL DEFAULT 0 CHECK (user_count >= 0),
					client_count INT NOT NULL DEFAULT 0 CHECK (client_count >= 0),
					site_count   INT NOT NULL DEFAULT 0 CHECK (site_count >= 0),
					recorded_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_usage_snapshots_msp_time
					ON usage_snapshots(msp_id, recorded_at DESC);

				CREATE TABLE IF NOT EXISTS control_plane_audit (
					id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id        UUID REFERENCES msp_tenants(id) ON DELETE SET NULL,
					actor_user_id TEXT NOT NULL DEFAULT '',
					action        TEXT NOT NULL,
					resource_type TEXT NOT NULL,
					resource_id   TEXT NOT NULL DEFAULT '',
					details       JSONB NOT NULL DEFAULT '{}',
					created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_control_plane_audit_msp_time
					ON control_plane_audit(msp_id, created_at DESC);

				ALTER TABLE usage_snapshots ENABLE ROW LEVEL SECURITY;
				ALTER TABLE control_plane_audit ENABLE ROW LEVEL SECURITY;

				CREATE POLICY tenant_scope ON usage_snapshots
					USING (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id))
					WITH CHECK (app_is_platform_admin() OR msp_id = safe_msp_id());
				CREATE POLICY tenant_scope ON control_plane_audit
					USING (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id))
					WITH CHECK (app_is_platform_admin() OR msp_id = safe_msp_id());

				ALTER TABLE usage_snapshots FORCE ROW LEVEL SECURITY;
				ALTER TABLE control_plane_audit FORCE ROW LEVEL SECURITY;
			`,
			Down: `
				DROP TABLE IF EXISTS control_plane_audit CASCADE;
				DROP TABLE IF EXISTS usage_snapshots CASCADE;
			`,
		},
		{
			ID:   55,
			Name: "create_endpoint_approval_policies",
			Up: `
				CREATE TABLE IF NOT EXISTS endpoint_approval_policies (
					id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id                UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					action_name           TEXT NOT NULL,
					approval_required     BOOLEAN NOT NULL DEFAULT true,
					min_approvers         INT NOT NULL DEFAULT 1 CHECK (min_approvers >= 1),
					allowed_roles         TEXT[] NOT NULL DEFAULT ARRAY['msp_owner','msp_admin'],
					require_separation    BOOLEAN NOT NULL DEFAULT true,
					approval_expires_secs INT NOT NULL DEFAULT 3600,
					allow_emergency       BOOLEAN NOT NULL DEFAULT false,
					created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE(msp_id, action_name)
				);

				CREATE TABLE IF NOT EXISTS endpoint_approval_requests (
					id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id             UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					client_id          UUID REFERENCES client_organizations(id) ON DELETE SET NULL,
					site_id            UUID REFERENCES sites(id) ON DELETE SET NULL,
					requester_user_id  TEXT NOT NULL,
					action_name        TEXT NOT NULL,
					reason             TEXT NOT NULL DEFAULT '',
					device_ids         UUID[] NOT NULL DEFAULT '{}',
					device_count       INT NOT NULL DEFAULT 0,
					schedule_at        TIMESTAMPTZ,
					target_hash        TEXT NOT NULL DEFAULT '',
					status             TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','cancelled','expired','dispatched')),
					policy_snapshot    JSONB NOT NULL DEFAULT '{}',
					correlation_id     TEXT,
					request_hash       TEXT,
					idempotency_key    TEXT,
					expires_at         TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '1 hour',
					emergency_override BOOLEAN NOT NULL DEFAULT false,
					decided_at         TIMESTAMPTZ,
					decided_by         TEXT,
					created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE TABLE IF NOT EXISTS endpoint_approval_decisions (
					id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					request_id      UUID NOT NULL REFERENCES endpoint_approval_requests(id) ON DELETE CASCADE,
					msp_id          UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					approver_user_id TEXT NOT NULL,
					decision        TEXT NOT NULL CHECK (decision IN ('approved','rejected')),
					reason          TEXT NOT NULL DEFAULT '',
					created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE(request_id, approver_user_id)
				);

				ALTER TABLE endpoint_approval_policies ENABLE ROW LEVEL SECURITY;
				ALTER TABLE endpoint_approval_requests ENABLE ROW LEVEL SECURITY;
				ALTER TABLE endpoint_approval_decisions ENABLE ROW LEVEL SECURITY;

				CREATE POLICY tenant_scope ON endpoint_approval_policies
					USING (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id))
					WITH CHECK (app_is_platform_admin() OR msp_id = safe_msp_id());
				CREATE POLICY tenant_scope ON endpoint_approval_requests
					USING (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id))
					WITH CHECK (app_is_platform_admin() OR msp_id = safe_msp_id());
				CREATE POLICY tenant_scope ON endpoint_approval_decisions
					USING (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id))
					WITH CHECK (app_is_platform_admin() OR msp_id = safe_msp_id());

				ALTER TABLE endpoint_approval_policies FORCE ROW LEVEL SECURITY;
				ALTER TABLE endpoint_approval_requests FORCE ROW LEVEL SECURITY;
				ALTER TABLE endpoint_approval_decisions FORCE ROW LEVEL SECURITY;

				CREATE INDEX IF NOT EXISTS idx_approval_requests_msp_status ON endpoint_approval_requests(msp_id, status);
				CREATE INDEX IF NOT EXISTS idx_approval_requests_requester ON endpoint_approval_requests(requester_user_id);
				CREATE INDEX IF NOT EXISTS idx_approval_decisions_request ON endpoint_approval_decisions(request_id);
			`,
			Down: `
				DROP TABLE IF EXISTS endpoint_approval_decisions CASCADE;
				DROP TABLE IF EXISTS endpoint_approval_requests CASCADE;
				DROP TABLE IF EXISTS endpoint_approval_policies CASCADE;
			`,
		},
		{
			ID:   56,
			Name: "create_agent_capabilities",
			Up: `
				CREATE TABLE IF NOT EXISTS agent_capabilities (
					id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					device_id          UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					msp_id             UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					agent_version      TEXT NOT NULL DEFAULT '',
					protocol_version   INT NOT NULL DEFAULT 1,
					os                 TEXT NOT NULL DEFAULT '',
					arch               TEXT NOT NULL DEFAULT '',
					supported_job_types TEXT[] NOT NULL DEFAULT '{}',
					features           JSONB NOT NULL DEFAULT '{}',
					inventory_schema   INT NOT NULL DEFAULT 1,
					last_updated       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE(device_id)
				);
				CREATE INDEX IF NOT EXISTS idx_agent_capabilities_msp ON agent_capabilities(msp_id);

				ALTER TABLE agent_capabilities ENABLE ROW LEVEL SECURITY;
				CREATE POLICY tenant_scope ON agent_capabilities
					USING (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id))
					WITH CHECK (app_is_platform_admin() OR msp_id = safe_msp_id());
				ALTER TABLE agent_capabilities FORCE ROW LEVEL SECURITY;
			`,
			Down: `DROP TABLE IF EXISTS agent_capabilities CASCADE;`,
		},
		{
			ID:   57,
			Name: "create_endpoint_audit_evidence",
			Up: `
				CREATE TABLE IF NOT EXISTS endpoint_audit_evidence (
					id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id              UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					client_id           UUID REFERENCES client_organizations(id) ON DELETE SET NULL,
					site_id             UUID REFERENCES sites(id) ON DELETE SET NULL,
					device_id           UUID REFERENCES devices(id) ON DELETE SET NULL,
					actor_user_id       TEXT NOT NULL DEFAULT '',
					actor_role          TEXT NOT NULL DEFAULT '',
					support_grant_id    TEXT,
					request_source      TEXT NOT NULL DEFAULT 'api',
					normalized_ip       TEXT,
					action              TEXT NOT NULL,
					targets             JSONB NOT NULL DEFAULT '[]',
					reason              TEXT NOT NULL DEFAULT '',
					request_hash        TEXT NOT NULL DEFAULT '',
					idempotency_key     TEXT,
					policy_snapshot     JSONB NOT NULL DEFAULT '{}',
					approval_state      TEXT NOT NULL DEFAULT 'none',
					approval_decisions  JSONB NOT NULL DEFAULT '[]',
					job_id              TEXT,
					target_id           TEXT,
					correlation_id      TEXT,
					schedule_at         TIMESTAMPTZ,
					maintenance_window  JSONB,
					state_transition    TEXT NOT NULL DEFAULT '',
					agent_receipt_at    TIMESTAMPTZ,
					execution_started_at TIMESTAMPTZ,
					execution_result    JSONB,
					exit_code           INT,
					result_summary      TEXT NOT NULL DEFAULT '',
					failure_reason      TEXT NOT NULL DEFAULT '',
					created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_endpoint_audit_msp_time ON endpoint_audit_evidence(msp_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_endpoint_audit_device ON endpoint_audit_evidence(device_id);
				CREATE INDEX IF NOT EXISTS idx_endpoint_audit_job ON endpoint_audit_evidence(job_id);
				CREATE INDEX IF NOT EXISTS idx_endpoint_audit_correlation ON endpoint_audit_evidence(correlation_id);

				ALTER TABLE endpoint_audit_evidence ENABLE ROW LEVEL SECURITY;

				CREATE POLICY tenant_scope ON endpoint_audit_evidence
					USING (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id))
					WITH CHECK (false);

				CREATE POLICY insert_endpoint_audit_evidence ON endpoint_audit_evidence
					FOR INSERT
					WITH CHECK (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id));

				ALTER TABLE endpoint_audit_evidence FORCE ROW LEVEL SECURITY;
			`,
			Down: `DROP TABLE IF EXISTS endpoint_audit_evidence CASCADE;`,
		},
		{
			ID:   58,
			Name: "enhance_maintenance_windows",
			Up: `
				ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS msp_id UUID REFERENCES msp_tenants(id) ON DELETE CASCADE;
				ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS client_id UUID REFERENCES client_organizations(id) ON DELETE CASCADE;
				ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS site_id UUID REFERENCES sites(id) ON DELETE CASCADE;
				ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS device_group_id UUID REFERENCES device_groups(id) ON DELETE CASCADE;
				ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS device_id UUID REFERENCES devices(id) ON DELETE CASCADE;
				ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'UTC';
				ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
				ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS is_recurring BOOLEAN NOT NULL DEFAULT false;
				ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS recurrence_rule TEXT;
				ALTER TABLE maintenance_windows ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

				CREATE INDEX IF NOT EXISTS idx_mw_msp ON maintenance_windows(msp_id) WHERE msp_id IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_mw_client ON maintenance_windows(client_id) WHERE client_id IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_mw_site ON maintenance_windows(site_id) WHERE site_id IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_mw_device ON maintenance_windows(device_id) WHERE device_id IS NOT NULL;
			`,
			Down: `
				DROP INDEX IF EXISTS idx_mw_msp;
				DROP INDEX IF EXISTS idx_mw_client;
				DROP INDEX IF EXISTS idx_mw_site;
				DROP INDEX IF EXISTS idx_mw_device;
				ALTER TABLE maintenance_windows DROP COLUMN IF EXISTS msp_id;
				ALTER TABLE maintenance_windows DROP COLUMN IF EXISTS client_id;
				ALTER TABLE maintenance_windows DROP COLUMN IF EXISTS site_id;
				ALTER TABLE maintenance_windows DROP COLUMN IF EXISTS device_group_id;
				ALTER TABLE maintenance_windows DROP COLUMN IF EXISTS device_id;
				ALTER TABLE maintenance_windows DROP COLUMN IF EXISTS timezone;
				ALTER TABLE maintenance_windows DROP COLUMN IF EXISTS expires_at;
				ALTER TABLE maintenance_windows DROP COLUMN IF EXISTS is_recurring;
				ALTER TABLE maintenance_windows DROP COLUMN IF EXISTS recurrence_rule;
				ALTER TABLE maintenance_windows DROP COLUMN IF EXISTS description;
			`,
		},
		{
			ID:   59,
			Name: "create_inventory_results",
			Up: `
				CREATE TABLE IF NOT EXISTS inventory_results (
					id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					device_id         UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					msp_id            UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					job_id            UUID REFERENCES jobs(id) ON DELETE SET NULL,
					target_id         UUID REFERENCES job_targets(id) ON DELETE SET NULL,
					correlation_id    TEXT,
					schema_version    INT NOT NULL DEFAULT 1,
					payload           JSONB NOT NULL DEFAULT '{}',
					payload_hash      TEXT NOT NULL DEFAULT '',
					collection_time   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					received_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					is_stale          BOOLEAN NOT NULL DEFAULT false,
					is_failure        BOOLEAN NOT NULL DEFAULT false,
					failure_message   TEXT NOT NULL DEFAULT '',
					accepted          BOOLEAN NOT NULL DEFAULT false,
					created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_inventory_results_device ON inventory_results(device_id, received_at DESC);
				CREATE INDEX IF NOT EXISTS idx_inventory_results_msp ON inventory_results(msp_id);
				CREATE INDEX IF NOT EXISTS idx_inventory_results_job ON inventory_results(job_id);
				CREATE INDEX IF NOT EXISTS idx_inventory_results_correlation ON inventory_results(correlation_id);

				ALTER TABLE inventory_results ENABLE ROW LEVEL SECURITY;
				CREATE POLICY tenant_scope ON inventory_results
					USING (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id))
					WITH CHECK (app_is_platform_admin() OR msp_id = safe_msp_id());
				ALTER TABLE inventory_results FORCE ROW LEVEL SECURITY;
			`,
			Down: `DROP TABLE IF EXISTS inventory_results CASCADE;`,
		},
		{
			ID:   60,
			Name: "add_offline_queue_device_fields",
			Up: `
				ALTER TABLE devices ADD COLUMN IF NOT EXISTS offline_queue_enabled BOOLEAN NOT NULL DEFAULT false;
				ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_capability_update TIMESTAMPTZ;
				ALTER TABLE devices ADD COLUMN IF NOT EXISTS inventory_last_success TIMESTAMPTZ;
				ALTER TABLE devices ADD COLUMN IF NOT EXISTS inventory_fresh BOOLEAN NOT NULL DEFAULT false;

				ALTER TABLE jobs ADD COLUMN IF NOT EXISTS approval_request_id UUID REFERENCES endpoint_approval_requests(id) ON DELETE SET NULL;
				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'none' CHECK (approval_status IN ('none','pending','approved','rejected','cancelled','expired'));
				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS offline_at TIMESTAMPTZ;
				ALTER TABLE job_targets ADD COLUMN IF NOT EXISTS reconnect_at TIMESTAMPTZ;
			`,
			Down: `
				ALTER TABLE devices DROP COLUMN IF EXISTS offline_queue_enabled;
				ALTER TABLE devices DROP COLUMN IF EXISTS last_capability_update;
				ALTER TABLE devices DROP COLUMN IF EXISTS inventory_last_success;
				ALTER TABLE devices DROP COLUMN IF EXISTS inventory_fresh;
				ALTER TABLE jobs DROP COLUMN IF EXISTS approval_request_id;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS approval_status;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS offline_at;
				ALTER TABLE job_targets DROP COLUMN IF EXISTS reconnect_at;
			`,
		},
		{
			ID:   61,
			Name: "harden_phase7_approval_inventory_audit",
			Up: `
				ALTER TABLE endpoint_approval_requests
					ADD COLUMN IF NOT EXISTS operation_payload JSONB NOT NULL DEFAULT '{}';

				CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_approval_request_unique
					ON jobs(approval_request_id) WHERE approval_request_id IS NOT NULL;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_approval_requests_idempotency
					ON endpoint_approval_requests(msp_id, idempotency_key)
					WHERE idempotency_key IS NOT NULL;

				DROP POLICY IF EXISTS tenant_scope ON endpoint_audit_evidence;
				DROP POLICY IF EXISTS endpoint_audit_select ON endpoint_audit_evidence;
				CREATE POLICY endpoint_audit_select ON endpoint_audit_evidence
					FOR SELECT
					USING (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id));

				DROP POLICY IF EXISTS insert_endpoint_audit_evidence ON endpoint_audit_evidence;
				CREATE POLICY insert_endpoint_audit_evidence ON endpoint_audit_evidence
					FOR INSERT
					WITH CHECK (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id));
			`,
			Down: `
				DROP POLICY IF EXISTS endpoint_audit_select ON endpoint_audit_evidence;
				CREATE POLICY tenant_scope ON endpoint_audit_evidence
					USING (app_is_platform_admin() OR msp_id = safe_msp_id() OR support_access_allowed(msp_id))
					WITH CHECK (false);
				DROP INDEX IF EXISTS idx_approval_requests_idempotency;
				DROP INDEX IF EXISTS idx_jobs_approval_request_unique;
				ALTER TABLE endpoint_approval_requests DROP COLUMN IF EXISTS operation_payload;
			`,
		},
		{
			ID:   62,
			Name: "add_waiting_status_to_job_targets",
			Up: `
				ALTER TABLE job_targets DROP CONSTRAINT IF EXISTS job_targets_status_check;
				ALTER TABLE job_targets ADD CONSTRAINT job_targets_status_check
					CHECK (status IN ('pending','queued','dispatched','acknowledged','running','succeeded','failed','cancelled','expired','waiting'));
			`,
			Down: `
				ALTER TABLE job_targets DROP CONSTRAINT IF EXISTS job_targets_status_check;
				ALTER TABLE job_targets ADD CONSTRAINT job_targets_status_check
					CHECK (status IN ('pending','queued','dispatched','acknowledged','running','succeeded','failed','cancelled','expired'));
			`,
		},
		{
			ID:   63,
			Name: "add_backup_recovery_tables",
			Up: `
				CREATE TABLE IF NOT EXISTS backup_records (
					id                  TEXT PRIMARY KEY,
					database_type       TEXT NOT NULL DEFAULT 'postgresql',
					version             TEXT NOT NULL DEFAULT '1.0.0',
					table_count         INT DEFAULT 0,
					row_estimate        BIGINT DEFAULT 0,
					data_size           BIGINT NOT NULL DEFAULT 0,
					compression         TEXT NOT NULL DEFAULT 'gzip',
					encryption_scheme   TEXT NOT NULL DEFAULT 'aes-256-gcm',
					key_reference       TEXT,
					integrity_digest    TEXT NOT NULL,
					status              TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed', 'failed', 'corrupted')),
					error_message       TEXT,
					created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					completed_at        TIMESTAMPTZ,
					restored_at         TIMESTAMPTZ
				);
				CREATE INDEX IF NOT EXISTS idx_backup_records_status ON backup_records(status);
				CREATE INDEX IF NOT EXISTS idx_backup_records_created ON backup_records(created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_backup_records_digest ON backup_records(integrity_digest);

				CREATE TABLE IF NOT EXISTS recovery_operations (
					id              BIGSERIAL PRIMARY KEY,
					recovery_id     TEXT NOT NULL UNIQUE,
					operation       TEXT NOT NULL,
					phase           TEXT NOT NULL DEFAULT 'unknown',
					state           TEXT NOT NULL DEFAULT 'idle',
					status          TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed', 'released')),
					started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					completed_at    TIMESTAMPTZ,
					error_message   TEXT
				);
				CREATE INDEX IF NOT EXISTS idx_recovery_ops_recovery_id ON recovery_operations(recovery_id);
				CREATE INDEX IF NOT EXISTS idx_recovery_ops_operation ON recovery_operations(operation);
				CREATE INDEX IF NOT EXISTS idx_recovery_ops_status ON recovery_operations(status);

				CREATE TABLE IF NOT EXISTS backup_audit_log (
					id              BIGSERIAL PRIMARY KEY,
					backup_id       TEXT,
					recovery_id     TEXT,
					action          TEXT NOT NULL CHECK (action IN ('backup_created', 'backup_verified', 'backup_deleted', 'restore_started', 'restore_completed', 'restore_failed', 'rollback_executed')),
					details         JSONB DEFAULT '{}',
					performed_by    TEXT,
					timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				CREATE INDEX IF NOT EXISTS idx_backup_audit_backup_id ON backup_audit_log(backup_id);
				CREATE INDEX IF NOT EXISTS idx_backup_audit_recovery_id ON backup_audit_log(recovery_id);
				CREATE INDEX IF NOT EXISTS idx_backup_audit_timestamp ON backup_audit_log(timestamp DESC);
			`,
			Down: `
				DROP TABLE IF EXISTS backup_audit_log;
				DROP TABLE IF EXISTS recovery_operations;
				DROP TABLE IF EXISTS backup_records;
			`,
		},
		{
			ID:   64,
			Name: "add_recovery_state_enum",
			Up: `
				-- Create enum type with idempotent check using pg_type catalog
				DO $$ BEGIN
				    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'recovery_state_enum') THEN
				        CREATE TYPE recovery_state_enum AS ENUM (
				            'idle', 'discovery', 'preflight', 'quiesce',
				            'backup_database', 'backup_jetstream', 'backup_object_storage', 'verify_integrity',
				            'pre_restore_validation', 'restore_database', 'restore_jetstream', 'restore_object_storage',
				            'post_restore_validation', 'health_check', 'verification',
				            'rpo_validation', 'rto_validation',
				            'rollback', 'cleanup', 'completed'
				        );
				    END IF;
				END $$;

				ALTER TABLE recovery_operations ADD COLUMN IF NOT EXISTS recovery_state recovery_state_enum DEFAULT 'idle'::recovery_state_enum;
				CREATE INDEX IF NOT EXISTS idx_recovery_ops_state ON recovery_operations(recovery_state);

				-- FK references primary key (id), not recovery_id
				ALTER TABLE backup_records ADD COLUMN IF NOT EXISTS recovery_id BIGINT REFERENCES recovery_operations(id) ON DELETE SET NULL;
				CREATE INDEX IF NOT EXISTS idx_backup_records_recovery_id ON backup_records(recovery_id);
			`,
			Down: `
				ALTER TABLE backup_records DROP COLUMN IF EXISTS recovery_id;
				ALTER TABLE recovery_operations DROP COLUMN IF EXISTS recovery_state;
				DROP TYPE IF EXISTS recovery_state_enum;
			`,
		},
		{
			ID:   65,
			Name: "add_recovery_mutation_gate",
			Up: `
				CREATE TABLE IF NOT EXISTS recovery_mutation_gate (
					singleton     BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
					quiesced      BOOLEAN NOT NULL DEFAULT FALSE,
					operation_id  TEXT,
					updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
				INSERT INTO recovery_mutation_gate (singleton, quiesced)
				VALUES (TRUE, FALSE)
				ON CONFLICT (singleton) DO NOTHING;

				CREATE OR REPLACE FUNCTION enforce_recovery_mutation_gate()
				RETURNS trigger
				LANGUAGE plpgsql
				AS $$
				DECLARE
					is_quiesced BOOLEAN;
				BEGIN
					IF NOT pg_try_advisory_xact_lock_shared(6004514643731632718) THEN
						RAISE EXCEPTION 'mutations are unavailable during a recovery operation'
							USING ERRCODE = '55006';
					END IF;
					SELECT quiesced INTO is_quiesced
					FROM recovery_mutation_gate
					WHERE singleton = TRUE;
					IF COALESCE(is_quiesced, TRUE) THEN
						RAISE EXCEPTION 'mutations are unavailable during a recovery operation'
							USING ERRCODE = '55006';
					END IF;
					RETURN NULL;
				END;
				$$;

				DO $$
				DECLARE
					table_record RECORD;
				BEGIN
					FOR table_record IN
						SELECT tablename
						FROM pg_tables
						WHERE schemaname = 'public'
						  AND tablename NOT IN ('recovery_mutation_gate', 'schema_migrations')
					LOOP
						EXECUTE format(
							'DROP TRIGGER IF EXISTS recovery_mutation_gate_trigger ON %I',
							table_record.tablename
						);
						EXECUTE format(
							'CREATE TRIGGER recovery_mutation_gate_trigger
							 BEFORE INSERT OR UPDATE OR DELETE ON %I
							 FOR EACH STATEMENT EXECUTE FUNCTION enforce_recovery_mutation_gate()',
							table_record.tablename
						);
					END LOOP;
				END
				$$;
			`,
			Down: `
				DROP FUNCTION IF EXISTS enforce_recovery_mutation_gate() CASCADE;
				DROP TABLE IF EXISTS recovery_mutation_gate;
			`,
		},
		{
			ID:   66,
			Name: "add_msp_lifecycle_controls",
			Up: `
				ALTER TABLE plan_entitlements
					ADD COLUMN IF NOT EXISTS grace_period_ends_at TIMESTAMPTZ;

				CREATE TABLE IF NOT EXISTS msp_offboarding (
					id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id                UUID NOT NULL UNIQUE REFERENCES msp_tenants(id) ON DELETE RESTRICT,
					state                 TEXT NOT NULL DEFAULT 'requested'
						CHECK (state IN ('requested', 'access_revoked', 'retained', 'deletion_approved')),
					reason                TEXT NOT NULL,
					requested_by          TEXT NOT NULL,
					requested_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					access_revoked_at     TIMESTAMPTZ,
					retention_until       TIMESTAMPTZ NOT NULL,
					deletion_approved_by  TEXT,
					deletion_approved_at  TIMESTAMPTZ,
					updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					CHECK (retention_until >= requested_at),
					CHECK (
						(state <> 'deletion_approved')
						OR (deletion_approved_by IS NOT NULL AND deletion_approved_at IS NOT NULL)
					)
				);
				CREATE INDEX IF NOT EXISTS idx_msp_offboarding_state_retention
					ON msp_offboarding(state, retention_until);

				ALTER TABLE msp_offboarding ENABLE ROW LEVEL SECURITY;
				DROP POLICY IF EXISTS platform_only ON msp_offboarding;
				CREATE POLICY platform_only ON msp_offboarding
					USING (app_is_platform_admin())
					WITH CHECK (app_is_platform_admin());
				ALTER TABLE msp_offboarding FORCE ROW LEVEL SECURITY;

				DROP TRIGGER IF EXISTS recovery_mutation_gate_trigger ON msp_offboarding;
				CREATE TRIGGER recovery_mutation_gate_trigger
					BEFORE INSERT OR UPDATE OR DELETE ON msp_offboarding
					FOR EACH STATEMENT EXECUTE FUNCTION enforce_recovery_mutation_gate();
			`,
			Down: `
				DROP TABLE IF EXISTS msp_offboarding;
				ALTER TABLE plan_entitlements DROP COLUMN IF EXISTS grace_period_ends_at;
			`,
		},
		{
			ID:   67,
			Name: "add_provider_business_profile",
			Up: `
				ALTER TABLE platforms
					ADD COLUMN IF NOT EXISTS legal_name TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS contact_name TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS support_email TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS billing_email TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS business_phone TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS website_url TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS address_line1 TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS address_line2 TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS city TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS state_province TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS postal_code TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS country_code TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS default_timezone TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS default_locale TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS default_currency TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS tax_identifier TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS setup_completed_at TIMESTAMPTZ,
					ADD COLUMN IF NOT EXISTS setup_completed_by UUID REFERENCES users(id) ON DELETE SET NULL,
					ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

				-- The migration transaction is trusted platform maintenance. Establish the
				-- same fail-closed RLS role used by authenticated platform requests before
				-- repairing installs created by the local bootstrap command.
				SELECT set_config('app.role', 'platform_admin', true);

				INSERT INTO memberships (
					user_id, role, scope_type, scope_id, created_by, status
				)
				SELECT DISTINCT
					a.user_id::text,
					'platform_owner',
					'platform',
					'00000000-0000-0000-0000-000000000001',
					a.user_id::text,
					'active'
				FROM audit_log a
				JOIN users u ON u.id = a.user_id
				WHERE a.action = 'platform.bootstrap_admin'
				  AND a.user_id IS NOT NULL
				ON CONFLICT (user_id, scope_type, scope_id, role)
					WHERE status = 'active'
				DO NOTHING;

				CREATE OR REPLACE FUNCTION prevent_control_plane_audit_mutation()
				RETURNS trigger
				LANGUAGE plpgsql
				AS $$
				BEGIN
					RAISE EXCEPTION 'control plane audit records are immutable'
						USING ERRCODE = '55000';
				END;
				$$;

				DROP TRIGGER IF EXISTS control_plane_audit_immutable ON control_plane_audit;
				CREATE TRIGGER control_plane_audit_immutable
					BEFORE UPDATE OR DELETE ON control_plane_audit
					FOR EACH ROW EXECUTE FUNCTION prevent_control_plane_audit_mutation();
			`,
			Down: `
				DROP TRIGGER IF EXISTS control_plane_audit_immutable ON control_plane_audit;
				DROP FUNCTION IF EXISTS prevent_control_plane_audit_mutation();
				ALTER TABLE platforms
					DROP COLUMN IF EXISTS legal_name,
					DROP COLUMN IF EXISTS display_name,
					DROP COLUMN IF EXISTS contact_name,
					DROP COLUMN IF EXISTS support_email,
					DROP COLUMN IF EXISTS billing_email,
					DROP COLUMN IF EXISTS business_phone,
					DROP COLUMN IF EXISTS website_url,
					DROP COLUMN IF EXISTS address_line1,
					DROP COLUMN IF EXISTS address_line2,
					DROP COLUMN IF EXISTS city,
					DROP COLUMN IF EXISTS state_province,
					DROP COLUMN IF EXISTS postal_code,
					DROP COLUMN IF EXISTS country_code,
					DROP COLUMN IF EXISTS default_timezone,
					DROP COLUMN IF EXISTS default_locale,
					DROP COLUMN IF EXISTS default_currency,
					DROP COLUMN IF EXISTS tax_identifier,
					DROP COLUMN IF EXISTS setup_completed_at,
					DROP COLUMN IF EXISTS setup_completed_by,
					DROP COLUMN IF EXISTS updated_at;
			`,
		},
		{
			ID:   68,
			Name: "add_msp_owner_activation",
			Up: `
				ALTER TABLE users
					ADD COLUMN IF NOT EXISTS normalized_email TEXT
						GENERATED ALWAYS AS (lower(btrim(email))) STORED,
					ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ;

				DO $$
				DECLARE
					duplicate_report TEXT;
				BEGIN
					SELECT string_agg(normalized_email || ' (' || duplicate_count || ')', ', ' ORDER BY normalized_email)
					INTO duplicate_report
					FROM (
						SELECT normalized_email, COUNT(*) AS duplicate_count
						FROM users
						GROUP BY normalized_email
						HAVING COUNT(*) > 1
					) duplicates;
					IF duplicate_report IS NOT NULL THEN
						RAISE EXCEPTION 'migration 68 cannot enforce global normalized email uniqueness; duplicates: %', duplicate_report
							USING ERRCODE = '23505';
					END IF;
					IF EXISTS (SELECT 1 FROM users WHERE normalized_email = '') THEN
						RAISE EXCEPTION 'migration 68 cannot normalize blank user email addresses'
							USING ERRCODE = '23514';
					END IF;
				END
				$$;

				CREATE UNIQUE INDEX IF NOT EXISTS idx_users_normalized_email_unique
					ON users(normalized_email);
				ALTER TABLE users DROP CONSTRAINT IF EXISTS users_normalized_email_nonempty;
				ALTER TABLE users ADD CONSTRAINT users_normalized_email_nonempty
					CHECK (normalized_email <> '');
				UPDATE users
				SET email_verified_at = COALESCE(email_verified_at, created_at)
				WHERE is_active = TRUE;
				ALTER TABLE users ALTER COLUMN tenant_id DROP NOT NULL;

				ALTER TABLE msp_tenants
					ADD COLUMN IF NOT EXISTS onboarding_status TEXT NOT NULL DEFAULT 'active';
				UPDATE msp_tenants SET onboarding_status = 'active';
				ALTER TABLE msp_tenants DROP CONSTRAINT IF EXISTS msp_tenants_onboarding_status_check;
				ALTER TABLE msp_tenants ADD CONSTRAINT msp_tenants_onboarding_status_check
					CHECK (onboarding_status IN ('pending_owner', 'active'));

				CREATE TABLE IF NOT EXISTS account_invitations (
					id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id           UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					email_normalized TEXT NOT NULL CHECK (
						email_normalized = lower(btrim(email_normalized))
						AND email_normalized <> ''
						AND length(email_normalized) <= 320
					),
					purpose          TEXT NOT NULL DEFAULT 'msp_owner_activation'
						CHECK (purpose = 'msp_owner_activation'),
					token_hash       CHAR(64) NOT NULL UNIQUE
						CHECK (token_hash ~ '^[0-9a-f]{64}$'),
					created_by       UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
					expires_at       TIMESTAMPTZ NOT NULL,
					accepted_at      TIMESTAMPTZ,
					revoked_at       TIMESTAMPTZ,
					created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					delivery_status  TEXT NOT NULL DEFAULT 'pending'
						CHECK (delivery_status IN ('pending', 'delivered', 'failed', 'unconfigured')),
					delivered_at     TIMESTAMPTZ,
					CHECK (expires_at > created_at),
					CHECK (accepted_at IS NULL OR revoked_at IS NULL),
					CHECK ((delivery_status = 'delivered') = (delivered_at IS NOT NULL))
				);
				CREATE INDEX IF NOT EXISTS idx_account_invitations_msp_created
					ON account_invitations(msp_id, created_at DESC);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_account_invitations_one_unconsumed_msp
					ON account_invitations(msp_id)
					WHERE accepted_at IS NULL AND revoked_at IS NULL;

				ALTER TABLE account_invitations ENABLE ROW LEVEL SECURITY;
				DROP POLICY IF EXISTS account_invitation_access ON account_invitations;
				CREATE POLICY account_invitation_access ON account_invitations
					USING (
						app_is_platform_admin()
						OR token_hash = safe_app_setting('app.invitation_hash')
					)
					WITH CHECK (
						app_is_platform_admin()
						OR token_hash = safe_app_setting('app.invitation_hash')
					);
				ALTER TABLE account_invitations FORCE ROW LEVEL SECURITY;

				DROP POLICY IF EXISTS tenant_isolation_users ON users;
				DROP POLICY IF EXISTS identity_scope ON users;
				CREATE POLICY identity_scope ON users
					USING (
						app_is_platform_admin()
						OR id::text = safe_app_setting('app.user_id')
						OR tenant_id::text = safe_app_setting('app.tenant_id')
						OR normalized_email = safe_app_setting('app.login_email')
						OR EXISTS (
							SELECT 1 FROM account_invitations invitation
							WHERE invitation.token_hash = safe_app_setting('app.invitation_hash')
							  AND invitation.email_normalized = users.normalized_email
							  AND invitation.accepted_at IS NULL
							  AND invitation.revoked_at IS NULL
							  AND invitation.expires_at > statement_timestamp()
						)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR id::text = safe_app_setting('app.user_id')
						OR tenant_id::text = safe_app_setting('app.tenant_id')
						OR EXISTS (
							SELECT 1 FROM account_invitations invitation
							WHERE invitation.token_hash = safe_app_setting('app.invitation_hash')
							  AND invitation.email_normalized = users.normalized_email
							  AND invitation.accepted_at IS NULL
							  AND invitation.revoked_at IS NULL
							  AND invitation.expires_at > statement_timestamp()
						)
					);
				ALTER TABLE users FORCE ROW LEVEL SECURITY;

				DROP TRIGGER IF EXISTS recovery_mutation_gate_trigger ON account_invitations;
				CREATE TRIGGER recovery_mutation_gate_trigger
					BEFORE INSERT OR UPDATE OR DELETE ON account_invitations
					FOR EACH STATEMENT EXECUTE FUNCTION enforce_recovery_mutation_gate();
			`,
			Down: `
				SELECT set_config('app.role', 'platform_admin', true);
				UPDATE plan_entitlements entitlement
				SET status = 'active', updated_at = NOW()
				FROM msp_tenants msp
				WHERE entitlement.msp_id = msp.id
				  AND msp.onboarding_status = 'pending_owner';
				UPDATE msp_tenants
				SET is_active = TRUE
				WHERE onboarding_status = 'pending_owner';

				ALTER TABLE users NO FORCE ROW LEVEL SECURITY;
				DROP POLICY IF EXISTS identity_scope ON users;
				DROP TABLE IF EXISTS account_invitations;
				CREATE POLICY tenant_isolation_users ON users
					USING (tenant_id = current_setting('app.tenant_id')::UUID);

				DO $$
				BEGIN
					IF EXISTS (SELECT 1 FROM users WHERE tenant_id IS NULL) THEN
						RAISE EXCEPTION 'cannot rollback migration 68 while tenant-neutral user identities exist';
					END IF;
				END
				$$;
				ALTER TABLE users ALTER COLUMN tenant_id SET NOT NULL;
				ALTER TABLE users DROP CONSTRAINT IF EXISTS users_normalized_email_nonempty;
				DROP INDEX IF EXISTS idx_users_normalized_email_unique;
				ALTER TABLE users
					DROP COLUMN IF EXISTS normalized_email,
					DROP COLUMN IF EXISTS email_verified_at;
				ALTER TABLE msp_tenants
					DROP CONSTRAINT IF EXISTS msp_tenants_onboarding_status_check,
					DROP COLUMN IF EXISTS onboarding_status;
			`,
		},
		{
			ID:   69,
			Name: "enforce_scope_bound_authorization",
			Up: `
				-- This migration transaction is trusted maintenance under the legacy
				-- policy long enough to inspect every existing membership. The function
				-- is hardened below before the transaction commits.
				SELECT set_config('app.role', 'platform_admin', true);

				-- Memberships are the sole authorization source. Legacy users.role and
				-- user_tenant_access are deliberately not used to create authority.
				CREATE TABLE IF NOT EXISTS authorization_migration_issues (
					id          BIGSERIAL PRIMARY KEY,
					issue_type  TEXT NOT NULL,
					user_id     TEXT NOT NULL DEFAULT '',
					scope_type  TEXT NOT NULL DEFAULT '',
					scope_id    TEXT NOT NULL DEFAULT '',
					role        TEXT NOT NULL DEFAULT '',
					details     JSONB NOT NULL DEFAULT '{}'::jsonb,
					detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE(issue_type, user_id, scope_type, scope_id, role)
				);

				INSERT INTO authorization_migration_issues (issue_type, user_id, scope_type, scope_id, role, details)
				SELECT 'invalid_membership', mb.user_id, mb.scope_type, mb.scope_id, mb.role,
				       jsonb_build_object('status', mb.status, 'reason', 'illegal role/scope or unverified target')
				FROM memberships mb
				LEFT JOIN users u ON u.id::text = mb.user_id
				LEFT JOIN platforms p ON mb.scope_type = 'platform' AND p.id::text = mb.scope_id
				LEFT JOIN msp_tenants m ON mb.scope_type = 'msp' AND m.id::text = mb.scope_id
				LEFT JOIN client_organizations c ON mb.scope_type = 'client' AND c.id::text = mb.scope_id
				LEFT JOIN sites s ON mb.scope_type = 'site' AND s.id::text = mb.scope_id
				WHERE u.id IS NULL
				   OR (mb.scope_type = 'platform' AND (
					p.id IS NULL OR mb.scope_id <> '00000000-0000-0000-0000-000000000001'
					OR mb.role NOT IN ('platform_owner','platform_admin','platform_support','platform_billing','platform_security_auditor','platform_viewer')
				   ))
				   OR (mb.scope_type = 'msp' AND (m.id IS NULL OR mb.role NOT IN ('msp_owner','msp_admin','msp_technician','msp_viewer')))
				   OR (mb.scope_type = 'client' AND (c.id IS NULL OR mb.role NOT IN ('client_admin','client_viewer')))
				   OR (mb.scope_type = 'site' AND (s.id IS NULL OR mb.role NOT IN ('client_admin','client_viewer')))
				ON CONFLICT (issue_type, user_id, scope_type, scope_id, role) DO NOTHING;

				INSERT INTO authorization_migration_issues (issue_type, user_id, details)
				SELECT 'legacy_role_without_membership', u.id::text,
				       jsonb_build_object('legacy_role', u.role, 'disposition', 'compatibility_mirror_only')
				FROM users u
				WHERE NOT EXISTS (SELECT 1 FROM memberships mb WHERE mb.user_id = u.id::text)
				ON CONFLICT (issue_type, user_id, scope_type, scope_id, role) DO NOTHING;

				INSERT INTO authorization_migration_issues (issue_type, user_id, scope_id, details)
				SELECT 'legacy_tenant_access_without_membership', uta.user_id::text, uta.tenant_id::text,
				       jsonb_build_object('disposition', 'compatibility_mirror_only')
				FROM user_tenant_access uta
				WHERE NOT EXISTS (
					SELECT 1 FROM memberships mb
					WHERE mb.user_id = uta.user_id::text
					  AND mb.status = 'active'
					  AND (
						(mb.scope_type = 'client' AND mb.scope_id = uta.tenant_id::text)
						OR (mb.scope_type = 'site' AND EXISTS (
							SELECT 1 FROM sites child_site
							WHERE child_site.id::text = mb.scope_id AND child_site.client_id = uta.tenant_id
						))
					  )
				)
				ON CONFLICT (issue_type, user_id, scope_type, scope_id, role) DO NOTHING;

				ALTER TABLE memberships DROP CONSTRAINT IF EXISTS memberships_role_scope_check;
				ALTER TABLE memberships ADD CONSTRAINT memberships_role_scope_check CHECK (
					(scope_type = 'platform' AND scope_id = '00000000-0000-0000-0000-000000000001'
					 AND role IN ('platform_owner','platform_admin','platform_support','platform_billing','platform_security_auditor','platform_viewer'))
					OR (scope_type = 'msp' AND role IN ('msp_owner','msp_admin','msp_technician','msp_viewer'))
					OR (scope_type IN ('client','site') AND role IN ('client_admin','client_viewer'))
				) NOT VALID;

				CREATE OR REPLACE FUNCTION app_is_trusted_runtime()
				RETURNS boolean
				LANGUAGE SQL STABLE
				AS $$
					SELECT session_user = pg_get_userbyid(
						(SELECT relowner FROM pg_class WHERE oid = 'public.memberships'::regclass)
					)
				$$;

				CREATE OR REPLACE FUNCTION app_is_platform_admin()
				RETURNS boolean
				LANGUAGE SQL STABLE SECURITY DEFINER
				SET search_path = pg_catalog, public
				AS $$
					SELECT public.app_is_trusted_runtime()
					   AND COALESCE(public.safe_app_setting('app.role') = 'platform_admin', false)
					   AND COALESCE(public.safe_app_setting('app.scope_type'), 'platform') = 'platform'
					   AND public.safe_app_setting('app.msp_id') IS NULL
					   AND public.safe_app_setting('app.client_id') IS NULL
					   AND public.safe_app_setting('app.site_id') IS NULL
					   AND EXISTS (
						SELECT 1
						FROM public.memberships mb
						JOIN public.users u ON u.id::text = mb.user_id
						WHERE mb.user_id = public.safe_app_setting('app.user_id')
						  AND u.is_active = true AND u.email_verified_at IS NOT NULL
						  AND mb.scope_type = 'platform'
						  AND mb.scope_id = '00000000-0000-0000-0000-000000000001'
						  AND mb.role IN ('platform_owner','platform_admin')
						  AND mb.status = 'active'
						  AND (mb.expires_at IS NULL OR mb.expires_at > statement_timestamp())
					   )
				$$;

				-- Bootstrap is deliberately narrower than platform administration. It is
				-- available only to the table-owning application runtime and disappears
				-- as soon as the first authoritative platform membership is created.
				CREATE OR REPLACE FUNCTION app_is_initial_bootstrap()
				RETURNS boolean
				LANGUAGE SQL STABLE SECURITY DEFINER
				SET search_path = pg_catalog, public
				AS $$
					SELECT public.app_is_trusted_runtime()
					   AND COALESCE(public.safe_app_setting('app.initial_bootstrap') = 'true', false)
					   AND NOT EXISTS (
						SELECT 1 FROM public.memberships mb
						WHERE mb.scope_type = 'platform'
						  AND mb.scope_id = '00000000-0000-0000-0000-000000000001'
						  AND mb.role IN ('platform_owner','platform_admin')
						  AND mb.status = 'active'
						  AND (mb.expires_at IS NULL OR mb.expires_at > statement_timestamp())
					   )
				$$;

				-- The bootstrap command needs one idempotency check even after bootstrap
				-- RLS has closed. This helper exposes only the count, never user records.
				CREATE OR REPLACE FUNCTION app_bootstrap_user_count()
				RETURNS bigint
				LANGUAGE SQL STABLE SECURITY DEFINER
				SET search_path = pg_catalog, public
				AS $$
					SELECT COUNT(*) FROM public.users
				$$;

				CREATE OR REPLACE FUNCTION app_scope_is_authorized()
				RETURNS boolean
				LANGUAGE SQL STABLE SECURITY DEFINER
				SET search_path = pg_catalog, public
				AS $$
					SELECT public.app_is_trusted_runtime()
					AND CASE public.safe_app_setting('app.scope_type')
						WHEN 'platform' THEN
							public.safe_app_setting('app.msp_id') IS NULL
							AND public.safe_app_setting('app.client_id') IS NULL
							AND public.safe_app_setting('app.site_id') IS NULL
						WHEN 'msp' THEN
							public.safe_app_setting('app.client_id') IS NULL
							AND public.safe_app_setting('app.site_id') IS NULL
							AND EXISTS (
								SELECT 1 FROM public.msp_tenants m
								WHERE m.id::text = public.safe_app_setting('app.msp_id')
								  AND m.is_active = true AND m.onboarding_status = 'active'
							)
						WHEN 'client' THEN
							public.safe_app_setting('app.site_id') IS NULL
							AND EXISTS (
								SELECT 1 FROM public.client_organizations c
								JOIN public.msp_tenants m ON m.id = c.msp_id
								WHERE c.id::text = public.safe_app_setting('app.client_id')
								  AND c.msp_id::text = public.safe_app_setting('app.msp_id')
								  AND c.is_active = true AND m.is_active = true
								  AND m.onboarding_status = 'active'
							)
						WHEN 'site' THEN EXISTS (
							SELECT 1 FROM public.sites s
							JOIN public.client_organizations c ON c.id = s.client_id
							JOIN public.msp_tenants m ON m.id = c.msp_id
							WHERE s.id::text = public.safe_app_setting('app.site_id')
							  AND s.client_id::text = public.safe_app_setting('app.client_id')
							  AND c.msp_id::text = public.safe_app_setting('app.msp_id')
							  AND s.is_active = true AND c.is_active = true
							  AND m.is_active = true AND m.onboarding_status = 'active'
						)
						ELSE false
					END
					AND EXISTS (
						SELECT 1
						FROM public.memberships mb
						JOIN public.users u ON u.id::text = mb.user_id
						WHERE mb.user_id = public.safe_app_setting('app.user_id')
						  AND u.is_active = true AND u.email_verified_at IS NOT NULL
						  AND mb.status = 'active'
						  AND (mb.expires_at IS NULL OR mb.expires_at > statement_timestamp())
						  AND (
							(public.safe_app_setting('app.scope_type') = 'platform'
							 AND mb.scope_type = 'platform'
							 AND mb.scope_id = '00000000-0000-0000-0000-000000000001'
							 AND mb.role IN ('platform_owner','platform_admin','platform_support','platform_billing','platform_security_auditor','platform_viewer'))
							OR (public.safe_app_setting('app.scope_type') = 'msp' AND (
								(mb.scope_type = 'platform' AND mb.scope_id = '00000000-0000-0000-0000-000000000001'
								 AND mb.role IN ('platform_owner','platform_admin','platform_support','platform_billing','platform_security_auditor','platform_viewer'))
								OR (mb.scope_type = 'msp' AND mb.scope_id = public.safe_app_setting('app.msp_id')
								 AND mb.role IN ('msp_owner','msp_admin','msp_technician','msp_viewer'))
							))
							OR (public.safe_app_setting('app.scope_type') = 'client' AND (
								(mb.scope_type = 'platform' AND mb.scope_id = '00000000-0000-0000-0000-000000000001'
								 AND mb.role IN ('platform_owner','platform_admin','platform_support','platform_billing','platform_security_auditor','platform_viewer'))
								OR (mb.scope_type = 'msp' AND mb.scope_id = public.safe_app_setting('app.msp_id')
								 AND mb.role IN ('msp_owner','msp_admin','msp_technician','msp_viewer'))
								OR (mb.scope_type = 'client' AND mb.scope_id = public.safe_app_setting('app.client_id')
								 AND mb.role IN ('client_admin','client_viewer'))
							))
							OR (public.safe_app_setting('app.scope_type') = 'site' AND (
								(mb.scope_type = 'platform' AND mb.scope_id = '00000000-0000-0000-0000-000000000001'
								 AND mb.role IN ('platform_owner','platform_admin','platform_support','platform_billing','platform_security_auditor','platform_viewer'))
								OR (mb.scope_type = 'msp' AND mb.scope_id = public.safe_app_setting('app.msp_id')
								 AND mb.role IN ('msp_owner','msp_admin','msp_technician','msp_viewer'))
								OR (mb.scope_type = 'client' AND mb.scope_id = public.safe_app_setting('app.client_id')
								 AND mb.role IN ('client_admin','client_viewer'))
								OR (mb.scope_type = 'site' AND mb.scope_id = public.safe_app_setting('app.site_id')
								 AND mb.role IN ('client_admin','client_viewer'))
							))
						  )
					)
				$$;

				CREATE OR REPLACE FUNCTION app_actor_can_manage_scope()
				RETURNS boolean
				LANGUAGE SQL STABLE SECURITY DEFINER
				SET search_path = pg_catalog, public
				AS $$
					SELECT public.app_scope_is_authorized() AND EXISTS (
						SELECT 1
						FROM public.memberships mb
						WHERE mb.user_id = public.safe_app_setting('app.user_id')
						  AND mb.status = 'active'
						  AND (mb.expires_at IS NULL OR mb.expires_at > statement_timestamp())
						  AND (
							(public.safe_app_setting('app.scope_type') IN ('platform','msp','client','site')
							 AND mb.scope_type = 'platform'
							 AND mb.scope_id = '00000000-0000-0000-0000-000000000001'
							 AND mb.role IN ('platform_owner','platform_admin'))
							OR (public.safe_app_setting('app.scope_type') IN ('msp','client','site')
							 AND mb.scope_type = 'msp'
							 AND mb.scope_id = public.safe_app_setting('app.msp_id')
							 AND mb.role IN ('msp_owner','msp_admin'))
							OR (public.safe_app_setting('app.scope_type') IN ('client','site')
							 AND mb.scope_type = 'client'
							 AND mb.scope_id = public.safe_app_setting('app.client_id')
							 AND mb.role = 'client_admin')
							OR (public.safe_app_setting('app.scope_type') = 'site'
							 AND mb.scope_type = 'site'
							 AND mb.scope_id = public.safe_app_setting('app.site_id')
							 AND mb.role = 'client_admin')
						  )
					)
				$$;

				CREATE OR REPLACE FUNCTION app_may_manage_membership(target_scope_type text, target_scope_id text)
				RETURNS boolean
				LANGUAGE SQL STABLE SECURITY DEFINER
				SET search_path = pg_catalog, public
				AS $$
					SELECT public.app_is_trusted_runtime()
					   AND COALESCE(public.safe_app_setting('app.scope_manager') = 'true', false)
					   AND public.app_actor_can_manage_scope()
					   AND CASE public.safe_app_setting('app.scope_type')
						WHEN 'platform' THEN true
						WHEN 'msp' THEN
							(target_scope_type = 'msp' AND target_scope_id = public.safe_app_setting('app.msp_id'))
							OR (target_scope_type = 'client' AND EXISTS (
								SELECT 1 FROM public.client_organizations c
								WHERE c.id::text = target_scope_id
								  AND c.msp_id::text = public.safe_app_setting('app.msp_id')
								  AND c.is_active = true
							))
							OR (target_scope_type = 'site' AND EXISTS (
								SELECT 1 FROM public.sites s
								JOIN public.client_organizations c ON c.id = s.client_id
								WHERE s.id::text = target_scope_id
								  AND c.msp_id::text = public.safe_app_setting('app.msp_id')
								  AND s.is_active = true AND c.is_active = true
							))
						WHEN 'client' THEN
							(target_scope_type = 'client' AND target_scope_id = public.safe_app_setting('app.client_id'))
							OR (target_scope_type = 'site' AND EXISTS (
								SELECT 1 FROM public.sites s
								WHERE s.id::text = target_scope_id
								  AND s.client_id::text = public.safe_app_setting('app.client_id')
								  AND s.is_active = true
							))
						WHEN 'site' THEN target_scope_type = 'site'
							AND target_scope_id = public.safe_app_setting('app.site_id')
						ELSE false
					   END
				$$;

				CREATE OR REPLACE FUNCTION app_is_scope_manager()
				RETURNS boolean
				LANGUAGE SQL STABLE
				AS $$
					SELECT app_actor_can_manage_scope()
					   AND COALESCE(safe_app_setting('app.scope_manager') = 'true', false)
				$$;

				DROP POLICY IF EXISTS tenant_scope ON client_organizations;
				CREATE POLICY tenant_scope ON client_organizations
					USING (
						app_is_platform_admin()
						OR (app_scope_is_authorized() AND (
							(safe_app_setting('app.scope_type') = 'msp' AND msp_id::text = safe_app_setting('app.msp_id'))
							OR (safe_app_setting('app.scope_type') IN ('client','site') AND id::text = safe_app_setting('app.client_id'))
						))
						OR support_access_allowed(msp_id)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR (app_is_scope_manager() AND safe_app_setting('app.scope_type') = 'msp'
							AND msp_id::text = safe_app_setting('app.msp_id'))
					);

				DROP POLICY IF EXISTS tenant_scope ON sites;
				CREATE POLICY tenant_scope ON sites
					USING (
						app_is_platform_admin()
						OR (app_scope_is_authorized() AND (
							(safe_app_setting('app.scope_type') = 'msp' AND EXISTS (
								SELECT 1 FROM client_organizations c WHERE c.id = sites.client_id
								  AND c.msp_id::text = safe_app_setting('app.msp_id')
							))
							OR (safe_app_setting('app.scope_type') = 'client' AND client_id::text = safe_app_setting('app.client_id'))
							OR (safe_app_setting('app.scope_type') = 'site' AND id::text = safe_app_setting('app.site_id'))
						))
					)
					WITH CHECK (
						app_is_platform_admin()
						OR (app_is_scope_manager() AND (
							(safe_app_setting('app.scope_type') = 'client' AND client_id::text = safe_app_setting('app.client_id'))
							OR (safe_app_setting('app.scope_type') = 'site' AND id::text = safe_app_setting('app.site_id'))
						))
					);

				DROP POLICY IF EXISTS tenant_scope ON devices;
				CREATE POLICY tenant_scope ON devices
					USING (
						app_is_platform_admin()
						OR (app_scope_is_authorized() AND (
							(safe_app_setting('app.scope_type') = 'msp' AND msp_id::text = safe_app_setting('app.msp_id'))
							OR (safe_app_setting('app.scope_type') = 'client' AND client_id::text = safe_app_setting('app.client_id'))
							OR (safe_app_setting('app.scope_type') = 'site' AND site_id::text = safe_app_setting('app.site_id'))
						))
						OR support_access_allowed(msp_id)
						OR EXISTS (
							SELECT 1 FROM agent_registrations ar
							WHERE ar.device_id = devices.id
							  AND ar.agent_id = safe_app_setting('app.user_id') AND ar.approved = true
						)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR (app_is_scope_manager() AND (
							(safe_app_setting('app.scope_type') = 'msp' AND msp_id::text = safe_app_setting('app.msp_id'))
							OR (safe_app_setting('app.scope_type') = 'client' AND client_id::text = safe_app_setting('app.client_id'))
							OR (safe_app_setting('app.scope_type') = 'site' AND site_id::text = safe_app_setting('app.site_id'))
						))
					);

				DROP POLICY IF EXISTS tenant_scope ON memberships;
				CREATE POLICY tenant_scope ON memberships
					USING (
						user_id = safe_app_setting('app.user_id')
						OR app_is_platform_admin()
						OR app_may_manage_membership(scope_type, scope_id)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR app_is_initial_bootstrap()
						OR app_may_manage_membership(scope_type, scope_id)
					);

				DROP POLICY IF EXISTS identity_scope ON users;
				CREATE POLICY identity_scope ON users
					USING (
						app_is_platform_admin()
						OR app_is_initial_bootstrap()
						OR id::text = safe_app_setting('app.user_id')
						OR normalized_email = safe_app_setting('app.login_email')
						OR app_is_scope_manager()
						OR EXISTS (
							SELECT 1 FROM account_invitations invitation
							WHERE invitation.token_hash = safe_app_setting('app.invitation_hash')
							  AND invitation.email_normalized = users.normalized_email
							  AND invitation.accepted_at IS NULL AND invitation.revoked_at IS NULL
							  AND invitation.expires_at > statement_timestamp()
						)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR app_is_initial_bootstrap()
						OR id::text = safe_app_setting('app.user_id')
						OR app_is_scope_manager()
						OR EXISTS (
							SELECT 1 FROM account_invitations invitation
							WHERE invitation.token_hash = safe_app_setting('app.invitation_hash')
							  AND invitation.email_normalized = users.normalized_email
							  AND invitation.accepted_at IS NULL AND invitation.revoked_at IS NULL
							  AND invitation.expires_at > statement_timestamp()
						)
					);

				DROP POLICY IF EXISTS tenant_scope ON control_plane_audit;
				DROP POLICY IF EXISTS control_plane_audit_select ON control_plane_audit;
				DROP POLICY IF EXISTS control_plane_audit_insert ON control_plane_audit;
				CREATE POLICY control_plane_audit_select ON control_plane_audit FOR SELECT
					USING (
						app_is_platform_admin()
						OR (app_scope_is_authorized() AND safe_app_setting('app.scope_type') = 'msp'
							AND msp_id::text = safe_app_setting('app.msp_id'))
						OR support_access_allowed(msp_id)
					);
				CREATE POLICY control_plane_audit_insert ON control_plane_audit FOR INSERT
					WITH CHECK (
						app_is_platform_admin()
						OR (app_scope_is_authorized() AND msp_id::text = safe_app_setting('app.msp_id'))
					);

				DROP TRIGGER IF EXISTS recovery_mutation_gate_trigger ON authorization_migration_issues;
				CREATE TRIGGER recovery_mutation_gate_trigger
					BEFORE INSERT OR UPDATE OR DELETE ON authorization_migration_issues
					FOR EACH STATEMENT EXECUTE FUNCTION enforce_recovery_mutation_gate();
			`,
			Down: `
				SELECT 1;
				-- Authorization hardening, issue evidence, and compatibility-field
				-- disposition are intentionally retained. Reverting these controls would
				-- recreate a privilege-escalation path and is not a safe automated down migration.
			`,
		},
		{
			ID:   70,
			Name: "correlate_alert_lifecycle",
			Up: `
				ALTER TABLE alerts ADD COLUMN IF NOT EXISTS correlation_key TEXT;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_alerts_tenant_correlation
					ON alerts(tenant_id, correlation_key)
					WHERE correlation_key IS NOT NULL AND status IN ('firing', 'acknowledged');
			`,
			Down: `
				DROP INDEX IF EXISTS idx_alerts_tenant_correlation;
				ALTER TABLE alerts DROP COLUMN IF EXISTS correlation_key;
			`,
		},
		{
			ID:   71,
			Name: "policy_lifecycle_and_revisions",
			Up: `
				ALTER TABLE policies ADD COLUMN IF NOT EXISTS device_id UUID REFERENCES devices(id) ON DELETE CASCADE;
				ALTER TABLE policies ADD COLUMN IF NOT EXISTS validated_at TIMESTAMPTZ;
				ALTER TABLE policies ADD COLUMN IF NOT EXISTS previewed_at TIMESTAMPTZ;
				ALTER TABLE policies ADD COLUMN IF NOT EXISTS published_version INT;
				ALTER TABLE policies ADD COLUMN IF NOT EXISTS published_config JSONB;
				ALTER TABLE policies ADD COLUMN IF NOT EXISTS maintenance_start TIME;
				ALTER TABLE policies ADD COLUMN IF NOT EXISTS maintenance_end TIME;
				ALTER TABLE policies ADD COLUMN IF NOT EXISTS maintenance_days JSONB DEFAULT '["monday","tuesday","wednesday","thursday","friday"]';
				ALTER TABLE policies ADD COLUMN IF NOT EXISTS maintenance_timezone TEXT DEFAULT 'UTC';
				CREATE INDEX IF NOT EXISTS idx_policies_device ON policies(device_id);

				CREATE TABLE IF NOT EXISTS policy_revisions (
					id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					policy_id UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
					msp_id UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					version INT NOT NULL,
					name TEXT NOT NULL,
					category TEXT NOT NULL,
					description TEXT NOT NULL,
					config JSONB NOT NULL,
					scope_level TEXT NOT NULL,
					client_id UUID,
					site_id UUID,
					device_id UUID,
					published_by TEXT NOT NULL,
					published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					maintenance_start TIME,
					maintenance_end TIME,
					maintenance_days JSONB DEFAULT '["monday","tuesday","wednesday","thursday","friday"]',
					maintenance_timezone TEXT DEFAULT 'UTC',
					UNIQUE(policy_id, version)
				);
				CREATE INDEX IF NOT EXISTS idx_policy_revisions_policy ON policy_revisions(policy_id, version DESC);

				UPDATE policies
				SET published_version = version, published_config = config,
				    validated_at = COALESCE(validated_at, updated_at),
				    previewed_at = COALESCE(previewed_at, updated_at)
				WHERE status = 'active' AND published_version IS NULL;
				INSERT INTO policy_revisions (
					policy_id,msp_id,version,name,category,description,config,scope_level,
					client_id,site_id,device_id,published_by,published_at,
					maintenance_start,maintenance_end,maintenance_days,maintenance_timezone
				)
				SELECT id,msp_id,version,name,category,description,config,scope_level,
				       client_id,site_id,device_id,'migration-71',updated_at,
				       NULL,NULL,'["monday","tuesday","wednesday","thursday","friday"]','UTC'
				FROM policies
				WHERE status = 'active' AND msp_id IS NOT NULL
				ON CONFLICT (policy_id, version) DO NOTHING;

				ALTER TABLE policies ENABLE ROW LEVEL SECURITY;
				DROP POLICY IF EXISTS policy_scope ON policies;
				CREATE POLICY policy_scope ON policies
					USING (
						app_is_platform_admin()
						OR (app_scope_is_authorized() AND safe_app_setting('app.scope_type') = 'msp'
							AND msp_id::text = safe_app_setting('app.msp_id'))
						OR support_access_allowed(msp_id)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR (app_is_scope_manager() AND safe_app_setting('app.scope_type') = 'msp'
							AND msp_id::text = safe_app_setting('app.msp_id'))
					);
				ALTER TABLE policies FORCE ROW LEVEL SECURITY;

				ALTER TABLE policy_revisions ENABLE ROW LEVEL SECURITY;
				CREATE POLICY policy_revision_scope ON policy_revisions
					USING (
						app_is_platform_admin()
						OR (app_scope_is_authorized() AND safe_app_setting('app.scope_type') = 'msp'
							AND msp_id::text = safe_app_setting('app.msp_id'))
						OR support_access_allowed(msp_id)
					)
					WITH CHECK (
						app_is_platform_admin()
						OR (app_is_scope_manager() AND safe_app_setting('app.scope_type') = 'msp'
							AND msp_id::text = safe_app_setting('app.msp_id'))
					);
				ALTER TABLE policy_revisions FORCE ROW LEVEL SECURITY;
			`,
			Down: `
				ALTER TABLE policies NO FORCE ROW LEVEL SECURITY;
				DROP POLICY IF EXISTS policy_scope ON policies;
				ALTER TABLE policies DISABLE ROW LEVEL SECURITY;
				UPDATE policies SET config=published_config, version=published_version, status='active'
				WHERE published_version IS NOT NULL AND status='draft';
				DROP TABLE IF EXISTS policy_revisions;
				DROP INDEX IF EXISTS idx_policies_device;
				ALTER TABLE policies DROP COLUMN IF EXISTS published_config;
				ALTER TABLE policies DROP COLUMN IF EXISTS published_version;
				ALTER TABLE policies DROP COLUMN IF EXISTS previewed_at;
				ALTER TABLE policies DROP COLUMN IF EXISTS validated_at;
				ALTER TABLE policies DROP COLUMN IF EXISTS device_id;
			`,
		},
		{
			ID:   72,
			Name: "tenant_retention_settings",
			Up: `
				CREATE TABLE IF NOT EXISTS tenant_retention_settings (
					tenant_id     UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
					metrics_days  INT NOT NULL DEFAULT 365 CHECK (metrics_days >= 1 AND metrics_days <= 10000),
					heartbeats_days INT NOT NULL DEFAULT 90 CHECK (heartbeats_days >= 1 AND heartbeats_days <= 10000),
					alerts_days   INT NOT NULL DEFAULT 365 CHECK (alerts_days >= 1 AND alerts_days <= 10000),
					snmp_polls_days INT NOT NULL DEFAULT 90 CHECK (snmp_polls_days >= 1 AND snmp_polls_days <= 10000),
					flow_records_days INT NOT NULL DEFAULT 30 CHECK (flow_records_days >= 1 AND flow_records_days <= 10000),
					topology_edges_days INT NOT NULL DEFAULT 90 CHECK (topology_edges_days >= 1 AND topology_edges_days <= 10000),
					created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				-- Insert default retention for existing tenants
				INSERT INTO tenant_retention_settings (tenant_id)
				SELECT id FROM tenants
				ON CONFLICT (tenant_id) DO NOTHING;

				CREATE INDEX IF NOT EXISTS idx_tenant_retention_updated
					ON tenant_retention_settings (updated_at);
			`,
			Down: `
				DROP TABLE IF EXISTS tenant_retention_settings;
			`,
		},
		{
			ID:   74,
			Name: "billing_accounts_table",
			Up: `
				CREATE TABLE IF NOT EXISTS billing_accounts (
					id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id                 UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					provider               TEXT NOT NULL CHECK (provider IN ('stripe', 'chargebee', 'custom')),
					provider_customer_id   TEXT NOT NULL,
					provider_subscription_id TEXT,
					payment_provider_id    TEXT,
					billing_cycle          TEXT NOT NULL DEFAULT 'monthly' CHECK (billing_cycle IN ('monthly', 'annual')),
					status                 TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'past_due', 'suspended', 'cancelled')),
					billing_email          TEXT,
					created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE (msp_id)
				);

				CREATE INDEX IF NOT EXISTS idx_billing_accounts_msp ON billing_accounts(msp_id);
				CREATE INDEX IF NOT EXISTS idx_billing_accounts_status ON billing_accounts(status);
			`,
			Down: `
				DROP TABLE IF EXISTS billing_accounts;
			`,
		},
		{
			ID:   75,
			Name: "subscriptions_table",
			Up: `
				CREATE TABLE IF NOT EXISTS subscriptions (
					id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id                 UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					plan_id                UUID NOT NULL REFERENCES plans(id),
					status                 TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'past_due', 'cancelled', 'expired')),
					billing_period         TEXT NOT NULL CHECK (billing_period IN ('monthly', 'annual')),
					started_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					current_period_end     TIMESTAMPTZ NOT NULL,
					cancelled_at           TIMESTAMPTZ,
					cancel_at_period_end   BOOLEAN NOT NULL DEFAULT false,
					created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_subscriptions_msp ON subscriptions(msp_id);
				CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(status);
				CREATE INDEX IF NOT EXISTS idx_subscriptions_period_end ON subscriptions(current_period_end);
			`,
			Down: `
				DROP TABLE IF EXISTS subscriptions;
			`,
		},
		{
			ID:   76,
			Name: "invoices_table",
			Up: `
				CREATE TABLE IF NOT EXISTS invoices (
					id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id             UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					subscription_id    UUID REFERENCES subscriptions(id) ON DELETE SET NULL,
					invoice_number     TEXT NOT NULL,
					period_start       TIMESTAMPTZ NOT NULL,
					period_end         TIMESTAMPTZ NOT NULL,
					subtotal           DECIMAL(10,2) NOT NULL,
					tax                DECIMAL(10,2) DEFAULT 0,
					total              DECIMAL(10,2) NOT NULL,
					currency           TEXT NOT NULL DEFAULT 'USD',
					status             TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','issued','paid','past_due','cancelled','void')),
					paid_at            TIMESTAMPTZ,
					due_at             TIMESTAMPTZ,
					invoice_pdf_url    TEXT,
					external_invoice_id TEXT,
					created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_invoices_msp ON invoices(msp_id);
				CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status);
				CREATE INDEX IF NOT EXISTS idx_invoices_number ON invoices(invoice_number);
				CREATE INDEX IF NOT EXISTS idx_invoices_period ON invoices(period_start, period_end);
			`,
			Down: `
				DROP TABLE IF EXISTS invoices;
			`,
		},
		{
			ID:   77,
			Name: "invoice_items_table",
			Up: `
				CREATE TABLE IF NOT EXISTS invoice_items (
					id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					invoice_id   UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
					description  TEXT NOT NULL,
					quantity     INT NOT NULL DEFAULT 1,
					unit_price   DECIMAL(10,2) NOT NULL,
					total        DECIMAL(10,2) NOT NULL,
					metadata     JSONB DEFAULT '{}',
					created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_invoice_items_invoice ON invoice_items(invoice_id);
			`,
			Down: `
				DROP TABLE IF EXISTS invoice_items;
			`,
		},
		{
			ID:   78,
			Name: "usage_records_table",
			Up: `
				CREATE TABLE IF NOT EXISTS usage_records (
					id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id       UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					meter_name   TEXT NOT NULL,
					quantity     INT NOT NULL DEFAULT 1,
					unit         TEXT NOT NULL,
					recorded_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					source       TEXT NOT NULL,
					external_id  TEXT,
					created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_usage_records_msp ON usage_records(msp_id);
				CREATE INDEX IF NOT EXISTS idx_usage_records_meter ON usage_records(meter_name);
				CREATE INDEX IF NOT EXISTS idx_usage_records_recorded_at ON usage_records(recorded_at);
			`,
			Down: `
				DROP TABLE IF EXISTS usage_records;
			`,
		},
		{
			ID:   79,
			Name: "payment_methods_table",
			Up: `
				CREATE TABLE IF NOT EXISTS payment_methods (
					id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id                 UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					provider_payment_method_id TEXT NOT NULL,
					type                   TEXT NOT NULL CHECK (type IN ('card','bank','paypal')),
					card_brand             TEXT,
					last_four              TEXT,
					exp_month              INT,
					exp_year               INT,
					is_default             BOOLEAN NOT NULL DEFAULT false,
					provider_data          JSONB DEFAULT '{}',
					created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_payment_methods_msp ON payment_methods(msp_id);
				CREATE INDEX IF NOT EXISTS idx_payment_methods_is_default ON payment_methods(is_default);
			`,
			Down: `
				DROP TABLE IF EXISTS payment_methods;
			`,
		},
		{
			ID:   80,
			Name: "device_relationships_table",
			Up: `
				CREATE TABLE IF NOT EXISTS device_relationships (
					id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					msp_id                 UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					client_id              UUID REFERENCES client_organizations(id) ON DELETE CASCADE,
					site_id                UUID REFERENCES sites(id) ON DELETE CASCADE,
					source_device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					target_device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					relationship_type      TEXT NOT NULL CHECK (relationship_type IN (
						'depends-on', 'dependency-of',
						'child-of', 'parent-of',
						'connected-to', 'connects-to',
						'member-of', 'contains',
						'backup-of', 'backup-for',
						'failover-to', 'failover-from'
					)),
					metadata               JSONB DEFAULT '{}',
					is_active              BOOLEAN NOT NULL DEFAULT true,
					verified_at            TIMESTAMPTZ,
					created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					CONSTRAINT no_self_relationship CHECK (source_device_id <> target_device_id),
					UNIQUE (msp_id, source_device_id, target_device_id, relationship_type)
				);

				CREATE INDEX idx_device_relationships_source ON device_relationships(source_device_id);
				CREATE INDEX idx_device_relationships_target ON device_relationships(target_device_id);
				CREATE INDEX idx_device_relationships_msp ON device_relationships(msp_id);
			`,
			Down: `
				DROP TABLE IF EXISTS device_relationships;
			`,
		},
		{
			ID:   81,
			Name: "network_addresses_table",
			Up: `
				CREATE TABLE IF NOT EXISTS network_addresses (
					id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					device_id        UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					msp_id           UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					client_id        UUID REFERENCES client_organizations(id) ON DELETE CASCADE,
					site_id          UUID REFERENCES sites(id) ON DELETE CASCADE,
					ip_address       INET NOT NULL,
					ip_family        INT NOT NULL DEFAULT 4 CHECK (ip_family IN (4, 6)),
					network_type     TEXT NOT NULL DEFAULT 'internal' CHECK (network_type IN ('internal', 'external', 'management', 'storage')),
					interface_name   TEXT,
					vlan_id          INT,
					subnet_cidr      TEXT,
					is_primary       BOOLEAN NOT NULL DEFAULT false,
					created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE (device_id, ip_address)
				);

				CREATE INDEX idx_network_addresses_device ON network_addresses(device_id);
				CREATE INDEX idx_network_addresses_msp ON network_addresses(msp_id);
			`,
			Down: `
				DROP TABLE IF EXISTS network_addresses;
			`,
		},
		{
			ID:   82,
			Name: "device_packages_table",
			Up: `
				CREATE TABLE IF NOT EXISTS device_packages (
					id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					device_id      UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					msp_id         UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					name           TEXT NOT NULL,
					version        TEXT NOT NULL,
					release        TEXT,
					arch           TEXT,
					source         TEXT,
					install_date   TIMESTAMPTZ,
					package_type   TEXT NOT NULL DEFAULT 'deb' CHECK (package_type IN ('deb', 'rpm', 'msi', 'exe', 'npm', 'pip', 'go', 'other')),
					status         TEXT NOT NULL DEFAULT 'installed' CHECK (status IN ('installed', 'pending', 'orphaned')),
					created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX idx_device_packages_device ON device_packages(device_id);
				CREATE INDEX idx_device_packages_name ON device_packages(name);
			`,
			Down: `
				DROP TABLE IF EXISTS device_packages;
			`,
		},
		{
			ID:   83,
			Name: "device_services_table",
			Up: `
				CREATE TABLE IF NOT EXISTS device_services (
					id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					device_id      UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					msp_id         UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					name           TEXT NOT NULL,
					port           INT,
					protocol       TEXT NOT NULL DEFAULT 'tcp' CHECK (protocol IN ('tcp', 'udp', 'sctp')),
					state          TEXT NOT NULL DEFAULT 'listening' CHECK (state IN ('listening', 'established', 'closed', 'waiting')),
					process_name   TEXT,
					binary_path    TEXT,
					created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX idx_device_services_device ON device_services(device_id);
				CREATE UNIQUE INDEX idx_device_services_port ON device_services(device_id, port, protocol) WHERE port IS NOT NULL;
			`,
			Down: `
				DROP TABLE IF EXISTS device_services;
			`,
		},
		{
			ID:   84,
			Name: "device_mounts_table",
			Up: `
				CREATE TABLE IF NOT EXISTS device_mounts (
					id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					device_id      UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
					msp_id         UUID NOT NULL REFERENCES msp_tenants(id) ON DELETE CASCADE,
					mount_point    TEXT NOT NULL,
					device_path    TEXT,
					filesystem     TEXT NOT NULL,
					size_bytes     BIGINT,
					used_bytes     BIGINT,
					available_bytes BIGINT,
					mount_options  TEXT[],
					created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE (device_id, mount_point)
				);

				CREATE INDEX idx_device_mounts_device ON device_mounts(device_id);
			`,
			Down: `
				DROP TABLE IF EXISTS device_mounts;
			`,
		},
		{
			ID:   85,
			Name: "enhance_topology_edges",
			Up: `
				CREATE TABLE IF NOT EXISTS topology_edges (
					id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
					src_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
					dst_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
					connection_type TEXT NOT NULL DEFAULT 'ethernet',
					bandwidth_mbps  INT,
					recorded_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_topology_edges_tenant ON topology_edges(tenant_id);
				CREATE INDEX IF NOT EXISTS idx_topology_edges_recorded ON topology_edges(recorded_at);

				ALTER TABLE topology_edges
					ADD COLUMN IF NOT EXISTS src_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
					ADD COLUMN IF NOT EXISTS dst_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
					ADD COLUMN IF NOT EXISTS connection_type TEXT NOT NULL DEFAULT 'ethernet',
					ADD COLUMN IF NOT EXISTS bandwidth_mbps INT;
			`,
			Down: `
				DROP TABLE IF EXISTS topology_edges;
			`,
		},
		{
			ID:   86,
			Name: "client_auth_providers",
			Up: `
-- Create client_auth_providers table for SSO provider configuration
CREATE TABLE IF NOT EXISTS client_auth_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id UUID NOT NULL REFERENCES client_organizations(id) ON DELETE CASCADE,
    provider_name TEXT NOT NULL CHECK (provider_name IN ('google', 'microsoft', 'okta', 'github', 'gitlab', 'saml')),
    provider_id TEXT NOT NULL,
    client_secret_hash TEXT NOT NULL,
    discovery_url TEXT,
    jwks_uri TEXT,
    auth_endpoint TEXT,
    token_endpoint TEXT,
    user_info_endpoint TEXT,
    redirect_uri TEXT NOT NULL,
    is_active BOOLEAN DEFAULT true,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(client_id, provider_name),
    UNIQUE(client_id, provider_id)
);

-- Create client_sessions table for client session tracking
CREATE TABLE IF NOT EXISTS client_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id UUID NOT NULL REFERENCES client_organizations(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    session_token TEXT NOT NULL UNIQUE,
    session_data JSONB DEFAULT '{}',
    ip_address INET,
    user_agent TEXT,
    last_activity_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create client_portal_settings table for per-client portal settings
CREATE TABLE IF NOT EXISTS client_portal_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id UUID NOT NULL REFERENCES client_organizations(id) ON DELETE CASCADE,
    UNIQUE(client_id),
    allow_self_registration BOOLEAN DEFAULT false,
    self_registration_domains TEXT[] DEFAULT '{}',
    enable_sso BOOLEAN DEFAULT false,
    enable_password_login BOOLEAN DEFAULT true,
    branding_override JSONB DEFAULT '{}',
    welcome_message TEXT,
    support_email TEXT,
    support_phone TEXT,
    support_url TEXT,
    logo_url TEXT,
    favicon_url TEXT,
    primary_color TEXT,
    accent_color TEXT,
    sidebar_bg TEXT,
    header_bg TEXT,
    login_bg TEXT,
    portal_title TEXT,
    welcome_text TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_client_auth_providers_client_id ON client_auth_providers(client_id);
CREATE INDEX IF NOT EXISTS idx_client_auth_providers_provider_name ON client_auth_providers(provider_name);
CREATE INDEX IF NOT EXISTS idx_client_sessions_client_id ON client_sessions(client_id);
CREATE INDEX IF NOT EXISTS idx_client_sessions_session_token ON client_sessions(session_token);
CREATE INDEX IF NOT EXISTS idx_client_sessions_expires_at ON client_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_client_sessions_last_activity_at ON client_sessions(last_activity_at);

-- Enable Row Level Security (RLS) for client data isolation
ALTER TABLE client_auth_providers ENABLE ROW LEVEL SECURITY;
ALTER TABLE client_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE client_portal_settings ENABLE ROW LEVEL SECURITY;

-- RLS policies for client_auth_providers
DROP POLICY IF EXISTS "Users can read auth providers for their client" ON client_auth_providers;

CREATE POLICY "Users can read auth providers for their client" ON client_auth_providers FOR SELECT
    USING (
        EXISTS (
            SELECT 1 FROM client_organizations c
            WHERE c.id = client_auth_providers.client_id
            AND (
                c.id = NULLIF(current_setting('app.client_id', true), '')::UUID
                OR c.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID
            )
        )
    );

DROP POLICY IF EXISTS "Platform admins can manage all auth providers" ON client_auth_providers;

CREATE POLICY "Platform admins can manage all auth providers" ON client_auth_providers FOR ALL
    USING (
        current_setting('app.role', true) IN ('platform_owner', 'platform_admin')
    );

DROP POLICY IF EXISTS "MSP admins can manage auth providers for their MSP clients" ON client_auth_providers;

CREATE POLICY "MSP admins can manage auth providers for their MSP clients" ON client_auth_providers FOR ALL
    USING (
        EXISTS (
            SELECT 1 FROM client_organizations c
            WHERE c.id = client_auth_providers.client_id
            AND c.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID
        )
    );

-- RLS policies for client_sessions
DROP POLICY IF EXISTS "Users can read sessions for their client" ON client_sessions;

CREATE POLICY "Users can read sessions for their client" ON client_sessions FOR SELECT
    USING (
        EXISTS (
            SELECT 1 FROM client_organizations c
            WHERE c.id = client_sessions.client_id
            AND (
                c.id = NULLIF(current_setting('app.client_id', true), '')::UUID
                OR c.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID
            )
        )
    );

DROP POLICY IF EXISTS "Users can manage their own sessions" ON client_sessions;

CREATE POLICY "Users can manage their own sessions" ON client_sessions FOR ALL
    USING (
        EXISTS (
            SELECT 1 FROM client_organizations c
            WHERE c.id = client_sessions.client_id
            AND (
                c.id = NULLIF(current_setting('app.client_id', true), '')::UUID
                OR c.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID
            )
        )
    );

DROP POLICY IF EXISTS "Platform admins can manage all sessions" ON client_sessions;

CREATE POLICY "Platform admins can manage all sessions" ON client_sessions FOR ALL
    USING (
        current_setting('app.role', true) IN ('platform_owner', 'platform_admin')
    );

-- RLS policies for client_portal_settings
DROP POLICY IF EXISTS "Users can read portal settings for their client" ON client_portal_settings;

CREATE POLICY "Users can read portal settings for their client" ON client_portal_settings FOR SELECT
    USING (
        EXISTS (
            SELECT 1 FROM client_organizations c
            WHERE c.id = client_portal_settings.client_id
            AND (
                c.id = NULLIF(current_setting('app.client_id', true), '')::UUID
                OR c.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID
            )
        )
    );

DROP POLICY IF EXISTS "Platform admins can manage all portal settings" ON client_portal_settings;

CREATE POLICY "Platform admins can manage all portal settings" ON client_portal_settings FOR ALL
    USING (
        current_setting('app.role', true) IN ('platform_owner', 'platform_admin')
    );

DROP POLICY IF EXISTS "MSP admins can manage portal settings for their MSP clients" ON client_portal_settings;

CREATE POLICY "MSP admins can manage portal settings for their MSP clients" ON client_portal_settings FOR ALL
    USING (
        EXISTS (
            SELECT 1 FROM client_organizations c
            WHERE c.id = client_portal_settings.client_id
            AND c.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID
        )
    );

-- Update updated_at trigger for all tables
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
   NEW.updated_at = NOW();
   RETURN NEW;
END;
$$ language 'plpgsql';

DROP TRIGGER IF EXISTS "update_client_auth_providers_updated_at" ON client_auth_providers;

CREATE TRIGGER "update_client_auth_providers_updated_at"
    BEFORE UPDATE ON client_auth_providers
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS "update_client_sessions_updated_at" ON client_sessions;

CREATE TRIGGER "update_client_sessions_updated_at"
    BEFORE UPDATE ON client_sessions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS "update_client_portal_settings_updated_at" ON client_portal_settings;

CREATE TRIGGER "update_client_portal_settings_updated_at"
    BEFORE UPDATE ON client_portal_settings
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
`,
			Down: `
-- Drop RLS policies for client_portal_settings
DROP POLICY IF NOT EXISTS "MSP admins can manage portal settings for their MSP clients" ON client_portal_settings;
DROP POLICY IF NOT EXISTS "Platform admins can manage all portal settings" ON client_portal_settings;
DROP POLICY IF NOT EXISTS "Users can read portal settings for their client" ON client_portal_settings;

-- Drop RLS policies for client_sessions
DROP POLICY IF NOT EXISTS "Users can manage their own sessions" ON client_sessions;
DROP POLICY IF NOT EXISTS "Platform admins can manage all sessions" ON client_sessions;
DROP POLICY IF NOT EXISTS "Users can read sessions for their client" ON client_sessions;

-- Drop RLS policies for client_auth_providers
DROP POLICY IF NOT EXISTS "MSP admins can manage auth providers for their MSP clients" ON client_auth_providers;
DROP POLICY IF NOT EXISTS "Platform admins can manage all auth providers" ON client_auth_providers;
DROP POLICY IF NOT EXISTS "Users can read auth providers for their client" ON client_auth_providers;

-- Drop triggers
DROP TRIGGER IF EXISTS update_client_portal_settings_updated_at ON client_portal_settings;
DROP TRIGGER IF EXISTS update_client_sessions_updated_at ON client_sessions;
DROP TRIGGER IF EXISTS update_client_auth_providers_updated_at ON client_auth_providers;

-- Disable RLS for all tables
ALTER TABLE client_portal_settings DISABLE ROW LEVEL SECURITY;
ALTER TABLE client_sessions DISABLE ROW LEVEL SECURITY;
ALTER TABLE client_auth_providers DISABLE ROW LEVEL SECURITY;

-- Drop indexes
DROP INDEX IF EXISTS idx_client_portal_settings_client_id;
DROP INDEX IF EXISTS idx_client_sessions_last_activity_at;
DROP INDEX IF EXISTS idx_client_sessions_expires_at;
DROP INDEX IF EXISTS idx_client_sessions_session_token;
DROP INDEX IF EXISTS idx_client_sessions_client_id;
DROP INDEX IF EXISTS idx_client_auth_providers_provider_name;
DROP INDEX IF EXISTS idx_client_auth_providers_client_id;

-- Drop client_portal_settings table
DROP TABLE IF EXISTS client_portal_settings;

-- Drop client_sessions table
DROP TABLE IF EXISTS client_sessions;

-- Drop client_auth_providers table
DROP TABLE IF EXISTS client_auth_providers;

-- Drop helper function
DROP FUNCTION IF EXISTS update_updated_at_column();
`,
		},
		{
			ID:   87,
			Name: "client_portal_enhancements",
			Up: `
-- Add provider configuration columns for enhanced SSO support
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS issuer TEXT;
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS oauth_client_id TEXT;
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS scope TEXT DEFAULT 'openid profile email';
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS mapping JSONB DEFAULT '{}';
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS auto_provision BOOLEAN DEFAULT false;
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS auto_invite BOOLEAN DEFAULT false;

-- Add session validation columns
ALTER TABLE client_sessions ADD COLUMN IF NOT EXISTS mfa_enabled BOOLEAN DEFAULT false;
ALTER TABLE client_sessions ADD COLUMN IF NOT EXISTS mfa_verified_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE client_sessions ADD COLUMN IF NOT EXISTS authenticated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW();

-- Add portal settings for security and compliance
ALTER TABLE client_portal_settings ADD COLUMN IF NOT EXISTS session_timeout_minutes INTEGER DEFAULT 60;
ALTER TABLE client_portal_settings ADD COLUMN IF NOT EXISTS max_concurrent_sessions INTEGER DEFAULT 5;
ALTER TABLE client_portal_settings ADD COLUMN IF NOT EXISTS require_2fa BOOLEAN DEFAULT false;
ALTER TABLE client_portal_settings ADD COLUMN IF NOT EXISTS login_attempts_before_lockout INTEGER DEFAULT 5;
ALTER TABLE client_portal_settings ADD COLUMN IF NOT EXISTS lockout_duration_minutes INTEGER DEFAULT 15;
ALTER TABLE client_portal_settings ADD COLUMN IF NOT EXISTS password_policy JSONB DEFAULT '{"min_length": 12, "require_uppercase": true, "require_lowercase": true, "require_numbers": true, "require_special": true}';
ALTER TABLE client_portal_settings ADD COLUMN IF NOT EXISTS audit_log_retention_days INTEGER DEFAULT 365;

-- Add logging columns to sessions for audit trail
ALTER TABLE client_sessions ADD COLUMN IF NOT EXISTS session_type TEXT DEFAULT 'web';
ALTER TABLE client_sessions ADD COLUMN IF NOT EXISTS auth_provider TEXT;
ALTER TABLE client_sessions ADD COLUMN IF NOT EXISTS last_ip INET;
ALTER TABLE client_sessions ADD COLUMN IF NOT EXISTS last_user_agent TEXT;
ALTER TABLE client_sessions ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE client_sessions ADD COLUMN IF NOT EXISTS revocation_reason TEXT;

-- Add provider configuration for custom SAML
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS saml_metadata_url TEXT;
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS saml_entity_id TEXT;
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS saml_name_id_format TEXT DEFAULT 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress';
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS saml_signature_cert TEXT;
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS saml_encryption_cert TEXT;
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS saml_requested_authn_context TEXT[];
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS saml_attribute_mapping JSONB DEFAULT '{}';

-- Add audit columns
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id);
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS updated_by UUID REFERENCES users(id);

-- Add index for MFA verification tracking
CREATE INDEX IF NOT EXISTS idx_client_sessions_mfa_verified ON client_sessions(mfa_verified_at) WHERE mfa_verified_at IS NOT NULL;

-- Add index for active sessions
CREATE INDEX IF NOT EXISTS idx_client_sessions_active ON client_sessions(client_id, expires_at) WHERE revoked_at IS NULL;
`,
			Down: `
-- Drop indexes
DROP INDEX IF EXISTS idx_client_sessions_mfa_verified;
DROP INDEX IF EXISTS idx_client_sessions_active;

-- Remove enhanced columns from client_auth_providers
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS issuer;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS oauth_client_id;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS scope;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS mapping;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS auto_provision;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS auto_invite;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS saml_metadata_url;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS saml_entity_id;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS saml_name_id_format;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS saml_signature_cert;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS saml_encryption_cert;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS saml_requested_authn_context;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS saml_attribute_mapping;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS created_by;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS updated_by;

-- Remove enhanced columns from client_sessions
ALTER TABLE client_sessions DROP COLUMN IF EXISTS mfa_enabled;
ALTER TABLE client_sessions DROP COLUMN IF EXISTS mfa_verified_at;
ALTER TABLE client_sessions DROP COLUMN IF EXISTS authenticated_at;
ALTER TABLE client_sessions DROP COLUMN IF EXISTS session_type;
ALTER TABLE client_sessions DROP COLUMN IF EXISTS auth_provider;
ALTER TABLE client_sessions DROP COLUMN IF EXISTS last_ip;
ALTER TABLE client_sessions DROP COLUMN IF EXISTS last_user_agent;
ALTER TABLE client_sessions DROP COLUMN IF EXISTS revoked_at;
ALTER TABLE client_sessions DROP COLUMN IF EXISTS revocation_reason;

-- Remove enhanced columns from client_portal_settings
ALTER TABLE client_portal_settings DROP COLUMN IF EXISTS session_timeout_minutes;
ALTER TABLE client_portal_settings DROP COLUMN IF EXISTS max_concurrent_sessions;
ALTER TABLE client_portal_settings DROP COLUMN IF EXISTS require_2fa;
ALTER TABLE client_portal_settings DROP COLUMN IF EXISTS login_attempts_before_lockout;
ALTER TABLE client_portal_settings DROP COLUMN IF EXISTS lockout_duration_minutes;
ALTER TABLE client_portal_settings DROP COLUMN IF EXISTS password_policy;
ALTER TABLE client_portal_settings DROP COLUMN IF EXISTS audit_log_retention_days;
`,
		},
		{
			ID:   88,
			Name: "client_session_activity",
			Up: `
-- Add session activity tracking table for audit and analytics
CREATE TABLE IF NOT EXISTS client_session_activity (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES client_sessions(id) ON DELETE CASCADE,
    activity_type TEXT NOT NULL CHECK (activity_type IN ('login', 'logout', 'page_view', 'api_call', 'action')),
    resource_type TEXT,
    resource_id TEXT,
    action TEXT,
    metadata JSONB DEFAULT '{}',
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Add client session activity index
CREATE INDEX IF NOT EXISTS idx_client_session_activity_session_id ON client_session_activity(session_id);
CREATE INDEX IF NOT EXISTS idx_client_session_activity_created_at ON client_session_activity(created_at);
CREATE INDEX IF NOT EXISTS idx_client_session_activity_type ON client_session_activity(activity_type);

-- Enable RLS for session activity
ALTER TABLE client_session_activity ENABLE ROW LEVEL SECURITY;

-- RLS policies for client_session_activity
DROP POLICY IF EXISTS "Users can read activity for their client sessions" ON client_session_activity;

CREATE POLICY "Users can read activity for their client sessions" ON client_session_activity FOR SELECT
    USING (
        EXISTS (
            SELECT 1 FROM client_sessions cs
            JOIN client_organizations c ON c.id = cs.client_id
            WHERE cs.id = client_session_activity.session_id
            AND (
                c.id = NULLIF(current_setting('app.client_id', true), '')::UUID
                OR c.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID
            )
        )
    );

DROP POLICY IF EXISTS "Platform admins can read all activity" ON client_session_activity;

CREATE POLICY "Platform admins can read all activity" ON client_session_activity FOR SELECT
    USING (
        current_setting('app.role', true) IN ('platform_owner', 'platform_admin')
    );

-- Create index on client_id for easier filtering
CREATE INDEX IF NOT EXISTS idx_client_session_activity_client ON client_session_activity(
    session_id
) INCLUDE (activity_type, created_at);

-- Add trigger to update updated_at
DROP TRIGGER IF EXISTS "update_client_session_activity_updated_at" ON client_session_activity;

CREATE TRIGGER "update_client_session_activity_updated_at"
    BEFORE UPDATE ON client_session_activity
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
`,
			Down: `
-- Drop session activity trigger
DROP TRIGGER IF EXISTS update_client_session_activity_updated_at ON client_session_activity;

-- Drop RLS policies for client_session_activity
DROP POLICY IF EXISTS "Platform admins can read all activity" ON client_session_activity;
DROP POLICY IF EXISTS "Users can read activity for their client sessions" ON client_session_activity;

-- Disable RLS
ALTER TABLE client_session_activity DISABLE ROW LEVEL SECURITY;

-- Drop indexes
DROP INDEX IF EXISTS idx_client_session_activity_client;
DROP INDEX IF EXISTS idx_client_session_activity_type;
DROP INDEX IF EXISTS idx_client_session_activity_created_at;
DROP INDEX IF EXISTS idx_client_session_activity_session_id;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop table
DROP TABLE IF EXISTS client_session_activity;
`,
		},
		{
			ID:   89,
			Name: "force_rls_billing_retention_cmdb",
			Up: `
-- Enable and force RLS on billing tables (migration 74-79)
ALTER TABLE billing_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing_accounts FORCE ROW LEVEL SECURITY;
ALTER TABLE subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscriptions FORCE ROW LEVEL SECURITY;
ALTER TABLE invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoices FORCE ROW LEVEL SECURITY;
ALTER TABLE invoice_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoice_items FORCE ROW LEVEL SECURITY;
ALTER TABLE usage_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE usage_records FORCE ROW LEVEL SECURITY;
ALTER TABLE payment_methods ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_methods FORCE ROW LEVEL SECURITY;

-- Enable and force RLS on retention table (migration 73)
ALTER TABLE tenant_retention_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_retention_settings FORCE ROW LEVEL SECURITY;

-- Enable and force RLS on CMDB tables (migration 80-84)
ALTER TABLE device_relationships ENABLE ROW LEVEL SECURITY;
ALTER TABLE device_relationships FORCE ROW LEVEL SECURITY;
ALTER TABLE network_addresses ENABLE ROW LEVEL SECURITY;
ALTER TABLE network_addresses FORCE ROW LEVEL SECURITY;
ALTER TABLE device_packages ENABLE ROW LEVEL SECURITY;
ALTER TABLE device_packages FORCE ROW LEVEL SECURITY;
ALTER TABLE device_services ENABLE ROW LEVEL SECURITY;
ALTER TABLE device_services FORCE ROW LEVEL SECURITY;
ALTER TABLE device_mounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE device_mounts FORCE ROW LEVEL SECURITY;
ALTER TABLE topology_edges ENABLE ROW LEVEL SECURITY;
ALTER TABLE topology_edges FORCE ROW LEVEL SECURITY;

-- RLS policies for billing tables
DROP POLICY IF EXISTS "msp_isolation_billing_accounts" ON billing_accounts;
CREATE POLICY "msp_isolation_billing_accounts" ON billing_accounts FOR ALL
    USING (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    WITH CHECK (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID);

DROP POLICY IF EXISTS "platform_admin_billing_accounts" ON billing_accounts;
CREATE POLICY "platform_admin_billing_accounts" ON billing_accounts FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin', 'platform_billing'));

DROP POLICY IF EXISTS "msp_isolation_subscriptions" ON subscriptions;
CREATE POLICY "msp_isolation_subscriptions" ON subscriptions FOR ALL
    USING (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    WITH CHECK (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID);

DROP POLICY IF EXISTS "platform_admin_subscriptions" ON subscriptions;
CREATE POLICY "platform_admin_subscriptions" ON subscriptions FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin', 'platform_billing'));

DROP POLICY IF EXISTS "msp_isolation_invoices" ON invoices;
CREATE POLICY "msp_isolation_invoices" ON invoices FOR ALL
    USING (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    WITH CHECK (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID);

DROP POLICY IF EXISTS "platform_admin_invoices" ON invoices;
CREATE POLICY "platform_admin_invoices" ON invoices FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin', 'platform_billing'));

DROP POLICY IF EXISTS "msp_isolation_invoice_items" ON invoice_items;
CREATE POLICY "msp_isolation_invoice_items" ON invoice_items FOR ALL
    USING (
        EXISTS (SELECT 1 FROM invoices i WHERE i.id = invoice_items.invoice_id AND i.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    )
    WITH CHECK (
        EXISTS (SELECT 1 FROM invoices i WHERE i.id = invoice_items.invoice_id AND i.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    );

DROP POLICY IF EXISTS "platform_admin_invoice_items" ON invoice_items;
CREATE POLICY "platform_admin_invoice_items" ON invoice_items FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin', 'platform_billing'));

DROP POLICY IF EXISTS "msp_isolation_usage_records" ON usage_records;
CREATE POLICY "msp_isolation_usage_records" ON usage_records FOR ALL
    USING (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    WITH CHECK (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID);

DROP POLICY IF EXISTS "platform_admin_usage_records" ON usage_records;
CREATE POLICY "platform_admin_usage_records" ON usage_records FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin', 'platform_billing'));

DROP POLICY IF EXISTS "msp_isolation_payment_methods" ON payment_methods;
CREATE POLICY "msp_isolation_payment_methods" ON payment_methods FOR ALL
    USING (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    WITH CHECK (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID);

DROP POLICY IF EXISTS "platform_admin_payment_methods" ON payment_methods;
CREATE POLICY "platform_admin_payment_methods" ON payment_methods FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin', 'platform_billing'));

-- RLS policies for retention table
DROP POLICY IF EXISTS "tenant_isolation_retention_settings" ON tenant_retention_settings;
CREATE POLICY "tenant_isolation_retention_settings" ON tenant_retention_settings FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::UUID)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::UUID);

DROP POLICY IF EXISTS "platform_admin_retention_settings" ON tenant_retention_settings;
CREATE POLICY "platform_admin_retention_settings" ON tenant_retention_settings FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));

-- RLS policies for CMDB tables
DROP POLICY IF EXISTS "msp_isolation_device_relationships" ON device_relationships;
CREATE POLICY "msp_isolation_device_relationships" ON device_relationships FOR ALL
    USING (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    WITH CHECK (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID);

DROP POLICY IF EXISTS "platform_admin_device_relationships" ON device_relationships;
CREATE POLICY "platform_admin_device_relationships" ON device_relationships FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));

DROP POLICY IF EXISTS "msp_isolation_network_addresses" ON network_addresses;
CREATE POLICY "msp_isolation_network_addresses" ON network_addresses FOR ALL
    USING (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    WITH CHECK (msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID);

DROP POLICY IF EXISTS "platform_admin_network_addresses" ON network_addresses;
CREATE POLICY "platform_admin_network_addresses" ON network_addresses FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));

DROP POLICY IF EXISTS "msp_isolation_device_packages" ON device_packages;
CREATE POLICY "msp_isolation_device_packages" ON device_packages FOR ALL
    USING (
        EXISTS (SELECT 1 FROM devices d WHERE d.id = device_packages.device_id AND d.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    )
    WITH CHECK (
        EXISTS (SELECT 1 FROM devices d WHERE d.id = device_packages.device_id AND d.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    );

DROP POLICY IF EXISTS "platform_admin_device_packages" ON device_packages;
CREATE POLICY "platform_admin_device_packages" ON device_packages FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));

DROP POLICY IF EXISTS "msp_isolation_device_services" ON device_services;
CREATE POLICY "msp_isolation_device_services" ON device_services FOR ALL
    USING (
        EXISTS (SELECT 1 FROM devices d WHERE d.id = device_services.device_id AND d.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    )
    WITH CHECK (
        EXISTS (SELECT 1 FROM devices d WHERE d.id = device_services.device_id AND d.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    );

DROP POLICY IF EXISTS "platform_admin_device_services" ON device_services;
CREATE POLICY "platform_admin_device_services" ON device_services FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));

DROP POLICY IF EXISTS "msp_isolation_device_mounts" ON device_mounts;
CREATE POLICY "msp_isolation_device_mounts" ON device_mounts FOR ALL
    USING (
        EXISTS (SELECT 1 FROM devices d WHERE d.id = device_mounts.device_id AND d.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    )
    WITH CHECK (
        EXISTS (SELECT 1 FROM devices d WHERE d.id = device_mounts.device_id AND d.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    );

DROP POLICY IF EXISTS "platform_admin_device_mounts" ON device_mounts;
CREATE POLICY "platform_admin_device_mounts" ON device_mounts FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));

DROP POLICY IF EXISTS "msp_isolation_topology_edges" ON topology_edges;
CREATE POLICY "msp_isolation_topology_edges" ON topology_edges FOR ALL
    USING (
        EXISTS (SELECT 1 FROM devices d WHERE d.id = topology_edges.src_device_id AND d.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    )
    WITH CHECK (
        EXISTS (SELECT 1 FROM devices d WHERE d.id = topology_edges.src_device_id AND d.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    );

DROP POLICY IF EXISTS "platform_admin_topology_edges" ON topology_edges;
CREATE POLICY "platform_admin_topology_edges" ON topology_edges FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));
`,
			Down: `
-- Disable FORCE RLS on billing tables
ALTER TABLE billing_accounts DISABLE ROW LEVEL SECURITY;
ALTER TABLE subscriptions DISABLE ROW LEVEL SECURITY;
ALTER TABLE invoices DISABLE ROW LEVEL SECURITY;
ALTER TABLE invoice_items DISABLE ROW LEVEL SECURITY;
ALTER TABLE usage_records DISABLE ROW LEVEL SECURITY;
ALTER TABLE payment_methods DISABLE ROW LEVEL SECURITY;

-- Disable FORCE RLS on retention table
ALTER TABLE tenant_retention_settings DISABLE ROW LEVEL SECURITY;

-- Disable FORCE RLS on CMDB tables
ALTER TABLE device_relationships DISABLE ROW LEVEL SECURITY;
ALTER TABLE network_addresses DISABLE ROW LEVEL SECURITY;
ALTER TABLE device_packages DISABLE ROW LEVEL SECURITY;
ALTER TABLE device_services DISABLE ROW LEVEL SECURITY;
ALTER TABLE device_mounts DISABLE ROW LEVEL SECURITY;
ALTER TABLE topology_edges DISABLE ROW LEVEL SECURITY;

-- Drop RLS policies for billing tables
DROP POLICY IF EXISTS "msp_isolation_billing_accounts" ON billing_accounts;
DROP POLICY IF EXISTS "platform_admin_billing_accounts" ON billing_accounts;
DROP POLICY IF EXISTS "msp_isolation_subscriptions" ON subscriptions;
DROP POLICY IF EXISTS "platform_admin_subscriptions" ON subscriptions;
DROP POLICY IF EXISTS "msp_isolation_invoices" ON invoices;
DROP POLICY IF EXISTS "platform_admin_invoices" ON invoices;
DROP POLICY IF EXISTS "msp_isolation_invoice_items" ON invoice_items;
DROP POLICY IF EXISTS "platform_admin_invoice_items" ON invoice_items;
DROP POLICY IF EXISTS "msp_isolation_usage_records" ON usage_records;
DROP POLICY IF EXISTS "platform_admin_usage_records" ON usage_records;
DROP POLICY IF EXISTS "msp_isolation_payment_methods" ON payment_methods;
DROP POLICY IF EXISTS "platform_admin_payment_methods" ON payment_methods;

-- Drop RLS policies for retention table
DROP POLICY IF EXISTS "tenant_isolation_retention_settings" ON tenant_retention_settings;
DROP POLICY IF EXISTS "platform_admin_retention_settings" ON tenant_retention_settings;

-- Drop RLS policies for CMDB tables
DROP POLICY IF EXISTS "msp_isolation_device_relationships" ON device_relationships;
DROP POLICY IF EXISTS "platform_admin_device_relationships" ON device_relationships;
DROP POLICY IF EXISTS "msp_isolation_network_addresses" ON network_addresses;
DROP POLICY IF EXISTS "platform_admin_network_addresses" ON network_addresses;
DROP POLICY IF EXISTS "msp_isolation_device_packages" ON device_packages;
DROP POLICY IF EXISTS "platform_admin_device_packages" ON device_packages;
DROP POLICY IF EXISTS "msp_isolation_device_services" ON device_services;
DROP POLICY IF EXISTS "platform_admin_device_services" ON device_services;
DROP POLICY IF EXISTS "msp_isolation_device_mounts" ON device_mounts;
DROP POLICY IF EXISTS "platform_admin_device_mounts" ON device_mounts;
DROP POLICY IF EXISTS "msp_isolation_topology_edges" ON topology_edges;
DROP POLICY IF EXISTS "platform_admin_topology_edges" ON topology_edges;
`,
		},
	}
}

type SchemaManager struct {
	db       *sql.DB
	lockConn *sql.Conn
}

func NewSchemaManager(db *sql.DB) *SchemaManager {
	return &SchemaManager{db: db}
}

var (
	ErrTableNotFound         = errors.New("required table not found")
	ErrLockHeld              = errors.New("advisory lock held by another process")
	ErrLockTimeout           = errors.New("advisory lock acquisition timed out")
	ErrLockReleaseFailed     = errors.New("advisory lock release returned error")
	ErrSchemaVersionConflict = errors.New("schema version mismatch")
)

var migrationLockID = GetLockID() // int64, safe for pg_try_advisory_lock

func logLockAttempt(schemaVersion int) {
	fmt.Fprintf(os.Stderr, "[INFO] attempting to acquire migration lock for schema version %d\n", schemaVersion)
}

func logMigrationAttempt(migrationID int) {
	fmt.Fprintf(os.Stderr, "[INFO] applying migration %d\n", migrationID)
}

func (m *SchemaManager) acquireLock(ctx context.Context, schemaVersion int) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := m.db.Conn(timeoutCtx)
	if err != nil {
		return fmt.Errorf("get database connection: %w", err)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	logLockAttempt(schemaVersion)

	for i := 0; i < 5; i++ {
		select {
		case <-timeoutCtx.Done():
			_ = conn.Close()
			return fmt.Errorf("advisory lock timed out: %w", context.DeadlineExceeded)
		default:
		}

		var acquired bool
		err := conn.QueryRowContext(timeoutCtx, "SELECT pg_try_advisory_lock($1)", migrationLockID).Scan(&acquired)
		if err != nil {
			if i < 4 {
				<-ticker.C
				continue
			}
			_ = conn.Close()
			return fmt.Errorf("lock attempt %d/%d: %w", i+1, 5, err)
		}
		if acquired {
			m.lockConn = conn
			return nil
		}
		if i < 4 {
			<-ticker.C
		}
	}

	_ = conn.Close()
	return fmt.Errorf("lock attempt %d/%d: %w", 5, 5, ErrLockTimeout)
}

func (m *SchemaManager) releaseLock(ctx context.Context) error {
	if m.lockConn == nil {
		return fmt.Errorf("lock already released: %w", ErrLockReleaseFailed)
	}
	_, err := m.lockConn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationLockID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] error releasing advisory lock: %v\n", err)
		return fmt.Errorf("release advisory lock: %w", ErrLockReleaseFailed)
	}
	closeErr := m.lockConn.Close()
	if closeErr != nil {
		fmt.Fprintf(os.Stderr, "[WARN] error closing lock connection: %v\n", closeErr)
		if err == nil {
			return fmt.Errorf("close lock connection: %w", ErrLockReleaseFailed)
		}
	}
	m.lockConn = nil
	return nil
}

func (m *SchemaManager) Apply(ctx context.Context) error {
	finalSchemaVersion := 0
	if migs := Migrations(); len(migs) > 0 {
		finalSchemaVersion = migs[len(migs)-1].ID
	}

	if err := m.acquireLock(ctx, finalSchemaVersion); err != nil {
		return fmt.Errorf("acquiring migration lock: %w", err)
	}
	defer func() {
		if releaseErr := m.releaseLock(ctx); releaseErr != nil {
			fmt.Fprintf(os.Stderr, "[WARN] migration lock release failed: %v\n", releaseErr)
		}
	}()

	_, err := m.lockConn.ExecContext(ctx, `
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
		logMigrationAttempt(mig.ID)

		var exists bool
		err := m.lockConn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE id = $1)`, mig.ID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", mig.ID, err)
		}
		if exists {
			continue
		}

		tx, err := m.lockConn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d (%s): %w", mig.ID, mig.Name, err)
		}
		if _, err := tx.Exec(mig.Up); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d (%s): %w", mig.ID, mig.Name, err)
		}

		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (id, name) VALUES ($1, $2)`,
			mig.ID, mig.Name,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", mig.ID, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d (%s): %w", mig.ID, mig.Name, err)
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
	email := os.Getenv("STRATA_DEV_ADMIN_EMAIL")
	passwordHash := os.Getenv("STRATA_DEV_ADMIN_PASSWORD_HASH")
	if email == "" || passwordHash == "" {
		return fmt.Errorf("STRATA_DEV_ADMIN_EMAIL and STRATA_DEV_ADMIN_PASSWORD_HASH are required")
	}
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
		INSERT INTO users (id, tenant_id, email, password_hash, role, email_verified_at)
		VALUES (
			'00000000-0000-0000-0000-000000000010',
			'00000000-0000-0000-0000-000000000001',
			$1,
			$2,
			'viewer',
			NOW()
		) ON CONFLICT DO NOTHING
	`, email, passwordHash)
	if err != nil {
		return fmt.Errorf("seed dev user: %w", err)
	}

	return nil
}
