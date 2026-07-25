export type LoginResponse = {
  token: string;
  user_id: string;
  email: string;
  role: 'admin' | 'technician' | 'viewer';
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
