import { useState, useEffect } from 'react';
import { api } from '@/api/client';
import { useToast } from '@/components/shared/Toast';
import { Skeleton } from '@/components/shared/Skeleton';
import { EmptyState } from '@/components/shared/EmptyState';
import type { User, TenantInfo } from '@/api/types';

export default function UserManagementPage() {
  const { showToast } = useToast();
  const [users, setUsers] = useState<User[]>([]);
  const [customers, setCustomers] = useState<TenantInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [editingUser, setEditingUser] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([api.getUsers(), api.getCustomers()])
      .then(([u, c]) => {
        setUsers(u.users);
        setCustomers(c.customers.map(cust => ({ id: cust.id, name: cust.name, slug: cust.slug })));
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, []);

  const refreshUsers = async () => {
    const u = await api.getUsers();
    setUsers(u.users);
  };

  if (loading) return <div className="text-center py-12 text-slate-500">Loading...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">User Management</h1>
        <button
          onClick={() => setShowCreate(true)}
          className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded-md hover:bg-blue-700"
        >
          + Create User
        </button>
      </div>

      <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 dark:bg-slate-800">
            <tr>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Email</th>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Role</th>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Status</th>
              <th className="text-left px-4 py-3 font-medium text-slate-500">Tenants</th>
              <th className="text-right px-4 py-3 font-medium text-slate-500">Last Login</th>
              <th className="text-right px-4 py-3 font-medium text-slate-500"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
            {users.length === 0 ? (
              <tr><td colSpan={6} className="px-4 py-8"><EmptyState icon="👤" title="No users" description="Create your first user to get started" action={{ label: '+ Create User', onClick: () => setShowCreate(true) }} /></td></tr>
            ) : users.map(u => (
              <tr key={u.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                <td className="px-4 py-3 font-medium">{u.email}</td>
                <td className="px-4 py-3 capitalize">{u.role}</td>
                <td className="px-4 py-3">
                  <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                    u.is_active ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'
                  }`}>{u.is_active ? 'Active' : 'Inactive'}</span>
                </td>
                <td className="px-4 py-3">
                  <div className="flex flex-wrap gap-1">
                    {(u.accessible_tenants || []).map(t => (
                      <span key={t.id} className="px-1.5 py-0.5 bg-slate-100 dark:bg-slate-700 rounded text-xs">
                        {t.name}
                      </span>
                    ))}
                    {(!u.accessible_tenants || u.accessible_tenants.length === 0) && u.role === 'admin' && (
                      <span className="text-xs text-slate-400">All tenants</span>
                    )}
                  </div>
                </td>
                <td className="px-4 py-3 text-right text-slate-500">
                  {u.last_login ? new Date(u.last_login).toLocaleDateString() : '-'}
                </td>
                <td className="px-4 py-3 text-right">
                  <button
                    onClick={() => setEditingUser(editingUser === u.id ? null : u.id)}
                    className="text-blue-600 hover:underline text-xs"
                  >
                    {editingUser === u.id ? 'Done' : 'Scope'}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Inline tenant scoping */}
      {editingUser && users.find(u => u.id === editingUser) && (
        <TenantScoper
          userId={editingUser}
          user={users.find(u => u.id === editingUser)!}
          customers={customers}
          onSave={() => { setEditingUser(null); refreshUsers(); }}
        />
      )}

      {/* Create User Modal */}
      {showCreate && (
        <CreateUserModal
          customers={customers}
          onClose={() => setShowCreate(false)}
          onCreated={() => { setShowCreate(false); refreshUsers(); }}
        />
      )}
    </div>
  );
}

function TenantScoper({ userId, user, customers, onSave }: {
  userId: string;
  user: User;
  customers: TenantInfo[];
  onSave: () => void;
}) {
  const { showToast } = useToast();
  const currentIds = (user.accessible_tenants || []).map(t => t.id);
  const [selected, setSelected] = useState<string[]>(currentIds);
  const [saving, setSaving] = useState(false);

  const toggle = (id: string) => {
    setSelected(prev => prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]);
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await api.updateUserTenants(userId, selected);
      onSave();
      showToast('success', 'User tenant access updated');
    } catch (e) {
      showToast('error', 'Failed to update: ' + (e instanceof Error ? e.message : 'unknown'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
      <h3 className="font-semibold mb-3">Scoped Tenants for {user.email}</h3>
      <div className="grid grid-cols-2 md:grid-cols-3 gap-2 mb-4">
        {customers.map(c => (
          <label key={c.id} className="flex items-center gap-2 p-2 rounded hover:bg-slate-50 dark:hover:bg-slate-800 cursor-pointer">
            <input
              type="checkbox" checked={selected.includes(c.id)}
              onChange={() => toggle(c.id)}
              className="rounded border-slate-300"
            />
            <span className="text-sm">{c.name}</span>
          </label>
        ))}
      </div>
      <div className="flex gap-2">
        <button onClick={handleSave} disabled={saving}
          className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded hover:bg-blue-700 disabled:opacity-50">
          {saving ? 'Saving...' : 'Save'}
        </button>
      </div>
    </div>
  );
}

function CreateUserModal({ customers, onClose, onCreated }: {
  customers: TenantInfo[];
  onClose: () => void;
  onCreated: () => void;
}) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState('technician');
  const [tenantIDs, setTenantIDs] = useState<string[]>([]);
  const [error, setError] = useState('');
  const [creating, setCreating] = useState(false);

  const toggle = (id: string) => {
    setTenantIDs(prev => prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]);
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setCreating(true);
    try {
      await api.createUser(email, password, role, tenantIDs);
      onCreated();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'creation failed');
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <form onSubmit={handleCreate} className="p-6 space-y-4">
          <h2 className="text-lg font-bold">Create User</h2>

          <div>
            <label className="block text-sm font-medium mb-1">Email</label>
            <input type="email" value={email} onChange={e => setEmail(e.target.value)}
              className="w-full px-3 py-2 border rounded-md dark:bg-slate-800" required />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Password</label>
            <input type="password" value={password} onChange={e => setPassword(e.target.value)}
              className="w-full px-3 py-2 border rounded-md dark:bg-slate-800" required minLength={8} />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Role</label>
            <select value={role} onChange={e => setRole(e.target.value)}
              className="w-full px-3 py-2 border rounded-md dark:bg-slate-800">
              <option value="admin">Admin</option>
              <option value="technician">Technician</option>
              <option value="viewer">Viewer</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Tenant Access</label>
            <div className="grid grid-cols-2 gap-2 max-h-48 overflow-y-auto border rounded-md p-2">
              {customers.map(c => (
                <label key={c.id} className="flex items-center gap-2 text-sm cursor-pointer">
                  <input type="checkbox" checked={tenantIDs.includes(c.id)} onChange={() => toggle(c.id)} />
                  {c.name}
                </label>
              ))}
            </div>
          </div>

          {error && <p className="text-red-500 text-sm">{error}</p>}

          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="px-4 py-2 text-sm rounded-md border hover:bg-slate-50 dark:hover:bg-slate-800">Cancel</button>
            <button type="submit" disabled={creating}
              className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700 disabled:opacity-50">
              {creating ? 'Creating...' : 'Create User'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
