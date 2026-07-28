import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider, useAuth } from '@/hooks/useAuth';
import { WorkspaceProvider } from '@/hooks/useWorkspace';
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

function AppRoutes() {
  const { user, loading } = useAuth();

  if (loading) return <div className="min-h-screen flex items-center justify-center text-slate-500">Loading...</div>;

  return (
    <Routes>
      <Route path="/login" element={user ? <Navigate to="/" replace /> : <LoginPage />} />
      <Route path="/" element={<ProtectedRoute><DashboardPage /></ProtectedRoute>} />
      <Route path="/customers" element={<ProtectedRoute><CustomersPage /></ProtectedRoute>} />
      <Route path="/customers/:id" element={<ProtectedRoute><CustomerDetailPage /></ProtectedRoute>} />
      <Route path="/admin/users" element={<ProtectedRoute><UserManagementPage /></ProtectedRoute>} />
      <Route path="/admin/settings" element={<ProtectedRoute><AdminSettingsPage /></ProtectedRoute>} />
      <Route path="/settings" element={<ProtectedRoute><SettingsPage /></ProtectedRoute>} />
      <Route path="/scripts" element={<ProtectedRoute><ScriptsPage /></ProtectedRoute>} />
      <Route path="/software" element={<ProtectedRoute><SoftwarePage /></ProtectedRoute>} />
      <Route path="/thirdparty" element={<ProtectedRoute><ThirdPartyPage /></ProtectedRoute>} />
      <Route path="/reports" element={<ProtectedRoute><ReportsPage /></ProtectedRoute>} />
      <Route path="/jobs" element={<ProtectedRoute><JobsPage /></ProtectedRoute>} />
      <Route path="/jobs/health" element={<ProtectedRoute><JobHealthPage /></ProtectedRoute>} />
      <Route path="/platform/msps" element={<PlatformRoute><MSPListPage /></PlatformRoute>} />
      <Route path="/msp" element={<ProtectedRoute><MSPWorkspacePage /></ProtectedRoute>} />
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
