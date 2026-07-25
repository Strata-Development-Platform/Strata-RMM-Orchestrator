import { useState, useEffect } from 'react';
import { useAuth } from '@/hooks/useAuth';

export default function ThirdPartyPage() {
  const { user } = useAuth();
  const tenantID = user?.accessible_tenants?.[0]?.id || '';
  const [apps, setApps] = useState<Record<string, unknown>[]>([]);
  const [packages, setPackages] = useState<Record<string, unknown>[]>([]);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    try {
      const [a, p] = await Promise.all([
        fetch('/api/v1/thirdparty/apps').then(r => r.json()),
        fetch('/api/v1/thirdparty/packages').then(r => r.json()),
      ]);
      setApps(a.apps || []);
      setPackages(p.packages || []);
    } catch (err) { console.error(err); }
    setLoading(false);
  };

  useEffect(() => { load(); }, []);

  const handleSyncAll = async () => {
    setSyncing('all');
    try {
      await fetch('/api/v1/thirdparty/sync', { method: 'POST' });
      setTimeout(load, 2000);
    } catch { alert('Sync failed'); }
    setSyncing(null);
  };

  const handleSyncApp = async (name: string) => {
    setSyncing(name);
    try {
      await fetch(`/api/v1/thirdparty/sync/${encodeURIComponent(name)}`, { method: 'POST' });
      setTimeout(load, 2000);
    } catch { alert('Sync failed'); }
    setSyncing(null);
  };

  if (loading) return <div className="text-center py-12 text-slate-500">Loading...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">Third-Party Patching</h1>
        <button onClick={handleSyncAll} disabled={syncing === 'all'}
          className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700 disabled:opacity-50">
          {syncing === 'all' ? 'Syncing...' : 'Sync All'}
        </button>
      </div>

      {packages.length > 0 && (
        <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
          <div className="px-4 py-3 border-b border-slate-200 dark:border-slate-700 font-medium text-sm text-slate-500">
            Recently Created Packages
          </div>
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-800">
              <tr>
                <th className="text-left px-4 py-2 font-medium text-slate-500">Name</th>
                <th className="text-left px-4 py-2 font-medium text-slate-500">Version</th>
                <th className="text-center px-4 py-2 font-medium text-slate-500">Platform</th>
                <th className="text-center px-4 py-2 font-medium text-slate-500">Type</th>
                <th className="text-right px-4 py-2 font-medium text-slate-500">Created</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
              {packages.slice(0, 20).map((p: Record<string, unknown>) => (
                <tr key={p.id as string}>
                  <td className="px-4 py-2 font-medium">{p.name as string}</td>
                  <td className="px-4 py-2">v{p.version as string}</td>
                  <td className="px-4 py-2 text-center capitalize">{p.platform as string}</td>
                  <td className="px-4 py-2 text-center">
                    <span className="px-1.5 py-0.5 rounded text-xs font-mono bg-slate-100 dark:bg-slate-700">
                      {(p.package_type as string)?.toUpperCase()}
                    </span>
                  </td>
                  <td className="px-4 py-2 text-right text-slate-500">
                    {p.created_at ? new Date(p.created_at as string).toLocaleDateString() : '-'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {apps.map((app: Record<string, unknown>) => (
          <div key={app.Name as string} className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
            <div className="flex items-center justify-between mb-2">
              <div>
                <h3 className="font-medium">{app.Name as string}</h3>
                <p className="text-xs text-slate-500">{app.Vendor as string}</p>
              </div>
              <span className="px-2 py-0.5 rounded text-xs font-mono bg-slate-100 dark:bg-slate-700">
                {(app.PackageType as string)?.toUpperCase()}
              </span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-slate-500 capitalize">{app.Platform as string}</span>
              <button
                onClick={() => handleSyncApp(app.Name as string)}
                disabled={syncing === app.Name}
                className="px-3 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {syncing === app.Name ? '...' : 'Check for Update'}
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
