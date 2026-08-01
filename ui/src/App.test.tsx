import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from '@/App';
import { api } from '@/api/client';
import type {
  LoginResponse,
  ProviderBusinessProfile,
  WorkspaceContext,
} from '@/api/types';

const providerUser: LoginResponse = {
  token: 'test-token',
  user_id: 'provider-user',
  email: 'owner@example.test',
  role: 'platform_owner',
  roles: ['platform_owner'],
  permissions: ['platform:manage'],
  selected_scope: { type: 'platform', id: 'platform-1', platform_id: 'platform-1' },
  grants: [{ role: 'platform_owner', source_type: 'platform', source_id: 'platform-1', inherited: false }],
  tenant_id: '',
  provider_display_name: 'Original Provider',
  setup_complete: false,
  accessible_tenants: [],
};

const providerWorkspace: WorkspaceContext = {
  user_id: providerUser.user_id,
  email: providerUser.email,
  roles: ['platform_owner'],
  permissions: ['platform:manage'],
  selected_scope: providerUser.selected_scope,
  grants: providerUser.grants,
  available_scopes: [],
  msp_id: '',
  msp_name: '',
  msp_active: false,
  client_id: '',
  client_name: '',
  site_id: '',
  site_name: '',
  platform_role: true,
  platform_id: 'platform-1',
  provider_display_name: 'Original Provider',
  setup_complete: false,
  authenticated_at: '2026-08-01T00:00:00Z',
};

const completeWorkspace: WorkspaceContext = { ...providerWorkspace, setup_complete: true };

const providerProfile: ProviderBusinessProfile = {
  id: 'platform-1',
  slug: 'provider',
  legal_name: 'Original Provider LLC',
  display_name: 'Original Provider',
  contact_name: 'Ada Owner',
  support_email: 'support@example.test',
  billing_email: 'billing@example.test',
  business_phone: '+1 415 555 0123',
  website_url: 'https://example.test',
  address_line1: '1 Main Street',
  address_line2: '',
  city: 'San Francisco',
  state_province: 'CA',
  postal_code: '94105',
  country_code: 'US',
  default_timezone: 'UTC',
  default_locale: 'en-US',
  default_currency: 'USD',
  tax_identifier: '',
  setup_complete: true,
  setup_completed_at: '2026-07-31T12:00:00Z',
  setup_completed_by: providerUser.user_id,
  updated_at: '2026-07-31T12:00:00Z',
};

function installApi(workspace: WorkspaceContext = providerWorkspace, user: LoginResponse = providerUser) {
  api.setToken('test-token');
  vi.spyOn(api, 'me').mockResolvedValue(user);
  vi.spyOn(api, 'getWorkspaceContext').mockResolvedValue(workspace);
  vi.spyOn(api, 'getOverview').mockResolvedValue({
    total_devices: 0,
    online_devices: 0,
    offline_devices: 0,
    active_alerts: 0,
    critical_alerts: 0,
    open_cves: 0,
    total_customers: 0,
    timestamp: '2026-08-01T00:00:00Z',
  });
  vi.spyOn(api, 'getCustomers').mockResolvedValue({ customers: [] });
}

function renderAt(path: string) {
  window.history.replaceState({}, '', path);
  return render(<App />);
}

async function fillBusiness(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/Legal business name/), 'Example Provider LLC');
  await user.type(screen.getByLabelText(/Display name/), 'Example Provider');
  await user.click(screen.getByRole('button', { name: /Continue/ }));
}

async function fillContact(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/Primary contact name/), 'Ada Admin');
  await user.type(screen.getByLabelText(/Business phone/), '+1 415 555 0199');
  await user.type(screen.getByLabelText(/Support email/), 'support@example.test');
  await user.type(screen.getByLabelText(/Billing email/), 'billing@example.test');
  await user.click(screen.getByRole('button', { name: /Continue/ }));
}

