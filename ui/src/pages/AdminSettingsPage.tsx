import { useState, useEffect } from 'react';
import { api } from '@/api/client';
import { useToast } from '@/components/shared/Toast';
import { Skeleton } from '@/components/shared/Skeleton';
import type { PlatformOverview } from '@/api/types';

export default function AdminSettingsPage() {
  const [overview, setOverview] = useState<PlatformOverview | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.getOverview()
      .then(setOverview)
      .catch(console.error)
      .finally(() => setLoading(false));
  }, []);

  const handleCVESync = async () => {
    try {
      const res = await fetch('/api/v1/cve/sync', { method: 'POST' });
      showToast('success', 'Sync triggered');
    } catch {
      showToast('error', 'Failed to trigger sync');
    }
  };

  const handleCopyToken = () => {
    const token = localStorage.getItem('strata_auth_token');
    if (token) {
      navigator.clipboard.writeText(token);
      showToast('success', 'Auth token copied to clipboard');
    } else {
      showToast('error', 'No auth token found');
    }
  };

  if (loading) return <Skeleton type="card" count={4} />;

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-bold text-slate-900 dark:text-white">Platform Settings</h1>

      {/* Platform Info */}
      <section>
        <h2 className="text-lg font-semibold mb-3">Platform Status</h2>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <div className="bg-white dark:bg-slate-900 p-4 rounded-lg border border-slate-200 dark:border-slate-700">
            <p className="text-sm text-slate-500">Customers</p>
            <p className="text-xl font-bold mt-1">{overview?.total_customers ?? '-'}</p>
          </div>
          <div className="bg-white dark:bg-slate-900 p-4 rounded-lg border border-slate-200 dark:border-slate-700">
            <p className="text-sm text-slate-500">Devices</p>
            <p className="text-xl font-bold mt-1">{overview?.total_devices ?? '-'}</p>
          </div>
          <div className="bg-white dark:bg-slate-900 p-4 rounded-lg border border-slate-200 dark:border-slate-700">
            <p className="text-sm text-slate-500">Active Alerts</p>
            <p className="text-xl font-bold mt-1">{overview?.active_alerts ?? '-'}</p>
          </div>
          <div className="bg-white dark:bg-slate-900 p-4 rounded-lg border border-slate-200 dark:border-slate-700">
            <p className="text-sm text-slate-500">Open CVEs</p>
            <p className="text-xl font-bold mt-1">{overview?.open_cves ?? '-'}</p>
          </div>
        </div>
      </section>

      {/* Quick Actions */}
      <section>
        <h2 className="text-lg font-semibold mb-3">Quick Actions</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="bg-white dark:bg-slate-900 p-4 rounded-lg border border-slate-200 dark:border-slate-700">
            <h3 className="font-medium mb-2">CVE Database</h3>
            <p className="text-sm text-slate-500 mb-3">Trigger an immediate sync of the CVE database from OSV.dev.</p>
            <button onClick={handleCVESync} className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded hover:bg-blue-700">
              Sync CVEs Now
            </button>
          </div>

          <div className="bg-white dark:bg-slate-900 p-4 rounded-lg border border-slate-200 dark:border-slate-700">
            <h3 className="font-medium mb-2">Auth Token</h3>
            <p className="text-sm text-slate-500 mb-3">Copy your current auth token for API access.</p>
            <button onClick={handleCopyToken} className="px-3 py-1.5 bg-slate-600 text-white text-sm rounded hover:bg-slate-700">
              Copy Token
            </button>
          </div>
        </div>
      </section>

      {/* API Reference */}
      <section>
        <h2 className="text-lg font-semibold mb-3">API Endpoints</h2>
        <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
          <div className="max-h-96 overflow-y-auto">
            <table className="w-full text-sm">
              <thead className="bg-slate-50 dark:bg-slate-800 sticky top-0">
                <tr>
                  <th className="text-left px-4 py-3 font-medium text-slate-500 w-16">Method</th>
                  <th className="text-left px-4 py-3 font-medium text-slate-500">Path</th>
                  <th className="text-left px-4 py-3 font-medium text-slate-500">Description</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-200 dark:divide-slate-700 font-mono text-xs">
                {endpoints.map((ep, i) => (
                  <tr key={i} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                    <td className="px-4 py-2">
                      <span className={`px-1.5 py-0.5 rounded text-xs font-medium ${
                        ep.method === 'GET' ? 'bg-green-100 text-green-800' :
                        ep.method === 'POST' ? 'bg-blue-100 text-blue-800' :
                        ep.method === 'PUT' ? 'bg-amber-100 text-amber-800' :
                        'bg-red-100 text-red-800'
                      }`}>{ep.method}</span>
                    </td>
                    <td className="px-4 py-2 text-slate-800 dark:text-slate-200">{ep.path}</td>
                    <td className="px-4 py-2 text-slate-500">{ep.desc}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </section>
    </div>
  );
}

const endpoints = [
  { method: 'GET', path: '/health', desc: 'Health check' },
  { method: 'POST', path: '/api/v1/auth/login', desc: 'User login (email + password)' },
  { method: 'GET', path: '/api/v1/auth/me', desc: 'Current user profile' },
  { method: 'POST', path: '/api/v1/enroll', desc: 'Agent enrollment token' },
  { method: 'GET', path: '/api/v1/platform/overview', desc: 'Platform aggregate stats' },
  { method: 'GET', path: '/api/v1/platform/customers', desc: 'Customer list with counts' },
  { method: 'GET', path: '/api/v1/platform/customers/{id}/devices', desc: 'Customer devices' },
  { method: 'GET', path: '/api/v1/metrics', desc: 'Query metrics (tenant_id, metric_name)' },
  { method: 'GET', path: '/api/v1/heartbeat/{tid}/{did}', desc: 'Device heartbeat' },
  { method: 'GET', path: '/api/v1/alerts/{tid}', desc: 'Active alerts' },
  { method: 'GET', path: '/api/v1/alerts/{tid}/history', desc: 'Alert history' },
  { method: 'POST', path: '/api/v1/alerts/{id}/acknowledge', desc: 'Acknowledge alert' },
  { method: 'GET', path: '/api/v1/rules/{tid}', desc: 'List alert rules' },
  { method: 'POST', path: '/api/v1/rules/{tid}', desc: 'Create alert rule' },
  { method: 'DELETE', path: '/api/v1/rules/{tid}/{rid}', desc: 'Delete alert rule' },
  { method: 'GET', path: '/api/v1/vulnerabilities/tenant/{tid}', desc: 'Tenant vulnerabilities' },
  { method: 'GET', path: '/api/v1/vulnerabilities/tenant/{tid}/summary', desc: 'Vulnerability summary' },
  { method: 'POST', path: '/api/v1/vulnerabilities/{id}/resolve', desc: 'Resolve vulnerability' },
  { method: 'POST', path: '/api/v1/vulnerabilities/{id}/ignore', desc: 'Ignore vulnerability' },
  { method: 'GET', path: '/api/v1/cve/stats', desc: 'CVE database stats' },
  { method: 'POST', path: '/api/v1/cve/sync', desc: 'Trigger CVE sync' },
  { method: 'GET', path: '/api/v1/cve/packages', desc: 'Tracked packages' },
  { method: 'GET', path: '/api/v1/cve/sync/status', desc: 'CVE sync status' },
  { method: 'GET', path: '/api/v1/recordings/{tid}', desc: 'Session recordings' },
  { method: 'GET', path: '/api/v1/recordings/{id}/playback', desc: 'Recording playback URL' },
  { method: 'DELETE', path: '/api/v1/recordings/{id}', desc: 'Delete recording' },
  { method: 'GET', path: '/api/v1/keys/{tid}', desc: 'List encryption keys' },
  { method: 'POST', path: '/api/v1/keys/{tid}', desc: 'Create encryption key' },
  { method: 'POST', path: '/api/v1/keys/{tid}/rotate', desc: 'Rotate encryption key' },
  { method: 'DELETE', path: '/api/v1/keys/{tid}/{kid}', desc: 'Revoke encryption key' },
  { method: 'GET', path: '/api/v1/access/users/{tid}', desc: 'Tenant users' },
  { method: 'GET', path: '/api/v1/access/audit/{tid}', desc: 'Audit log' },
  { method: 'GET', path: '/api/v1/access/permissions/{tid}', desc: 'Permissions matrix' },
  { method: 'GET', path: '/api/v1/admin/users', desc: 'List all users (admin)' },
  { method: 'POST', path: '/api/v1/admin/users', desc: 'Create user (admin)' },
  { method: 'PUT', path: '/api/v1/admin/users/{id}/tenants', desc: 'Update user tenant scope (admin)' },
  { method: 'POST', path: '/api/v1/mfa/enroll/{uid}', desc: 'Enroll MFA' },
  { method: 'POST', path: '/api/v1/mfa/verify/{uid}', desc: 'Verify MFA code' },
  { method: 'GET', path: '/api/v1/mfa/status/{uid}', desc: 'MFA enrollment status' },
  { method: 'DELETE', path: '/api/v1/mfa/{uid}', desc: 'Disable MFA' },
  { method: 'GET', path: '/api/v1/inventory/{did}', desc: 'Device inventory detail' },
];
