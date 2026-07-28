import { useState, useEffect } from 'react';
import { api } from '@/api/client';
import type { MSPTenant } from '@/api/types';

export default function MSPListPage() {
  const [msps, setMsps] = useState<MSPTenant[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    api.getMSPs()
      .then(({ msps: tenants }) => setMsps(tenants || []))
      .catch((requestError: Error) => setError(requestError.message))
      .finally(() => setLoading(false));
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
              </tr>
            ))}
            {msps.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-12 text-center text-slate-500">
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
