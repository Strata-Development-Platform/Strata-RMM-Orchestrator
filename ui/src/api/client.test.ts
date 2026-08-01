import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '@/api/client';

beforeEach(() => {
  localStorage.clear();
  api.setToken(null);
});

afterEach(() => {
  vi.unstubAllGlobals();
  api.setToken(null);
});

describe('owner invitation API contracts', () => {
  it('sends invitation tokens only in public POST bodies and accepts a 204 response', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        msp: { name: 'Example MSP' },
        masked_email: 'o***@e***.test',
        expires_at: '2999-01-01T00:00:00Z',
      }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await api.inspectInvitation('hash-token');
    await expect(api.acceptInvitation('hash-token', 'a secure owner password')).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/auth/invitations/inspect', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ token: 'hash-token' }),
      headers: { 'Content-Type': 'application/json' },
    }));
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/auth/invitations/accept', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ token: 'hash-token', password: 'a secure owner password' }),
      headers: { 'Content-Type': 'application/json' },
    }));
  });

  it('posts the owner email on create and no request body on resend', async () => {
    api.setToken('platform-session');
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        id: 'msp-1', status: 'pending_owner', delivery_status: 'delivered',
      }), { status: 201, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        status: 'invitation_rotated', delivery_status: 'delivered',
      }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);

    await api.createMSPWithOwner({
      name: 'Example MSP', slug: 'example-msp', plan: 'starter', owner_email: 'owner@example.test',
    });
    await api.resendOwnerInvitation('msp-1');

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v2/platform/msps', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        name: 'Example MSP', slug: 'example-msp', plan: 'starter', owner_email: 'owner@example.test',
      }),
    }));
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v2/platform/msps/msp-1/owner-invitation', expect.objectContaining({
      method: 'POST',
      body: undefined,
    }));
  });
});
