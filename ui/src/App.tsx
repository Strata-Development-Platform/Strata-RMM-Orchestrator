import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom';
import { AuthProvider, useAuth } from '@/hooks/useAuth';
import { WorkspaceProvider, useWorkspace } from '@/hooks/useWorkspace';
import Layout from '@/components/layout/Layout';
import LoginPage from '@/pages/LoginPage';
import DashboardPage from '@/pages/DashboardPage';
import CustomerDetailPage from '@/pages/CustomerDetailPage';
import CustomersPage from '@/pages/CustomersPage';
import UserManagementPage from '@/pages/UserManagementPage';
import AdminSettingsPage from '@/pages/AdminSettingsPage';
import SettingsPage from '@/pages/SettingsPage';
import ScriptsPage from '@/pages/ScriptsPage';
import SoftwarePage from '@/pages/SoftwarePage';
import ThirdPartyPage from '@/pages/ThirdPartyPage';
import ReportsPage from '@/pages/ReportsPage';
import JobsPage from '@/pages/JobsPage';
import JobHealthPage from '@/pages/JobHealthPage';
import MSPListPage from '@/pages/MSPListPage';
import DeviceWorkspacePage from '@/pages/DeviceWorkspacePage';
import DeviceRemotePage from '@/pages/DeviceRemotePage';
import MSPWorkspacePage from '@/pages/MSPWorkspacePage';
import LegalPage from '@/pages/LegalPage';
import ProviderSetupPage from '@/pages/ProviderSetupPage';
import ActivateAccountPage from '@/pages/ActivateAccountPage';
import { ToastProvider } from '@/components/shared/Toast';

function LoadingScreen() {
  return <div className="min-h-screen flex items-center justify-center text-slate-500 dark:text-slate-400">Loading...</div>;
}

function WorkspaceError() {
  const { refresh } = useWorkspace();
  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-50 p-6 dark:bg-slate-950">
      <div role="alert" className="max-w-md rounded-lg border border-red-200 bg-white p-6 text-center dark:border-red-900 dark:bg-slate-900">
        <h1 className="text-lg font-semibold text-slate-900 dark:text-white">Workspace unavailable</h1>
        <p className="mt-2 text-sm text-slate-600 dark:text-slate-400">We could not load your server workspace context.</p>
        <button onClick={() => void refresh().catch(() => undefined)} className="mt-4 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700">
          Try again
        </button>
      </div>
    </div>
  );
}

function isProviderAdministrator(platformRole: boolean | undefined) {
  return platformRole ?? false;
}

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { user, loading: authLoading } = useAuth();
  const { workspace, loading: workspaceLoading, error } = useWorkspace();
  if (authLoading || (user && workspaceLoading)) return <LoadingScreen />;
  if (!user) return <Navigate to="/login" replace />;
  if (error || !workspace) return <WorkspaceError />;
  if (isProviderAdministrator(workspace.platform_role) && !workspace.setup_complete) {
    return <Navigate to="/provider/setup" replace />;
  }
  return <Layout>{children}</Layout>;
}

function PlatformRoute({ children }: { children: React.ReactNode }) {
  const { user, loading: authLoading } = useAuth();
  const { workspace, loading: workspaceLoading, error } = useWorkspace();
  if (authLoading || (user && workspaceLoading)) return <LoadingScreen />;
  if (!user) return <Navigate to="/login" replace />;
  if (error || !workspace) return <WorkspaceError />;
  if (!isProviderAdministrator(workspace.platform_role)) return <Navigate to="/" replace />;
  if (!workspace.setup_complete) return <Navigate to="/provider/setup" replace />;
  return <Layout>{children}</Layout>;
}

