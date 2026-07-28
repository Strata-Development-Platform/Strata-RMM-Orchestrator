import type {
  CustomerSummary,
  LoginResponse,
  MSPTenant,
  PlatformOverview,
  User,
  WorkspaceContext,
} from './types';

const BASE = '';

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

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (this.token) headers['Authorization'] = `Bearer ${this.token}`;

    const res = await fetch(`${BASE}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });

    if (res.status === 401) {
      this.setToken(null);
      window.location.href = '/login';
      throw new Error('unauthorized');
    }

    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'request failed');
    return data as T;
  }

  // Auth
  login = (email: string, password: string) =>
    this.request<LoginResponse>('POST', '/api/v1/auth/login', { email, password });

  me = () => this.request<LoginResponse>('GET', '/api/v1/auth/me');

  // Platform
  getOverview = () => this.request<PlatformOverview>('GET', '/api/v1/platform/overview');
  getCustomers = () => this.request<{ customers: CustomerSummary[] }>('GET', '/api/v1/platform/customers');
  getWorkspaceContext = () => this.request<WorkspaceContext>('GET', '/api/v2/context');
  getMSPs = () => this.request<{ msps: MSPTenant[] }>('GET', '/api/v2/platform/msps');

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
