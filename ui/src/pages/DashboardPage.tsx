import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '@/api/client';
import { useAuth } from '@/hooks/useAuth';
import type { PlatformOverview, CustomerSummary } from '@/api/types';

function StatCard({ label, value, color = 'text-slate-900 dark:text-white' }: { label: string; value: number | string; color?: string }) {
  return (
    <div className="bg-white dark:bg-slate-900 p-4 rounded-lg border border-slate-200 dark:border-slate-700">
      <p className="text-sm text-slate-500 dark:text-slate-400">{label}</p>
      <p className={`text-2xl font-bold mt-1 ${color}`}>{value}</p>
    </div>
  );
}

export default function DashboardPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const [overview, setOverview] = useState<PlatformOverview | null>(null);
  const [customers, setCustomers] = useState<CustomerSummary[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([api.getOverview(), api.getCustomers()])
      .then(([ov, cust]) => {
        setOverview(ov);
        setCustomers(cust.customers);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="text-center py-12 text-slate-500">Loading...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">Platform Overview</h1>
        {overview && (
          <p className="text-sm text-slate-500">Last updated: {new Date(overview.timestamp).toLocaleString()}</p>
        )}
      </div>

      {overview && (
        <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-4">
          <StatCard label="Total Customers" value={overview.total_customers} />
          <StatCard label="Total Devices" value={overview.total_devices} />
          <StatCard label="Online" value={overview.online_devices} color="text-green-600" />
          <StatCard label="Offline" value={overview.offline_devices} color={overview.offline_devices > 0 ? 'text-red-600' : 'text-green-600'} />
          <StatCard label="Active Alerts" value={overview.active_alerts} color={overview.active_alerts > 0 ? 'text-amber-600' : 'text-green-600'} />
          <StatCard label="Critical Alerts" value={overview.critical_alerts} color={overview.critical_alerts > 0 ? 'text-red-600' : 'text-green-600'} />
          <StatCard label="Open CVEs" value={overview.open_cves} color={overview.open_cves > 0 ? 'text-red-600' : 'text-green-600'} />
        </div>
      )}

      {/* Priority Issues */}
      {(overview?.critical_alerts ?? 0) > 0 || (overview?.offline_devices ?? 0) > 0 || (overview?.open_cves ?? 0) > 0 ? (
        <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
          <h2 className="font-semibold text-red-800 dark:text-red-300 mb-2">Priority Issues</h2>
          <ul className="space-y-1 text-sm text-red-700 dark:text-red-400">
            {overview!.critical_alerts > 0 && <li>• {overview!.critical_alerts} critical {overview!.critical_alerts === 1 ? 'alert' : 'alerts'} need immediate attention</li>}
            {overview!.offline_devices > 0 && <li>• {overview!.offline_devices} {overview!.offline_devices === 1 ? 'device is' : 'devices are'} offline</li>}
            {overview!.open_cves > 0 && <li>• {overview!.open_cves} open {overview!.open_cves === 1 ? 'CVE' : 'CVEs'} on managed devices</li>}
          </ul>
        </div>
      ) : null}

      {/* Customer Table */}
      <div>
        <h2 className="text-lg font-semibold text-slate-900 dark:text-white mb-3">Customers</h2>
        <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-800">
              <tr>
                <th className="text-left px-4 py-3 font-medium text-slate-500">Name</th>
                <th className="text-center px-4 py-3 font-medium text-slate-500">Devices</th>
                <th className="text-center px-4 py-3 font-medium text-slate-500">Online</th>
                <th className="text-center px-4 py-3 font-medium text-slate-500">Alerts</th>
                <th className="text-center px-4 py-3 font-medium text-slate-500">CVEs</th>
                <th className="text-center px-4 py-3 font-medium text-slate-500">Plan</th>
                <th className="text-right px-4 py-3 font-medium text-slate-500"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
              {customers.map(c => (
                <tr key={c.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50 cursor-pointer"
                    onClick={() => navigate(`/customers/${c.id}`)}>
                  <td className="px-4 py-3 font-medium text-slate-900 dark:text-white">{c.name}</td>
                  <td className="px-4 py-3 text-center">{c.device_count}</td>
                  <td className="px-4 py-3 text-center">
                    <span className={c.online_count > 0 ? 'text-green-600' : 'text-slate-400'}>{c.online_count}</span>
                  </td>
                  <td className="px-4 py-3 text-center">
                    <span className={c.alert_count > 0 ? 'text-amber-600 font-medium' : ''}>{c.alert_count}</span>
                  </td>
                  <td className="px-4 py-3 text-center">
                    <span className={c.cve_count > 0 ? 'text-red-600 font-medium' : ''}>{c.cve_count}</span>
                  </td>
                  <td className="px-4 py-3 text-center capitalize">{c.plan}</td>
                  <td className="px-4 py-3 text-right text-blue-600">→</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
