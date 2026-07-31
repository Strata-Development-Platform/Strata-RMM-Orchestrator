import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
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

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return <div className="min-h-screen flex items-center justify-center text-slate-500">Loading...</div>;
  if (!user) return <Navigate to="/login" replace />;
  return <Layout>{children}</Layout>;
}

function PlatformRoute({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return <div className="min-h-screen flex items-center justify-center text-slate-500">Loading...</div>;
  if (!user) return <Navigate to="/login" replace />;
  if (!['platform_owner', 'platform_admin'].includes(user.role)) {
    return <Navigate to="/" replace />;
  }
  return <Layout>{children}</Layout>;
}

function CapabilityRoute({ permissions, children }: { permissions: string[]; children: React.ReactNode }) {
  const { user, loading: authLoading } = useAuth();
  const { workspace, loading: workspaceLoading } = useWorkspace();
  if (authLoading || workspaceLoading) {
    return <div className="min-h-screen flex items-center justify-center text-slate-500">Loading...</div>;
  }
  if (!user) return <Navigate to="/login" replace />;
  const platformRole = ['platform_owner', 'platform_admin'].includes(user.role);
  const allowed = platformRole || permissions.some(permission => workspace?.permissions.includes(permission));
  if (!allowed) return <Navigate to="/" replace />;
  return <Layout>{children}</Layout>;
}

function AppRoutes() {
  const { user, loading } = useAuth();

  if (loading) return <div className="min-h-screen flex items-center justify-center text-slate-500">Loading...</div>;

  return (
    <Routes>
      <Route path="/login" element={user ? <Navigate to="/" replace /> : <LoginPage />} />
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

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <WorkspaceProvider>
          <AppRoutes />
        </WorkspaceProvider>
      </AuthProvider>
    </BrowserRouter>
  );
}