function CapabilityRoute({ permissions, children }: { permissions: string[]; children: React.ReactNode }) {
  const { user, loading: authLoading } = useAuth();
  const { workspace, loading: workspaceLoading, error } = useWorkspace();
  if (authLoading || workspaceLoading) {
    return <LoadingScreen />;
  }
  if (!user) return <Navigate to="/login" replace />;
  if (error || !workspace) return <WorkspaceError />;
  const platformRole = isProviderAdministrator(workspace.platform_role);
  if (platformRole && !workspace.setup_complete) return <Navigate to="/provider/setup" replace />;
  const allowed = platformRole || permissions.some(permission => workspace?.permissions.includes(permission));
  if (!allowed) return <Navigate to="/" replace />;
  return <Layout>{children}</Layout>;
}

function ProviderSetupRoute() {
  const { user, loading: authLoading } = useAuth();
  const { workspace, loading: workspaceLoading, error } = useWorkspace();
  if (authLoading || (user && workspaceLoading)) return <LoadingScreen />;
  if (!user) return <Navigate to="/login" replace />;
  if (error || !workspace) return <WorkspaceError />;
  if (!isProviderAdministrator(workspace.platform_role) || workspace.setup_complete) {
    return <Navigate to="/" replace />;
  }
  return <ProviderSetupPage />;
}

function AppRoutes() {
  const { user, loading } = useAuth();

  if (loading) return <LoadingScreen />;

  return (
    <Routes>
      <Route path="/login" element={user ? <Navigate to="/" replace /> : <LoginPage />} />
      <Route path="/provider/setup" element={<ProviderSetupRoute />} />
      <Route path="/" element={<ProtectedRoute><DashboardPage /></ProtectedRoute>} />
      <Route path="/legal" element={<LegalPage />} />
      <Route path="/customers" element={<CapabilityRoute permissions={['client:read', 'client:manage', 'msp:manage']}><CustomersPage /></CapabilityRoute>} />
      <Route path="/customers/:id" element={<CapabilityRoute permissions={['client:read', 'client:manage', 'msp:manage']}><CustomerDetailPage /></CapabilityRoute>} />
      <Route path="/admin/users" element={<PlatformRoute><UserManagementPage /></PlatformRoute>} />
      <Route path="/admin/settings" element={<CapabilityRoute permissions={['msp:manage', 'platform:manage']}><AdminSettingsPage /></CapabilityRoute>} />
      <Route path="/settings" element={<ProtectedRoute><SettingsPage /></ProtectedRoute>} />
      <Route path="/scripts" element={<CapabilityRoute permissions={['device:manage', 'job:manage']}><ScriptsPage /></CapabilityRoute>} />
      <Route path="/software" element={<CapabilityRoute permissions={['device:read', 'device:manage']}><SoftwarePage /></CapabilityRoute>} />
      <Route path="/thirdparty" element={<CapabilityRoute permissions={['device:manage', 'job:manage']}><ThirdPartyPage /></CapabilityRoute>} />
      <Route path="/reports" element={<CapabilityRoute permissions={['device:read', 'device:manage']}><ReportsPage /></CapabilityRoute>} />
      <Route path="/jobs" element={<CapabilityRoute permissions={['job:read', 'job:manage']}><JobsPage /></CapabilityRoute>} />
      <Route path="/jobs/health" element={<CapabilityRoute permissions={['job:read', 'job:manage', 'platform:manage']}><JobHealthPage /></CapabilityRoute>} />
      <Route path="/platform/msps" element={<PlatformRoute><MSPListPage /></PlatformRoute>} />
      <Route path="/msp" element={<CapabilityRoute permissions={['msp:manage', 'client:read', 'client:manage']}><MSPWorkspacePage /></CapabilityRoute>} />
      <Route path="/devices/:deviceID" element={<ProtectedRoute><DeviceWorkspacePage /></ProtectedRoute>} />
      <Route path="/remote/:tid/:did" element={<ProtectedRoute><DeviceRemotePage /></ProtectedRoute>} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

function AppContent() {
  const location = useLocation();
  if (location.pathname === '/activate-account') return <ActivateAccountPage />;

  return (
    <AuthProvider>
      <WorkspaceProvider>
        <AppRoutes />
      </WorkspaceProvider>
    </AuthProvider>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <ToastProvider>
        <AppContent />
      </ToastProvider>
    </BrowserRouter>
  );
}
