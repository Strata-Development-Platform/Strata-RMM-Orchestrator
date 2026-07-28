import { useState, useEffect, useCallback } from 'react';
import { useParams } from 'react-router-dom';
import { api } from '@/api/client';
import { useAuth } from '@/hooks/useAuth';

type Job = Record<string, unknown>;
type Tab = 'overview' | 'inventory' | 'services' | 'processes' | 'jobs' | 'history' | 'audit' | 'approvals' | 'actions';

interface Capability {
  supported_job_types: string[];
  agent_version: string;
  os: string;
  arch: string;
}

const actionDefs: { action: string; label: string; cls: string; destructive: boolean; requiresReason: boolean; requiresService: boolean; requiresPid: boolean; supportedOffline: boolean }[] = [
  { action: 'refresh', label: 'Refresh Inventory', cls: 'bg-blue-600 hover:bg-blue-700', destructive: false, requiresReason: false, requiresService: false, requiresPid: false, supportedOffline: true },
  { action: 'service_restart', label: 'Restart Service', cls: 'bg-indigo-600 hover:bg-indigo-700', destructive: false, requiresReason: false, requiresService: true, requiresPid: false, supportedOffline: false },
  { action: 'service_start', label: 'Start Service', cls: 'bg-teal-600 hover:bg-teal-700', destructive: false, requiresReason: false, requiresService: true, requiresPid: false, supportedOffline: false },
  { action: 'service_stop', label: 'Stop Service', cls: 'bg-amber-600 hover:bg-amber-700', destructive: false, requiresReason: false, requiresService: true, requiresPid: false, supportedOffline: false },
  { action: 'process_kill', label: 'Kill Process', cls: 'bg-orange-600 hover:bg-orange-700', destructive: true, requiresReason: true, requiresService: false, requiresPid: true, supportedOffline: false },
  { action: 'reboot', label: 'Reboot', cls: 'bg-red-600 hover:bg-red-700', destructive: true, requiresReason: true, requiresService: false, requiresPid: false, supportedOffline: true },
  { action: 'shutdown', label: 'Shutdown', cls: 'bg-rose-700 hover:bg-rose-800', destructive: true, requiresReason: true, requiresService: false, requiresPid: false, supportedOffline: true },
];