async function fillRegional(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/Address line 1/), '100 Market Street');
  await user.type(screen.getByLabelText(/^City/), 'San Francisco');
  await user.type(screen.getByLabelText(/Postal code/), '94105');
  await user.click(screen.getByRole('button', { name: /Continue/ }));
}

async function advanceToReview(user: ReturnType<typeof userEvent.setup>) {
  await fillBusiness(user);
  await fillContact(user);
  await fillRegional(user);
  await screen.findByRole('heading', { name: 'Review' });
}

beforeEach(() => {
  localStorage.clear();
  document.documentElement.className = '';
});

afterEach(() => {
  api.setToken(null);
  vi.restoreAllMocks();
  window.history.replaceState({}, '', '/');
  document.documentElement.className = '';
});

describe('provider setup routing', () => {
  it('redirects an incomplete provider administrator to setup', async () => {
    installApi();
    renderAt('/');

    expect(await screen.findByRole('heading', { name: /Set up your provider business profile/ })).toBeVisible();
    expect(window.location.pathname).toBe('/provider/setup');
  });

  it.each([
    ['msp_admin', 'MSP Management Dashboard'],
    ['msp_technician', 'Technician Dashboard'],
    ['client_admin', 'Client Operations Dashboard'],
  ])('does not show setup to the prohibited %s role', async (role, dashboardTitle) => {
    const mspUser: LoginResponse = {
      ...providerUser,
      role,
      roles: [role],
      permissions: ['device:manage'],
      setup_complete: false,
    };
    const mspWorkspace: WorkspaceContext = {
      ...providerWorkspace,
      roles: [role],
      permissions: ['device:manage'],
      platform_role: false,
      msp_id: 'msp-1',
      setup_complete: false,
    };
    installApi(mspWorkspace, mspUser);
    renderAt('/provider/setup');

    expect(await screen.findByRole('heading', { name: dashboardTitle })).toBeVisible();
    expect(screen.queryByText(/First-time setup/)).not.toBeInTheDocument();
    expect(window.location.pathname).toBe('/');
  });

  it('skips setup on a later sign-in after completion', async () => {
    installApi(completeWorkspace, { ...providerUser, setup_complete: true });
    renderAt('/provider/setup');

    expect(await screen.findByRole('heading', { name: 'SaaS Provider Dashboard' })).toBeVisible();
    expect(screen.queryByText(/First-time setup/)).not.toBeInTheDocument();
  });
});

