import { useState, useCallback } from 'react';
import {
  Users,
  Settings2,
  Play,
  Plus,
  Trash2,
  Edit2,
  Eye,
  EyeOff,
  Copy,
  Check,
  AlertCircle,
  Clock,
  Shield,
  Code,
} from 'lucide-react';
import { useToast } from '@/components/shared/Toast';
import type {
  SmartGroup,
  SmartGroupMember,
  SmartGroupScriptBinding,
} from './SmartGroupTypes';
import {
  createMockSmartGroups,
  createMockMembers,
} from './SmartGroupTypes';
import SmartGroupExpressionBuilder from './SmartGroupExpressionBuilder';
import type { SmartGroupExpression } from './SmartGroupTypes';

type TabId = 'overview' | 'members' | 'bindings' | 'expression';

interface SmartGroupDetailPageProps {
  groupId?: string;
}

export default function SmartGroupDetailPage({ groupId }: SmartGroupDetailPageProps) {
  const { showToast } = useToast();
  const [activeTab, setActiveTab] = useState<TabId>('overview');
  const [groups] = useState<SmartGroup[]>(createMockSmartGroups);
  const [members] = useState<SmartGroupMember[]>(createMockMembers);
  const [revealedBindings, setRevealedBindings] = useState<Set<string>>(new Set());
  const [editingName, setEditingName] = useState(false);
  const [groupName, setGroupName] = useState('Linux Servers');
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const group = groups.find((g) => g.id === groupId || g.id === 'group-001');
  const groupMembers = members;

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
                onClick={() => setEditingName(false)}
                className="text-green-600 dark:text-green-400"
              >
                <Check className="h-5 w-5" />
              </button>
            </div>
          ) : (
            <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">
              {group?.name || 'Smart Group'}
            </h1>
          )}
          <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
            {group?.description || 'Smart group detail'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setEditingName(true)}
            className="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
            title="Edit group name"
          >
            <Edit2 className="h-4 w-4" />
          </button>
          <button
            onClick={() => showToast('info', `Group evaluated: ${groupMembers.length} devices`)}
            className="flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-green-600 dark:text-green-400 bg-green-50 dark:bg-green-900/20 rounded-md hover:bg-green-100 dark:hover:bg-green-900/30"
          >
            <Play className="h-4 w-4" />
            Evaluate Now
          </button>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-4 gap-4">
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <Users className="h-5 w-5 text-blue-600 dark:text-blue-400" />
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {group?.memberCount || groupMembers.length}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">Members</p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <Shield className="h-5 w-5 text-purple-600 dark:text-purple-400" />
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {group?.isSmart ? 'Smart' : 'Static'}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">Group Type</p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <Clock className="h-5 w-5 text-amber-600 dark:text-amber-400" />
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {group?.status}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">Status</p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <Settings2 className="h-5 w-5 text-slate-600 dark:text-slate-400" />
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {group?.bindings?.length || 0}
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
                    {group?.id}
                  </code>
                  <button
                    onClick={() => handleCopyId(group?.id || '')}
                    className="text-slate-400 hover:text-slate-600"
                  >
                    {copiedId === group?.id ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
                  </button>
                </div>
              </div>
              <div>
                <label className="text-xs text-slate-500 dark:text-slate-400">Client</label>
                <p className="text-sm text-slate-900 dark:text-slate-100 mt-1">
                  {group?.clientId || 'client-001'}
                </p>
              </div>
              <div>
                <label className="text-xs text-slate-500 dark:text-slate-400">Created</label>
                <p className="text-sm text-slate-900 dark:text-slate-100 mt-1">
                  {group?.createdAt || '2026-06-01T10:00:00Z'}
                </p>
              </div>
              <div>
                <label className="text-xs text-slate-500 dark:text-slate-400">Last Evaluated</label>
                <p className="text-sm text-slate-900 dark:text-slate-100 mt-1">
                  {group?.lastEvaluated || 'Never'}
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
                Group Members ({groupMembers.length})
              </h3>
              <button className="text-xs text-blue-600 dark:text-blue-400 hover:underline">
                Export CSV
              </button>
            </div>
          </div>
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
                  IP
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
              {groupMembers.map((member) => (
                <tr key={member.id} className="hover:bg-slate-50 dark:hover:bg-slate-700/50">
                  <td className="px-4 py-2 text-sm text-slate-900 dark:text-slate-100">
                    {member.hostname}
                  </td>
                  <td className="px-4 py-2 text-sm text-slate-600 dark:text-slate-400">
                    {member.os}
                  </td>
                  <td className="px-4 py-2 text-sm text-slate-600 dark:text-slate-400">
                    {member.ip}
                  </td>
                  <td className="px-4 py-2 text-sm text-slate-600 dark:text-slate-400">
                    {member.lastSeen}
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
        </div>
      )}

      {activeTab === 'bindings' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
              Script Bindings ({group?.bindings?.length || 0})
            </h3>
            <button className="flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/20 rounded-md hover:bg-blue-100 dark:hover:bg-blue-900/30">
              <Plus className="h-3 w-3" />
              Add Binding
            </button>
          </div>

          {(group?.bindings || []).length === 0 ? (
            <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-8 text-center">
              <AlertCircle className="h-8 w-8 text-slate-400 mx-auto mb-2" />
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
                  {group!.bindings!.map((binding) => (
                    <tr key={binding.id} className="hover:bg-slate-50 dark:hover:bg-slate-700/50">
                      <td className="px-4 py-2 text-sm text-slate-900 dark:text-slate-100">
                        {binding.scheduleName}
                      </td>
                      <td className="px-4 py-2 text-sm text-slate-600 dark:text-slate-400">
                        {binding.bindingType}
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
                        <div className="flex gap-2">
                          <button className="text-slate-400 hover:text-slate-600">
                            <Edit2 className="h-4 w-4" />
                          </button>
                          <button className="text-red-400 hover:text-red-600">
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </div>
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
            initialExpression={group?.filterExpression}
            onExpressionChange={handleExpressionChange}
          />
        </div>
      )}
    </div>
  );
}
