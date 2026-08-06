import { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { useWorkspace } from '@/hooks/useWorkspace';
import { useAuth } from '@/hooks/useAuth';
import { api } from '@/api/client';
import type { DeviceGroup } from '@/api/types';
import { useToast } from '@/components/shared/Toast';
import { Skeleton } from '@/components/shared/Skeleton';

export default function DeviceGroupListPage() {
  const { showToast } = useToast();
  const navigate = useNavigate();
  const { workspace } = useWorkspace();
  const { user } = useAuth();
  const [groups, setGroups] = useState<DeviceGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newGroupName, setNewGroupName] = useState('');
  const [newGroupDesc, setNewGroupDesc] = useState('');
  const [isSmartGroup, setIsSmartGroup] = useState(true);
  const [creating, setCreating] = useState(false);

  const mspID = workspace?.msp_id || '';
  const clientID = workspace?.client_id || '';

  const loadGroups = useCallback(async () => {
    if (!mspID || !clientID) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const result = await api.getDeviceGroups(mspID, clientID);
      setGroups(result.device_groups);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load groups';
      setError(message);
      showToast('error', message);
    } finally {
      setLoading(false);
    }
  }, [mspID, clientID, showToast]);

  useEffect(() => {
    loadGroups();
  }, [loadGroups]);

  const handleCreate = useCallback(async () => {
    if (!newGroupName.trim()) return;
    setCreating(true);
    try {
      let result;
      if (isSmartGroup) {
        result = await api.createSmartGroup(mspID, clientID, {
          name: newGroupName.trim(),
          description: newGroupDesc.trim() || undefined,
          filter_expression: { condition: 'AND', filters: [] },
        });
      } else {
        result = await api.createDeviceGroup(mspID, clientID, {
          name: newGroupName.trim(),
          description: newGroupDesc.trim() || undefined,
        });
      }
      showToast('success', `Group created: ${result.id}`);
      setShowCreateModal(false);
      setNewGroupName('');
      setNewGroupDesc('');
      loadGroups();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to create group';
      showToast('error', message);
    } finally {
      setCreating(false);
    }
  }, [newGroupName, newGroupDesc, isSmartGroup, mspID, clientID, showToast, loadGroups]);

  const handleNavigate = useCallback((groupId: string) => {
    navigate(`/groups/${groupId}`);
  }, [navigate]);

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton type="text" count={2} />
        <Skeleton type="card" count={3} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">
            Device Groups
          </h1>
          <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
            Manage device groups for automated grouping and script dispatch
          </p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700"
        >
          <span>+</span> New Group
        </button>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-3 gap-4">
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {groups.length}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">Total Groups</p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {groups.filter(g => g.is_smart).length}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">Smart Groups</p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {groups.filter(g => !g.is_smart).length}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">Static Groups</p>
        </div>
      </div>

      {/* Groups List */}
      {groups.length === 0 ? (
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-12 text-center">
          <p className="text-slate-500 dark:text-slate-400">No device groups yet</p>
          <button
            onClick={() => setShowCreateModal(true)}
            className="mt-3 px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700"
          >
            Create your first group
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4">
          {groups.map((group) => (
            <button
              key={group.id}
              onClick={() => handleNavigate(group.id)}
              className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4 text-left hover:border-blue-500 dark:hover:border-blue-400 transition-colors"
            >
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                    {group.name}
                  </h3>
                  <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                    {group.description || 'No description'}
                  </p>
                  <div className="flex gap-3 mt-2">
                    <span className="text-xs text-slate-400 dark:text-slate-500">
                      {group.member_count} members
                    </span>
                    <span className={`px-2 py-0.5 text-xs rounded ${
                      group.is_smart
                        ? 'bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-400'
                        : 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
                    }`}>
                      {group.is_smart ? 'Smart' : 'Static'}
                    </span>
                  </div>
                </div>
                <div className="text-xs text-slate-400 dark:text-slate-500">
                  Created {new Date(group.created_at).toLocaleDateString()}
                </div>
              </div>
            </button>
          ))}
        </div>
      )}

      {/* Create Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-slate-800 rounded-lg p-6 w-full max-w-md">
            <h2 className="text-lg font-bold text-slate-900 dark:text-slate-100 mb-4">
              Create New Group
            </h2>
            <div className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
                  Group Name *
                </label>
                <input
                  type="text"
                  value={newGroupName}
                  onChange={(e) => setNewGroupName(e.target.value)}
                  className="w-full px-3 py-2 text-sm border border-slate-300 dark:border-slate-600 rounded-md bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                  placeholder="e.g., Production Servers"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
                  Description
                </label>
                <textarea
                  value={newGroupDesc}
                  onChange={(e) => setNewGroupDesc(e.target.value)}
                  className="w-full px-3 py-2 text-sm border border-slate-300 dark:border-slate-600 rounded-md bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                  rows={2}
                  placeholder="Optional description"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
                  Group Type
                </label>
                <div className="flex gap-4">
                  <label className="flex items-center gap-2">
                    <input
                      type="radio"
                      checked={isSmartGroup}
                      onChange={() => setIsSmartGroup(true)}
                      className="text-blue-600"
                    />
                    <span className="text-sm text-slate-900 dark:text-slate-100">Smart Group</span>
                  </label>
                  <label className="flex items-center gap-2">
                    <input
                      type="radio"
                      checked={!isSmartGroup}
                      onChange={() => setIsSmartGroup(false)}
                      className="text-blue-600"
                    />
                    <span className="text-sm text-slate-900 dark:text-slate-100">Static Group</span>
                  </label>
                </div>
              </div>
              {isSmartGroup && (
                <p className="text-xs text-slate-500 dark:text-slate-400">
                  Smart groups automatically update members based on filter expressions.
                </p>
              )}
            </div>
            <div className="flex gap-3 mt-6">
              <button
                onClick={() => setShowCreateModal(false)}
                className="flex-1 px-4 py-2 text-sm font-medium text-slate-700 dark:text-slate-300 bg-slate-100 dark:bg-slate-700 rounded-md hover:bg-slate-200 dark:hover:bg-slate-600"
              >
                Cancel
              </button>
              <button
                onClick={handleCreate}
                disabled={!newGroupName.trim() || creating}
                className="flex-1 px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {creating ? 'Creating...' : 'Create'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
