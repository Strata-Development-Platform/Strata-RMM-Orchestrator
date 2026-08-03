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
    client_id UUID NOT NULL REFERENCES client_organizations(id) ON DELETE CASCADE UNIQUE,
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

-- RLS policies for client_auth_providers (use DO blocks because PostgreSQL
-- does not support IF NOT EXISTS for CREATE POLICY)
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'Users can read auth providers for their client' AND tablename = 'client_auth_providers'
    ) THEN
        CREATE POLICY "Users can read auth providers for their client"
            ON client_auth_providers FOR SELECT
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
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'Platform admins can manage all auth providers' AND tablename = 'client_auth_providers'
    ) THEN
        CREATE POLICY "Platform admins can manage all auth providers"
            ON client_auth_providers FOR ALL
            USING (
                current_setting('app.role', true) IN ('platform_owner', 'platform_admin')
            );
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'MSP admins can manage auth providers for their MSP clients' AND tablename = 'client_auth_providers'
    ) THEN
        CREATE POLICY "MSP admins can manage auth providers for their MSP clients"
            ON client_auth_providers FOR ALL
            USING (
                EXISTS (
                    SELECT 1 FROM client_organizations c
                    WHERE c.id = client_auth_providers.client_id
                    AND c.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID
                )
            );
    END IF;
END $$;

-- RLS policies for client_sessions
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'Users can read sessions for their client' AND tablename = 'client_sessions'
    ) THEN
        CREATE POLICY "Users can read sessions for their client"
            ON client_sessions FOR SELECT
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
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'Users can manage their own sessions' AND tablename = 'client_sessions'
    ) THEN
        CREATE POLICY "Users can manage their own sessions"
            ON client_sessions FOR ALL
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
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'Platform admins can manage all sessions' AND tablename = 'client_sessions'
    ) THEN
        CREATE POLICY "Platform admins can manage all sessions"
            ON client_sessions FOR ALL
            USING (
                current_setting('app.role', true) IN ('platform_owner', 'platform_admin')
            );
    END IF;
END $$;

-- RLS policies for client_portal_settings
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'Users can read portal settings for their client' AND tablename = 'client_portal_settings'
    ) THEN
        CREATE POLICY "Users can read portal settings for their client"
            ON client_portal_settings FOR SELECT
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
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'Platform admins can manage all portal settings' AND tablename = 'client_portal_settings'
    ) THEN
        CREATE POLICY "Platform admins can manage all portal settings"
            ON client_portal_settings FOR ALL
            USING (
                current_setting('app.role', true) IN ('platform_owner', 'platform_admin')
            );
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'MSP admins can manage portal settings for their MSP clients' AND tablename = 'client_portal_settings'
    ) THEN
        CREATE POLICY "MSP admins can manage portal settings for their MSP clients"
            ON client_portal_settings FOR ALL
            USING (
                EXISTS (
                    SELECT 1 FROM client_organizations c
                    WHERE c.id = client_portal_settings.client_id
                    AND c.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID
                )
            );
    END IF;
END $$;

-- Update updated_at trigger for all tables
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
   NEW.updated_at = NOW();
   RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers (use DO blocks because PostgreSQL does not support
-- IF NOT EXISTS for CREATE TRIGGER)
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_client_auth_providers_updated_at'
    ) THEN
        CREATE TRIGGER "update_client_auth_providers_updated_at"
            BEFORE UPDATE ON client_auth_providers
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_client_sessions_updated_at'
    ) THEN
        CREATE TRIGGER "update_client_sessions_updated_at"
            BEFORE UPDATE ON client_sessions
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_client_portal_settings_updated_at'
    ) THEN
        CREATE TRIGGER "update_client_portal_settings_updated_at"
            BEFORE UPDATE ON client_portal_settings
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;
