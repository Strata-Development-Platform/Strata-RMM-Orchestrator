import { useState, useCallback, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useWorkspace } from '@/hooks/useWorkspace';
import { useAuth } from '@/hooks/useAuth';
import { api } from '@/api/client';
import type { DeviceGroup, DeviceGroupMember, DeviceGroupScriptBinding, DeviceGroupEvaluationStatus } from '@/api/types';
import { useToast } from '@/components/shared/Toast';
import { Skeleton } from '@/components/shared/Skeleton';
import SmartGroupExpressionBuilder from './SmartGroupExpressionBuilder';
import type { SmartGroupExpression, SmartGroupFilter } from './SmartGroupTypes';

type TabId = 'overview' | 'members' | 'bindings' | 'expression';

interface SmartGroupDetailPageProps {
  groupId?: string;
}

export default function SmartGroupDetailPage({ groupId }: SmartGroupDetailPageProps) {
  const { showToast } = useToast();
  const navigate = useNavigate();
  const { workspace } = useWorkspace();
  const { user } = useAuth();
  const [activeTab, setActiveTab] = useState<TabId>('overview');
  const [group, setGroup] = useState<DeviceGroup | null>(null);
  const [members, setMembers] = useState<DeviceGroupMember[]>([]);
  const [bindings, setBindings] = useState<DeviceGroupScriptBinding[]>([]);
  const [evalStatus, setEvalStatus] = useState<DeviceGroupEvaluationStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [evaluating, setEvaluating] = useState(false);
  const [revealedBindings, setRevealedBindings] = useState<Set<string>>(new Set());
  const [editingName, setEditingName] = useState(false);
  const [groupName, setGroupName] = useState('');
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const mspID = workspace?.msp_id || '';
  const clientID = workspace?.client_id || '';

  const loadGroup = useCallback(async () => {
    if (!groupId) {
      setError('No group ID provided');
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const [detail, membersRes, bindingsRes] = await Promise.all([
        api.getDeviceGroupDetail(groupId),
        api.getDeviceGroupMembers(groupId).catch(() => ({ members: [], total: 0 })),
        api.listDeviceGroupBindings(groupId).catch(() => ({ bindings: [] })),
      ]);
      setGroup(detail);
      setMembers(membersRes.members);
      setBindings(bindingsRes.bindings);
      setGroupName(detail.name);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load group';
      setError(message);
      showToast('error', message);
    } finally {
      setLoading(false);
    }
  }, [groupId, showToast]);

  useEffect(() => {
    loadGroup();
  }, [loadGroup]);

  const handleCopyId = useCallback((id: string) => {
    navigator.clipboard.writeText(id);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  }, []);

  const handleRevealBinding = useCallback((id: string) => {
    setRevealedBindings((prev) => new Set(prev).add(id));
  }, []);

  const handleExpressionChange = useCallback((expr: SmartGroupExpression) => {
    console.log('Expression changed:', expr);
  }, []);

  const handleEvaluate = useCallback(async () => {
    if (!groupId) return;
    setEvaluating(true);
    try {
      const result = await api.triggerGroupEvaluation(groupId);
      showToast('success', `Evaluation started: ${result.evaluation_id}`);
      setTimeout(() => {
        api.getEvaluationStatus(groupId).then((status) => {
          setEvalStatus(status);
          if (status.status === 'completed') {
            loadGroup();
          }
        }).catch(() => {});
      }, 2000);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to evaluate group';
      showToast('error', message);
    } finally {
      setEvaluating(false);
    }
  }, [groupId, showToast, loadGroup]);

  const handleNameSave = useCallback(async () => {
    if (!groupId || !groupName.trim()) return;
    try {
      await api.updateDeviceGroup(groupId, { name: groupName.trim() });
      showToast('success', 'Group name updated');
      setEditingName(false);
      loadGroup();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to update group name';
      showToast('error', message);
    }
  }, [groupId, groupName, showToast, loadGroup]);

  const handleDeleteGroup = useCallback(async () => {
    if (!groupId || !window.confirm('Delete this group? This action cannot be undone.')) return;
    try {
      await api.deleteDeviceGroup(groupId);
      showToast('success', 'Group deleted');
      navigate('/groups');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to delete group';
      showToast('error', message);
    }
  }, [groupId, showToast, navigate]);

  const handleUnbindScript = useCallback(async (bindingId: string) => {
    if (!groupId) return;
    try {
      await api.unbindScriptFromGroup(groupId, bindingId);
      showToast('success', 'Script binding removed');
      loadGroup();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to remove binding';
      showToast('error', message);
    }
  }, [groupId, showToast, loadGroup]);

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton type="text" count={2} />
        <Skeleton type="card" count={4} />
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 p-6 text-center">
        <p className="text-red-700 dark:text-red-400 font-medium">Failed to load group</p>
        <p className="text-sm text-red-600 dark:text-red-500 mt-1">{error}</p>
        <button
          onClick={loadGroup}
          className="mt-3 px-4 py-2 text-sm font-medium text-white bg-red-600 rounded-md hover:bg-red-700"
        >
          Try again
        </button>
      </div>
    );
  }

  if (!group) {
    return (
      <div className="text-center py-12">
        <p className="text-slate-500 dark:text-slate-400">Group not found</p>
        <button
          onClick={() => navigate('/groups')}
          className="mt-3 px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700"
        >
          Back to Groups
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          {editingName ? (
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={groupName}
                onChange={(e) => setGroupName(e.target.value)}
                className="text-xl font-bold text-slate-900 dark:text-slate-100 border-b-2 border-blue-500 bg-transparent outline-none"
                autoFocus
              />
              <button
                onClick={handleNameSave}
                className="text-green-600 dark:text-green-400"
              >
                Save
              </button>
              <button
                onClick={() => { setEditingName(false); setGroupName(group.name); }}
                className="text-red-600 dark:text-red-400"
              >
                Cancel
              </button>
            </div>
          ) : (
            <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">
              {group.name}
            </h1>
          )}
          <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
            {group.description || 'Smart group detail'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setEditingName(true)}
            className="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
            title="Edit group name"
          >
            <span className="text-xs">Edit</span>
          </button>
          <button
            onClick={handleDeleteGroup}
            className="p-2 text-red-400 hover:text-red-600 dark:hover:text-red-300"
            title="Delete group"
          >
            <span className="text-xs">Delete</span>
          </button>
          <button
            onClick={handleEvaluate}
            disabled={evaluating}
            className={`flex items-center gap-1 px-3 py-1.5 text-sm font-medium rounded-md ${
              evaluating
                ? 'text-slate-400 bg-slate-100 dark:bg-slate-700 cursor-not-allowed'
                : 'text-green-600 dark:text-green-400 bg-green-50 dark:bg-green-900/20 hover:bg-green-100 dark:hover:bg-green-900/30'
            }`}
          >
            {evaluating ? 'Evaluating...' : 'Evaluate Now'}
          </button>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-4 gap-4">
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {group.member_count}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">Members</p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {group.is_smart ? 'Smart' : 'Static'}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">Group Type</p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {evalStatus?.status || 'idle'}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">Status</p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {bindings.length}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">Script Bindings</p>
        </div>
      </div>

      {/* Tabs */}
      <div className="border-b border-slate-200 dark:border-slate-700">
        <div className="flex gap-1">
          {(['overview', 'members', 'bindings', 'expression'] as TabId[]).map((tab) => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={`px-4 py-2 text-sm font-medium border-b-2 ${
                activeTab === tab
                  ? 'border-blue-600 text-blue-600 dark:text-blue-400 dark:border-blue-400'
                  : 'border-transparent text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-300'
              }`}
            >
              {tab.charAt(0).toUpperCase() + tab.slice(1)}
            </button>
          ))}
        </div>
      </div>

      {/* Tab Content */}
      {activeTab === 'overview' && (
        <div className="space-y-4">
          <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-2">
              Group Details
            </h3>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="text-xs text-slate-500 dark:text-slate-400">ID</label>
                <div className="flex items-center gap-2 mt-1">
                  <code className="text-sm bg-slate-100 dark:bg-slate-700 px-2 py-1 rounded">
                    {group.id}
                  </code>
                  <button
                    onClick={() => handleCopyId(group.id)}
                    className="text-slate-400 hover:text-slate-600"
                  >
                    {copiedId === group.id ? '✓' : '📋'}
                  </button>
                </div>
              </div>
              <div>
                <label className="text-xs text-slate-500 dark:text-slate-400">Client</label>
                <p className="text-sm text-slate-900 dark:text-slate-100 mt-1">
                  {group.client_id}
                </p>
              </div>
              <div>
                <label className="text-xs text-slate-500 dark:text-slate-400">Created</label>
                <p className="text-sm text-slate-900 dark:text-slate-100 mt-1">
                  {new Date(group.created_at).toLocaleString()}
                </p>
              </div>
              <div>
                <label className="text-xs text-slate-500 dark:text-slate-400">Last Evaluated</label>
                <p className="text-sm text-slate-900 dark:text-slate-100 mt-1">
                  {group.last_evaluated ? new Date(group.last_evaluated).toLocaleString() : 'Never'}
                </p>
              </div>
            </div>
          </div>
        </div>
      )}

      {activeTab === 'members' && (
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800">
          <div className="p-4 border-b border-slate-200 dark:border-slate-700">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                Group Members ({members.length})
              </h3>
            </div>
          </div>
          {members.length === 0 ? (
            <div className="p-8 text-center">
              <p className="text-sm text-slate-500 dark:text-slate-400">
                No members yet. Run evaluation to discover members.
              </p>
            </div>
          ) : (
            <table className="w-full">
              <thead className="bg-slate-50 dark:bg-slate-900">
                <tr>
                  <th className="text-left text-xs font-medium text-slate-500 dark:text-slate-400 px-4 py-2">
                    Hostname
                  </th>
                  <th className="text-left text-xs font-medium text-slate-500 dark:text-slate-400 px-4 py-2">
                    OS
                  </th>
                  <th className="text-left text-xs font-medium text-slate-500 dark:text-slate-400 px-4 py-2">
                    IP Addresses
                  </th>
                  <th className="text-left text-xs font-medium text-slate-500 dark:text-slate-400 px-4 py-2">
                    Last Seen
                  </th>
                  <th className="text-left text-xs font-medium text-slate-500 dark:text-slate-400 px-4 py-2">
                    Tags
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
                {members.map((member) => (
                  <tr key={member.id} className="hover:bg-slate-50 dark:hover:bg-slate-700/50">
                    <td className="px-4 py-2 text-sm text-slate-900 dark:text-slate-100">
                      {member.hostname}
                    </td>
                    <td className="px-4 py-2 text-sm text-slate-600 dark:text-slate-400">
                      {member.os}
                    </td>
                    <td className="px-4 py-2 text-sm text-slate-600 dark:text-slate-400">
                      {member.ip_addresses.join(', ')}
                    </td>
                    <td className="px-4 py-2 text-sm text-slate-600 dark:text-slate-400">
                      {new Date(member.last_seen).toLocaleString()}
                    </td>
                    <td className="px-4 py-2">
                      <div className="flex gap-1">
                        {member.tags.map((tag) => (
                          <span
                            key={tag}
                            className="px-2 py-0.5 text-xs bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 rounded"
                          >
                            {tag}
                          </span>
                        ))}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {activeTab === 'bindings' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
              Script Bindings ({bindings.length})
            </h3>
          </div>

          {bindings.length === 0 ? (
            <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-8 text-center">
              <p className="text-sm text-slate-500 dark:text-slate-400">
                No script bindings configured
              </p>
              <p className="text-xs text-slate-400 dark:text-slate-500 mt-1">
                Bind schedules to this group to auto-dispatch scripts to all members
              </p>
            </div>
          ) : (
            <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800">
              <table className="w-full">
                <thead className="bg-slate-50 dark:bg-slate-900">
                  <tr>
                    <th className="text-left text-xs font-medium text-slate-500 dark:text-slate-400 px-4 py-2">
                      Script Schedule
                    </th>
                    <th className="text-left text-xs font-medium text-slate-500 dark:text-slate-400 px-4 py-2">
                      Type
                    </th>
                    <th className="text-left text-xs font-medium text-slate-500 dark:text-slate-400 px-4 py-2">
                      Priority
                    </th>
                    <th className="text-left text-xs font-medium text-slate-500 dark:text-slate-400 px-4 py-2">
                      Status
                    </th>
                    <th className="text-left text-xs font-medium text-slate-500 dark:text-slate-400 px-4 py-2">
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
                  {bindings.map((binding) => (
                    <tr key={binding.id} className="hover:bg-slate-50 dark:hover:bg-slate-700/50">
                      <td className="px-4 py-2 text-sm text-slate-900 dark:text-slate-100">
                        {binding.schedule_name}
                      </td>
                      <td className="px-4 py-2 text-sm text-slate-600 dark:text-slate-400">
                        {binding.binding_type}
                      </td>
                      <td className="px-4 py-2 text-sm text-slate-600 dark:text-slate-400">
                        {binding.priority}
                      </td>
                      <td className="px-4 py-2">
                        <span
                          className={`px-2 py-0.5 text-xs rounded ${
                            binding.enabled
                              ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
                              : 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400'
                          }`}
                        >
                          {binding.enabled ? 'Enabled' : 'Disabled'}
                        </span>
                      </td>
                      <td className="px-4 py-2">
                        <button
                          onClick={() => handleUnbindScript(binding.id)}
                          className="text-red-400 hover:text-red-600"
                        >
                          Remove
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {activeTab === 'expression' && (
        <div>
          <SmartGroupExpressionBuilder
            initialExpression={group.filter_expression as SmartGroupExpression | undefined}
            onExpressionChange={handleExpressionChange}
            readOnly={!group.is_smart}
          />
        </div>
      )}
    </div>
  );
}
