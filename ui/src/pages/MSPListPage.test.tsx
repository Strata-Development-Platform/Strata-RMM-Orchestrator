import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, api } from '@/api/client';
import type { MSPTenant } from '@/api/types';
import { ToastProvider } from '@/components/shared/Toast';
import MSPListPage from '@/pages/MSPListPage';

vi.mock('@/hooks/useWorkspace', () => ({
  useWorkspace: () => ({ switchWorkspace: vi.fn() }),
}));

const pendingMSP: MSPTenant = {
  id: 'msp-pending',
  name: 'Pending MSP',
  slug: 'pending-msp',
  plan: 'starter',
  client_count: 0,
  device_count: 0,
  is_active: false,
  onboarding_status: 'pending_owner',
  owner_invitation_delivery_status: 'failed',
  created_at: '2026-08-01T00:00:00Z',
};

const activeMSP: MSPTenant = {
  ...pendingMSP,
  id: 'msp-active',
  name: 'Active MSP',
  slug: 'active-msp',
  is_active: true,
  onboarding_status: 'active',
  owner_invitation_delivery_status: 'delivered',
};

const suspendedMSP: MSPTenant = {
  ...activeMSP,
  id: 'msp-suspended',
  name: 'Suspended MSP',
  slug: 'suspended-msp',
  is_active: false,
};

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <MSPListPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.spyOn(api, 'getMSPs').mockResolvedValue({ msps: [] });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('platform MSP tenants', () => {
  it('renders pending owner activation, active, and suspended as distinct states', async () => {
    vi.mocked(api.getMSPs).mockResolvedValue({ msps: [pendingMSP, activeMSP, suspendedMSP] });

    renderPage();

    expect(await screen.findByText('Pending owner activation')).toBeVisible();
    expect(screen.getByText('Active')).toBeVisible();
    expect(screen.getByText('Suspended')).toBeVisible();
    expect(screen.getByText('Delivery failed')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Resend invitation for Pending MSP' })).toBeVisible();
  });

  it('requires an owner email and sends it in the create request without a password', async () => {
    const create = vi.spyOn(api, 'createMSPWithOwner').mockResolvedValue({
      id: 'msp-new',
      status: 'pending_owner',
      delivery_status: 'delivered',
    });
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole('heading', { name: 'Create MSP tenant' });

    await user.type(screen.getByLabelText('MSP name'), 'New MSP');
    await user.type(screen.getByLabelText('Slug'), 'new-msp');
    await user.click(screen.getByRole('button', { name: 'Create MSP' }));

    expect(screen.getByRole('alert')).toHaveTextContent('Owner email is required.');
    expect(create).not.toHaveBeenCalled();
    expect(screen.queryByLabelText(/password/i)).not.toBeInTheDocument();

    await user.type(screen.getByLabelText('Owner email'), 'owner@example.test');
    await user.click(screen.getByRole('button', { name: 'Create MSP' }));

    expect(await screen.findByRole('status')).toHaveTextContent('owner invitation delivered');
    expect(create).toHaveBeenCalledWith({
      name: 'New MSP',
      slug: 'new-msp',
      plan: 'free',
      owner_email: 'owner@example.test',
    });
  });

  it('resends a pending owner invitation and reports delivery success', async () => {
    vi.mocked(api.getMSPs).mockResolvedValue({ msps: [pendingMSP] });
    const resend = vi.spyOn(api, 'resendOwnerInvitation').mockResolvedValue({
      status: 'invitation_rotated',
      delivery_status: 'delivered',
    });
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Resend invitation for Pending MSP' }));

    expect(await screen.findByRole('status')).toHaveTextContent('Owner invitation for Pending MSP was delivered.');
    expect(resend).toHaveBeenCalledWith('msp-pending');
  });

  it('explains that a 409 invitation is still valid and already delivered', async () => {
    vi.mocked(api.getMSPs).mockResolvedValue({ msps: [pendingMSP] });
    vi.spyOn(api, 'resendOwnerInvitation').mockRejectedValue(new ApiError(409, 'owner invitation is still valid and delivered'));
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Resend invitation for Pending MSP' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('still valid and has already been delivered');
  });
});
