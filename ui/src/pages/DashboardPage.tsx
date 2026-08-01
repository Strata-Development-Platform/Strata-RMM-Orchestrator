import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Link } from 'react-router-dom';
import { api } from '@/api/client';
import { useAuth } from '@/hooks/useAuth';
import { useWorkspace } from '@/hooks/useWorkspace';
import { EmptyState } from '@/components/shared/EmptyState';
import type { PlatformOverview, CustomerSummary } from '@/api/types';
import {
  Building2, CloudCog, HardDrive, Network, ShieldCheck, UsersRound, type LucideIcon,
} from 'lucide-react';

function StatCard({ label, value, color = 'text-slate-900 dark:text-white' }: { label: string; value: number | string; color?: string }) {
  return (
    <div className="bg-white dark:bg-slate-900 p-4 rounded-lg border border-slate-200 dark:border-slate-700">
      <p className="text-sm text-slate-500 dark:text-slate-400">{label}</p>
      <p className={`text-2xl font-bold mt-1 ${color}`}>{value}</p>
    </div>
  );
}

function FeatureCard({ icon: Icon, title, description }: { icon: LucideIcon; title: string; description: string }) {
  return (
    <div className="group rounded-xl border border-slate-200 bg-white p-5 transition hover:-translate-y-0.5 hover:border-blue-300 hover:shadow-lg hover:shadow-blue-950/5 dark:border-slate-800 dark:bg-slate-900 dark:hover:border-blue-800">
      <div className="mb-4 inline-flex rounded-lg bg-blue-50 p-2.5 text-blue-700 dark:bg-blue-950/50 dark:text-blue-300">
        <Icon size={20} strokeWidth={1.8} />
      </div>
      <h3 className="font-semibold text-slate-950 dark:text-white">{title}</h3>
      <p className="mt-1.5 text-sm leading-6 text-slate-500 dark:text-slate-400">{description}</p>
    </div>
  );
}

