import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '@/hooks/useAuth';
import { ThemeToggle } from '@/components/shared/ThemeToggle';
import { useState } from 'react';
import {
  LayoutDashboard, Users, Building2, Terminal, Package,
  FileText, Settings, LogOut, ChevronLeft, ChevronRight,
  UserCheck, RefreshCw, ListChecks, Activity, Globe
} from 'lucide-react';

const navItems = [
  { path: '/', label: 'Overview', icon: LayoutDashboard },
  { path: '/customers', label: 'Customers', icon: Building2 },
  { path: '/platform/msps', label: 'MSP Tenants', icon: Globe },
  { path: '/admin/users', label: 'Users', icon: Users },
  { path: '/scripts', label: 'Scripts', icon: Terminal },
  { path: '/software', label: 'Software', icon: Package },
  { path: '/thirdparty', label: 'Patch Mgmt', icon: RefreshCw },
  { path: '/jobs', label: 'Jobs', icon: ListChecks },
  { path: '/jobs/health', label: 'Job Health', icon: Activity },
  { path: '/reports', label: 'Reports', icon: FileText },
  { path: '/admin/settings', label: 'Settings', icon: Settings },
];

const bottomNav = [
  { path: '/settings', label: 'My Settings', icon: UserCheck },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const { user, logout } = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const [collapsed, setCollapsed] = useState(false);

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  if (!user) return <>{children}</>;

  return (
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950 flex">
      <aside className={`${collapsed ? 'w-16' : 'w-56'} bg-slate-900 text-white flex flex-col transition-all duration-200 flex-shrink-0`}>
        <div className="p-4 border-b border-slate-700 flex items-center justify-between">
          {!collapsed && <h2 className="font-bold text-lg">Strata RMM</h2>}
          <button onClick={() => setCollapsed(!collapsed)} className="text-slate-400 hover:text-white p-1">
            {collapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
          </button>
        </div>

        <nav className="flex-1 p-2 space-y-1 overflow-y-auto">
          {navItems.map(item => {
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

        <div className="p-3 border-t border-slate-700">
          <div className={`flex items-center gap-2 ${collapsed ? 'justify-center' : ''}`}>
            <div className="w-8 h-8 rounded-full bg-blue-500 flex items-center justify-center text-sm font-bold flex-shrink-0">
              {user.email[0].toUpperCase()}
            </div>
            {!collapsed && (
              <div className="flex-1 min-w-0">
                <p className="text-sm truncate">{user.email}</p>
                <p className="text-xs text-slate-400 capitalize">{user.role}</p>
              </div>
            )}
          </div>
          {!collapsed && (
            <div className="flex items-center justify-between mt-2">
              <button onClick={handleLogout} className="text-xs text-slate-400 hover:text-white flex items-center gap-1">
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