export default function DeviceWorkspacePage() {
  const { deviceID } = useParams<{ deviceID: string }>();
  const { user } = useAuth();
  const [device, setDevice] = useState<Record<string, unknown> | null>(null);
  const [inventory, setInventory] = useState<Record<string, unknown> | null>(null);
  const [capabilities, setCapabilities] = useState<Capability | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [auditEntries, setAuditEntries] = useState<Record<string, unknown>[]>([]);
  const [pendingApprovals, setPendingApprovals] = useState<Record<string, unknown>[]>([]);
  const [tab, setTab] = useState<Tab>('overview');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [actionMsg, setActionMsg] = useState('');
  const [actionError, setActionError] = useState('');
  const [showConfirm, setShowConfirm] = useState<string | null>(null);
  const [actionReason, setActionReason] = useState('');
  const [actionService, setActionService] = useState('');
  const [actionPid, setActionPid] = useState('');
  const [scheduleAt, setScheduleAt] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [selectedActions, setSelectedActions] = useState<string[]>([]);

  const isAdmin = user?.role === 'msp_admin' || user?.role === 'msp_owner' || user?.role === 'platform_admin' || user?.role === 'platform_owner';

  const fetchDevice = useCallback(async () => {
    if (!deviceID) return;
    setLoading(true);
    setError('');

    const authToken = api.getToken();
    const headers = { Authorization: `Bearer ${authToken}`, 'Content-Type': 'application/json' };

    try {
      const abort = new AbortController();
      const [dRes, iRes, jRes, cRes, aRes, pRes] = await Promise.allSettled([
        fetch(`/api/v2/devices/${deviceID}`, { headers, signal: abort.signal }),
        fetch(`/api/v2/devices/${deviceID}/inventory`, { headers, signal: abort.signal }),
        fetch(`/api/v1/devices/${deviceID}/jobs?limit=50`, { headers, signal: abort.signal }),
        fetch(`/api/v2/devices/${deviceID}/capabilities`, { headers, signal: abort.signal }),
        fetch(`/api/v2/audit/endpoint?device_id=${deviceID}&limit=20`, { headers, signal: abort.signal }),
        fetch(`/api/v2/approvals?status=pending`, { headers, signal: abort.signal }),
      ]);

      if (dRes.status === 'fulfilled' && dRes.value.ok) setDevice(await dRes.value.json());
      else setError('Device not found');

      if (iRes.status === 'fulfilled' && iRes.value.ok) setInventory(await iRes.value.json());
      if (jRes.status === 'fulfilled' && jRes.value.ok) { const jd = await jRes.value.json(); setJobs(jd.jobs || []); }
      if (cRes.status === 'fulfilled' && cRes.value.ok) setCapabilities(await cRes.value.json());
      if (aRes.status === 'fulfilled' && aRes.value.ok) { const ad = await aRes.value.json(); setAuditEntries(ad.evidence || []); }
      if (pRes.status === 'fulfilled' && pRes.value.ok) { const pd = await pRes.value.json(); setPendingApprovals(pd.approvals || []); }

      return () => abort.abort();
    } catch (e) {
      if ((e as Error).name !== 'AbortError') setError('Failed to load device');
    } finally {
      setLoading(false);
    }
  }, [deviceID]);

  useEffect(() => {
    const cleanup = fetchDevice();
    return () => { cleanup.then(fn => fn?.()); };
  }, [fetchDevice]);

  const supportedActions = actionDefs.filter(a => {
    if (!capabilities) return !a.destructive;
    return capabilities.supported_job_types.includes(`device.${a.action}`) || capabilities.supported_job_types.includes(a.action);
  });

  const visibleActions = isAdmin ? supportedActions : supportedActions.filter(a => !a.destructive);

  const confirmAndRun = (action: string) => {
    setShowConfirm(action);
    setActionReason('');
    setActionService('');
    setActionPid('');
    setScheduleAt('');
    setActionError('');
  };

  const toggleAction = (action: string) => {
    setSelectedActions(prev =>
      prev.includes(action) ? prev.filter(a => a !== action) : [...prev, action]
    );
  };

  const executeAction = useCallback(async () => {
    if (!showConfirm || !deviceID || submitting) return;
    setSubmitting(true);
    setActionMsg('');
    setActionError('');

    const def = actionDefs.find(a => a.action === showConfirm);
    if (def?.requiresReason && !actionReason) { setActionError('Reason is required'); setSubmitting(false); return; }

    const body: Record<string, unknown> = {
      action: showConfirm,
      reason: actionReason,
      schedule_at: scheduleAt || undefined,
    };
    if (def?.requiresService && !actionService) { setActionError('Service name required'); setSubmitting(false); return; }
    if (def?.requiresService) body.service = actionService;
    if (def?.requiresPid) {
      const pid = parseInt(actionPid, 10);
      if (!pid || pid <= 0) { setActionError('Valid process ID required'); setSubmitting(false); return; }
      body.process_id = pid;
    }

    try {
      const idempotencyKey = crypto.randomUUID();
      const authToken = api.getToken();
      const res = await fetch(`/api/v2/devices/${deviceID}/action`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': idempotencyKey,
          Authorization: `Bearer ${authToken}`,
        },
        body: JSON.stringify(body),
      });
      const data = await res.json();
      if (res.ok) {
        setActionMsg(`${showConfirm} queued — Job: ${data.job_id?.slice(0, 12)}...`);
      } else {
        setActionError(data.error || 'Action failed');
        if (data.error?.includes('approval')) {
          setActionError(`${data.error}. Navigate to Approvals tab to create an approval request.`);
        }
      }
    } catch {
      setActionError('Request failed');
    }
    setSubmitting(false);
    setShowConfirm(null);
  }, [showConfirm, deviceID, submitting, actionReason, scheduleAt, actionService, actionPid]);

  const isOnline = (inventory as any)?.online;
  const statusClass = isOnline === true ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200' :
    isOnline === false ? 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200' : 'bg-slate-100 text-slate-800 dark:bg-slate-800 dark:text-slate-300';

  const tabs: { key: Tab; label: string }[] = [
    { key: 'overview', label: 'Overview' },
    { key: 'inventory', label: 'Hardware' },
    { key: 'services', label: 'Services' },
    { key: 'processes', label: 'Processes' },
    { key: 'jobs', label: `Jobs (${jobs.length})` },
    { key: 'history', label: 'Operation History' },
    { key: 'audit', label: `Audit (${auditEntries.length})` },
    { key: 'approvals', label: `Approvals (${pendingApprovals.length})` },
    { key: 'actions', label: 'Actions' },
  ];

  if (loading) return <div className="text-center py-12 text-slate-500 dark:text-slate-400" role="status">Loading device...</div>;
  if (error) return <div className="text-center py-12 text-red-500" role="alert">{error}</div>;
  if (!device) return <div className="text-center py-12 text-slate-400">Device not found</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between flex-wrap gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-white">
            {(device as any).hostname || 'Device'}
          </h1>
          <p className="text-sm text-slate-500 dark:text-slate-400">
            {(device as any).os || ''} · {(device as any).arch || ''}
            {capabilities && <span className="ml-2">· v{capabilities.agent_version}</span>}
          </p>
        </div>
        <div className="flex items-center gap-3">
          {capabilities && (
            <span className="text-xs text-slate-400" title={`Supported: ${capabilities.supported_job_types.join(', ')}`}>
              {capabilities.supported_job_types.length} capabilities
            </span>
          )}
          <span className={`px-3 py-1 rounded-full text-sm font-medium ${statusClass}`} role="status">
            {(device as any).status || 'unknown'}
          </span>
        </div>
      </div>

      <div className="border-b border-slate-200 dark:border-slate-700" role="tablist">
        <nav className="flex gap-2 overflow-x-auto pb-1">
          {tabs.map(t => (
            <button
              key={t.key}
              role="tab"
              aria-selected={tab === t.key}
              onClick={() => setTab(t.key)}
              className={`pb-2 px-1 text-sm font-medium border-b-2 whitespace-nowrap transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-slate-900 ${
                tab === t.key
                  ? 'border-blue-600 text-blue-600 dark:border-blue-400 dark:text-blue-400'
                  : 'border-transparent text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-300'
              }`}
            >
              {t.label}
            </button>
          ))}
        </nav>
      </div>

      {tab === 'overview' && (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
          {[
            ['Hostname', (device as any).hostname],
            ['OS', (device as any).os],
            ['Architecture', (device as any).arch],
            ['Agent Version', (device as any).agent_version],
            ['CPU Cores', (inventory as any)?.cpu_cores],
            ['Memory', (inventory as any)?.memory_mb ? `${((inventory as any).memory_mb / 1024).toFixed(1)} GB` : '-'],
            ['Disk', (inventory as any)?.disk_mb ? `${((inventory as any).disk_mb / 1024).toFixed(0)} GB` : '-'],
            ['Status', (device as any).status],
            ['Last Seen', (inventory as any)?.last_heartbeat ? new Date((inventory as any).last_heartbeat).toLocaleString() : 'never'],
            ['Data Age', (inventory as any)?.data_age_seconds ? `${Math.round((inventory as any).data_age_seconds)}s` : '-'],
            ['Online', isOnline === true ? 'Yes' : 'No'],
            ['Client ID', (device as any).client_id?.slice(0, 8) || '-'],
          ].map(([label, value]) => (
            <div key={label as string} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-4">
              <p className="text-xs text-slate-500 dark:text-slate-400">{label as string}</p>
              <p className="text-sm font-medium mt-1 text-slate-900 dark:text-white">{String(value ?? '-')}</p>
            </div>
          ))}
        </div>
      )}

      {tab === 'inventory' && (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-6">
          <h3 className="text-lg font-semibold text-slate-900 dark:text-white mb-4">Hardware Inventory</h3>
          <dl className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {[
              ['CPU Cores', inventory?.cpu_cores],
              ['Memory (MB)', inventory?.memory_mb],
              ['Disk (MB)', inventory?.disk_mb],
              ['Agent Version', inventory?.agent_version],
              ['OS', inventory?.os],
              ['Architecture', inventory?.arch],
            ].map(([label, value]) => (
              <div key={label as string} className="flex justify-between py-2 border-b border-slate-100 dark:border-slate-800">
                <dt className="text-sm text-slate-500 dark:text-slate-400">{label as string}</dt>
                <dd className="text-sm font-medium text-slate-900 dark:text-white">{String(value ?? '-')}</dd>
              </div>
            ))}
          </dl>
          <p className="text-xs text-slate-400 mt-4">
            Data age: {inventory?.data_age_seconds ? `${Math.round((inventory as any).data_age_seconds)}s` : 'unknown'}
          </p>
        </div>
      )}

      {tab === 'services' && (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-6">
          <h3 className="text-lg font-semibold text-slate-900 dark:text-white mb-4">Services</h3>
          <p className="text-sm text-slate-500">Service management requires agent inventory data.</p>
          <div className="mt-4 space-y-2">
            <div className="flex gap-2">
              <input
                type="text"
                placeholder="Service name"
                className="px-3 py-2 border rounded-md dark:bg-slate-800 dark:border-slate-600 text-sm flex-1"
                value={actionService}
                onChange={e => setActionService(e.target.value)}
                aria-label="Service name"
              />
              {['service_start', 'service_stop', 'service_restart'].map(a => {
                const def = actionDefs.find(d => d.action === a);
                return (
                  <button
                    key={a}
                    onClick={() => confirmAndRun(a)}
                    disabled={!actionService || submitting}
                    className={`px-3 py-2 text-sm rounded-md text-white ${def?.cls} disabled:opacity-50 disabled:cursor-not-allowed`}
                    aria-label={def?.label}
                  >
                    {def?.label}
                  </button>
                );
              })}
            </div>
          </div>
        </div>
      )}

      {tab === 'processes' && (
        <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-6">
          <h3 className="text-lg font-semibold text-slate-900 dark:text-white mb-4">Processes</h3>
          <div className="flex gap-2 items-end">
            <div className="flex-1">
              <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">Process ID</label>
              <input
                type="number"
                min="1"
                value={actionPid}
                onChange={e => setActionPid(e.target.value)}
                className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 dark:border-slate-600 text-sm"
                aria-label="Process ID"
              />
            </div>
            <button
              onClick={() => confirmAndRun('process_kill')}
              disabled={!actionPid || parseInt(actionPid) <= 0 || submitting}
              className="px-4 py-2 text-sm rounded-md bg-orange-600 text-white hover:bg-orange-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Kill Process
            </button>
          </div>
        </div>
      )}

      {tab === 'jobs' && (
        <div className="space-y-2">
          {jobs.length === 0 ? (
            <p className="text-slate-400 dark:text-slate-500 text-center py-8">No jobs for this device</p>
          ) : jobs.map((j: any, i: number) => (
            <div key={i} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-3 flex items-center justify-between">
              <div>
                <span className="text-sm font-medium text-slate-900 dark:text-white">{j.type || j.job_type}</span>
                <span className="ml-2 text-xs text-slate-400">{j.created_at ? new Date(j.created_at).toLocaleString() : ''}</span>
              </div>
              <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                j.target_status === 'succeeded' ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200' :
                j.target_status === 'failed' ? 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200' :
                j.target_status === 'cancelled' ? 'bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300' :
                j.target_status === 'running' ? 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200' :
                j.target_status === 'waiting' ? 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200' :
                'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300'
              }`}>{j.target_status || j.job_status || 'queued'}</span>
            </div>
          ))}
        </div>
      )}

      {tab === 'history' && (
        <div className="space-y-2">
          <h3 className="text-lg font-semibold text-slate-900 dark:text-white">Operation History</h3>
          {auditEntries.length === 0 ? (
            <p className="text-slate-400 dark:text-slate-500 text-center py-8">No operation history</p>
          ) : auditEntries.map((e: any, i: number) => (
            <div key={i} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-3">
              <div className="flex justify-between items-start">
                <div>
                  <span className="text-sm font-medium text-slate-900 dark:text-white">{e.action}</span>
                  <span className="ml-2 text-xs text-slate-400">{e.created_at ? new Date(e.created_at).toLocaleString() : ''}</span>
                </div>
                <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                  e.state_transition === 'succeeded' || e.state_transition === 'completed' ? 'bg-green-100 text-green-800' :
                  e.state_transition === 'failed' || e.failure_reason ? 'bg-red-100 text-red-800' :
                  'bg-slate-100 text-slate-600'
                }`}>{e.state_transition || 'recorded'}</span>
              </div>
              {e.result_summary && <p className="text-xs text-slate-500 mt-1">{e.result_summary}</p>}
              {e.failure_reason && <p className="text-xs text-red-500 mt-1">{e.failure_reason}</p>}
            </div>
          ))}
        </div>
      )}

      {tab === 'audit' && (
        <div className="space-y-2">
          <h3 className="text-lg font-semibold text-slate-900 dark:text-white">Audit Trail</h3>
          {auditEntries.length === 0 ? (
            <p className="text-slate-400 dark:text-slate-500 text-center py-8">No audit entries</p>
          ) : auditEntries.map((e: any, i: number) => (
            <div key={i} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-3 text-sm">
              <div className="flex justify-between">
                <span className="font-medium text-slate-900 dark:text-white">{e.action}</span>
                <span className="text-slate-400 text-xs">{e.created_at ? new Date(e.created_at).toLocaleString() : ''}</span>
              </div>
              <p className="text-slate-500 text-xs mt-1">Actor: {e.actor_user_id?.slice(0, 8) || 'unknown'}</p>
              {e.result_summary && <p className="text-slate-400 text-xs mt-1">{e.result_summary}</p>}
            </div>
          ))}
        </div>
      )}

      {tab === 'approvals' && (
        <div className="space-y-4">
          <h3 className="text-lg font-semibold text-slate-900 dark:text-white">Pending Approvals</h3>
          {pendingApprovals.length === 0 ? (
            <p className="text-slate-400 dark:text-slate-500 text-center py-8">No pending approvals</p>
          ) : pendingApprovals.map((a: any, i: number) => (
            <div key={i} className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-4">
              <div className="flex justify-between items-start">
                <div>
                  <p className="text-sm font-medium text-slate-900 dark:text-white">{a.action_name}</p>
                  <p className="text-xs text-slate-500">Requested by: {a.requester_user_id?.slice(0, 8)}</p>
                  <p className="text-xs text-slate-500">Devices: {a.device_count} · Expires: {a.expires_at ? new Date(a.expires_at).toLocaleString() : 'N/A'}</p>
                  {a.reason && <p className="text-xs text-slate-400 mt-1">Reason: {a.reason}</p>}
                </div>
                <span className="px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800">{a.status}</span>
              </div>
            </div>
          ))}
          <div className="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-4 mt-4">
            <h4 className="text-sm font-medium text-slate-900 dark:text-white mb-2">Create Approval Request</h4>
            <p className="text-xs text-slate-500 mb-3">Submit an approval request for destructive actions on this device.</p>
            {visibleActions.filter(a => a.destructive).map(a => (
              <button
                key={a.action}
                onClick={() => confirmAndRun(a.action)}
                className={`mr-2 mb-2 px-3 py-1.5 text-sm rounded-md text-white ${a.cls}`}
              >
                {a.label}
              </button>
            ))}
          </div>
        </div>
      )}

      {tab === 'actions' && (
        <div className="space-y-3">
          {actionError && (
            <div className="text-sm p-3 rounded-md bg-red-50 text-red-600 dark:bg-red-900/50 dark:text-red-200" role="alert">
              {actionError}
            </div>
          )}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
            {visibleActions.map(a => (
              <button
                key={a.action}
                onClick={() => confirmAndRun(a.action)}
                disabled={!isOnline && !a.supportedOffline}
                className={`px-4 py-3 ${a.cls} text-white text-sm rounded-md transition-colors text-left font-medium disabled:opacity-40 disabled:cursor-not-allowed focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2`}
                title={!isOnline && !a.supportedOffline ? 'Device offline' : a.label}
              >
                {a.label}
                {!isOnline && !a.supportedOffline && <span className="block text-xs opacity-75">requires online device</span>}
              </button>
            ))}
          </div>

          <div className="flex gap-2 items-center flex-wrap">
            {actionDefs.map(a => (
              <label key={a.action} className="flex items-center gap-1.5 text-sm cursor-pointer">
                <input
                  type="checkbox"
                  checked={selectedActions.includes(a.action)}
                  onChange={() => toggleAction(a.action)}
                  className="rounded"
                />
                <span>{a.label}</span>
              </label>
            ))}
          </div>
          {selectedActions.length > 0 && (
            <div className="text-sm text-slate-500">
              {selectedActions.length} action(s) selected for bulk workflow
            </div>
          )}
        </div>
      )}

      {actionMsg && (
        <div className={`text-sm p-3 rounded-md ${actionMsg.includes('queued') ? 'bg-green-50 text-green-800 dark:bg-green-900/50 dark:text-green-200' : 'bg-red-50 text-red-600 dark:bg-red-900/50 dark:text-red-200'}`} role="status">
          {actionMsg}
        </div>
      )}

      {showConfirm && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4" onClick={() => { if (!submitting) setShowConfirm(null); }} role="dialog" aria-modal="true" aria-label={`Confirm ${showConfirm}`}>
          <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-md w-full p-6 space-y-4" onClick={e => e.stopPropagation()}>
            <h3 className="text-lg font-bold text-slate-900 dark:text-white">Confirm {showConfirm}</h3>
            <p className="text-sm text-slate-500 dark:text-slate-400">Device: {(device as any).hostname}</p>

            {(actionDefs.find(a => a.action === showConfirm)?.requiresReason) && (
              <div>
                <label htmlFor="endpoint-action-reason" className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">Reason *</label>
                <input
                  id="endpoint-action-reason"
                  type="text"
                  value={actionReason}
                  onChange={e => setActionReason(e.target.value)}
                  className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 dark:border-slate-600 text-sm"
                  required
                  aria-required="true"
                />
              </div>
            )}

            <div>
              <label htmlFor="endpoint-action-schedule" className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">Schedule (optional)</label>
              <input
                id="endpoint-action-schedule"
                type="datetime-local"
                value={scheduleAt}
                onChange={e => setScheduleAt(e.target.value ? new Date(e.target.value).toISOString() : '')}
                className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 dark:border-slate-600 text-sm"
              />
            </div>

            {!isOnline && <p className="text-sm text-amber-600 dark:text-amber-400">Device is offline — action will be queued</p>}

            <div className="flex justify-end gap-3 pt-2">
              <button
                onClick={() => setShowConfirm(null)}
                disabled={submitting}
                className="px-4 py-2 text-sm rounded-md border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                onClick={executeAction}
                disabled={submitting}
                className="px-4 py-2 text-sm rounded-md bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
              >
                {submitting && <span className="animate-spin h-3 w-3 border-2 border-white border-t-transparent rounded-full" />}
                {submitting ? 'Submitting...' : 'Execute'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
