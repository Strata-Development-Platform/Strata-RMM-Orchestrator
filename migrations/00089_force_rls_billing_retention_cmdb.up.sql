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

-- RLS policies for billing tables (12 policies)
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

-- RLS policies for retention table (2 policies)
DROP POLICY IF EXISTS "tenant_isolation_retention_settings" ON tenant_retention_settings;
CREATE POLICY "tenant_isolation_retention_settings" ON tenant_retention_settings FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::UUID)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::UUID);

DROP POLICY IF EXISTS "platform_admin_retention_settings" ON tenant_retention_settings;
CREATE POLICY "platform_admin_retention_settings" ON tenant_retention_settings FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));

-- RLS policies for CMDB tables (12 policies)
-- device_relationships: check caller-supplied msp_id AND both referenced devices belong to that MSP
DROP POLICY IF EXISTS "msp_isolation_device_relationships" ON device_relationships;
CREATE POLICY "msp_isolation_device_relationships" ON device_relationships FOR ALL
    USING (
        msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID
        AND EXISTS (SELECT 1 FROM devices WHERE id = device_relationships.source_device_id AND msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
        AND EXISTS (SELECT 1 FROM devices WHERE id = device_relationships.target_device_id AND msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    )
    WITH CHECK (
        msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID
        AND EXISTS (SELECT 1 FROM devices WHERE id = device_relationships.source_device_id AND msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
        AND EXISTS (SELECT 1 FROM devices WHERE id = device_relationships.target_device_id AND msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    );

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
        AND EXISTS (SELECT 1 FROM devices d WHERE d.id = topology_edges.dst_device_id AND d.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    )
    WITH CHECK (
        EXISTS (SELECT 1 FROM devices d WHERE d.id = topology_edges.src_device_id AND d.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
        AND EXISTS (SELECT 1 FROM devices d WHERE d.id = topology_edges.dst_device_id AND d.msp_id = NULLIF(current_setting('app.msp_id', true), '')::UUID)
    );

DROP POLICY IF EXISTS "platform_admin_topology_edges" ON topology_edges;
CREATE POLICY "platform_admin_topology_edges" ON topology_edges FOR ALL
    USING (current_setting('app.role', true) IN ('platform_owner', 'platform_admin'));
