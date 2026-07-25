import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '@/api/client';
import { useToast } from '@/components/shared/Toast';
import { Skeleton } from '@/components/shared/Skeleton';
import type { CustomerSummary } from '@/api/types';

export default function CustomersPage() {
  const { showToast } = useToast();
  const [customers, setCustomers] = useState<CustomerSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const navigate = useNavigate();

  const load = async () => {
    try {
      const r = await api.getCustomers();
      setCustomers(r.customers);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const [newCust, setNewCust] = useState({ name: '', slug: '', plan: 'free', adminEmail: '' });
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');
  const [createdResult, setCreatedResult] = useState<{ deployment_id: string; name: string } | null>(null);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setCreating(true);
    try {
      const res = await api.createCustomer(newCust.name, newCust.slug || '', newCust.plan, newCust.adminEmail);
      setCreatedResult({ deployment_id: res.deployment_id, name: res.name });
      load();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'creation failed');
    } finally {
      setCreating(false);
    }
  };

  if (loading) return <Skeleton type="table" rows={5} count={7} />;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">Customers</h1>
        <button onClick={() => { setShowCreate(true); setCreatedResult(null); }}
          className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded-md hover:bg-blue-700">
          + Add Customer
        </button>
      </div>

      {/* Priority issues across all customers */}
      {customers.filter(c => c.alert_count > 0 || c.cve_count > 0 || (c.device_count - c.online_count) > 0).length > 0 && (
        <div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg p-4">
          <h2 className="font-semibold text-amber-800 dark:text-amber-300 mb-2">⚠ Priority Issues</h2>
          <div className="space-y-1 text-sm text-amber-700 dark:text-amber-400">
            {customers.filter(c => c.alert_count > 0).map(c => (
              <div key={c.id} className="flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-red-500" />
                <span>{c.name}: <strong>{c.alert_count}</strong> active {c.alert_count === 1 ? 'alert' : 'alerts'}</span>
                <button onClick={() => navigate(`/customers/${c.id}`)} className="text-blue-600 hover:underline ml-auto text-xs">View</button>
              </div>
            ))}
            {customers.filter(c => c.cve_count > 0).map(c => (
              <div key={c.id} className="flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-orange-500" />
                <span>{c.name}: <strong>{c.cve_count}</strong> open {c.cve_count === 1 ? 'CVE' : 'CVEs'}</span>
                <button onClick={() => navigate(`/customers/${c.id}`)} className="text-blue-600 hover:underline ml-auto text-xs">View</button>
              </div>
            ))}
            {customers.filter(c => (c.device_count - c.online_count) > 0).map(c => (
              <div key={c.id} className="flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-slate-400" />
                <span>{c.name}: <strong>{c.device_count - c.online_count}</strong> offline {c.device_count - c.online_count === 1 ? 'device' : 'devices'}</span>
                <button onClick={() => navigate(`/customers/${c.id}`)} className="text-blue-600 hover:underline ml-auto text-xs">View</button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Customer Table */}
      <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 dark:bg-slate-800">
            <tr>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Name</th>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Deployment ID</th>
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
              <tr key={c.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50 cursor-pointer" onClick={() => navigate(`/customers/${c.id}`)}>
                <td className="px-4 py-3 font-medium">{c.name}</td>
                <td className="px-4 py-3 font-mono text-xs text-slate-500">{c.deployment_id || '-'}</td>
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

      {/* Create Customer Modal */}
      {showCreate && !createdResult && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowCreate(false)}>
          <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-lg w-full mx-4 p-6" onClick={e => e.stopPropagation()}>
            <form onSubmit={handleCreate} className="space-y-4">
              <h2 className="text-lg font-bold">Add Customer</h2>

              <div>
                <label className="block text-sm font-medium mb-1">Company Name *</label>
                <input type="text" value={newCust.name} onChange={e => setNewCust({ ...newCust, name: e.target.value, slug: e.target.value.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-]/g, '') })}
                  className="w-full px-3 py-2 border rounded-md dark:bg-slate-800" required />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Slug</label>
                <input type="text" value={newCust.slug} onChange={e => setNewCust({ ...newCust, slug: e.target.value })}
                  className="w-full px-3 py-2 border rounded-md dark:bg-slate-800" />
                <p className="text-xs text-slate-400 mt-1">URL-friendly identifier. Auto-generated from name.</p>
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Plan</label>
                <select value={newCust.plan} onChange={e => setNewCust({ ...newCust, plan: e.target.value })}
                  className="w-full px-3 py-2 border rounded-md dark:bg-slate-800">
                  <option value="free">Free</option>
                  <option value="pro">Pro</option>
                  <option value="enterprise">Enterprise</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Admin Email</label>
                <input type="email" value={newCust.adminEmail} onChange={e => setNewCust({ ...newCust, adminEmail: e.target.value })}
                  className="w-full px-3 py-2 border rounded-md dark:bg-slate-800" />
                <p className="text-xs text-slate-400 mt-1">Optional. Creates admin user with default password.</p>
              </div>

              {error && <p className="text-red-500 text-sm">{error}</p>}

              <div className="flex justify-end gap-2 pt-2">
                <button type="button" onClick={() => setShowCreate(false)}
                  className="px-4 py-2 text-sm rounded-md border hover:bg-slate-50 dark:hover:bg-slate-800">Cancel</button>
                <button type="submit" disabled={creating}
                  className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700 disabled:opacity-50">
                  {creating ? 'Creating...' : 'Create Customer'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Success message with deployment ID */}
      {createdResult && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => { setShowCreate(false); setCreatedResult(null); }}>
          <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-lg w-full mx-4 p-6" onClick={e => e.stopPropagation()}>
            <h2 className="text-lg font-bold text-green-600 mb-2">✅ {createdResult.name} Created</h2>

            <div className="bg-slate-50 dark:bg-slate-800 rounded-lg p-4 mb-4">
              <label className="text-sm font-medium text-slate-500">Deployment ID</label>
              <div className="flex items-center gap-2 mt-1">
                <code className="flex-1 text-lg bg-white dark:bg-slate-900 px-3 py-2 rounded border font-mono font-bold">
                  {createdResult.deployment_id}
                </code>
                <button onClick={() => { navigator.clipboard.writeText(createdResult.deployment_id); alert('Copied!'); }}
                  className="px-3 py-2 bg-blue-600 text-white text-sm rounded hover:bg-blue-700 whitespace-nowrap">Copy</button>
              </div>
            </div>

            <div className="bg-slate-50 dark:bg-slate-800 rounded-lg p-4 mb-4">
              <p className="text-sm font-medium mb-2">Install Command</p>
              <code className="block text-xs bg-white dark:bg-slate-900 p-3 rounded border font-mono break-all">
                curl -sSL https://releases.strata-rmm.io/install.sh | sudo bash -s -- --deployment-id {createdResult.deployment_id}
              </code>
            </div>

            <div className="flex justify-end gap-2">
              <button onClick={() => navigate(`/customers/${customers.find(c => c.deployment_id === createdResult.deployment_id)?.id || ''}`)}
                className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700">View Customer</button>
              <button onClick={() => { setShowCreate(false); setCreatedResult(null); }}
                className="px-4 py-2 text-sm rounded-md border hover:bg-slate-50 dark:hover:bg-slate-800">Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
