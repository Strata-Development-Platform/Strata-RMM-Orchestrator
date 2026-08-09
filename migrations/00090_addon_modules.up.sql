-- Durable platform-controlled add-on module registry.
-- Alpha intentionally keeps installation authority at the platform layer. MSP-scoped
-- executable module installation requires a separate service-identity design.

CREATE TABLE addon_modules (
    module_id TEXT PRIMARY KEY,
    manifest JSONB NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('installed', 'enabled', 'disabled', 'quarantined')),
    reason TEXT NOT NULL DEFAULT '',
    installed_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT addon_modules_manifest_object CHECK (jsonb_typeof(manifest) = 'object')
);

CREATE INDEX idx_addon_modules_state ON addon_modules(state);

CREATE TABLE addon_module_audit (
    id BIGSERIAL PRIMARY KEY,
    module_id TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('install', 'enable', 'disable', 'quarantine', 'restore', 'uninstall')),
    previous_state TEXT,
    new_state TEXT,
    reason TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_addon_module_audit_module_time
    ON addon_module_audit(module_id, created_at DESC, id DESC);

ALTER TABLE addon_modules ENABLE ROW LEVEL SECURITY;
ALTER TABLE addon_modules FORCE ROW LEVEL SECURITY;
ALTER TABLE addon_module_audit ENABLE ROW LEVEL SECURITY;
ALTER TABLE addon_module_audit FORCE ROW LEVEL SECURITY;

CREATE POLICY platform_admin_addon_modules ON addon_modules FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'))
    WITH CHECK (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));

CREATE POLICY platform_admin_addon_module_audit ON addon_module_audit
    FOR SELECT
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));

CREATE POLICY platform_admin_addon_module_audit_insert ON addon_module_audit
    FOR INSERT
    WITH CHECK (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));

-- Audit evidence is append-only, including for platform administrators.
CREATE OR REPLACE FUNCTION prevent_addon_module_audit_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'addon module audit rows are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER addon_module_audit_no_update
    BEFORE UPDATE ON addon_module_audit
    FOR EACH ROW EXECUTE FUNCTION prevent_addon_module_audit_mutation();

CREATE TRIGGER addon_module_audit_no_delete
    BEFORE DELETE ON addon_module_audit
    FOR EACH ROW EXECUTE FUNCTION prevent_addon_module_audit_mutation();
