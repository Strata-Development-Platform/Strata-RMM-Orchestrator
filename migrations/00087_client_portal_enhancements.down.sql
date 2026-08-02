-- Drop indexes
DROP INDEX IF EXISTS idx_client_sessions_mfa_verified;
DROP INDEX IF EXISTS idx_client_sessions_active;

-- Remove enhanced columns from client_auth_providers
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS issuer;
ALTER TABLE client_auth_providers DROP COLUMN IF EXISTS client_id;
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
