import { useState, useEffect } from 'react';
import { api } from '@/api/client';
import { useAuth } from '@/hooks/useAuth';

export default function MSPListPage() {
  const { user } = useAuth();
  const [msps, setMsps] = useState<Record<string, unknown>[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetch('/api/v2/platform/msps', {
      headers: { 'Authorization': `Bearer ${api.getToken()}` },
    }).then(r => r.json()).then(d => {
      setMsps(d.msps || []);
      setLoading(false);
    }).catch(() => setLoading(false));
  }, []);

  if (loading) return <div className="text-center py-12 text-slate-500">Loading...</div>;

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
              <tr key={m.id as string} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                <td className="px-4 py-3 font-medium">{m.name as string}</td>
                <td className="px-4 py-3 text-slate-500">{m.slug as string}</td>
                <td className="px-4 py-3 text-center capitalize">{(m.plan as string) || 'free'}</td>
                <td className="px-4 py-3 text-center">{m.client_count as number || 0}</td>
                <td className="px-4 py-3 text-center">{m.device_count as number || 0}</td>
                <td className="px-4 py-3 text-center">
                  <span className={`px-2 py-0.5 rounded text-xs font-medium ${m.is_active ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'}`}>
                    {m.is_active ? 'Active' : 'Suspended'}
                  </span>
                </td>
                <td className="px-4 py-3 text-right text-slate-500">{m.created_at ? new Date(m.created_at as string).toLocaleDateString() : '-'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
