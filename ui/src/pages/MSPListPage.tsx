import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ApiError, api } from '@/api/client';
import type { CreateMSPWithOwnerRequest, MSPTenant } from '@/api/types';
import { useToast } from '@/components/shared/Toast';
import { useWorkspace } from '@/hooks/useWorkspace';

type CreateField = 'name' | 'slug' | 'owner_email';
type CreateErrors = Partial<Record<CreateField, string>>;

function tenantStatus(msp: MSPTenant) {
  if (msp.onboarding_status === 'pending_owner') {
    return {
      label: 'Pending owner activation',
      className: 'bg-amber-100 text-amber-900 dark:bg-amber-950/60 dark:text-amber-200',
    };
  }
  if (msp.is_active) {
    return {
      label: 'Active',
      className: 'bg-green-100 text-green-800 dark:bg-green-950/60 dark:text-green-200',
    };
  }
  return {
    label: 'Suspended',
    className: 'bg-red-100 text-red-800 dark:bg-red-950/60 dark:text-red-200',
  };
}

function invitationDeliveryLabel(status: string) {
  const labels: Record<string, string> = {
    delivered: 'Delivered',
    failed: 'Delivery failed',
    pending: 'Delivery pending',
    unconfigured: 'Email not configured',
  };
  return labels[status] || 'Not available';
}

function createErrorMessage(caught: unknown) {
  if (caught instanceof ApiError) {
    if (caught.status === 409 && caught.message === 'msp slug already exists') return 'That MSP slug is already in use.';
    if (caught.status === 409 && caught.message === 'owner email is already registered') return 'That owner email is already registered.';
    if (caught.status === 409) return 'A valid owner invitation has already been delivered.';
    if (caught.status === 400) return caught.message;
    if (caught.status === 403) return 'A top-level platform administrator session is required.';
  }
  return 'The MSP tenant could not be created. Please try again.';
}

function resendErrorMessage(caught: unknown) {
  if (caught instanceof ApiError) {
    if (caught.status === 409) return 'The owner invitation is still valid and has already been delivered.';
    if (caught.status === 404) return 'The pending owner invitation is no longer available.';
    if (caught.status === 403) return 'A top-level platform administrator session is required.';
  }
  return 'The owner invitation could not be resent. Please try again.';
}

