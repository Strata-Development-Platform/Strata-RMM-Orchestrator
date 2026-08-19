import { expect, test, type Page, type Route } from '@playwright/test';

const AUTH_TOKEN = 'browser-test-token';
const UPDATED_AUTH_TOKEN = 'browser-relogin-token';

const platformOwner = {
  token: AUTH_TOKEN,
  user_id: 'platform-owner-1',
  email: 'owner@example.test',
  role: 'platform_owner',
  roles: ['platform_owner'],
  permissions: ['platform:manage'],
  tenant_id: '',
  provider_display_name: 'Strata RMM',
  setup_complete: false,
  accessible_tenants: [],
};

const setupValues = {
  legal_name: 'Northstar Managed Services LLC',
  display_name: 'Northstar Managed IT',
  contact_name: 'Ada Admin',
  support_email: 'support@northstar.example',
  billing_email: 'billing@northstar.example',
  business_phone: '+1 415 555 0199',
  website_url: 'https://northstar.example',
  address_line1: '100 Market Street',
  address_line2: 'Suite 400',
  city: 'San Francisco',
  state_province: 'CA',
  postal_code: '94105',
  country_code: 'US',
  default_timezone: 'America/Los_Angeles',
  default_locale: 'en-US',
  default_currency: 'USD',
  tax_identifier: 'US-12-3456789',
  logo_light_url: 'https://northstar.example/logo-light.svg',
  logo_dark_url: 'https://northstar.example/logo-dark.svg',
  favicon_url: 'https://northstar.example/favicon.svg',
  brand_light_color: '#2563EB',
  brand_dark_color: '#60A5FA',
  terms_url: 'https://northstar.example/terms',
  privacy_url: 'https://northstar.example/privacy',
  support_url: 'https://northstar.example/support',
  public_saas_enabled: true,
  public_saas_headline: 'Reliable endpoint operations',
  public_saas_description: 'Secure remote monitoring and management for modern service providers.',
};

type ProviderProfile = typeof setupValues & {
  id: string;
  slug: string;
  setup_complete: boolean;
  setup_completed_at: string;
  setup_completed_by: string;
  updated_at: string;
  setup_contract_version: number;
  outbound_email_status: 'configured' | 'not_configured';
};

type OwnerApiState = {
  setupComplete: boolean;
  providerDisplayName: string;
  profile: ProviderProfile | null;
  setupPayloads: unknown[];
  patchPayloads: unknown[];
  loginPayloads: unknown[];
};

async function json(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
}

function workspaceContext(state: OwnerApiState) {
  return {
    user_id: platformOwner.user_id,
    email: platformOwner.email,
    roles: ['platform_owner'],
    permissions: ['platform:manage'],
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
    provider_display_name: state.providerDisplayName,
    setup_complete: state.setupComplete,
    authenticated_at: '2026-08-01T00:00:00Z',
  };
}

async function installApi(page: Page, state: OwnerApiState) {
  await page.addInitScript(token => {
    localStorage.setItem('strata_auth_token', token);
    localStorage.setItem('theme', 'dark');
  }, AUTH_TOKEN);

  await page.route(/\/api\/v\d+\//, async route => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const method = route.request().method();

    if (path === '/api/v1/auth/me') {
      return json(route, {
        ...platformOwner,
        provider_display_name: state.providerDisplayName,
        setup_complete: state.setupComplete,
      });
    }
    if (path === '/api/v1/auth/login' && method === 'POST') {
      state.loginPayloads.push(route.request().postDataJSON());
      return json(route, {
        ...platformOwner,
        token: UPDATED_AUTH_TOKEN,
        provider_display_name: state.providerDisplayName,
        setup_complete: state.setupComplete,
      });
    }
    if (path === '/api/v2/context') return json(route, workspaceContext(state));
    if (path === '/api/v2/platform/provider/setup' && method === 'POST') {
      const payload = route.request().postDataJSON() as typeof setupValues;
      state.setupPayloads.push(payload);
      state.setupComplete = true;
      state.providerDisplayName = payload.display_name;
      state.profile = {
        ...payload,
        id: 'platform-1',
        slug: 'northstar-managed-it',
        setup_complete: true,
        setup_completed_at: '2026-08-01T12:00:00Z',
        setup_completed_by: platformOwner.user_id,
        updated_at: '2026-08-01T12:00:00Z',
        setup_contract_version: 2,
        outbound_email_status: 'configured',
      };
      return json(route, state.profile);
    }
    if (path === '/api/v2/platform/provider/profile' && method === 'GET') {
      return json(route, state.profile);
    }
    if (path === '/api/v2/platform/provider/profile' && method === 'PATCH') {
      const payload = route.request().postDataJSON() as Partial<typeof setupValues>;
      state.patchPayloads.push(payload);
      state.profile = {
        ...state.profile!,
        ...payload,
        updated_at: '2026-08-01T13:00:00Z',
      };
      state.providerDisplayName = state.profile.display_name;
      return json(route, state.profile);
    }
    if (path === '/api/v1/platform/overview') {
      return json(route, {
        total_devices: 0,
        online_devices: 0,
        offline_devices: 0,
        active_alerts: 0,
        critical_alerts: 0,
        open_cves: 0,
        total_customers: 0,
        timestamp: '2026-08-01T12:00:00Z',
      });
    }
    if (path === '/api/v1/platform/customers') return json(route, { customers: [] });

    return json(route, {});
  });
}

