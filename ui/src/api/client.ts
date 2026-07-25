import type { LoginResponse, PlatformOverview, CustomerSummary, TenantInfo, User } from './types';

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
    if (this.token) headers['Authorization'] = this.token;

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

  // Admin
  getUsers = () => this.request<{ users: User[] }>('GET', '/api/v1/admin/users');
  createUser = (email: string, password: string, role: string, tenant_ids: string[]) =>
    this.request<{ id: string }>('POST', '/api/v1/admin/users', { email, password, role, tenant_ids });
  updateUserTenants = (userId: string, tenant_ids: string[]) =>
    this.request<{ status: string }>('PUT', `/api/v1/admin/users/${userId}/tenants`, { tenant_ids });

  // Tenants
  getTenant = (id: string) => this.request<Record<string, unknown>>('GET', `/api/v1/tenants/${id}`);
}

export const api = new ApiClient();
