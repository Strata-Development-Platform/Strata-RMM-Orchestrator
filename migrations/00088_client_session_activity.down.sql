-- Drop session activity trigger
DROP TRIGGER IF EXISTS "update_client_session_activity_updated_at" ON client_session_activity;

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
DROP FUNCTION IF EXISTS cleanup_expired_sessions();

-- Drop table
DROP TABLE IF EXISTS client_session_activity;