export default function DashboardPage() {
  const { user } = useAuth();
  const { workspace } = useWorkspace();
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

  const roles = workspace?.roles ?? (user ? [user.role] : []);
  const isPlatform = Boolean(workspace?.platform_role);
  const isPlatformSupport = roles.includes('platform_support');
  const isPlatformBilling = roles.includes('platform_billing');
  const isSecurityAuditor = roles.some(role => ['platform_security_auditor', 'platform_viewer'].includes(role));
  const isMSPManager = roles.some(role => ['msp_owner', 'msp_admin'].includes(role));
  const isTechnician = roles.some(role => role === 'msp_technician');
  const isClientAdmin = roles.includes('client_admin');
  const dashboardTitle = isPlatform
    ? 'SaaS Provider Dashboard'
    : isPlatformSupport
      ? 'Support Operations Dashboard'
      : isPlatformBilling
        ? 'Billing & Usage Dashboard'
        : isSecurityAuditor
          ? 'Security & Audit Dashboard'
    : isMSPManager
      ? 'MSP Management Dashboard'
      : isTechnician
        ? 'Technician Dashboard'
        : isClientAdmin
          ? 'Client Operations Dashboard'
        : 'Service Dashboard';
  const dashboardDescription = isPlatform
    ? 'Platform health, tenant operations, and service-wide risk.'
    : isPlatformSupport
      ? 'Authorized support scopes, service health, incidents, and resolution priorities.'
      : isPlatformBilling
        ? 'Plan state, usage reconciliation, capacity, and account lifecycle.'
        : isSecurityAuditor
          ? 'Read-only security posture, audit evidence, and tenant-isolation signals.'
    : isMSPManager
      ? 'Customers, entitlement capacity, devices, and operational priorities.'
      : isTechnician
        ? 'Assigned customer health, active work, and endpoint priorities.'
        : isClientAdmin
          ? 'Devices, sites, reports, and approved work in your delegated scope.'
        : 'Read-only service health for your authorized scope.';

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-white">{dashboardTitle}</h1>
          <p className="mt-1 text-sm text-slate-500">{dashboardDescription}</p>
        </div>
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

      <div className="grid gap-4 md:grid-cols-3">
        <div className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900">
          <p className="text-xs font-semibold uppercase tracking-wide text-slate-500">Current scope</p>
          <p className="mt-2 font-semibold text-slate-900 dark:text-white">
            {workspace?.site_name || workspace?.client_name || workspace?.msp_name || (isPlatform ? 'Platform' : 'Assigned workspace')}
          </p>
          <p className="mt-1 text-xs text-slate-500">{roles.join(', ') || 'authenticated user'}</p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900">
          <p className="text-xs font-semibold uppercase tracking-wide text-slate-500">Service access</p>
          <p className="mt-2 font-semibold capitalize text-slate-900 dark:text-white">
            {workspace?.entitlement?.status ?? (isPlatform ? 'Platform operator' : 'Loading')}
          </p>
          {workspace?.entitlement && (
            <p className="mt-1 text-xs text-slate-500">
              {workspace.entitlement.plan_slug} · {workspace.entitlement.max_devices || 'Unlimited'} devices
            </p>
          )}
        </div>
        <div className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900">
          <p className="text-xs font-semibold uppercase tracking-wide text-slate-500">Quick workspace</p>
          <div className="mt-2 flex flex-wrap gap-2 text-sm">
            {isPlatform && <Link className="text-blue-600 hover:underline" to="/platform/msps">MSP tenants</Link>}
            {(isPlatform || isMSPManager) && <Link className="text-blue-600 hover:underline" to="/msp">Manage MSP</Link>}
            <Link className="text-blue-600 hover:underline" to="/jobs">Jobs</Link>
            <Link className="text-blue-600 hover:underline" to="/reports">Reports</Link>
          </div>
        </div>
      </div>

      {isPlatform && (
        <section className="overflow-hidden rounded-2xl border border-slate-800 bg-slate-950 text-white shadow-xl shadow-slate-950/10">
          <div className="grid gap-8 p-6 lg:grid-cols-[1.1fr_1.9fr] lg:p-8">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.2em] text-cyan-300">One control plane</p>
              <h2 className="mt-3 text-2xl font-semibold tracking-tight">Deliver managed IT without operational sprawl.</h2>
              <p className="mt-3 max-w-xl text-sm leading-6 text-slate-300">
                Strata RMM unifies tenant operations, endpoint automation, durable job execution,
                patching, remote access, reporting, and audit evidence in one provider-owned service.
              </p>
              <div className="mt-5 flex flex-wrap gap-3">
                <Link to="/platform/msps" className="rounded-lg bg-cyan-400 px-4 py-2 text-sm font-semibold text-slate-950 hover:bg-cyan-300">
                  Onboard an MSP
                </Link>
                <Link to="/jobs/health" className="rounded-lg border border-slate-700 px-4 py-2 text-sm font-medium text-slate-200 hover:border-slate-500">
                  Review service health
                </Link>
              </div>
            </div>
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              <FeatureCard icon={Building2} title="Multi-tenant by design" description="Provider, MSP, client, site, and device scopes with fail-closed access." />
              <FeatureCard icon={CloudCog} title="Durable automation" description="Approved endpoint work, retries, reconciliation, and auditable execution." />
              <FeatureCard icon={ShieldCheck} title="Security controls" description="Scoped enrollment, tenant isolation, immutable evidence, and recovery gates." />
              <FeatureCard icon={Network} title="Remote operations" description="Inventory, monitoring, patching, scripts, and remote workflows from one console." />
              <FeatureCard icon={HardDrive} title="Operational resilience" description="Backups, restore verification, telemetry, synthetics, and thresholded testing." />
              <FeatureCard icon={UsersRound} title="MSP-ready service" description="Branding, domains, entitlements, usage, and delegated technician roles." />
            </div>
          </div>
        </section>
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
        <h2 className="text-lg font-semibold text-slate-900 dark:text-white mb-3">
          {isPlatform ? 'Managed customer organizations' : 'Customers in this workspace'}
        </h2>
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
              {customers.length === 0 ? (
                <tr><td colSpan={7} className="px-4 py-8"><EmptyState icon={Building2} title="No customers yet" description="Create your first customer to get started" /></td></tr>
              ) : customers.map(c => (
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
