-- Migration 00090: Client support request workflow

-- Create client_support_requests table
CREATE TABLE IF NOT EXISTS client_support_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES client_organizations(id) ON DELETE CASCADE,
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    category VARCHAR(32) NOT NULL DEFAULT 'technical',
    subject VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    priority VARCHAR(16) NOT NULL DEFAULT 'normal',
    status VARCHAR(16) NOT NULL DEFAULT 'open',
    platform_reply TEXT,
    platform_reply_at TIMESTAMPTZ,
    reply_from UUID,
    reply_at TIMESTAMPTZ,
    reply_body JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_client_support_requests_tenant ON client_support_requests(tenant_id);
CREATE INDEX IF NOT EXISTS idx_client_support_requests_status ON client_support_requests(status);
CREATE INDEX IF NOT EXISTS idx_client_support_requests_created ON client_support_requests(created_at DESC);

-- RLS policies
ALTER TABLE client_support_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE client_support_requests FORCE ROW LEVEL SECURITY;

CREATE POLICY client_support_read ON client_support_requests FOR SELECT
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::UUID);

CREATE POLICY client_support_insert ON client_support_requests FOR INSERT
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::UUID);

CREATE POLICY client_support_update ON client_support_requests FOR UPDATE
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::UUID);

-- Grant permissions
GRANT SELECT, INSERT, UPDATE ON client_support_requests TO strata_user;
