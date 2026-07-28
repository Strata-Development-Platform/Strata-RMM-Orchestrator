import { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { api } from '@/api/client';

type Tab = 'overview' | 'jobs' | 'alerts' | 'actions';

export default function DeviceWorkspacePage() {
  const { deviceID } = useParams<{ deviceID: string }>();
  const [device, setDevice] = useState<Record<string, unknown> | null>(null);
  const [inventory, setInventory] = useState<Record<string, unknown> | null>(null);
  const [jobs, setJobs] = useState<Record<string, unknown>[]>([]);
  const [tab, setTab] = useState<Tab>('overview');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!deviceID) return;
    Promise.all([
      fetch(`/api/v2/devices/${deviceID}`, { headers: { Authorization: `Bearer ${api.getToken()}` } }),
      fetch(`/api/v2/devices/${deviceID}/inventory`, { headers: { Authorization: `Bearer ${api.getToken()}` } }),
      fetch(`/api/v1/devices/${deviceID}/jobs`, { headers: { Authorization: `Bearer ${api.getToken()}` } }),
    ]).then(async ([d, i, j]) => {
      setDevice(await d.json());
      setInventory(await i.json());
      const jd = await j.json();
      setJobs(jd.jobs || []);
    }).catch(() => {}).finally(() => setLoading(false));
  }, [deviceID]);

  const runAction = async (action: string) => {
    if (!deviceID) return;
    const reason = ['reboot', 'shutdown', 'disable'].includes(action) ? prompt(`Reason for ${action}:`) : '';
    if (['reboot', 'shutdown', 'disable'].includes(action) && !reason) return;
    await fetch(`/api/v2/devices/${deviceID}/action`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${api.getToken()}` },
      body: JSON.stringify({ action, reason }),
    });
  };

  if (loading) return <div className="text-center py-12 text-slate-500">Loading device...</div>;
  if (!device) return <div className="text-center py-12 text-slate-400">Device not found</div>;

  const tabs: { key: Tab; label: string }[] = [
    { key: 'overview', label: 'Overview' },
    { key: 'jobs', label: `Jobs (${jobs.length})` },
    { key: 'alerts', label: 'Alerts' },
    { key: 'actions', label: 'Actions' },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{device.hostname as string || 'Device'}</h1>
          <p className="text-sm text-slate-500">{(device as any).os || ''} · {(device as any).arch || ''}</p>
        </div>
        <span className={`px-3 py-1 rounded-full text-sm font-medium ${
          (device as any).status === 'online' ? 'bg-green-100 text-green-800' :
          (device as any).status === 'offline' ? 'bg-red-100 text-red-800' :
          'bg-slate-100 text-slate-800'
        }`}>{(device as any).status || 'unknown'}</span>
      </div>

      <div className="border-b">
        <nav className="flex gap-4">
          {tabs.map(t => (
            <button key={t.key} onClick={() => setTab(t.key)}
              className={`pb-2 text-sm font-medium border-b-2 ${tab === t.key ? 'border-blue-600 text-blue-600' : 'border-transparent text-slate-500'}`}>
              {t.label}
            </button>
          ))}
        </nav>
      </div>

      {tab === 'overview' && inventory && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {[
            ['CPU Cores', (inventory as any).cpu_cores],
            ['Memory', (inventory as any).memory_mb ? `${((inventory as any).memory_mb / 1024).toFixed(1)} GB` : '-'],
            ['Disk', (inventory as any).disk_mb ? `${((inventory as any).disk_mb / 1024).toFixed(0)} GB` : '-'],
            ['Agent', (inventory as any).agent_version],
            ['Status', (inventory as any).status],
            ['Last Seen', (inventory as any).last_heartbeat ? new Date((inventory as any).last_heartbeat).toLocaleString() : '-'],
            ['Data Age', (inventory as any).data_age_seconds ? `${Math.round((inventory as any).data_age_seconds)}s ago` : '-'],
          ].map(([label, value]) => (
            <div key={label as string} className="bg-white dark:bg-slate-900 rounded-lg border p-4">
              <p className="text-xs text-slate-500">{label as string}</p>
              <p className="text-sm font-medium mt-1">{String(value ?? '-')}</p>
            </div>
          ))}
        </div>
      )}

      {tab === 'jobs' && (
        <div className="space-y-2">
          {jobs.length === 0 ? (
            <p className="text-slate-400 text-center py-8">No jobs for this device</p>
          ) : jobs.slice(0, 20).map((j: any, i: number) => (
            <div key={i} className="bg-white dark:bg-slate-900 rounded-lg border p-3 flex items-center justify-between">
              <div>
                <span className="text-sm font-medium">{j.type || j.job_type}</span>
                <span className="ml-2 text-xs text-slate-400">{new Date(j.created_at).toLocaleString()}</span>
              </div>
              <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                j.target_status === 'succeeded' ? 'bg-green-100 text-green-800' :
                j.target_status === 'failed' ? 'bg-red-100 text-red-800' :
                j.target_status === 'running' ? 'bg-amber-100 text-amber-800' :
                'bg-slate-100 text-slate-600'
              }`}>{j.target_status || j.job_status}</span>
            </div>
          ))}
        </div>
      )}

      {tab === 'alerts' && (
        <p className="text-slate-400 text-center py-8">Alerts view — coming soon</p>
      )}

      {tab === 'actions' && (
        <div className="space-y-3">
          {[
            { action: 'refresh', label: 'Refresh Inventory', color: 'bg-blue-600 hover:bg-blue-700' },
            { action: 'reboot', label: 'Reboot', color: 'bg-orange-600 hover:bg-orange-700' },
            { action: 'shutdown', label: 'Shutdown', color: 'bg-red-600 hover:bg-red-700' },
            { action: 'disable', label: 'Disable Device', color: 'bg-zinc-600 hover:bg-zinc-700' },
          ].map(a => (
            <button key={a.action} onClick={() => runAction(a.action)}
              className={`w-full px-4 py-3 ${a.color} text-white text-sm rounded-md transition-colors text-left`}>
              {a.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
