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
		INSERT INTO users (id, tenant_id, email, password_hash, role)
		VALUES (
			'00000000-0000-0000-0000-000000000010',
			'00000000-0000-0000-0000-000000000001',
			$1,
			$2,
			'viewer'
		) ON CONFLICT DO NOTHING
	`, email, passwordHash)
	if err != nil {
		return fmt.Errorf("seed dev user: %w", err)
	}

	return nil
}
