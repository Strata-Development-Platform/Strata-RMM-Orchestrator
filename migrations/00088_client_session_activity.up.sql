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

-- RLS policies for client_session_activity (use DO blocks because PostgreSQL
-- does not support IF NOT EXISTS for CREATE POLICY)
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'Users can read activity for their client sessions' AND tablename = 'client_session_activity'
    ) THEN
        CREATE POLICY "Users can read activity for their client sessions"
            ON client_session_activity FOR SELECT
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
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'Platform admins can read all activity' AND tablename = 'client_session_activity'
    ) THEN
        CREATE POLICY "Platform admins can read all activity"
            ON client_session_activity FOR SELECT
            USING (
                current_setting('app.role', true) IN ('platform_owner', 'platform_admin')
            );
    END IF;
END $$;

-- Create index on client_id for easier filtering
CREATE INDEX IF NOT EXISTS idx_client_session_activity_client ON client_session_activity(
    session_id
) INCLUDE (activity_type, created_at);

-- Add trigger to update updated_at (use DO block because PostgreSQL does not
-- support IF NOT EXISTS for CREATE TRIGGER)
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_client_session_activity_updated_at'
    ) THEN
        CREATE TRIGGER "update_client_session_activity_updated_at"
            BEFORE UPDATE ON client_session_activity
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

-- Add function to clean up old sessions and activity
CREATE OR REPLACE FUNCTION cleanup_expired_sessions()
RETURNS TRIGGER AS $$
BEGIN
    -- Clean up expired sessions
    DELETE FROM client_sessions
    WHERE expires_at < NOW() AND revoked_at IS NULL;

    -- Clean up session activity for deleted sessions
    DELETE FROM client_session_activity
    WHERE session_id NOT IN (SELECT id FROM client_sessions);

    RETURN NULL;
END;
$$ language 'plpgsql';

-- Schedule cleanup (requires pg_cron extension)
-- SELECT cron.schedule('cleanup-sessions', '0 0 * * *', 'SELECT cleanup_expired_sessions()');
