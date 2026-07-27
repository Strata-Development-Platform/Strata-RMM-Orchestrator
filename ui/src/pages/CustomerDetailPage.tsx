import { useState, useEffect } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { api } from '@/api/client';
import { useToast } from '@/components/shared/Toast';
import { Skeleton } from '@/components/shared/Skeleton';
import { StatusBadge } from '@/components/shared/StatusBadge';
import type { CustomerSummary } from '@/api/types';

type Tab = 'devices' | 'alerts' | 'vulnerabilities' | 'recordings' | 'settings';

export default function CustomerDetailPage() {
  const { tenantID } = useParams<{ tenantID: string }>();
  const navigate = useNavigate();
  const [customer, setCustomer] = useState<CustomerSummary | null>(null);
  const [tab, setTab] = useState<Tab>('devices');
  const [devices, setDevices] = useState<Record<string, unknown>[]>([]);
  const [alerts, setAlerts] = useState<Record<string, unknown>[]>([]);
  const [vulns, setVulns] = useState<Record<string, unknown>[]>([]);
  const [recordings, setRecordings] = useState<Record<string, unknown>[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!tenantID) return;
    setLoading(true);
    Promise.all([
      api.getCustomers().then(r => r.customers.find(c => c.id === tenantID)),
      api.getTenantDevices(tenantID).then(r => setDevices(r.devices)),
      api.getTenantAlerts(tenantID).then(r => setAlerts(r.alerts)),
      api.getTenantVulns(tenantID).then(r => setVulns(r.vulnerabilities)),
      api.getTenantRecordings(tenantID).then(r => setRecordings(r.recordings)),
    ]).then(([cust]) => {
      setCustomer(cust || null);
    }).catch(console.error).finally(() => setLoading(false));
  }, [tenantID]);

  if (loading) return <Skeleton type="card" count={4} />;
  if (!customer) return <div className="text-center py-12 text-red-500">Customer not found</div>;

  const tabs: { key: Tab; label: string; badge?: number }[] = [
    { key: 'devices', label: 'Devices', badge: customer.device_count },
    { key: 'alerts', label: 'Alerts', badge: customer.alert_count },
    { key: 'vulnerabilities', label: 'Vulnerabilities', badge: customer.cve_count },
    { key: 'recordings', label: 'Recordings', badge: recordings.length },
    { key: 'settings', label: 'Settings' },
  ];

  return (
    <div className="space-y-6">
      <div>
        <Link to="/" className="text-sm text-blue-600 hover:text-blue-800">&larr; Back to Overview</Link>
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white mt-1">{customer.name}</h1>
        <p className="text-sm text-slate-500">{customer.slug} &middot; {customer.plan} plan</p>
      </div>

      <div className="border-b border-slate-200 dark:border-slate-700">
        <nav className="flex gap-1">
          {tabs.map(t => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                tab === t.key
                  ? 'border-blue-600 text-blue-600'
                  : 'border-transparent text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
              }`}
            >
              {t.label}
              {t.badge !== undefined && t.badge > 0 && (
                <span className={`ml-2 px-1.5 py-0.5 text-xs rounded-full ${
                  t.key === 'alerts' ? 'bg-amber-100 text-amber-800' :
                  t.key === 'vulnerabilities' ? 'bg-red-100 text-red-800' :
                  'bg-slate-100 text-slate-600'
                }`}>{t.badge}</span>
              )}
            </button>
          ))}
        </nav>
      </div>

      {tab === 'devices' && <DevicesTab devices={devices} tenantID={tenantID!} />}
      {tab === 'alerts' && <AlertsTab alerts={alerts} tenantID={tenantID!} />}
      {tab === 'vulnerabilities' && <VulnsTab vulns={vulns} tenantID={tenantID!} />}
      {tab === 'recordings' && <RecordingsTab recordings={recordings} />}
      {tab === 'settings' && <SettingsTab tenantID={tenantID!} customer={customer} />}
    </div>
  );
}

function DevicesTab({ devices, tenantID, showToast: st }: { devices: Record<string, unknown>[]; tenantID: string; showToast?: (t: string, m: string) => void }) {
  return (
    <div className="space-y-3">
      {devices.length > 0 && <DeviceUpdateBar devices={devices} tenantID={tenantID} showToast={st} />}
      <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 dark:bg-slate-800">
            <tr>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Hostname</th>
              <th className="text-left px-4 py-3 font-medium text-slate-500">OS</th>
              <th className="text-center px-4 py-3 font-medium text-slate-500">Agent Version</th>
              <th className="text-center px-4 py-3 font-medium text-slate-500">Status</th>
              <th className="text-right px-4 py-3 font-medium text-slate-500">Last Heartbeat</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
            {devices.length === 0 ? (
              <tr><td colSpan={5} className="px-4 py-8 text-center text-slate-400">No devices enrolled</td></tr>
            ) : devices.map((d: Record<string, unknown>) => (
              <tr key={d.id as string} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                <td className="px-4 py-3 font-medium">{d.hostname as string}</td>
                <td className="px-4 py-3 text-slate-600">{(d.os as string) || '-'} {(d.os_version as string) || ''}</td>
                <td className="px-4 py-3 text-center">
                  <span className="font-mono text-xs">{(d.agent_version as string) || 'unknown'}</span>
                </td>
                <td className="px-4 py-3 text-center">
                  <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${
                    d.status === 'online' ? 'bg-green-100 text-green-800' :
                    d.status === 'offline' ? 'bg-red-100 text-red-800' :
                    'bg-slate-100 text-slate-600'
                  }`}>
                    <span className={`w-1.5 h-1.5 rounded-full ${
                      d.status === 'online' ? 'bg-green-500' :
                      d.status === 'offline' ? 'bg-red-500' : 'bg-slate-400'
                    }`} />
                    {d.status as string}
                  </span>
                </td>
                <td className="px-4 py-3 text-right text-slate-500">
                  {d.last_heartbeat ? new Date(d.last_heartbeat as string).toLocaleString() : '-'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function DeviceUpdateBar({ devices, tenantID, showToast: st }: { devices: Record<string, unknown>[]; tenantID: string; showToast?: (t: string, m: string) => void }) {
  const [updating, setUpdating] = useState(false);
  const showToast = st || ((_t: string, _m: string) => {});

  const handleUpdateAll = async () => {
    setUpdating(true);
    try {
      const res = await fetch(`/api/v1/platform/customers/${tenantID}/devices/update-all`, { method: 'POST' });
      const data = await res.json();
      showToast('success', `Update triggered for ${data.count} devices`);
    } catch {
      showToast('error', 'Failed to trigger updates');
    }
    setUpdating(false);
  };

  return (
    <div className="flex items-center justify-between text-sm bg-slate-50 dark:bg-slate-800/50 px-4 py-2 rounded-lg border border-slate-200 dark:border-slate-700">
      <span className="text-slate-500">{devices.length} device{devices.length !== 1 ? 's' : ''}</span>
      <button onClick={handleUpdateAll} disabled={updating}
        className="px-3 py-1 bg-blue-600 text-white text-xs rounded hover:bg-blue-700 disabled:opacity-50">
        {updating ? 'Updating...' : 'Update All'}
      </button>
    </div>
  );
}

function AlertsTab({ alerts, tenantID }: { alerts: Record<string, unknown>[]; tenantID: string }) {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Active Alerts ({alerts.length})</h2>
        <a href={`/api/v1/rules/${tenantID}`} className="text-sm text-blue-600 hover:underline" onClick={e => { e.preventDefault(); window.open(`/api/v1/rules/${tenantID}`, '_blank'); }}>
          Manage Rules
        </a>
      </div>
      <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 dark:bg-slate-800">
            <tr>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Severity</th>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Message</th>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Device</th>
              <th className="text-right px-4 py-3 font-medium text-slate-500">Fired At</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
            {alerts.length === 0 ? (
              <tr><td colSpan={4} className="px-4 py-8 text-center text-slate-400">No active alerts</td></tr>
            ) : alerts.map((a: Record<string, unknown>) => (
              <tr key={a.id as string}>
                <td className="px-4 py-3">
                  <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                    a.severity === 'critical' ? 'bg-red-100 text-red-800' :
                    a.severity === 'warning' ? 'bg-amber-100 text-amber-800' :
                    'bg-blue-100 text-blue-800'
                  }`}>{(a.severity as string)?.toUpperCase()}</span>
                </td>
                <td className="px-4 py-3 max-w-md truncate">{a.message as string}</td>
                <td className="px-4 py-3 text-slate-600 font-mono text-xs">{(a.device_id as string)?.slice(0, 8)}...</td>
                <td className="px-4 py-3 text-right text-slate-500">{a.fired_at ? new Date(a.fired_at as string).toLocaleString() : '-'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function VulnsTab({ vulns }: { vulns: Record<string, unknown>[]; tenantID: string }) {
  return (
    <div>
      {vulns.length === 0 ? (
        <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-8 text-center text-slate-400">
          No open vulnerabilities
        </div>
      ) : (
        <div className="space-y-2">
          {vulns.map((v: Record<string, unknown>) => (
            <div key={v.id as string} className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                    v.severity === 'critical' ? 'bg-red-100 text-red-800' :
                    v.severity === 'high' ? 'bg-orange-100 text-orange-800' :
                    'bg-yellow-100 text-yellow-800'
                  }`}>{(v.severity as string)?.toUpperCase()}</span>
                  <span className="font-mono text-sm font-medium">{v.cve_id as string}</span>
                </div>
                <span className={`text-xs font-medium ${
                  v.status === 'open' ? 'text-red-600' : 'text-green-600'
                }`}>{v.status as string}</span>
              </div>
              <p className="mt-1 text-sm text-slate-600">{v.package_name as string}: {v.current_version as string} → {v.fixed_in as string}</p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function RecordingsTab({ recordings }: { recordings: Record<string, unknown>[] }) {
  return (
    <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
      <table className="w-full text-sm">
        <thead className="bg-slate-50 dark:bg-slate-800">
          <tr>
            <th className="text-left px-4 py-3 font-medium text-slate-500">Session</th>
            <th className="text-center px-4 py-3 font-medium text-slate-500">Size</th>
            <th className="text-center px-4 py-3 font-medium text-slate-500">Format</th>
            <th className="text-right px-4 py-3 font-medium text-slate-500">Created</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
          {recordings.length === 0 ? (
            <tr><td colSpan={4} className="px-4 py-8 text-center text-slate-400">No recordings</td></tr>
          ) : recordings.map((r: Record<string, unknown>) => (
            <tr key={r.id as string}>
              <td className="px-4 py-3 font-mono text-xs">{(r.session_id as string)?.slice(0, 12)}...</td>
              <td className="px-4 py-3 text-center">{(r.size_bytes as number) > 1024 ? `${Math.round((r.size_bytes as number) / 1024)}KB` : `${r.size_bytes}B`}</td>
              <td className="px-4 py-3 text-center uppercase text-xs">{r.format as string}</td>
              <td className="px-4 py-3 text-right text-slate-500">{r.created_at ? new Date(r.created_at as string).toLocaleString() : '-'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function SettingsTab({ tenantID, customer }: { tenantID: string; customer: CustomerSummary }) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
      <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
        <h3 className="font-semibold mb-3">Customer Info</h3>
        <dl className="space-y-2 text-sm">
          <div className="flex justify-between"><dt className="text-slate-500">Name</dt><dd>{customer.name}</dd></div>
          <div className="flex justify-between"><dt className="text-slate-500">Slug</dt><dd>{customer.slug}</dd></div>
          <div className="flex justify-between"><dt className="text-slate-500">Plan</dt><dd className="capitalize">{customer.plan}</dd></div>
          <div className="flex justify-between"><dt className="text-slate-500">Active</dt><dd>{customer.is_active ? 'Yes' : 'No'}</dd></div>
          <div className="flex justify-between"><dt className="text-slate-500">Created</dt><dd>{new Date(customer.created_at).toLocaleDateString()}</dd></div>
        </dl>
        {customer.deployment_id && (
          <div className="mt-3 pt-3 border-t border-slate-200 dark:border-slate-700">
            <label className="text-sm font-medium text-slate-500">Deployment ID</label>
            <div className="flex items-center gap-2 mt-1">
              <code className="flex-1 text-sm bg-slate-100 dark:bg-slate-800 px-3 py-1.5 rounded font-mono">
                {customer.deployment_id}
              </code>
              <button
                onClick={() => { navigator.clipboard.writeText(customer.deployment_id); alert('Copied!'); }}
                className="text-xs px-2 py-1 bg-blue-600 text-white rounded hover:bg-blue-700"
              >Copy</button>
            </div>
            <p className="text-xs text-slate-400 mt-1">Use this ID when installing agents for this customer</p>
          </div>
        )}

        <div className="mt-3 pt-3 border-t border-slate-200 dark:border-slate-700">
          <label className="text-sm font-medium text-slate-500">Agent Update Source</label>
          <div className="flex items-center gap-2 mt-1">
            <select
              defaultValue="server"
              onChange={async (e) => {
                try {
                  await fetch(`/api/v1/platform/customers/${tenantID}/update-source`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ update_source: e.target.value, update_channel: 'stable' }),
                  });
                  alert('Update source updated');
                } catch { alert('Failed to update'); }
              }}
              className="flex-1 px-3 py-1.5 border rounded-md dark:bg-slate-800 text-sm"
            >
              <option value="server">Management Server</option>
              <option value="github">GitHub Releases</option>
            </select>
          </div>
          <p className="text-xs text-slate-400 mt-1">Choose where agents check for updates</p>
        </div>
      </div>

      <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
        <h3 className="font-semibold mb-3">Encryption Keys</h3>
        <p className="text-sm text-slate-500 mb-3">Manage per-customer encryption keys for data residency.</p>
        <a
          href={`/api/v1/keys/${tenantID}`}
          className="text-sm text-blue-600 hover:underline"
          onClick={e => { e.preventDefault(); window.open(`/api/v1/keys/${tenantID}`, '_blank'); }}
        >
          View Keys &rarr;
        </a>
      </div>

      <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
        <h3 className="font-semibold mb-3">Access & Audit</h3>
        <div className="space-y-2 text-sm">
          <a
            href={`/api/v1/access/users/${tenantID}`}
            className="block text-blue-600 hover:underline"
            onClick={e => { e.preventDefault(); window.open(`/api/v1/access/users/${tenantID}`, '_blank'); }}
          >View Users &rarr;</a>
          <a
            href={`/api/v1/access/audit/${tenantID}`}
            className="block text-blue-600 hover:underline"
            onClick={e => { e.preventDefault(); window.open(`/api/v1/access/audit/${tenantID}`, '_blank'); }}
          >View Audit Log &rarr;</a>
          <a
            href={`/api/v1/access/permissions/${tenantID}`}
            className="block text-blue-600 hover:underline"
            onClick={e => { e.preventDefault(); window.open(`/api/v1/access/permissions/${tenantID}`, '_blank'); }}
          >View Permissions &rarr;</a>
        </div>
      </div>

      <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
        <h3 className="font-semibold mb-3">CVE Management</h3>
        <p className="text-sm text-slate-500 mb-3">{customer.cve_count} open vulnerabilities</p>
        <button
          onClick={async () => {
            try {
              await fetch('/api/v1/cve/sync', { method: 'POST' });
              alert('CVE sync triggered');
            } catch { alert('Failed to trigger sync'); }
          }}
          className="text-sm px-3 py-1.5 bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          Trigger CVE Sync
        </button>
      </div>
    </div>
  );
}
