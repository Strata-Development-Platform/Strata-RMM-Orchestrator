-- Drop RLS policies for client_portal_settings
DROP POLICY IF EXISTS "MSP admins can manage portal settings for their MSP clients" ON client_portal_settings;
DROP POLICY IF EXISTS "Platform admins can manage all portal settings" ON client_portal_settings;
DROP POLICY IF EXISTS "Users can read portal settings for their client" ON client_portal_settings;

-- Drop RLS policies for client_sessions
DROP POLICY IF EXISTS "Users can manage their own sessions" ON client_sessions;
DROP POLICY IF EXISTS "Platform admins can manage all sessions" ON client_sessions;
DROP POLICY IF EXISTS "Users can read sessions for their client" ON client_sessions;

-- Drop RLS policies for client_auth_providers
DROP POLICY IF EXISTS "MSP admins can manage auth providers for their MSP clients" ON client_auth_providers;
DROP POLICY IF EXISTS "Platform admins can manage all auth providers" ON client_auth_providers;
DROP POLICY IF EXISTS "Users can read auth providers for their client" ON client_auth_providers;

-- Drop triggers
DROP TRIGGER IF EXISTS "update_client_portal_settings_updated_at" ON client_portal_settings;
DROP TRIGGER IF EXISTS "update_client_sessions_updated_at" ON client_sessions;
DROP TRIGGER IF EXISTS "update_client_auth_providers_updated_at" ON client_auth_providers;

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
