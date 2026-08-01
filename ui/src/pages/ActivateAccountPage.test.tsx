import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from '@/App';
import { ApiError, api } from '@/api/client';
import ActivateAccountPage from '@/pages/ActivateAccountPage';

const validInvitation = {
  msp: { name: 'Northstar Managed IT' },
  masked_email: 'o***@e***.test',
  expires_at: '2999-08-04T12:00:00Z',
};

function renderAt(hash = '#invitation-token') {
  window.history.replaceState({}, '', `/activate-account${hash}`);
  return render(
    <BrowserRouter>
      <ActivateAccountPage />
    </BrowserRouter>,
  );
}

beforeEach(() => {
  localStorage.clear();
  api.setToken(null);
});

afterEach(() => {
  vi.restoreAllMocks();
  api.setToken(null);
  window.history.replaceState({}, '', '/');
});

describe('MSP owner account activation', () => {
  it('is publicly routed without loading the authenticated session', async () => {
    const inspect = vi.spyOn(api, 'inspectInvitation').mockResolvedValue(validInvitation);
    const me = vi.spyOn(api, 'me');
    window.history.replaceState({}, '', '/activate-account#public-invitation');

    render(<App />);

    expect(await screen.findByRole('heading', { name: 'Northstar Managed IT' })).toBeVisible();
    expect(inspect).toHaveBeenCalledWith('public-invitation');
    expect(me).not.toHaveBeenCalled();
  });

  it('inspects the hash token and renders a valid invitation', async () => {
    const inspect = vi.spyOn(api, 'inspectInvitation').mockResolvedValue(validInvitation);

    renderAt();

    expect(await screen.findByRole('heading', { name: 'Northstar Managed IT' })).toBeVisible();
    expect(screen.getByText('o***@e***.test')).toBeVisible();
    expect(inspect).toHaveBeenCalledWith('invitation-token');
    expect(window.location.search).toBe('');
  });

  it('rejects an activation URL without a hash token', async () => {
    const inspect = vi.spyOn(api, 'inspectInvitation');

    renderAt('');

    expect(await screen.findByRole('alert')).toHaveTextContent('does not include a valid invitation');
    expect(inspect).not.toHaveBeenCalled();
  });

  it('shows a safe message for an invalid or unavailable invitation', async () => {
    vi.spyOn(api, 'inspectInvitation').mockRejectedValue(new ApiError(404, 'invitation is invalid or unavailable'));

    renderAt();

    expect(await screen.findByRole('alert')).toHaveTextContent('invalid, expired, or no longer available');
  });

  it('does not render the password form for an expired invitation', async () => {
    vi.spyOn(api, 'inspectInvitation').mockResolvedValue({
      ...validInvitation,
      expires_at: '2000-01-01T00:00:00Z',
    });

    renderAt();

    expect(await screen.findByRole('alert')).toHaveTextContent('invitation has expired');
    expect(screen.queryByLabelText('New password')).not.toBeInTheDocument();
  });

  it('validates password byte length and confirmation before accepting', async () => {
    vi.spyOn(api, 'inspectInvitation').mockResolvedValue(validInvitation);
    const accept = vi.spyOn(api, 'acceptInvitation');
    const user = userEvent.setup();
    renderAt();
    await screen.findByLabelText('New password');

    await user.type(screen.getByLabelText('New password'), 'too-short');
    await user.type(screen.getByLabelText('Confirm new password'), 'different');
    await user.click(screen.getByRole('button', { name: 'Activate account' }));

    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('Password must be between 14 and 72 bytes.');
    expect(alert).toHaveTextContent('Passwords do not match.');
    expect(screen.getByLabelText('New password')).toHaveAttribute('aria-invalid', 'true');
    expect(accept).not.toHaveBeenCalled();
  });

  it('accepts the invitation, clears the hash, and never signs the owner in automatically', async () => {
    vi.spyOn(api, 'inspectInvitation').mockResolvedValue(validInvitation);
    const accept = vi.spyOn(api, 'acceptInvitation').mockResolvedValue(undefined);
    const user = userEvent.setup();
    renderAt();
    await screen.findByLabelText('New password');

    await user.type(screen.getByLabelText('New password'), 'a secure owner password');
    await user.type(screen.getByLabelText('Confirm new password'), 'a secure owner password');
    await user.click(screen.getByRole('button', { name: 'Activate account' }));

    expect(await screen.findByRole('heading', { name: 'Account activated' })).toBeVisible();
    expect(screen.getByText(/You can now sign in/)).toBeVisible();
    expect(screen.getByRole('link', { name: 'Go to sign in' })).toHaveAttribute('href', '/login');
    expect(accept).toHaveBeenCalledWith('invitation-token', 'a secure owner password');
    expect(api.getToken()).toBeNull();
    expect(localStorage.getItem('strata_auth_token')).toBeNull();
    await waitFor(() => expect(window.location.hash).toBe(''));
  });
});
