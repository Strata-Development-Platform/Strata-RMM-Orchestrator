import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '@/hooks/useAuth';
import { ThemeToggle } from '@/components/shared/ThemeToggle';
import { useState } from 'react';
import { useWorkspace } from '@/hooks/useWorkspace';
import { ProductAttribution } from '@/components/layout/ProductAttribution';
import {
  LayoutDashboard, Users, Building2, Terminal, Package,
  FileText, Settings, LogOut, ChevronLeft, ChevronRight,
  UserCheck, RefreshCw, ListChecks, Activity, Globe
} from 'lucide-react';

const navItems = [
  { path: '/', label: 'Overview', icon: LayoutDashboard },
  { path: '/customers', label: 'Customers', icon: Building2, permissions: ['client:read', 'client:manage', 'msp:manage', 'platform:manage'] },
  { path: '/msp', label: 'MSP Workspace', icon: Globe, permissions: ['msp:manage', 'client:read', 'client:manage'] },
  { path: '/platform/msps', label: 'MSP Tenants', icon: Globe, platformOnly: true },
  { path: '/admin/users', label: 'Users', icon: Users, platformOnly: true },
  { path: '/scripts', label: 'Scripts', icon: Terminal, permissions: ['device:manage', 'job:manage'] },
  { path: '/software', label: 'Software', icon: Package, permissions: ['device:read', 'device:manage'] },
  { path: '/thirdparty', label: 'Patch Mgmt', icon: RefreshCw, permissions: ['device:manage', 'job:manage'] },
  { path: '/jobs', label: 'Jobs', icon: ListChecks, permissions: ['job:read', 'job:manage'] },
  { path: '/jobs/health', label: 'Job Health', icon: Activity, permissions: ['job:read', 'job:manage', 'platform:manage'] },
  { path: '/reports', label: 'Reports', icon: FileText, permissions: ['device:read', 'device:manage', 'platform:manage'] },
  { path: '/admin/settings', label: 'Settings', icon: Settings, permissions: ['msp:manage', 'platform:manage'] },
];

const bottomNav = [
  { path: '/settings', label: 'My Settings', icon: UserCheck },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const { user, logout } = useAuth();
  const { workspace, switchWorkspace } = useWorkspace();
  const location = useLocation();
  const navigate = useNavigate();
  const [collapsed, setCollapsed] = useState(false);

  const handleLogout = async () => {
    await logout();
    navigate('/login');
  };

  if (!user) return <>{children}</>;
  const platformRole = workspace?.platform_role ?? false;
  const permissions = new Set(workspace?.permissions ?? []);
  const visibleNavItems = navItems.filter(item => {
    if (item.platformOnly && !platformRole) return false;
    if (!item.permissions || item.permissions.length === 0 || platformRole) return true;
    return item.permissions.some(permission => permissions.has(permission));
  });
  const branding = workspace?.branding;
  const mspDisplayName = workspace?.msp_id && typeof branding?.display_name === 'string' ? branding.display_name : '';
  const displayName = mspDisplayName || workspace?.provider_display_name || 'Strata RMM';
  const sidebarBackground = typeof branding?.sidebar_bg === 'string' ? branding.sidebar_bg : undefined;
  const primaryColor = typeof branding?.primary_color === 'string' ? branding.primary_color : undefined;
  const switchableScopes = workspace?.available_scopes.filter(scope => scope.type === 'platform' || scope.type === 'msp') ?? [];

  return (
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950 flex">
      <aside
        style={{ backgroundColor: sidebarBackground }}
        className={`${collapsed ? 'w-16' : 'w-56'} bg-slate-900 text-white flex flex-col transition-all duration-200 flex-shrink-0`}
      >
        <div className="p-4 border-b border-slate-700 flex items-center justify-between">
          {!collapsed && <h2 className="font-bold text-lg truncate">{displayName}</h2>}
          <button onClick={() => setCollapsed(!collapsed)} className="text-slate-400 hover:text-white p-1">
            {collapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
          </button>
        </div>

        {!collapsed && workspace && switchableScopes.length > 0 && (
          <div className="p-2 border-b border-slate-700">
            <label htmlFor="workspace-scope" className="block text-[11px] uppercase tracking-wide text-slate-400 mb-1">
              Workspace
            </label>
            <select
              id="workspace-scope"
              value={workspace.selected_scope.id}
              onChange={event => {
                const scope = switchableScopes.find(candidate => candidate.id === event.target.value);
                if (scope?.type === 'platform') void switchWorkspace('', '', '', 'platform');
                if (scope?.type === 'msp') void switchWorkspace(scope.id, '', '', 'msp');
              }}
              className="w-full rounded bg-slate-800 border border-slate-700 px-2 py-1.5 text-xs text-white"
            >
              {switchableScopes.map(scope => (
                <option key={scope.id} value={scope.id}>{scope.name}</option>
              ))}
            </select>
          </div>
        )}

        <nav className="flex-1 p-2 space-y-1 overflow-y-auto">
          {visibleNavItems.map(item => {
            const Icon = item.icon;
            return (
              <Link
                key={item.path}
                to={item.path}
                className={`flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors ${
                  location.pathname === item.path
                    ? 'text-white'
                    : 'text-slate-300 hover:bg-slate-800 hover:text-white'
                }`}
                style={location.pathname === item.path ? { backgroundColor: primaryColor } : undefined}
                title={collapsed ? item.label : undefined}
              >
                <Icon size={18} className="flex-shrink-0" />
                {!collapsed && <span className="truncate">{item.label}</span>}
              </Link>
            );
          })}
        </nav>

        <div className="p-2 space-y-1 border-t border-slate-700/50">
          {bottomNav.map(item => {
            const Icon = item.icon;
            return (
              <Link
                key={item.path}
                to={item.path}
                className={`flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors ${
                  location.pathname === item.path
                    ? 'bg-blue-600 text-white'
                    : 'text-slate-300 hover:bg-slate-800 hover:text-white'
                }`}
                title={collapsed ? item.label : undefined}
              >
                <Icon size={18} className="flex-shrink-0" />
                {!collapsed && <span>{item.label}</span>}
              </Link>
            );
          })}
        </div>

        <ProductAttribution collapsed={collapsed} />

        <div className="p-3 border-t border-slate-700">
          <div className={`flex items-center gap-2 ${collapsed ? 'justify-center' : ''}`}>
            <div className="w-8 h-8 rounded-full bg-blue-500 flex items-center justify-center text-sm font-bold flex-shrink-0">
              {user.email[0].toUpperCase()}
            </div>
            {!collapsed && (
              <div className="flex-1 min-w-0">
                <p className="text-sm truncate">{user.email}</p>
                <p className="text-xs text-slate-400 capitalize">{workspace?.roles.join(', ') || 'authenticated'}</p>
              </div>
            )}
          </div>
          {!collapsed && (
            <div className="flex items-center justify-between mt-2">
              <button onClick={() => void handleLogout()} className="text-xs text-slate-400 hover:text-white flex items-center gap-1">
                <LogOut size={12} /> Sign Out
              </button>
              <ThemeToggle />
            </div>
          )}
          {collapsed && (
            <div className="flex flex-col items-center gap-2 mt-2">
              <ThemeToggle />
              <button onClick={handleLogout} className="text-slate-400 hover:text-white" title="Sign Out">
                <LogOut size={16} />
              </button>
            </div>
          )}
        </div>
      </aside>

      <main className="flex-1 overflow-auto">
        <div className="p-6 max-w-7xl mx-auto">{children}</div>
      </main>
    </div>
  );
}
