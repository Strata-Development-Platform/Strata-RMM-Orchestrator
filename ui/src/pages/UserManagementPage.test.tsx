import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '@/api/client';
import { ToastProvider } from '@/components/shared/Toast';
import { ClientMembershipEditor } from '@/pages/UserManagementPage';
import type { TenantInfo, User } from '@/api/types';

const customers: TenantInfo[] = [
  { id: 'client-admin', name: 'Admin Client', slug: 'admin-client' },
  { id: 'client-viewer', name: 'Viewer Client', slug: 'viewer-client' },
  { id: 'client-new', name: 'New Client', slug: 'new-client' },
];

const scopedUser: User = {
  id: 'user-1',
  email: 'technician@example.test',
  role: 'msp_technician',
  legacy_role: 'technician',
  is_active: true,
  created_at: '2026-08-01T00:00:00Z',
  memberships: [
    { scope_type: 'msp', scope_id: 'msp-1', role: 'msp_technician' },
    { scope_type: 'client', scope_id: 'client-admin', role: 'client_admin' },
    { scope_type: 'client', scope_id: 'client-viewer', role: 'client_viewer' },
  ],
};

function renderEditor() {
  render(
    <ToastProvider>
      <ClientMembershipEditor user={scopedUser} customers={customers} onSave={vi.fn()} />
    </ToastProvider>
  );
}

afterEach(() => vi.restoreAllMocks());

describe('ClientMembershipEditor', () => {
  it('preserves mixed client roles and non-client memberships on an unchanged save', async () => {
    const update = vi.spyOn(api, 'updateUserMemberships').mockResolvedValue({ status: 'updated', memberships: [] });
    renderEditor();

    expect(screen.getByLabelText('Role for Admin Client')).toHaveValue('client_admin');
    expect(screen.getByLabelText('Role for Viewer Client')).toHaveValue('client_viewer');

    await userEvent.click(screen.getByRole('button', { name: 'Save memberships' }));

    expect(update).toHaveBeenCalledWith('user-1', expect.arrayContaining([
      { scope_type: 'msp', scope_id: 'msp-1', role: 'msp_technician' },
      { scope_type: 'client', scope_id: 'client-admin', role: 'client_admin' },
      { scope_type: 'client', scope_id: 'client-viewer', role: 'client_viewer' },
    ]));
    expect(update.mock.calls[0][1]).toHaveLength(3);
  });

  it('changes only the selected client role and defaults a newly enabled client to viewer', async () => {
    const update = vi.spyOn(api, 'updateUserMemberships').mockResolvedValue({ status: 'updated', memberships: [] });
    const user = userEvent.setup();
    renderEditor();

    await user.selectOptions(screen.getByLabelText('Role for Viewer Client'), 'client_admin');
    await user.click(screen.getByLabelText('Enable New Client membership'));
    await user.click(screen.getByRole('button', { name: 'Save memberships' }));

    expect(update).toHaveBeenCalledWith('user-1', expect.arrayContaining([
      { scope_type: 'client', scope_id: 'client-admin', role: 'client_admin' },
      { scope_type: 'client', scope_id: 'client-viewer', role: 'client_admin' },
      { scope_type: 'client', scope_id: 'client-new', role: 'client_viewer' },
    ]));
  });
});
