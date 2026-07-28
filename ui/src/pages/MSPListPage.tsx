import { useState, useEffect } from 'react';
import { api } from '@/api/client';
import type { MSPTenant } from '@/api/types';
import { useWorkspace } from '@/hooks/useWorkspace';
import { useNavigate } from 'react-router-dom';
import { useToast } from '@/components/shared/Toast';

export default function MSPListPage() {
  const { switchWorkspace } = useWorkspace();
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [msps, setMsps] = useState<MSPTenant[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const reload = () => api.getMSPs()
    .then(({ msps: tenants }) => setMsps(tenants || []))
    .catch((requestError: Error) => setError(requestError.message))
    .finally(() => setLoading(false));

  useEffect(() => {
    reload();
  }, []);

  if (loading) return <div className="text-center py-12 text-slate-500">Loading...</div>;
  if (error) {
    return (
      <div role="alert" className="rounded-lg border border-red-200 bg-red-50 p-4 text-red-800 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">
        Unable to load MSP tenants: {error}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-slate-900 dark:text-white">MSP Tenants</h1>
      <form
        className="flex flex-wrap gap-3 rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900"
        onSubmit={event => {
          event.preventDefault();
          const form = event.currentTarget;
          const data = new FormData(form);
          api.createMSP(String(data.get('name')), String(data.get('slug')), String(data.get('plan')))
            .then(() => {
              form.reset();
              showToast('success', 'MSP tenant created');
              reload();
            })
            .catch((requestError: Error) => showToast('error', requestError.message));
        }}
      >
        <input required name="name" placeholder="MSP name" className="rounded border px-3 py-2 text-sm dark:border-slate-600 dark:bg-slate-800" />
        <input required name="slug" placeholder="msp-slug" pattern="[a-z0-9-]+" className="rounded border px-3 py-2 text-sm dark:border-slate-600 dark:bg-slate-800" />
        <select name="plan" className="rounded border px-3 py-2 text-sm dark:border-slate-600 dark:bg-slate-800">
          <option value="free">Free</option>
          <option value="starter">Starter</option>
          <option value="professional">Professional</option>
        </select>
        <button className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700">Create MSP</button>
      </form>
      <div className="bg-white dark:bg-slate-900 rounded-lg border overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 dark:bg-slate-800">
            <tr>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Name</th>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Slug</th>
              <th className="text-center px-4 py-3 font-medium text-slate-500">Plan</th>
              <th className="text-center px-4 py-3 font-medium text-slate-500">Clients</th>
              <th className="text-center px-4 py-3 font-medium text-slate-500">Devices</th>
              <th className="text-center px-4 py-3 font-medium text-slate-500">Status</th>
              <th className="text-right px-4 py-3 font-medium text-slate-500">Created</th>
              <th className="px-4 py-3"><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {msps.map(m => (
              <tr key={m.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                <td className="px-4 py-3 font-medium">{m.name}</td>
                <td className="px-4 py-3 text-slate-500">{m.slug}</td>
                <td className="px-4 py-3 text-center capitalize">{m.plan || 'free'}</td>
                <td className="px-4 py-3 text-center">{m.client_count || 0}</td>
                <td className="px-4 py-3 text-center">{m.device_count || 0}</td>
                <td className="px-4 py-3 text-center">
                  <span className={`px-2 py-0.5 rounded text-xs font-medium ${m.is_active ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'}`}>
                    {m.is_active ? 'Active' : 'Suspended'}
                  </span>
                </td>
                <td className="px-4 py-3 text-right text-slate-500">{m.created_at ? new Date(m.created_at).toLocaleDateString() : '-'}</td>
                <td className="px-4 py-3 text-right">
                  <button
                    className="rounded bg-blue-50 px-3 py-1.5 text-xs font-medium text-blue-700 hover:bg-blue-100 dark:bg-blue-950/40 dark:text-blue-300"
                    onClick={() => void switchWorkspace(m.id).then(() => navigate('/msp'))}
                  >
                    Open
                  </button>
                </td>
              </tr>
            ))}
            {msps.length === 0 && (
              <tr>
                <td colSpan={8} className="px-4 py-12 text-center text-slate-500">
                  No MSP tenants have been created.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
