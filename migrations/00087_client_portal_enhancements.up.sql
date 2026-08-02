-- Add provider configuration columns for enhanced SSO support
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS issuer TEXT;
ALTER TABLE client_auth_providers ADD COLUMN IF NOT EXISTS client_id TEXT;
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
