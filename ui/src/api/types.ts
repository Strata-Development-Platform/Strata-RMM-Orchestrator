export type LoginResponse = {
  token: string;
  user_id: string;
  email: string;
  role: string;
  accessible_tenants: TenantInfo[];
  expires_at?: string;
};

export type TenantInfo = {
  id: string;
  name: string;
  slug: string;
};

export type PlatformOverview = {
  total_devices: number;
  online_devices: number;
  offline_devices: number;
  active_alerts: number;
  critical_alerts: number;
  open_cves: number;
  total_customers: number;
  timestamp: string;
};

export type CustomerSummary = {
  id: string;
  name: string;
  slug: string;
  plan: string;
  is_active: boolean;
  deployment_id: string;
  device_count: number;
  online_count: number;
  alert_count: number;
  cve_count: number;
  created_at: string;
};

export type User = {
  id: string;
  email: string;
  role: string;
  is_active: boolean;
  last_login?: string;
  created_at: string;
  accessible_tenants?: TenantInfo[];
};

export type MSPTenant = {
  id: string;
  name: string;
  slug: string;
  plan?: string;
  client_count: number;
  device_count: number;
  is_active: boolean;
  created_at: string;
};

export type WorkspaceScope = {
  type: 'platform' | 'msp' | 'client' | 'site';
  id: string;
  name: string;
  parent_id?: string;
  role: string;
};

export type WorkspaceContext = {
  user_id: string;
  email: string;
  roles: string[];
  permissions: string[];
  available_scopes: WorkspaceScope[];
  msp_id: string;
  msp_name: string;
  msp_active: boolean;
  client_id: string;
  client_name: string;
  site_id: string;
  site_name: string;
  branding?: Record<string, unknown>;
  entitlement?: {
    plan_slug: string;
    status: string;
    max_devices: number;
    max_users: number;
    features: Record<string, unknown>;
  };
  platform_role: boolean;
  authenticated_at: string;
};
