import { useState, useEffect } from 'react';
import { useParams, Link } from 'react-router-dom';
import { api } from '@/api/client';
import { useAuth } from '@/hooks/useAuth';

type Job = Record<string, unknown>;
type Tab = 'overview' | 'jobs' | 'actions';

export default function DeviceWorkspacePage() {
  const { deviceID } = useParams<{ deviceID: string }>();
  const [device, setDevice] = useState<Record<string, unknown> | null>(null);
  const [inventory, setInventory] = useState<Record<string, unknown> | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [tab, setTab] = useState<Tab>('overview');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [actionMsg, setActionMsg] = useState('');
  const [showConfirm, setShowConfirm] = useState<string | null>(null);
  const [actionReason, setActionReason] = useState('');
  const [actionService, setActionService] = useState('');
  const [actionPid, setActionPid] = useState('');

  const authToken = api.getToken();
  const headers = { Authorization: `Bearer ${authToken}` };

  useEffect(() => {
    if (!deviceID) return;
    setLoading(true);
    setError('');
    Promise.all([
      fetch(`/api/v2/devices/${deviceID}`, { headers }),
      fetch(`/api/v2/devices/${deviceID}/inventory`, { headers }),
      fetch(`/api/v1/devices/${deviceID}/jobs`, { headers }),
    ]).then(async ([dRes, iRes, jRes]) => {
      if (!dRes.ok) { setError('Device not found'); return; }
      setDevice(await dRes.json());
      setInventory(await iRes.json());
      const jd = await jRes.json();
      setJobs(jd.jobs || []);
    }).catch(() => setError('Failed to load device')).finally(() => setLoading(false));
  }, [deviceID]);

  const confirmAndRun = (action: string) => {
    setShowConfirm(action);
    setActionReason('');
    setActionService('');
    setActionPid('');
  };

  const executeAction = async () => {
    if (!showConfirm || !deviceID) return;
    setActionMsg('');
    const destructive = ['reboot', 'shutdown'];
    if (destructive.includes(showConfirm) && !actionReason) { setActionMsg('Reason is required'); return; }
    const body: Record<string, unknown> = { action: showConfirm, reason: actionReason };
    if (showConfirm === 'service_start' || showConfirm === 'service_stop' || showConfirm === 'service_restart') {
      if (!actionService) { setActionMsg('Service name required'); return; }
      body.service = actionService;
    }
    if (showConfirm === 'process_kill') {
      const pid = parseInt(actionPid, 10);
      if (!pid || pid <= 0) { setActionMsg('Valid process ID required'); return; }
      body.process_id = pid;
    }
    try {
      const res = await fetch(`/api/v2/devices/${deviceID}/action`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...headers },
        body: JSON.stringify(body),
      });
      const data = await res.json();
      if (res.ok) {
        setActionMsg(`${showConfirm} queued — Job: ${data.job_id?.slice(0, 12)}...`);
      } else {
        setActionMsg(data.error || 'Action failed');
      }
    } catch { setActionMsg('Request failed'); }
    setShowConfirm(null);
  };

  if (loading) return <div className="text-center py-12 text-slate-500">Loading device...</div>;
  if (error) return <div className="text-center py-12 text-red-500">{error}</div>;
  if (!device) return <div className="text-center py-12 text-slate-400">Device not found</div>;

  const isOnline = (inventory as any)?.online;
  const statusClass = isOnline === true ? 'bg-green-100 text-green-800' :
    isOnline === false ? 'bg-red-100 text-red-800' : 'bg-slate-100 text-slate-800';

  const tabs: { key: Tab; label: string }[] = [
    { key: 'overview', label: 'Overview' },
    { key: 'jobs', label: `Jobs (${jobs.length})` },
    { key: 'actions', label: 'Actions' },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-white">
            {(device as any).hostname || 'Device'}
          </h1>
          <p className="text-sm text-slate-500">{(device as any).os || ''} · {(device as any).arch || ''}</p>
        </div>
        <span className={`px-3 py-1 rounded-full text-sm font-medium ${statusClass}`}>
          {(device as any).status || 'unknown'}
        </span>
      </div>

      <div className="border-b border-slate-200 dark:border-slate-700">
        <nav className="flex gap-4 overflow-x-auto">
          {tabs.map(t => (
            <button key={t.key} onClick={() => setTab(t.key)}
              className={`pb-2 text-sm font-medium border-b-2 whitespace-nowrap ${
                tab === t.key ? 'border-blue-600 text-blue-600' : 'border-transparent text-slate-500 hover:text-slate-700'
              }`}>{t.label}</button>
          ))}
        </nav>
      </div>

      {tab === 'overview' && inventory && (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
          {[
            ['CPU Cores', (inventory as any).cpu_cores],
            ['Memory', (inventory as any).memory_mb ? `${((inventory as any).memory_mb / 1024).toFixed(1)} GB` : '-'],
            ['Disk', (inventory as any).disk_mb ? `${((inventory as any).disk_mb / 1024).toFixed(0)} GB` : '-'],
            ['Agent', (inventory as any).agent_version],
            ['Status', (device as any).status],
            ['Last Seen', (inventory as any).last_heartbeat ? new Date((inventory as any).last_heartbeat).toLocaleString() : 'never'],
            ['Data Age', (inventory as any).data_age_seconds ? `${Math.round((inventory as any).data_age_seconds)}s` : '-'],
          ].map(([label, value]) => (
            <div key={label as string} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-4">
              <p className="text-xs text-slate-500 dark:text-slate-400">{label as string}</p>
              <p className="text-sm font-medium mt-1 text-slate-900 dark:text-white">{String(value ?? '-')}</p>
            </div>
          ))}
        </div>
      )}

      {tab === 'jobs' && (
        <div className="space-y-2">
          {jobs.length === 0 ? (
            <p className="text-slate-400 text-center py-8">No jobs for this device</p>
          ) : jobs.map((j: any, i: number) => (
            <div key={i} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-3 flex items-center justify-between">
              <div>
                <span className="text-sm font-medium text-slate-900 dark:text-white">{j.type || j.job_type}</span>
                <span className="ml-2 text-xs text-slate-400">{j.created_at ? new Date(j.created_at).toLocaleString() : ''}</span>
              </div>
              <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                j.target_status === 'succeeded' ? 'bg-green-100 text-green-800' :
                j.target_status === 'failed' ? 'bg-red-100 text-red-800' :
                j.target_status === 'cancelled' ? 'bg-zinc-100 text-zinc-600' :
                j.target_status === 'running' ? 'bg-amber-100 text-amber-800' :
                'bg-slate-100 text-slate-600'
              }`}>{j.target_status || j.job_status || 'queued'}</span>
            </div>
          ))}
        </div>
      )}

      {tab === 'actions' && (
        <div className="space-y-3">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {[
              { action: 'refresh', label: 'Refresh Inventory', cls: 'bg-blue-600 hover:bg-blue-700' },
              { action: 'service_restart', label: 'Restart Service', cls: 'bg-indigo-600 hover:bg-indigo-700' },
              { action: 'reboot', label: 'Reboot', cls: 'bg-orange-600 hover:bg-orange-700' },
              { action: 'shutdown', label: 'Shutdown', cls: 'bg-red-600 hover:bg-red-700' },
            ].map(a => (
              <button key={a.action} onClick={() => confirmAndRun(a.action)}
                className={`px-4 py-3 ${a.cls} text-white text-sm rounded-md transition-colors text-left font-medium`}>
                {a.label}
              </button>
            ))}
          </div>
          {actionMsg && (
            <div className={`text-sm p-3 rounded-md ${actionMsg.includes('queued') ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-600'}`}>
              {actionMsg}
            </div>
          )}
        </div>
      )}

      {/* Confirmation Modal */}
      {showConfirm && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4" onClick={() => setShowConfirm(null)}>
          <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-md w-full p-6 space-y-4" onClick={e => e.stopPropagation()}>
            <h3 className="text-lg font-bold text-slate-900 dark:text-white">Confirm {showConfirm}</h3>
            <p className="text-sm text-slate-500">Device: {(device as any).hostname}</p>
            {['reboot', 'shutdown'].includes(showConfirm) && (
              <div>
                <label className="block text-sm font-medium mb-1">Reason *</label>
                <input type="text" value={actionReason} onChange={e => setActionReason(e.target.value)}
                  className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm" required />
              </div>
            )}
            {['service_start', 'service_stop', 'service_restart'].includes(showConfirm) && (
              <div>
                <label className="block text-sm font-medium mb-1">Service Name *</label>
                <input type="text" value={actionService} onChange={e => setActionService(e.target.value)}
                  className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm font-mono" placeholder="sshd" />
              </div>
            )}
            {showConfirm === 'process_kill' && (
              <div>
                <label className="block text-sm font-medium mb-1">Process ID *</label>
                <input type="number" value={actionPid} onChange={e => setActionPid(e.target.value)}
                  className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm" min="1" />
              </div>
            )}
            <div className="flex justify-end gap-3 pt-2">
              <button onClick={() => setShowConfirm(null)} className="px-4 py-2 text-sm rounded-md border hover:bg-slate-50">Cancel</button>
              <button onClick={executeAction} className="px-4 py-2 text-sm rounded-md bg-blue-600 text-white hover:bg-blue-700">Execute</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
