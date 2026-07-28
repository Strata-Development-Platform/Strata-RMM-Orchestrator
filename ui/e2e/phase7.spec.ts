import { expect, test, type Page, type Route } from '@playwright/test';

const DEVICE_ID = 'device-1';
const user = {
  token: 'browser-test-token',
  user_id: 'user-1',
  email: 'owner@example.test',
  role: 'msp_owner',
  tenant_ids: ['msp-1'],
};

async function json(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
}

async function installApi(page: Page, actionHandler?: (route: Route) => Promise<void>) {
  await page.addInitScript(() => {
    localStorage.setItem('strata_auth_token', 'browser-test-token');
    localStorage.setItem('theme', 'dark');
  });

  await page.route(/\/api\/v\d+\//, async route => {
    const url = new URL(route.request().url());
    const path = url.pathname;

    if (path === '/api/v1/auth/me') return json(route, user);
    if (path === '/api/v2/context') {
      return json(route, {
        msp_id: 'msp-1',
        client_id: 'client-1',
        site_id: 'site-1',
        available_scopes: [],
        branding: { display_name: 'Test MSP' },
      });
    }
    if (path === `/api/v2/devices/${DEVICE_ID}`) {
      return json(route, {
        id: DEVICE_ID,
        hostname: 'workstation-01',
        os: 'windows',
        arch: 'amd64',
        agent_version: '7.0.0',
        status: 'online',
        client_id: 'client-1',
      });
    }
    if (path === `/api/v2/devices/${DEVICE_ID}/inventory`) {
      return json(route, {
        online: true,
        cpu_cores: 8,
        memory_mb: 16384,
        disk_mb: 512000,
        last_heartbeat: '2026-07-28T12:00:00Z',
        data_age_seconds: 5,
        services: [],
        processes: [],
      });
    }
    if (path === `/api/v2/devices/${DEVICE_ID}/capabilities`) {
      return json(route, {
        supported_job_types: ['device.refresh', 'device.reboot', 'device.shutdown'],
        agent_version: '7.0.0',
        os: 'windows',
        arch: 'amd64',
      });
    }
    if (path === `/api/v2/devices/${DEVICE_ID}/action` && route.request().method() === 'POST') {
      if (actionHandler) return actionHandler(route);
      return json(route, { job_id: 'job-1234567890abcdef' }, 202);
    }
    if (path === `/api/v1/devices/${DEVICE_ID}/jobs`) return json(route, { jobs: [] });
    if (path === '/api/v2/audit/endpoint') return json(route, { evidence: [] });
    if (path === '/api/v2/approvals') return json(route, { approvals: [] });

    return json(route, {});
  });
}

test('technician can inspect endpoint and receives approval guidance for destructive work', async ({ page }) => {
  await installApi(page, route =>
    json(route, { error: 'approval required for destructive operation' }, 409)
  );

  await page.goto(`/devices/${DEVICE_ID}`);

  await expect(page.getByRole('heading', { name: 'workstation-01' })).toBeVisible();
  await expect(page.getByText('3 capabilities')).toBeVisible();
  await expect(page.locator('html')).toHaveClass(/dark/);
  await expect(page.getByRole('status', { name: '' }).filter({ hasText: 'online' })).toBeVisible();

  await page.getByRole('tab', { name: 'Actions' }).click();
  await page.getByRole('button', { name: 'Reboot' }).click();
  await expect(page.getByRole('dialog', { name: 'Confirm reboot' })).toBeVisible();

  await page.getByRole('button', { name: 'Execute' }).click();
  await expect(page.getByText('Reason is required')).toBeVisible();

  await page.getByLabel('Reason *').fill('Emergency patch validation');
  await page.getByRole('button', { name: 'Execute' }).click();
  await expect(page.getByText(/Navigate to Approvals tab/)).toBeVisible();
});

test('submitting state prevents duplicate destructive action requests', async ({ page }) => {
  let requests = 0;
  await installApi(page, async route => {
    requests += 1;
    await new Promise(resolve => setTimeout(resolve, 300));
    await json(route, { job_id: 'job-1234567890abcdef' }, 202);
  });

  await page.goto(`/devices/${DEVICE_ID}`);
  await page.getByRole('tab', { name: 'Actions' }).click();
  await page.getByRole('button', { name: 'Reboot' }).click();
  await page.getByLabel('Reason *').fill('Approved maintenance');
  const execute = page.getByRole('button', { name: 'Execute' });
  await execute.dblclick({ delay: 20 });
  await expect(page.getByText(/reboot queued/)).toBeVisible();
  expect(requests).toBe(1);
});

test('unauthorized or hidden device fails closed without exposing workspace data', async ({ page }) => {
  await installApi(page);
  await page.route(`**/api/v2/devices/${DEVICE_ID}`, route =>
    json(route, { error: 'forbidden' }, 403)
  );

  await page.goto(`/devices/${DEVICE_ID}`);
  await expect(page.getByRole('alert')).toHaveText('Device not found');
  await expect(page.getByText('workstation-01')).toHaveCount(0);
});