describe('provider setup wizard', () => {
  it('shows inline validation and an accessible error summary', async () => {
    installApi();
    const user = userEvent.setup();
    renderAt('/provider/setup');
    await screen.findByRole('heading', { name: /Set up your provider business profile/ });

    await user.click(screen.getByRole('button', { name: /Continue/ }));

    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('Please correct the highlighted fields');
    expect(alert).toHaveTextContent('Legal business name is required');
    expect(screen.getByLabelText(/Legal business name/)).toHaveAttribute('aria-invalid', 'true');
  });

  it('preserves entered values while moving between steps', async () => {
    installApi();
    const user = userEvent.setup();
    renderAt('/provider/setup');
    await screen.findByRole('heading', { name: /Set up your provider business profile/ });

    await fillBusiness(user);
    expect(screen.getByRole('heading', { name: 'Contact' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: /Back/ }));

    expect(screen.getByLabelText(/Legal business name/)).toHaveValue('Example Provider LLC');
    expect(screen.getByLabelText(/Display name/)).toHaveValue('Example Provider');
  });

  it('shows Review without submitting when Regional Defaults continues', async () => {
    installApi();
    const complete = vi.spyOn(api, 'completeProviderSetup').mockResolvedValue(providerProfile);
    const user = userEvent.setup();
    renderAt('/provider/setup');
    await screen.findByRole('heading', { name: /Set up your provider business profile/ });

    await advanceToReview(user);

    expect(screen.getByRole('heading', { name: 'Review' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Complete setup' })).toHaveAttribute('type', 'submit');
    expect(complete).not.toHaveBeenCalled();
  });

  it('retries a recoverable failure without losing data', async () => {
    installApi();
    const context = vi.mocked(api.getWorkspaceContext);
    context.mockResolvedValueOnce(providerWorkspace).mockResolvedValue({ ...completeWorkspace, provider_display_name: 'Example Provider' });
    const complete = vi.spyOn(api, 'completeProviderSetup')
      .mockRejectedValueOnce(new Error('Temporary setup service failure'))
      .mockResolvedValue({ ...providerProfile, display_name: 'Example Provider' });
    const user = userEvent.setup();
    renderAt('/provider/setup');
    await screen.findByRole('heading', { name: /Set up your provider business profile/ });
    await advanceToReview(user);

    await user.click(screen.getByRole('button', { name: 'Complete setup' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Temporary setup service failure');
    expect(screen.getByText('Example Provider LLC')).toBeVisible();

    await user.click(screen.getByRole('button', { name: 'Complete setup' }));
    expect(await screen.findByRole('heading', { name: 'SaaS Provider Dashboard' })).toBeVisible();
    expect(complete).toHaveBeenCalledTimes(2);
    expect(complete.mock.calls[1][0]).toEqual(complete.mock.calls[0][0]);
  });

  it('routes to the provider dashboard after a successful submission', async () => {
    installApi();
    vi.mocked(api.getWorkspaceContext)
      .mockResolvedValueOnce(providerWorkspace)
      .mockResolvedValue({ ...completeWorkspace, provider_display_name: 'Example Provider' });
    vi.spyOn(api, 'completeProviderSetup').mockResolvedValue({ ...providerProfile, display_name: 'Example Provider' });
    const user = userEvent.setup();
    renderAt('/provider/setup');
    await screen.findByRole('heading', { name: /Set up your provider business profile/ });
    await advanceToReview(user);

    await user.click(screen.getByRole('button', { name: 'Complete setup' }));

    expect(await screen.findByRole('heading', { name: 'SaaS Provider Dashboard' })).toBeVisible();
    expect(window.location.pathname).toBe('/');
  });

  it.each(['light', 'dark'] as const)('renders the setup surface in %s mode', async theme => {
    localStorage.setItem('theme', theme);
    installApi();
    renderAt('/provider/setup');

    expect(await screen.findByRole('heading', { name: /Set up your provider business profile/ })).toBeVisible();
    await waitFor(() => expect(document.documentElement.classList.contains('dark')).toBe(theme === 'dark'));
    expect(screen.getByRole('heading', { name: 'Business' }).closest('form')).toHaveClass('dark:bg-slate-900');
  });
});

describe('provider business settings', () => {
  it('loads server profile data and saves only changed fields', async () => {
    installApi(completeWorkspace, { ...providerUser, setup_complete: true });
    vi.spyOn(api, 'getProviderProfile').mockResolvedValue(providerProfile);
    const updatedProfile = { ...providerProfile, display_name: 'Updated Provider', updated_at: '2026-08-01T13:00:00Z' };
    const update = vi.spyOn(api, 'updateProviderProfile').mockResolvedValue(updatedProfile);
    vi.mocked(api.getWorkspaceContext)
      .mockResolvedValueOnce(completeWorkspace)
      .mockResolvedValue({ ...completeWorkspace, provider_display_name: 'Updated Provider' });
    const user = userEvent.setup();
    renderAt('/admin/settings');

    expect(await screen.findByLabelText(/Legal business name/)).toHaveValue('Original Provider LLC');
    const displayName = screen.getByLabelText(/Display name/);
    await user.clear(displayName);
    await user.type(displayName, 'Updated Provider');
    await user.click(screen.getByRole('button', { name: /Save business profile/ }));

    expect(await screen.findByRole('status')).toHaveTextContent('recorded in the provider audit trail');
    expect(update).toHaveBeenCalledWith({ display_name: 'Updated Provider' });
    expect(screen.getByRole('heading', { name: 'Updated Provider' })).toBeVisible();
  });
});
