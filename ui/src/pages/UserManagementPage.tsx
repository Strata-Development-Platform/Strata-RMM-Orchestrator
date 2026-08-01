import { useEffect, useMemo, useState } from 'react';
import { UserRound } from 'lucide-react';
import { api } from '@/api/client';
import type { MSPTenant, ScopeType, TenantInfo, User, UserMembership } from '@/api/types';
import { EmptyState } from '@/components/shared/EmptyState';
import { useToast } from '@/components/shared/Toast';

const ROLES_BY_SCOPE: Record<ScopeType, string[]> = {
  platform: ['platform_admin', 'platform_support', 'platform_billing', 'platform_security_auditor', 'platform_viewer'],
  msp: ['msp_owner', 'msp_admin', 'msp_technician', 'msp_viewer'],
  client: ['client_admin', 'client_viewer'],
  site: ['client_admin', 'client_viewer'],
};

export default function UserManagementPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [customers, setCustomers] = useState<TenantInfo[]>([]);
  const [msps, setMSPs] = useState<MSPTenant[]>([]);
  const [platformID, setPlatformID] = useState('');
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [editingUser, setEditingUser] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([api.getUsers(), api.getCustomers(), api.getMSPs(), api.getWorkspaceContext()])
      .then(([userResponse, customerResponse, mspResponse, workspace]) => {
        setUsers(userResponse.users);
        setCustomers(customerResponse.customers.map(customer => ({ id: customer.id, name: customer.name, slug: customer.slug })));
        setMSPs(mspResponse.msps);
        setPlatformID(workspace.platform_id);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, []);

  const refreshUsers = async () => setUsers((await api.getUsers()).users);
  const editing = users.find(user => user.id === editingUser);

  if (loading) return <div className="py-12 text-center text-slate-500">Loading...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-white">User Management</h1>
          <p className="mt-1 text-sm text-slate-500">Memberships are explicit and scope-bound; legacy tenant access is display-only.</p>
        </div>
        <button onClick={() => setShowCreate(true)} className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700">
          + Create User
        </button>
      </div>

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-900">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 dark:bg-slate-800">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-slate-500">Email</th>
              <th className="px-4 py-3 text-left font-medium text-slate-500">Memberships</th>
              <th className="px-4 py-3 text-left font-medium text-slate-500">Status</th>
              <th className="px-4 py-3 text-right font-medium text-slate-500">Last Login</th>
              <th className="px-4 py-3 text-right font-medium text-slate-500"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
            {users.length === 0 ? (
              <tr><td colSpan={5} className="px-4 py-8"><EmptyState icon={UserRound} title="No users" description="Create your first scoped user" action={{ label: 'Create user', onClick: () => setShowCreate(true) }} /></td></tr>
            ) : users.map(user => (
              <tr key={user.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                <td className="px-4 py-3 font-medium">{user.email}</td>
                <td className="px-4 py-3">
                  <div className="flex flex-wrap gap-1">
                    {(user.memberships || []).map(membership => (
                      <span key={`${membership.scope_type}:${membership.scope_id}`} className="rounded bg-slate-100 px-1.5 py-0.5 text-xs dark:bg-slate-700">
                        {membership.scope_type}: {membership.role}
                      </span>
                    ))}
                    {(!user.memberships || user.memberships.length === 0) && <span className="text-xs text-slate-400">No active membership</span>}
                  </div>
                </td>
                <td className="px-4 py-3">
                  <span className={`rounded px-2 py-0.5 text-xs font-medium ${user.is_active ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'}`}>
                    {user.is_active ? 'Active' : 'Inactive'}
                  </span>
                </td>
                <td className="px-4 py-3 text-right text-slate-500">{user.last_login ? new Date(user.last_login).toLocaleDateString() : '-'}</td>
                <td className="px-4 py-3 text-right">
                  <button onClick={() => setEditingUser(editingUser === user.id ? null : user.id)} className="text-xs text-blue-600 hover:underline">
                    {editingUser === user.id ? 'Done' : 'Memberships'}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {editing && (
        <ClientMembershipEditor
          user={editing}
          customers={customers}
          onSave={() => { setEditingUser(null); void refreshUsers(); }}
        />
      )}

      {showCreate && (
        <CreateUserModal
          platformID={platformID}
          msps={msps}
          customers={customers}
          onClose={() => setShowCreate(false)}
          onCreated={() => { setShowCreate(false); void refreshUsers(); }}
        />
      )}
    </div>
  );
}

function ClientMembershipEditor({ user, customers, onSave }: { user: User; customers: TenantInfo[]; onSave: () => void }) {
  const { showToast } = useToast();
  const fixedMemberships = useMemo(() => (user.memberships || []).filter(membership => membership.scope_type !== 'client'), [user.memberships]);
  const currentClientIDs = (user.memberships || []).filter(membership => membership.scope_type === 'client').map(membership => membership.scope_id);
  const [selected, setSelected] = useState(currentClientIDs);
  const [role, setRole] = useState<'client_admin' | 'client_viewer'>('client_viewer');
  const [saving, setSaving] = useState(false);

  const toggle = (id: string) => setSelected(previous => previous.includes(id) ? previous.filter(value => value !== id) : [...previous, id]);
  const handleSave = async () => {
    setSaving(true);
    const memberships: UserMembership[] = [
      ...fixedMemberships.map(({ scope_type, scope_id, role: fixedRole }) => ({ scope_type, scope_id, role: fixedRole })),
      ...selected.map(scope_id => ({ scope_type: 'client' as const, scope_id, role })),
    ];
    try {
      await api.updateUserMemberships(user.id, memberships);
      showToast('success', 'Scoped memberships updated');
      onSave();
    } catch (caught) {
      showToast('error', `Failed to update: ${caught instanceof Error ? caught.message : 'unknown error'}`);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900">
      <div className="mb-3 flex items-center justify-between gap-4">
        <h3 className="font-semibold">Client memberships for {user.email}</h3>
        <select value={role} onChange={event => setRole(event.target.value as typeof role)} className="rounded-md border px-2 py-1.5 text-sm dark:bg-slate-800">
          <option value="client_viewer">Client viewer</option>
          <option value="client_admin">Client administrator</option>
        </select>
      </div>
      <div className="mb-4 grid grid-cols-2 gap-2 md:grid-cols-3">
        {customers.map(customer => (
          <label key={customer.id} className="flex cursor-pointer items-center gap-2 rounded p-2 hover:bg-slate-50 dark:hover:bg-slate-800">
            <input type="checkbox" checked={selected.includes(customer.id)} onChange={() => toggle(customer.id)} />
            <span className="text-sm">{customer.name}</span>
          </label>
        ))}
      </div>
      <button onClick={() => void handleSave()} disabled={saving || selected.length + fixedMemberships.length === 0} className="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-50">
        {saving ? 'Saving...' : 'Save memberships'}
      </button>
    </div>
  );
}

function CreateUserModal({ platformID, msps, customers, onClose, onCreated }: {
  platformID: string;
  msps: MSPTenant[];
  customers: TenantInfo[];
  onClose: () => void;
  onCreated: () => void;
}) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [scopeType, setScopeType] = useState<Exclude<ScopeType, 'site'>>('msp');
  const [scopeID, setScopeID] = useState(msps[0]?.id || '');
  const [role, setRole] = useState('msp_viewer');
  const [error, setError] = useState('');
  const [creating, setCreating] = useState(false);

  const scopes = scopeType === 'platform'
    ? [{ id: platformID, name: 'Provider platform' }]
    : scopeType === 'msp'
      ? msps
      : customers;
  const roles = ROLES_BY_SCOPE[scopeType];

  const changeScopeType = (next: Exclude<ScopeType, 'site'>) => {
    setScopeType(next);
    const nextScopes = next === 'platform' ? [{ id: platformID }] : next === 'msp' ? msps : customers;
    setScopeID(nextScopes[0]?.id || '');
    setRole(ROLES_BY_SCOPE[next][0]);
  };

  const handleCreate = async (event: React.FormEvent) => {
    event.preventDefault();
    setError('');
    setCreating(true);
    try {
      await api.createUser(email, password, { scope_type: scopeType, scope_id: scopeID, role });
      onCreated();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'creation failed');
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div className="mx-4 max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-lg bg-white shadow-xl dark:bg-slate-900" onClick={event => event.stopPropagation()}>
        <form onSubmit={handleCreate} className="space-y-4 p-6">
          <h2 className="text-lg font-bold">Create scoped user</h2>
          <label className="block text-sm font-medium">Email
            <input type="email" value={email} onChange={event => setEmail(event.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 dark:bg-slate-800" required />
          </label>
          <label className="block text-sm font-medium">Temporary password
            <input type="password" value={password} onChange={event => setPassword(event.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 dark:bg-slate-800" required minLength={12} />
          </label>
          <label className="block text-sm font-medium">Scope type
            <select value={scopeType} onChange={event => changeScopeType(event.target.value as Exclude<ScopeType, 'site'>)} className="mt-1 w-full rounded-md border px-3 py-2 dark:bg-slate-800">
              <option value="platform">Platform</option>
              <option value="msp">MSP</option>
              <option value="client">Client</option>
            </select>
          </label>
          <label className="block text-sm font-medium">Exact scope
            <select value={scopeID} onChange={event => setScopeID(event.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 dark:bg-slate-800" required>
              {scopes.map(scope => <option key={scope.id} value={scope.id}>{scope.name}</option>)}
            </select>
          </label>
          <label className="block text-sm font-medium">Role
            <select value={role} onChange={event => setRole(event.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 dark:bg-slate-800">
              {roles.map(value => <option key={value} value={value}>{value.replace(/_/g, ' ')}</option>)}
            </select>
          </label>
          {error && <p role="alert" className="text-sm text-red-500">{error}</p>}
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="rounded-md border px-4 py-2 text-sm hover:bg-slate-50 dark:hover:bg-slate-800">Cancel</button>
            <button type="submit" disabled={creating || !scopeID} className="rounded-md bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-50">
              {creating ? 'Creating...' : 'Create user'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