async function completeWizard(page: Page) {
  await page.getByLabel(/Legal business name/).fill(setupValues.legal_name);
  await page.getByLabel(/Display name/).fill(setupValues.display_name);
  await page.getByRole('button', { name: 'Continue' }).click();
  await expect(page.getByRole('heading', { name: 'Contact' })).toBeVisible();

  await page.getByLabel(/Primary contact name/).fill(setupValues.contact_name);
  await page.getByLabel(/Support email/).fill(setupValues.support_email);
  await page.getByLabel(/Billing email/).fill(setupValues.billing_email);
  await page.getByLabel(/Business phone/).fill(setupValues.business_phone);
  await page.getByRole('button', { name: 'Continue' }).click();
  await expect(page.getByRole('heading', { name: 'Regional Defaults' })).toBeVisible();

  await page.getByLabel(/Website URL/).fill(setupValues.website_url);
  await page.getByLabel(/Address line 1/).fill(setupValues.address_line1);
  await page.getByLabel(/Address line 2/).fill(setupValues.address_line2);
  await page.getByLabel(/^City/).fill(setupValues.city);
  await page.getByLabel(/State or province/).fill(setupValues.state_province);
  await page.getByLabel(/Postal code/).fill(setupValues.postal_code);
  await page.getByLabel(/Country/).selectOption(setupValues.country_code);
  await page.getByLabel(/Default timezone/).selectOption(setupValues.default_timezone);
  await page.getByLabel(/Default locale/).selectOption(setupValues.default_locale);
  await page.getByLabel(/Default currency/).selectOption(setupValues.default_currency);
  await page.getByLabel(/Tax identifier/).fill(setupValues.tax_identifier);
  await page.getByRole('button', { name: 'Continue' }).click();
  await expect(page.getByRole('heading', { name: 'Brand' })).toBeVisible();

  await page.getByLabel(/Light-mode logo URL/).fill(setupValues.logo_light_url);
  await page.getByLabel(/Dark-mode logo URL/).fill(setupValues.logo_dark_url);
  await page.getByLabel(/Favicon URL/).fill(setupValues.favicon_url);
  await page.getByRole('button', { name: 'Continue' }).click();
  await expect(page.getByRole('heading', { name: 'Publication' })).toBeVisible();

  await page.getByLabel(/Terms of service URL/).fill(setupValues.terms_url);
  await page.getByLabel(/Privacy policy URL/).fill(setupValues.privacy_url);
  await page.getByLabel(/Support URL/).fill(setupValues.support_url);
  await page.getByLabel(/Enable the provider-owned public SaaS site/).check();
  await page.getByLabel(/Public SaaS headline/).fill(setupValues.public_saas_headline);
  await page.getByLabel(/Public SaaS description/).fill(setupValues.public_saas_description);
  await page.getByRole('button', { name: 'Continue' }).click();
  await expect(page.getByRole('heading', { name: 'Review' })).toBeVisible();
}