export default function MSPListPage() {
  const { switchWorkspace } = useWorkspace();
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [msps, setMsps] = useState<MSPTenant[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [createErrors, setCreateErrors] = useState<CreateErrors>({});
  const [creationError, setCreationError] = useState('');
  const [creationSuccess, setCreationSuccess] = useState('');
  const [creating, setCreating] = useState(false);
  const [resendingMSPID, setResendingMSPID] = useState('');
  const createErrorRef = useRef<HTMLDivElement>(null);
  const hasCreateErrors = Object.values(createErrors).some(Boolean);

  const reload = () => api.getMSPs()
    .then(({ msps: tenants }) => {
      setMsps(tenants || []);
      setError('');
    })
    .catch((requestError: Error) => setError(requestError.message))
    .finally(() => setLoading(false));

  useEffect(() => {
    void reload();
  }, []);

  const focusCreateError = () => {
    window.requestAnimationFrame(() => createErrorRef.current?.focus());
  };

  const handleCreate = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (creating) return;

    const form = event.currentTarget;
    const data = new FormData(form);
    const request: CreateMSPWithOwnerRequest = {
      name: String(data.get('name')).trim(),
      slug: String(data.get('slug')).trim(),
      plan: String(data.get('plan')),
      owner_email: String(data.get('owner_email')).trim(),
    };
    setCreationSuccess('');
    const nextErrors: CreateErrors = {};
    if (!request.name) nextErrors.name = 'MSP name is required.';
    if (!request.slug) {
      nextErrors.slug = 'MSP slug is required.';
    } else if (!/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(request.slug)) {
      nextErrors.slug = 'Use 1–63 lowercase letters, numbers, or hyphens.';
    }
    if (!request.owner_email) {
      nextErrors.owner_email = 'Owner email is required.';
    } else {
      const ownerEmailInput = form.elements.namedItem('owner_email');
      if (ownerEmailInput instanceof HTMLInputElement && ownerEmailInput.validity.typeMismatch) {
        nextErrors.owner_email = 'Enter a valid owner email address.';
      }
    }

    setCreateErrors(nextErrors);
    setCreationError('');
    if (Object.keys(nextErrors).length > 0) {
      focusCreateError();
      return;
    }

    setCreating(true);
    try {
      const created = await api.createMSPWithOwner(request);
      form.reset();
      const delivery = invitationDeliveryLabel(created.delivery_status);
      setCreationSuccess(created.delivery_status === 'delivered' ? 'owner invitation delivered' : `invitation status: ${delivery}`);
      showToast('success', created.delivery_status === 'delivered'
        ? 'MSP tenant created and owner invitation delivered.'
        : `MSP tenant created. Invitation status: ${delivery}.`);
      await reload();
    } catch (caught) {
      setCreationError(createErrorMessage(caught));
      focusCreateError();
    } finally {
      setCreating(false);
    }
  };

  const handleResend = async (msp: MSPTenant) => {
    if (resendingMSPID) return;
    setResendingMSPID(msp.id);
    setCreationSuccess('');
    try {
      const result = await api.resendOwnerInvitation(msp.id);
      const delivery = invitationDeliveryLabel(result.delivery_status);
      setCreationSuccess(result.delivery_status === 'delivered'
        ? `Owner invitation for ${msp.name} was delivered.`
        : `Owner invitation for ${msp.name} was rotated. Invitation status: ${delivery}.`);
      showToast('success', result.delivery_status === 'delivered'
        ? `Owner invitation for ${msp.name} was delivered.`
        : `Owner invitation for ${msp.name} was rotated. Invitation status: ${delivery}.`);
      await reload();
    } catch (caught) {
      showToast('error', resendErrorMessage(caught));
    } finally {
      setResendingMSPID('');
    }
  };

  if (loading) return <div role="status" className="py-12 text-center text-slate-500">Loading MSP tenants...</div>;
  if (error) {
    return (
      <div role="alert" className="rounded-lg border border-red-200 bg-red-50 p-4 text-red-800 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">
        Unable to load MSP tenants: {error}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">MSP Tenants</h1>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">Create an MSP and invite its first verified owner to activate the workspace.</p>
      </div>

      <form noValidate className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900" onSubmit={handleCreate}>
        <h2 className="font-semibold text-slate-900 dark:text-white">Create MSP tenant</h2>
        {(creationError || hasCreateErrors) && (
          <div ref={createErrorRef} tabIndex={-1} role="alert" className="mt-3 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-900 outline-none focus:ring-2 focus:ring-red-500 dark:border-red-900 dark:bg-red-950/40 dark:text-red-100">
            <p className="font-medium">MSP tenant could not be created</p>
            {creationError && <p className="mt-1">{creationError}</p>}
            {hasCreateErrors && (
              <ul className="mt-1 list-disc pl-5">
                {createErrors.name && <li>{createErrors.name}</li>}
                {createErrors.slug && <li>{createErrors.slug}</li>}
                {createErrors.owner_email && <li>{createErrors.owner_email}</li>}
              </ul>
            )}
          </div>
        )}
        {creationSuccess && (
          <div role="status" className="mt-3 rounded-md border border-green-200 bg-green-50 p-3 text-sm text-green-900 dark:border-green-900 dark:bg-green-950/40 dark:text-green-100">
            <p>{creationSuccess}</p>
          </div>
        )}
        <div className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-5">
          <div>
            <label htmlFor="msp-name" className="block text-xs font-medium text-slate-600 dark:text-slate-300">MSP name</label>
            <input id="msp-name" required name="name" maxLength={200} aria-invalid={Boolean(createErrors.name)} onChange={() => setCreateErrors(current => ({ ...current, name: undefined }))} className="mt-1 w-full rounded border px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-800" />
            {createErrors.name && <p className="mt-1 text-xs text-red-700 dark:text-red-300">{createErrors.name}</p>}
          </div>
          <div>
            <label htmlFor="msp-slug" className="block text-xs font-medium text-slate-600 dark:text-slate-300">Slug</label>
            <input id="msp-slug" required name="slug" maxLength={63} placeholder="msp-slug" aria-invalid={Boolean(createErrors.slug)} onChange={() => setCreateErrors(current => ({ ...current, slug: undefined }))} className="mt-1 w-full rounded border px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-800" />
            {createErrors.slug && <p className="mt-1 text-xs text-red-700 dark:text-red-300">{createErrors.slug}</p>}
          </div>
          <div>
            <label htmlFor="msp-plan" className="block text-xs font-medium text-slate-600 dark:text-slate-300">Plan</label>
            <select id="msp-plan" name="plan" className="mt-1 w-full rounded border px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-800">
              <option value="free">Free</option>
              <option value="starter">Starter</option>
              <option value="professional">Professional</option>
            </select>
          </div>
          <div>
            <label htmlFor="msp-owner-email" className="block text-xs font-medium text-slate-600 dark:text-slate-300">Owner email</label>
            <input id="msp-owner-email" required name="owner_email" type="email" autoComplete="off" maxLength={320} aria-invalid={Boolean(createErrors.owner_email)} onChange={() => setCreateErrors(current => ({ ...current, owner_email: undefined }))} className="mt-1 w-full rounded border px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-800" />
            {createErrors.owner_email && <p className="mt-1 text-xs text-red-700 dark:text-red-300">{createErrors.owner_email}</p>}
          </div>
          <div className="flex items-end">
            <button type="submit" disabled={creating} className="w-full rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60 dark:focus:ring-offset-slate-900">
              {creating ? 'Creating MSP...' : 'Create MSP'}
            </button>
          </div>
        </div>
      </form>

      <div className="overflow-x-auto rounded-lg border bg-white dark:border-slate-700 dark:bg-slate-900">
        <table className="w-full min-w-[920px] text-sm">
          <thead className="bg-slate-50 dark:bg-slate-800">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-slate-500">Name</th>
              <th className="px-4 py-3 text-left font-medium text-slate-500">Slug</th>
              <th className="px-4 py-3 text-center font-medium text-slate-500">Plan</th>
              <th className="px-4 py-3 text-center font-medium text-slate-500">Clients</th>
              <th className="px-4 py-3 text-center font-medium text-slate-500">Devices</th>
              <th className="px-4 py-3 text-center font-medium text-slate-500">Status</th>
              <th className="px-4 py-3 text-center font-medium text-slate-500">Owner invitation</th>
              <th className="px-4 py-3 text-right font-medium text-slate-500">Created</th>
              <th className="px-4 py-3"><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-200 dark:divide-slate-800">
            {msps.map(msp => {
              const status = tenantStatus(msp);
              return (
                <tr key={msp.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                  <td className="px-4 py-3 font-medium">{msp.name}</td>
                  <td className="px-4 py-3 text-slate-500">{msp.slug}</td>
                  <td className="px-4 py-3 text-center capitalize">{msp.plan}</td>
                  <td className="px-4 py-3 text-center">{msp.client_count}</td>
                  <td className="px-4 py-3 text-center">{msp.device_count}</td>
                  <td className="px-4 py-3 text-center">
                    <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${status.className}`}>{status.label}</span>
                  </td>
                  <td className="px-4 py-3 text-center text-xs text-slate-500 dark:text-slate-400">{invitationDeliveryLabel(msp.owner_invitation_delivery_status)}</td>
                  <td className="px-4 py-3 text-right text-slate-500">{msp.created_at ? new Date(msp.created_at).toLocaleDateString() : '-'}</td>
                  <td className="px-4 py-3 text-right">
                    {msp.onboarding_status === 'pending_owner' ? (
                      <button
                        type="button"
                        disabled={Boolean(resendingMSPID)}
                        aria-label={`Resend invitation for ${msp.name}`}
                        onClick={() => void handleResend(msp)}
                        className="rounded bg-amber-50 px-3 py-1.5 text-xs font-medium text-amber-800 hover:bg-amber-100 focus:outline-none focus:ring-2 focus:ring-amber-500 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-amber-950/40 dark:text-amber-200"
                      >
                        {resendingMSPID === msp.id ? 'Resending...' : 'Resend invitation'}
                      </button>
                    ) : (
                      <button
                        type="button"
                        className="rounded bg-blue-50 px-3 py-1.5 text-xs font-medium text-blue-700 hover:bg-blue-100 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-blue-950/40 dark:text-blue-300"
                        onClick={() => void switchWorkspace(msp.id).then(() => navigate('/msp'))}
                      >
                        Open
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
            {msps.length === 0 && (
              <tr>
                <td colSpan={9} className="px-4 py-12 text-center text-slate-500">No MSP tenants have been created.</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
