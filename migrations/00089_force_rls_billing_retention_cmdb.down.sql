-- Disable FORCE RLS on billing tables
ALTER TABLE billing_accounts DISABLE ROW LEVEL SECURITY;
ALTER TABLE subscriptions DISABLE ROW LEVEL SECURITY;
ALTER TABLE invoices DISABLE ROW LEVEL SECURITY;
ALTER TABLE invoice_items DISABLE ROW LEVEL SECURITY;
ALTER TABLE usage_records DISABLE ROW LEVEL SECURITY;
ALTER TABLE payment_methods DISABLE ROW LEVEL SECURITY;

-- Disable FORCE RLS on retention table
ALTER TABLE tenant_retention_settings DISABLE ROW LEVEL SECURITY;

-- Disable FORCE RLS on CMDB tables
ALTER TABLE device_relationships DISABLE ROW LEVEL SECURITY;
ALTER TABLE network_addresses DISABLE ROW LEVEL SECURITY;
ALTER TABLE device_packages DISABLE ROW LEVEL SECURITY;
ALTER TABLE device_services DISABLE ROW LEVEL SECURITY;
ALTER TABLE device_mounts DISABLE ROW LEVEL SECURITY;
ALTER TABLE topology_edges DISABLE ROW LEVEL SECURITY;

-- Drop RLS policies for billing tables
DROP POLICY IF EXISTS "msp_isolation_billing_accounts" ON billing_accounts;
DROP POLICY IF EXISTS "platform_admin_billing_accounts" ON billing_accounts;
DROP POLICY IF EXISTS "msp_isolation_subscriptions" ON subscriptions;
DROP POLICY IF EXISTS "platform_admin_subscriptions" ON subscriptions;
DROP POLICY IF EXISTS "msp_isolation_invoices" ON invoices;
DROP POLICY IF EXISTS "platform_admin_invoices" ON invoices;
DROP POLICY IF EXISTS "msp_isolation_invoice_items" ON invoice_items;
DROP POLICY IF EXISTS "platform_admin_invoice_items" ON invoice_items;
DROP POLICY IF EXISTS "msp_isolation_usage_records" ON usage_records;
DROP POLICY IF EXISTS "platform_admin_usage_records" ON usage_records;
DROP POLICY IF EXISTS "msp_isolation_payment_methods" ON payment_methods;
DROP POLICY IF EXISTS "platform_admin_payment_methods" ON payment_methods;

-- Drop RLS policies for retention table
DROP POLICY IF EXISTS "tenant_isolation_retention_settings" ON tenant_retention_settings;
DROP POLICY IF EXISTS "platform_admin_retention_settings" ON tenant_retention_settings;

-- Drop RLS policies for CMDB tables
DROP POLICY IF EXISTS "msp_isolation_device_relationships" ON device_relationships;
DROP POLICY IF EXISTS "platform_admin_device_relationships" ON device_relationships;
DROP POLICY IF EXISTS "msp_isolation_network_addresses" ON network_addresses;
DROP POLICY IF EXISTS "platform_admin_network_addresses" ON network_addresses;
DROP POLICY IF EXISTS "msp_isolation_device_packages" ON device_packages;
DROP POLICY IF EXISTS "platform_admin_device_packages" ON device_packages;
DROP POLICY IF EXISTS "msp_isolation_device_services" ON device_services;
DROP POLICY IF EXISTS "platform_admin_device_services" ON device_services;
DROP POLICY IF EXISTS "msp_isolation_device_mounts" ON device_mounts;
DROP POLICY IF EXISTS "platform_admin_device_mounts" ON device_mounts;
DROP POLICY IF EXISTS "msp_isolation_topology_edges" ON topology_edges;
DROP POLICY IF EXISTS "platform_admin_topology_edges" ON topology_edges;