test('platform owner completes first-login setup, returns without repetition, and persists a profile edit', async ({ page }) => {
  const state: OwnerApiState = {
    setupComplete: false,
    providerDisplayName: 'Strata RMM',
    profile: null,
    setupPayloads: [],
    patchPayloads: [],
    loginPayloads: [],
  };
  await installApi(page, state);

  await page.goto('/');

  await expect(page).toHaveURL(/\/provider\/setup$/);
  await expect(page.getByRole('heading', { name: 'Set up your provider business profile' })).toBeVisible();
  const progress = page.getByRole('list', { name: 'Setup progress' });
  await expect(progress.getByRole('listitem')).toHaveCount(6);
  await expect(progress).toContainText('Business');
  await expect(progress).toContainText('Contact');
  await expect(progress).toContainText('Regional Defaults');
  await expect(progress).toContainText('Brand');
  await expect(progress).toContainText('Publication');
  await expect(progress).toContainText('Review');
  await expect(progress.locator('[aria-current="step"]')).toContainText('Business');

  await completeWizard(page);
  await expect(progress.locator('[aria-current="step"]')).toContainText('Review');
  expect(state.setupPayloads).toHaveLength(0);
  await page.getByRole('button', { name: 'Complete setup' }).click();

  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole('heading', { name: 'SaaS Provider Dashboard' })).toBeVisible();
  expect(state.setupPayloads).toEqual([setupValues]);
  await expect(page.locator('aside').getByRole('heading', { name: setupValues.display_name })).toBeVisible();

  await page.getByRole('button', { name: 'Sign Out' }).click();
  await expect(page).toHaveURL(/\/login$/);
  await page.getByLabel('Email').fill(platformOwner.email);
  await page.getByLabel('Password').fill('correct horse battery staple');
  await page.getByRole('button', { name: 'Sign In' }).click();

  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole('heading', { name: 'SaaS Provider Dashboard' })).toBeVisible();
  await expect(page.getByText('First-time setup')).toHaveCount(0);
  expect(state.loginPayloads).toEqual([{
    email: platformOwner.email,
    password: 'correct horse battery staple',
  }]);

  await page.getByRole('link', { name: 'Settings', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Platform Settings' })).toBeVisible();
  const displayName = page.getByLabel(/Display name/);
  await expect(displayName).toHaveValue(setupValues.display_name);
  await displayName.fill('Northstar Technology Group');
  await page.getByRole('button', { name: 'Save business profile' }).click();

  await expect(page.getByRole('status')).toContainText('Business profile saved');
  expect(state.patchPayloads).toEqual([{ display_name: 'Northstar Technology Group' }]);
  await expect(page.locator('aside').getByRole('heading', { name: 'Northstar Technology Group' })).toBeVisible();

  await page.reload();
  await expect(page.getByRole('heading', { name: 'Platform Settings' })).toBeVisible();
  await expect(page.getByLabel(/Display name/)).toHaveValue('Northstar Technology Group');
  await expect(page.getByText('First-time setup')).toHaveCount(0);
});

test('non-platform role stays out of setup and receives forbidden provider API responses', async ({ page }) => {
  const mspOwner = {
    token: AUTH_TOKEN,
    user_id: 'msp-owner-1',
    email: 'msp-owner@example.test',
    role: 'msp_owner',
    roles: ['msp_owner'],
    permissions: ['msp:manage'],
    tenant_id: 'msp-1',
    provider_display_name: 'Northstar Technology Group',
    setup_complete: false,
    accessible_tenants: [],
  };
  const forbiddenRequests: string[] = [];

  await page.addInitScript(() => {
    localStorage.setItem('strata_auth_token', 'browser-test-token');
    localStorage.setItem('theme', 'dark');
  });
  await page.route(/\/api\/v\d+\//, async route => {
    const url = new URL(route.request().url());
    const path = url.pathname;

    if (path === '/api/v1/auth/me') return json(route, mspOwner);
    if (path === '/api/v2/context') {
      return json(route, {
        user_id: mspOwner.user_id,
        email: mspOwner.email,
        roles: ['msp_owner'],
        permissions: ['msp:manage'],
        available_scopes: [],
        msp_id: 'msp-1',
        msp_name: 'Example MSP',
        msp_active: true,
        client_id: '',
        client_name: '',
        site_id: '',
        site_name: '',
        branding: { display_name: 'Example MSP' },
        platform_role: false,
        platform_id: 'platform-1',
        provider_display_name: 'Northstar Technology Group',
        setup_complete: false,
        authenticated_at: '2026-08-01T00:00:00Z',
      });
    }
    if (path === '/api/v1/platform/overview') {
      return json(route, {
        total_devices: 0,
        online_devices: 0,
        offline_devices: 0,
        active_alerts: 0,
        critical_alerts: 0,
        open_cves: 0,
        total_customers: 0,
        timestamp: '2026-08-01T12:00:00Z',
      });
    }
    if (path === '/api/v1/platform/customers') return json(route, { customers: [] });
    if (path === '/api/v2/platform/provider/setup' || path === '/api/v2/platform/provider/profile') {
      forbiddenRequests.push(`${route.request().method()} ${path}`);
      return json(route, { error: 'forbidden' }, 403);
    }

    return json(route, {});
  });

  await page.goto('/provider/setup');

  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole('heading', { name: 'MSP Management Dashboard' })).toBeVisible();
  await expect(page.getByText('First-time setup')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Set up your provider business profile' })).toHaveCount(0);

  const responses = await page.evaluate(async () => {
    const request = async (path: string, method: string) => {
      const response = await fetch(path, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: method === 'GET' ? undefined : '{}',
      });
      return { status: response.status, body: await response.json() };
    };
    return Promise.all([
      request('/api/v2/platform/provider/setup', 'POST'),
      request('/api/v2/platform/provider/profile', 'GET'),
      request('/api/v2/platform/provider/profile', 'PATCH'),
    ]);
  });

  expect(responses).toEqual([
    { status: 403, body: { error: 'forbidden' } },
    { status: 403, body: { error: 'forbidden' } },
    { status: 403, body: { error: 'forbidden' } },
  ]);
  expect(forbiddenRequests.sort()).toEqual([
    'GET /api/v2/platform/provider/profile',
    'PATCH /api/v2/platform/provider/profile',
    'POST /api/v2/platform/provider/setup',
  ]);
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByText('First-time setup')).toHaveCount(0);
});
