import type {
  CustomerSummary,
  BrandingProfile,
  ClientOrganization,
  CreateMSPWithOwnerRequest,
  CreateMSPWithOwnerResponse,
  CustomDomain,
  EnrollmentToken,
  Entitlement,
  InvitationInspection,
  LoginResponse,
  ManagedDevice,
  Membership,
  MSPTenant,
  PlatformOverview,
  ProviderBusinessProfile,
  ProviderBusinessProfilePatch,
  ProviderBusinessProfileValues,
  ResendOwnerInvitationResponse,
  Site,
  Usage,
  User,
  WorkspaceContext,
} from './types';

const BASE = '';

export class ApiError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

class ApiClient {
  private token: string | null = null;

  constructor() {
    this.token = localStorage.getItem('strata_auth_token');
  }

  setToken(token: string | null) {
    this.token = token;
    if (token) localStorage.setItem('strata_auth_token', token);
    else localStorage.removeItem('strata_auth_token');
  }

  getToken() { return this.token; }

  private async request<T>(method: string, path: string, body?: unknown, authenticated = true): Promise<T> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (authenticated && this.token) headers['Authorization'] = `Bearer ${this.token}`;

    const res = await fetch(`${BASE}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });

    if (res.status === 401 && authenticated) {
      this.setToken(null);
      window.location.href = '/login';
      throw new ApiError(res.status, 'unauthorized');
    }

    if (res.status === 204) return undefined as T;

    const data = await res.json().catch(() => undefined) as { error?: string } | undefined;
    if (!res.ok) throw new ApiError(res.status, data?.error || 'request failed');
    return data as T;
  }

  // Auth
  login = (email: string, password: string) =>
    this.request<LoginResponse>('POST', '/api/v1/auth/login', { email, password });

  me = () => this.request<LoginResponse>('GET', '/api/v1/auth/me');
  inspectInvitation = (token: string) =>
    this.request<InvitationInspection>('POST', '/api/v1/auth/invitations/inspect', { token }, false);
  acceptInvitation = (token: string, password: string) =>
    this.request<void>('POST', '/api/v1/auth/invitations/accept', { token, password }, false);

  // Platform
  getOverview = () => this.request<PlatformOverview>('GET', '/api/v1/platform/overview');
  getCustomers = () => this.request<{ customers: CustomerSummary[] }>('GET', '/api/v1/platform/customers');
  getWorkspaceContext = () => this.request<WorkspaceContext>('GET', '/api/v2/context');
  getProviderProfile = () =>
    this.request<ProviderBusinessProfile>('GET', '/api/v2/platform/provider/profile');
  completeProviderSetup = (profile: ProviderBusinessProfileValues) =>
    this.request<ProviderBusinessProfile>('POST', '/api/v2/platform/provider/setup', profile);
  updateProviderProfile = (profile: ProviderBusinessProfilePatch) =>
    this.request<ProviderBusinessProfile>('PATCH', '/api/v2/platform/provider/profile', profile);
  switchWorkspace = (msp_id: string, client_id = '', site_id = '') =>
    this.request<{ token: string; msp_id: string; client_id: string; site_id: string; expires_at: string }>(
      'POST', '/api/v2/context/switch', { msp_id, client_id, site_id }
    );
  getMSPs = () => this.request<{ msps: MSPTenant[] }>('GET', '/api/v2/platform/msps');
  createMSPWithOwner = (request: CreateMSPWithOwnerRequest) =>
    this.request<CreateMSPWithOwnerResponse>('POST', '/api/v2/platform/msps', request);
  resendOwnerInvitation = (mspID: string) =>
    this.request<ResendOwnerInvitationResponse>('POST', `/api/v2/platform/msps/${mspID}/owner-invitation`);
  getClients = (mspID: string) =>
    this.request<{ clients: ClientOrganization[] }>('GET', `/api/v2/msps/${mspID}/clients`);
  createClient = (mspID: string, name: string, slug: string) =>
    this.request<{ id: string; status: string }>('POST', `/api/v2/msps/${mspID}/clients`, { name, slug });
  archiveClient = (mspID: string, clientID: string) =>
    this.request<{ status: string }>('POST', `/api/v2/msps/${mspID}/clients/${clientID}/archive`);
  getSites = (clientID: string) =>
    this.request<{ sites: Site[] }>('GET', `/api/v2/clients/${clientID}/sites`);
  createSite = (clientID: string, name: string, slug: string) =>
    this.request<{ id: string; status: string }>('POST', `/api/v2/clients/${clientID}/sites`, { name, slug });
  archiveSite = (clientID: string, siteID: string) =>
    this.request<{ status: string }>('POST', `/api/v2/clients/${clientID}/sites/${siteID}/archive`);
  getMemberships = (mspID: string) =>
    this.request<{ memberships: Membership[] }>('GET', `/api/v2/msps/${mspID}/memberships`);
  createMembership = (mspID: string, user_id: string, role: string) =>
    this.request<{ id: string; status: string }>('POST', `/api/v2/msps/${mspID}/memberships`, {
      user_id, role, scope_type: 'msp', scope_id: mspID,
    });
  revokeMembership = (mspID: string, membershipID: string) =>
    this.request<{ status: string }>('DELETE', `/api/v2/msps/${mspID}/memberships/${membershipID}`);
  getBranding = () => this.request<BrandingProfile>('GET', '/api/v1/branding');
  updateBranding = (branding: Partial<BrandingProfile>) =>
    this.request<{ status: string }>('PUT', '/api/v1/branding', branding);
  getDomains = () => this.request<{ domains: CustomDomain[] }>('GET', '/api/v1/domains');
  createDomain = (hostname: string) =>
    this.request<{ id: string; hostname: string; verification_token: string; txt_name: string }>(
      'POST', '/api/v1/domains', { hostname }
    );
  verifyDomain = (domainID: string) =>
    this.request<{ status: string; certificate_status: string }>('POST', `/api/v1/domains/${domainID}/verify`);
  deleteDomain = (domainID: string) =>
    this.request<{ status: string }>('DELETE', `/api/v1/domains/${domainID}`);
  getEnrollmentTokens = () =>
    this.request<{ tokens: EnrollmentToken[] }>('GET', '/api/v1/enrollment/tokens');
  createEnrollmentToken = (client_id: string, site_id: string, max_uses: number, description: string) =>
    this.request<{ id: string; token: string; expires_at: string; max_uses: number }>(
      'POST', '/api/v1/enrollment/tokens', { client_id, site_id, max_uses, description }
    );
  revokeEnrollmentToken = (tokenID: string) =>
    this.request<{ status: string }>('DELETE', `/api/v1/enrollment/tokens/${tokenID}`);
  getEntitlement = (mspID: string) =>
    this.request<Entitlement>('GET', `/api/v2/msps/${mspID}/entitlement`);
  updateEntitlement = (mspID: string, plan_slug: string, status: string) =>
    this.request<{ status: string }>('PATCH', `/api/v2/platform/msps/${mspID}/entitlement`, { plan_slug, status });
  getUsage = (mspID: string) => this.request<Usage>('GET', `/api/v2/msps/${mspID}/usage`);
  getControlPlaneAudit = (mspID: string) =>
    this.request<{ entries: Record<string, unknown>[] }>('GET', `/api/v2/msps/${mspID}/audit`);
  getMSPDevices = (mspID: string) =>
    this.request<{ devices: ManagedDevice[] }>('GET', `/api/v2/msps/${mspID}/devices`);

  // Admin
  getUsers = () => this.request<{ users: User[] }>('GET', '/api/v1/admin/users');
  createUser = (email: string, password: string, role: string, tenant_ids: string[]) =>
    this.request<{ id: string }>('POST', '/api/v1/admin/users', { email, password, role, tenant_ids });
  updateUserTenants = (userId: string, tenant_ids: string[]) =>
    this.request<{ status: string }>('PUT', `/api/v1/admin/users/${userId}/tenants`, { tenant_ids });

  // Admin
  createCustomer = (name: string, slug: string, plan: string, adminEmail: string) =>
    this.request<{ id: string; name: string; slug: string; plan: string; deployment_id: string; status: string }>(
      'POST', '/api/v1/admin/customers', { name, slug, plan, admin_email: adminEmail }
    );

  // Devices
  getTenantDevices = (tenantID: string) =>
    this.request<{ devices: Record<string, unknown>[] }>('GET', `/api/v1/platform/customers/${tenantID}/devices`);
  getDevice = (deviceID: string) =>
    this.request<Record<string, unknown>>('GET', `/api/v1/platform/customers/_/devices/${deviceID}`);

  // Alerts
  getTenantAlerts = (tenantID: string) =>
    this.request<{ alerts: Record<string, unknown>[] }>('GET', `/api/v1/alerts/${tenantID}`);
  getAlertHistory = (tenantID: string) =>
    this.request<{ alerts: Record<string, unknown>[] }>('GET', `/api/v1/alerts/${tenantID}/history`);
  getRules = (tenantID: string) =>
    this.request<{ rules: Record<string, unknown>[] }>('GET', `/api/v1/rules/${tenantID}`);

  // Vulnerabilities
  getTenantVulns = (tenantID: string) =>
    this.request<{ vulnerabilities: Record<string, unknown>[] }>('GET', `/api/v1/vulnerabilities/tenant/${tenantID}`);
  getVulnSummary = (tenantID: string) =>
    this.request<Record<string, unknown>>('GET', `/api/v1/vulnerabilities/tenant/${tenantID}/summary`);

  // Recordings
  getTenantRecordings = (tenantID: string) =>
    this.request<{ recordings: Record<string, unknown>[] }>('GET', `/api/v1/recordings/${tenantID}`);

  // Keys
  getTenantKeys = (tenantID: string) =>
    this.request<{ keys: Record<string, unknown>[] }>('GET', `/api/v1/keys/${tenantID}`);

  // Scripts
  getScripts = (tenantID: string) =>
    this.request<{ scripts: Record<string, unknown>[] }>('GET', `/api/v1/scripts/${tenantID}`);
  getScript = (tenantID: string, scriptID: string) =>
    this.request<Record<string, unknown>>('GET', `/api/v1/scripts/${tenantID}/${scriptID}`);
  createScript = (tenantID: string, data: Record<string, unknown>) =>
    this.request<Record<string, unknown>>('POST', `/api/v1/scripts/${tenantID}`, data);
  deleteScript = (tenantID: string, scriptID: string) =>
    this.request<{ status: string }>('DELETE', `/api/v1/scripts/${tenantID}/${scriptID}`);
  runScript = (tenantID: string, scriptID: string, deviceIDs: string[], params?: Record<string, string>) =>
    this.request<{ script: string; executions: Record<string, unknown>[]; count: number }>(
      'POST', `/api/v1/scripts/${tenantID}/${scriptID}/run`, { device_ids: deviceIDs, parameters: params || {} }
    );
  getScriptExecutions = (tenantID: string) =>
    this.request<{ executions: Record<string, unknown>[] }>('GET', `/api/v1/scripts/${tenantID}/executions`);
  getScriptExecution = (tenantID: string, execID: string) =>
    this.request<Record<string, unknown>>('GET', `/api/v1/scripts/${tenantID}/executions/${execID}`);

  // Software
  getPackages = (tenantID: string) =>
    this.request<{ packages: Record<string, unknown>[] }>('GET', `/api/v1/software/packages/${tenantID}`);
  createPackage = (tenantID: string, data: Record<string, unknown>) =>
    this.request<Record<string, unknown>>('POST', `/api/v1/software/packages/${tenantID}`, data);
  deletePackage = (tenantID: string, pkgID: string) =>
    this.request<{ status: string }>('DELETE', `/api/v1/software/packages/${tenantID}/${pkgID}`);
  createDeployment = (tenantID: string, data: Record<string, unknown>) =>
    this.request<Record<string, unknown>>('POST', `/api/v1/software/deployments/${tenantID}`, data);
  getDeployments = (tenantID: string) =>
    this.request<{ deployments: Record<string, unknown>[] }>('GET', `/api/v1/software/deployments/${tenantID}`);
  getDeployment = (tenantID: string, deployID: string) =>
    this.request<Record<string, unknown>>('GET', `/api/v1/software/deployments/${tenantID}/${deployID}`);

  // Access
  getTenantUsers = (tenantID: string) =>
    this.request<{ users: Record<string, unknown>[] }>('GET', `/api/v1/access/users/${tenantID}`);
  getTenantAudit = (tenantID: string) =>
    this.request<{ audit_entries: Record<string, unknown>[] }>('GET', `/api/v1/access/audit/${tenantID}`);
  getTenantPermissions = (tenantID: string) =>
    this.request<{ permissions: Record<string, unknown>[] }>('GET', `/api/v1/access/permissions/${tenantID}`);
}

export const api = new ApiClient();
