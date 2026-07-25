import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '@/hooks/useAuth';
import { useState } from 'react';

const navItems = [
  { path: '/', label: 'Overview', icon: '⊞' },
  { path: '/customers', label: 'Customers', icon: '🏢' },
  { path: '/admin/users', label: 'Users', icon: '👤' },
  { path: '/scripts', label: 'Scripts', icon: '▶' },
  { path: '/admin/settings', label: 'Settings', icon: '⚙' },
];

const bottomNav = [
  { path: '/settings', label: 'My Settings', icon: '🔐' },
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
      <aside className={`${collapsed ? 'w-16' : 'w-56'} bg-slate-900 text-white flex flex-col transition-all duration-200`}>
        <div className="p-4 border-b border-slate-700">
          <h2 className={`font-bold text-lg ${collapsed ? 'hidden' : 'block'}`}>Strata RMM</h2>
          <button onClick={() => setCollapsed(!collapsed)} className="text-slate-400 hover:text-white text-sm mt-1">
            {collapsed ? '→' : '←'}
          </button>
        </div>

        <nav className="flex-1 p-2 space-y-1">
          {navItems.map(item => (
            <Link
              key={item.path}
              to={item.path}
              className={`flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors ${
                location.pathname === item.path
                  ? 'bg-blue-600 text-white'
                  : 'text-slate-300 hover:bg-slate-800 hover:text-white'
              }`}
            >
              <span className="text-lg">{item.icon}</span>
              {!collapsed && <span>{item.label}</span>}
            </Link>
          ))}
        </nav>

        <div className="p-2 space-y-1">
          {bottomNav.map(item => (
            <Link
              key={item.path}
              to={item.path}
              className={`flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors ${
                location.pathname === item.path
                  ? 'bg-blue-600 text-white'
                  : 'text-slate-300 hover:bg-slate-800 hover:text-white'
              }`}
            >
              <span className="text-lg">{item.icon}</span>
              {!collapsed && <span>{item.label}</span>}
            </Link>
          ))}
        </div>
        <div className="p-3 border-t border-slate-700">
          <div className={`flex items-center gap-2 ${collapsed ? 'justify-center' : ''}`}>
            <div className="w-8 h-8 rounded-full bg-blue-500 flex items-center justify-center text-sm font-bold">
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
            <button onClick={handleLogout} className="mt-2 text-xs text-slate-400 hover:text-white w-full text-left">
              Sign Out
            </button>
          )}
        </div>
      </aside>

      <main className="flex-1 overflow-auto">
        <div className="p-6 max-w-7xl mx-auto">{children}</div>
      </main>
    </div>
  );
}
