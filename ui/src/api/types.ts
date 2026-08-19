export type LoginResponse = {
  token: string;
  user_id: string;
  email: string;
  role: string;
  roles: string[];
  permissions: string[];
  selected_scope: AuthorizationScope;
  grants: AuthorizationGrant[];
  tenant_id: string;
  provider_display_name: string;
  setup_complete: boolean;
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
  legacy_role: string;
  is_active: boolean;
  last_login?: string;
  created_at: string;
  accessible_tenants?: TenantInfo[];
  memberships: UserMembership[];
};

export type ScopeType = 'platform' | 'msp' | 'client' | 'site';

export type AuthorizationScope = {
  type: ScopeType;
  id: string;
  platform_id?: string;
  msp_id?: string;
  client_id?: string;
  site_id?: string;
};

export type AuthorizationGrant = {
  role: string;
  source_type: ScopeType;
  source_id: string;
  inherited: boolean;
};

export type UserMembership = {
  id?: string;
  scope_type: ScopeType;
  scope_id: string;
  role: string;
  status?: string;
  expires_at?: string;
};

export type MSPTenant = {
  id: string;
  name: string;
  slug: string;
  plan: string;
  client_count: number;
  device_count: number;
  is_active: boolean;
  onboarding_status: 'pending_owner' | 'active';
  owner_invitation_delivery_status: string;
  created_at: string;
};

export type InvitationInspection = {
  msp: {
    name: string;
  };
  masked_email: string;
  expires_at: string;
};

export type CreateMSPWithOwnerRequest = {
  name: string;
  slug: string;
  plan: string;
  owner_email: string;
};

export type CreateMSPWithOwnerResponse = {
  id: string;
  status: 'pending_owner';
  delivery_status: string;
};

export type ResendOwnerInvitationResponse = {
  status: 'invitation_rotated';
  delivery_status: string;
};

export type WorkspaceScope = {
  type: 'platform' | 'msp' | 'client' | 'site';
  id: string;
  name: string;
  parent_id?: string;
};

export type WorkspaceContext = {
  user_id: string;
  email: string;
  roles: string[];
  permissions: string[];
  selected_scope: AuthorizationScope;
  grants: AuthorizationGrant[];
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
  platform_id: string;
  provider_display_name: string;
  setup_complete: boolean;
  authenticated_at: string;
};

export type ProviderBusinessProfileValues = {
  legal_name: string;
  display_name: string;
  contact_name: string;
  support_email: string;
  billing_email: string;
  business_phone: string;
  website_url: string;
  address_line1: string;
  address_line2: string;
  city: string;
  state_province: string;
  postal_code: string;
  country_code: string;
  default_timezone: string;
  default_locale: string;
  default_currency: string;
  tax_identifier: string;
  logo_light_url: string;
  logo_dark_url: string;
  favicon_url: string;
  brand_light_color: string;
  brand_dark_color: string;
  terms_url: string;
  privacy_url: string;
  support_url: string;
  public_saas_enabled: boolean;
  public_saas_headline: string;
  public_saas_description: string;
};

type OptionalProviderBusinessProfileFields =
  | 'website_url'
  | 'address_line2'
  | 'state_province'
  | 'tax_identifier'
  | 'logo_light_url'
  | 'logo_dark_url'
  | 'favicon_url'
  | 'support_url'
  | 'public_saas_headline'
  | 'public_saas_description';

export type ProviderBusinessProfile = Omit<ProviderBusinessProfileValues, OptionalProviderBusinessProfileFields> &
  Partial<Pick<ProviderBusinessProfileValues, OptionalProviderBusinessProfileFields>> & {
  id: string;
  slug: string;
  setup_complete: boolean;
  setup_contract_version: number;
  outbound_email_status: 'configured' | 'not_configured';
  setup_completed_at?: string;
  setup_completed_by?: string;
  updated_at: string;
  };

export type ProviderBusinessProfilePatch = Partial<ProviderBusinessProfileValues>;

export type ClientOrganization = {
  id: string;
  msp_id: string;
  name: string;
  slug: string;
  is_active: boolean;
  site_count?: number;
  device_count?: number;
  created_at: string;
};

export type Site = {
  id: string;
  client_id: string;
  name: string;
  slug: string;
  is_active: boolean;
  device_count?: number;
  created_at: string;
};

export type Membership = {
  id: string;
  user_id: string;
  role: string;
  scope_type: string;
  scope_id: string;
  created_at: string;
};

export type BrandingProfile = {
  id: string;
  msp_id: string;
  display_name: string;
  logo_light?: string;
  logo_dark?: string;
  favicon?: string;
  primary_color: string;
  accent_color: string;
  sidebar_bg: string;
  header_bg: string;
  login_bg: string;
  portal_title: string;
  welcome_text: string;
  support_email?: string;
  terms_url?: string;
  privacy_url?: string;
};

export type CustomDomain = {
  id: string;
  hostname: string;
  domain_type: string;
  verification_status: string;
  certificate_status: string;
  is_primary: boolean;
  verification_token?: string;
  txt_name?: string;
};

export type EnrollmentToken = {
  id: string;
  client_id: string;
  site_id: string;
  max_uses: number;
  use_count: number;
  expires_at: string;
  is_revoked: boolean;
  created_at: string;
};

export type Entitlement = {
  plan_slug: string;
  status: string;
  max_devices: number;
  max_users: number;
  device_count: number;
  user_count: number;
  features: Record<string, boolean>;
};

export type Usage = {
  msp_id: string;
  device_count: number;
  user_count: number;
  client_count: number;
  site_count: number;
  recorded_at: string;
};

export type ManagedDevice = {
  id: string;
  tenant_id: string;
  hostname: string;
  os: string;
  arch: string;
  status: string;
  agent_version: string;
  client_id: string;
  client_name: string;
  site_id: string;
  site_name: string;
  last_heartbeat?: string;
};

export type DeviceGroup = {
  id: string;
  name: string;
  description: string;
  client_id: string;
  msp_id: string;
  is_smart: boolean;
  filter_expression?: Record<string, unknown>;
  member_count: number;
  last_evaluated?: string;
  created_at: string;
  updated_at: string;
};

export type DeviceGroupMember = {
  id: string;
  hostname: string;
  os: string;
  last_seen: string;
  ip_addresses: string[];
  tags: string[];
  client_id: string;
  site_id: string;
};

export type DeviceGroupEvaluationStatus = {
  group_id: string;
  group_name: string;
  status: string;
  member_count: number;
  started_at: string;
  completed_at?: string;
  error?: string;
};

export type DeviceGroupScriptBinding = {
  id: string;
  group_id: string;
  schedule_id: string;
  schedule_name: string;
  binding_type: string;
  priority: number;
  enabled: boolean;
  created_at: string;
};

export type CreateDeviceGroupRequest = {
  name: string;
  description?: string;
  client_id?: string;
  device_ids?: string[];
  filter_expression?: Record<string, unknown>;
};

export type UpdateDeviceGroupRequest = {
  name?: string;
  description?: string;
  filter_expression?: Record<string, unknown>;
  device_ids?: string[];
};
