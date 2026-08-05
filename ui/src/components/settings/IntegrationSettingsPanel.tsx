import { useState, useCallback } from 'react';
import {
  Shield,
  Key,
  Webhook,
  Copy,
  Trash2,
  Plus,
  AlertCircle,
  RefreshCw,
  Eye,
  EyeOff,
  Activity,
  Plug,
} from 'lucide-react';
import { useToast } from '@/components/shared/Toast';
import { ConfirmDialog } from '@/components/shared/ConfirmDialog';
import {
  type ApiKey,
  type IntegrationSettingsData,
  createMockApiKeys,
  createMockWebhooks,
  createMockStatuses,
  INTEGRATION_LABELS,
  PROVIDER_LABELS,
  STATUS_COLORS,
  HEALTH_COLORS,
  type ApiKeyStatus,
  type IntegrationProvider,
} from './IntegrationSettingsTypes';

type TabId = 'keys' | 'webhooks' | 'status';

export default function IntegrationSettingsPanel() {
  const { showToast } = useToast();
  const [activeTab, setActiveTab] = useState<TabId>('keys');
  const [data, setData] = useState<IntegrationSettingsData>({
    apiKeys: createMockApiKeys(),
    webhooks: createMockWebhooks(),
    statuses: createMockStatuses(),
  });
  const [loading, setLoading] = useState(false);
  const [showCreateKey, setShowCreateKey] = useState(false);
  const [confirmDeleteKey, setConfirmDeleteKey] = useState<string | null>(null);
  const [confirmDeleteWebhook, setConfirmDeleteWebhook] = useState<string | null>(null);
  const [revealedKeys, setRevealedKeys] = useState<Set<string>>(new Set());
  const [newKeyName, setNewKeyName] = useState('');
  const [newKeyIntegration, setNewKeyIntegration] = useState<'edr' | 'backup' | 'psa'>('edr');
  const [newKeyProvider, setNewKeyProvider] = useState('');

  const generateApiKey = () => {
    const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
    let result = '';
    for (let i = 0; i < 32; i++) {
      result += chars.charAt(Math.floor(Math.random() * chars.length));
    }
    return result;
  };

  const handleCreateKey = useCallback(() => {
    if (!newKeyName.trim()) {
      showToast('error', 'Key name is required');
      return;
    }
    const rawKey = generateApiKey();
    const masked = rawKey.slice(0, 8) + '••••••••••••••••••';
    const newKey: ApiKey = {
      id: `key-${Date.now()}`,
      name: newKeyName.trim(),
      integration: newKeyIntegration,
      provider: (newKeyProvider || 'crowdstrike') as IntegrationProvider,
      key: rawKey,
      maskedKey: masked,
      status: 'active',
      createdAt: new Date().toISOString(),
    };
    setData(prev => ({
      ...prev,
      apiKeys: [newKey, ...prev.apiKeys],
    }));
    setShowCreateKey(false);
    setNewKeyName('');
    setRevealedKeys(prev => new Set(prev).add(newKey.id));
    showToast('success', `API key "${newKey.name}" created`);
  }, [newKeyName, newKeyIntegration, newKeyProvider, showToast]);

  const handleRevokeKey = useCallback((keyId: string) => {
    setData(prev => ({
      ...prev,
      apiKeys: prev.apiKeys.map(k =>
        k.id === keyId ? { ...k, status: 'revoked' as ApiKeyStatus } : k
      ),
    }));
    setConfirmDeleteKey(null);
    showToast('success', 'API key revoked');
  }, [showToast]);

  const handleCopyKey = useCallback((keyId: string, key: string) => {
    navigator.clipboard.writeText(key).then(() => {
      showToast('success', 'API key copied to clipboard');
    }).catch(() => {
      showToast('error', 'Failed to copy to clipboard');
    });
  }, [showToast]);

  const handleCopyWebhookUrl = useCallback((url: string) => {
    navigator.clipboard.writeText(url).then(() => {
      showToast('success', 'Webhook URL copied');
    }).catch(() => {
      showToast('error', 'Failed to copy URL');
    });
  }, [showToast]);

  const handleCopyWebhookSecret = useCallback((secret: string) => {
    navigator.clipboard.writeText(secret).then(() => {
      showToast('success', 'Webhook secret copied');
    }).catch(() => {
      showToast('error', 'Failed to copy secret');
    });
  }, [showToast]);

  const handleToggleWebhook = useCallback((webhookId: string) => {
    setData(prev => ({
      ...prev,
      webhooks: prev.webhooks.map(w =>
        w.id === webhookId ? { ...w, enabled: !w.enabled } : w
      ),
    }));
    showToast('info', 'Webhook configuration updated');
  }, [showToast]);

  const handleDeleteWebhook = useCallback((webhookId: string) => {
    setData(prev => ({
      ...prev,
      webhooks: prev.webhooks.filter(w => w.id !== webhookId),
    }));
    setConfirmDeleteWebhook(null);
    showToast('success', 'Webhook configuration deleted');
  }, [showToast]);

  const handleRefreshStatuses = useCallback(() => {
    setLoading(true);
    setTimeout(() => {
      setData(prev => ({
        ...prev,
        statuses: createMockStatuses().map(s => ({
          ...s,
          lastAlertAt: new Date().toISOString(),
          alertsProcessed: s.alertsReceived + Math.floor(Math.random() * 10),
        })),
      }));
      setLoading(false);
      showToast('success', 'Integration statuses refreshed');
    }, 1200);
  }, [showToast]);

  const handleToggleKeyReveal = useCallback((keyId: string) => {
    setRevealedKeys(prev => {
      const next = new Set(prev);
      if (next.has(keyId)) {
        next.delete(keyId);
      } else {
        next.add(keyId);
      }
      return next;
    });
  }, []);

  const getActiveKeysCount = () => data.apiKeys.filter(k => k.status === 'active').length;
  const getActiveWebhooksCount = () => data.webhooks.filter(w => w.enabled).length;
  const getHealthyStatusesCount = () => data.statuses.filter(s => s.health === 'healthy').length;

  const tabs: { id: TabId; label: string; icon: React.ReactNode; count?: number }[] = [
    { id: 'keys', label: 'API Keys', icon: <Key className="h-4 w-4" />, count: getActiveKeysCount() },
    { id: 'webhooks', label: 'Webhook URLs', icon: <Webhook className="h-4 w-4" />, count: getActiveWebhooksCount() },
    { id: 'status', label: 'Integration Status', icon: <Activity className="h-4 w-4" /> },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-900 dark:text-white">Integration Settings</h2>
        <div className="flex items-center gap-2">
          <span className="text-xs text-slate-500">
            {getActiveKeysCount()} active keys · {getActiveWebhooksCount()} active webhooks
          </span>
        </div>
      </div>

      {/* Status summary cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
          <div className="flex items-center gap-2 mb-2">
            <Plug className="h-4 w-4 text-blue-500" />
            <span className="text-sm font-medium text-slate-700 dark:text-slate-300">Active Keys</span>
          </div>
          <p className="text-2xl font-bold text-slate-900 dark:text-white">{getActiveKeysCount()}</p>
        </div>
        <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
          <div className="flex items-center gap-2 mb-2">
            <Webhook className="h-4 w-4 text-green-500" />
            <span className="text-sm font-medium text-slate-700 dark:text-slate-300">Active Webhooks</span>
          </div>
          <p className="text-2xl font-bold text-slate-900 dark:text-white">{getActiveWebhooksCount()}</p>
        </div>
        <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
          <div className="flex items-center gap-2 mb-2">
            <Shield className="h-4 w-4 text-purple-500" />
            <span className="text-sm font-medium text-slate-700 dark:text-slate-300">Healthy Integrations</span>
          </div>
          <p className="text-2xl font-bold text-slate-900 dark:text-white">{getHealthyStatusesCount()}</p>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b border-slate-200 dark:border-slate-700">
        {tabs.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${
              activeTab === tab.id
                ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                : 'border-transparent text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
            }`}
          >
            {tab.icon}
            {tab.label}
            {tab.count !== undefined && (
              <span className="ml-1 text-xs bg-slate-100 dark:bg-slate-800 text-slate-500 px-1.5 py-0.5 rounded">
                {tab.count}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* API Keys Tab */}
      {activeTab === 'keys' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <p className="text-sm text-slate-500">Manage API keys for integration providers. Keys are shown once on creation — copy them immediately.</p>
            <button
              onClick={() => setShowCreateKey(true)}
              className="flex items-center gap-1.5 px-3 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700"
            >
              <Plus className="h-3.5 w-3.5" />
              New Key
            </button>
          </div>

          {data.apiKeys.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <div className="mb-4 rounded-xl bg-slate-100 p-3 text-slate-500 dark:bg-slate-800 dark:text-slate-400">
                <Key size={28} strokeWidth={1.75} />
              </div>
              <h3 className="text-lg font-medium text-slate-900 dark:text-white mb-1">No API keys configured</h3>
              <p className="text-sm text-slate-500 mb-4">Create your first API key to connect an integration provider.</p>
              <button
                onClick={() => setShowCreateKey(true)}
                className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700"
              >
                Create API Key
              </button>
            </div>
          ) : (
            <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
              <div className="max-h-[400px] overflow-y-auto">
                <table className="w-full text-sm">
                  <thead className="bg-slate-50 dark:bg-slate-800 sticky top-0">
                    <tr>
                      <th className="text-left px-4 py-3 font-medium text-slate-500">Name</th>
                      <th className="text-left px-4 py-3 font-medium text-slate-500">Integration</th>
                      <th className="text-left px-4 py-3 font-medium text-slate-500">Key</th>
                      <th className="text-left px-4 py-3 font-medium text-slate-500">Status</th>
                      <th className="text-left px-4 py-3 font-medium text-slate-500">Last Used</th>
                      <th className="text-right px-4 py-3 font-medium text-slate-500">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
                    {data.apiKeys.map(key => {
                      const isRevealed = revealedKeys.has(key.id);
                      const statusColor = STATUS_COLORS[key.status];
                      return (
                        <tr key={key.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                          <td className="px-4 py-3">
                            <div className="font-medium text-slate-900 dark:text-white">{key.name}</div>
                            <div className="text-xs text-slate-500">{PROVIDER_LABELS[key.provider]}</div>
                          </td>
                          <td className="px-4 py-3">
                            <span className="text-slate-700 dark:text-slate-300">{INTEGRATION_LABELS[key.integration]}</span>
                          </td>
                          <td className="px-4 py-3">
                            <div className="flex items-center gap-2 font-mono text-xs">
                              <code className="bg-slate-100 dark:bg-slate-800 px-2 py-1 rounded">
                                {isRevealed ? key.key : key.maskedKey}
                              </code>
                              <button
                                onClick={() => handleToggleKeyReveal(key.id)}
                                className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
                                title={isRevealed ? 'Hide key' : 'Show key'}
                              >
                                {isRevealed ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                              </button>
                              <button
                                onClick={() => handleCopyKey(key.id, isRevealed ? key.key : key.maskedKey)}
                                className="text-slate-400 hover:text-blue-600 dark:hover:text-blue-400"
                                title="Copy key"
                              >
                                <Copy className="h-3.5 w-3.5" />
                              </button>
                            </div>
                          </td>
                          <td className="px-4 py-3">
                            <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${statusColor.bg} ${statusColor.text}`}>
                              <span className={`w-1.5 h-1.5 rounded-full ${statusColor.dot}`} />
                              {key.status}
                            </span>
                          </td>
                          <td className="px-4 py-3 text-slate-500">
                            {key.lastUsedAt ? new Date(key.lastUsedAt).toLocaleDateString() : 'Never'}
                          </td>
                          <td className="px-4 py-3 text-right">
                            <div className="flex items-center justify-end gap-1">
                              <button
                                onClick={() => handleCopyKey(key.id, isRevealed ? key.key : key.maskedKey)}
                                className="p-1.5 text-slate-400 hover:text-blue-600 dark:hover:text-blue-400 hover:bg-slate-100 dark:hover:bg-slate-800 rounded"
                                title="Copy key"
                              >
                                <Copy className="h-3.5 w-3.5" />
                              </button>
                              {key.status === 'active' && (
                                <button
                                  onClick={() => setConfirmDeleteKey(key.id)}
                                  className="p-1.5 text-slate-400 hover:text-red-600 dark:hover:text-red-400 hover:bg-slate-100 dark:hover:bg-slate-800 rounded"
                                  title="Revoke key"
                                >
                                  <Trash2 className="h-3.5 w-3.5" />
                                </button>
                              )}
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Webhook URLs Tab */}
      {activeTab === 'webhooks' && (
        <div className="space-y-4">
          <p className="text-sm text-slate-500">
            Webhook endpoints for receiving events from integration providers. Copy the URL and configure it in your provider&apos;s webhook settings.
          </p>

          {data.webhooks.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <div className="mb-4 rounded-xl bg-slate-100 p-3 text-slate-500 dark:bg-slate-800 dark:text-slate-400">
                <Webhook size={28} strokeWidth={1.75} />
              </div>
              <h3 className="text-lg font-medium text-slate-900 dark:text-white mb-1">No webhooks configured</h3>
              <p className="text-sm text-slate-500">Webhook endpoints will appear here once integrations are added.</p>
            </div>
          ) : (
            <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
              <div className="max-h-[400px] overflow-y-auto">
                <table className="w-full text-sm">
                  <thead className="bg-slate-50 dark:bg-slate-800 sticky top-0">
                    <tr>
                      <th className="text-left px-4 py-3 font-medium text-slate-500">Name</th>
                      <th className="text-left px-4 py-3 font-medium text-slate-500">Integration</th>
                      <th className="text-left px-4 py-3 font-medium text-slate-500">URL</th>
                      <th className="text-left px-4 py-3 font-medium text-slate-500">Secret</th>
                      <th className="text-left px-4 py-3 font-medium text-slate-500">Status</th>
                      <th className="text-right px-4 py-3 font-medium text-slate-500">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
                    {data.webhooks.map(webhook => (
                      <tr key={webhook.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                        <td className="px-4 py-3">
                          <div className="font-medium text-slate-900 dark:text-white">{webhook.name}</div>
                          <div className="text-xs text-slate-500">{PROVIDER_LABELS[webhook.provider]}</div>
                        </td>
                        <td className="px-4 py-3">
                          <span className="text-slate-700 dark:text-slate-300">{INTEGRATION_LABELS[webhook.integration]}</span>
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-2">
                            <code className="bg-slate-100 dark:bg-slate-800 px-2 py-1 rounded text-xs font-mono text-slate-700 dark:text-slate-300 truncate max-w-[200px]">
                              {webhook.url}
                            </code>
                            <button
                              onClick={() => handleCopyWebhookUrl(webhook.url)}
                              className="text-slate-400 hover:text-blue-600 dark:hover:text-blue-400 flex-shrink-0"
                              title="Copy URL"
                            >
                              <Copy className="h-3.5 w-3.5" />
                            </button>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-2">
                            <code className="bg-slate-100 dark:bg-slate-800 px-2 py-1 rounded text-xs font-mono text-slate-500">
                              {webhook.secret}
                            </code>
                            <button
                              onClick={() => handleCopyWebhookSecret(webhook.secret)}
                              className="text-slate-400 hover:text-blue-600 dark:hover:text-blue-400 flex-shrink-0"
                              title="Copy secret"
                            >
                              <Copy className="h-3.5 w-3.5" />
                            </button>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <button
                            onClick={() => handleToggleWebhook(webhook.id)}
                            className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium transition-colors ${
                              webhook.enabled
                                ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
                                : 'bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400'
                            }`}
                          >
                            <span className={`w-1.5 h-1.5 rounded-full ${webhook.enabled ? 'bg-green-500' : 'bg-slate-400'}`} />
                            {webhook.enabled ? 'enabled' : 'disabled'}
                          </button>
                        </td>
                        <td className="px-4 py-3 text-right">
                          <button
                            onClick={() => setConfirmDeleteWebhook(webhook.id)}
                            className="p-1.5 text-slate-400 hover:text-red-600 dark:hover:text-red-400 hover:bg-slate-100 dark:hover:bg-slate-800 rounded"
                            title="Delete webhook"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Integration Status Tab */}
      {activeTab === 'status' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <p className="text-sm text-slate-500">
              Monitor the health and activity of all connected integration providers.
            </p>
            <button
              onClick={handleRefreshStatuses}
              disabled={loading}
              className="flex items-center gap-1.5 px-3 py-2 text-sm rounded-md border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-50"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </button>
          </div>

          {loading ? (
            <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-8 flex items-center justify-center">
              <RefreshCw className="h-6 w-6 text-slate-400 animate-spin" />
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {data.statuses.map((status, index) => {
                const healthColor = HEALTH_COLORS[status.health];
                return (
                  <div
                    key={index}
                    className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4 hover:border-slate-300 dark:hover:border-slate-600 transition-colors"
                  >
                    <div className="flex items-start justify-between mb-3">
                      <div>
                        <h3 className="font-medium text-slate-900 dark:text-white">{PROVIDER_LABELS[status.provider]}</h3>
                        <p className="text-xs text-slate-500">{INTEGRATION_LABELS[status.integration]}</p>
                      </div>
                      <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${healthColor.bg} ${healthColor.text}`}>
                        <span className={`w-1.5 h-1.5 rounded-full ${healthColor.dot}`} />
                        {status.health.replace('_', ' ')}
                      </span>
                    </div>

                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <p className="text-xs text-slate-500 mb-1">Alerts Received</p>
                        <p className="text-lg font-bold text-slate-900 dark:text-white">{status.alertsReceived}</p>
                      </div>
                      <div>
                        <p className="text-xs text-slate-500 mb-1">Alerts Processed</p>
                        <p className="text-lg font-bold text-slate-900 dark:text-white">{status.alertsProcessed}</p>
                      </div>
                    </div>

                    {status.lastAlertAt && (
                      <p className="text-xs text-slate-400 mt-3">
                        Last alert: {new Date(status.lastAlertAt).toLocaleString()}
                      </p>
                    )}

                    {status.alertsReceived !== status.alertsProcessed && status.alertsReceived > 0 && (
                      <div className="flex items-center gap-1.5 mt-3 text-xs text-amber-600 dark:text-amber-400">
                        <AlertCircle className="h-3.5 w-3.5" />
                        {status.alertsReceived - status.alertsProcessed} alerts pending
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {/* Create Key Dialog */}
      {showCreateKey && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowCreateKey(false)}>
          <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-md w-full mx-4 p-6" onClick={e => e.stopPropagation()}>
            <h3 className="text-lg font-bold text-slate-900 dark:text-white mb-1">Create API Key</h3>
            <p className="text-sm text-slate-500 mb-6">Configure a new API key for an integration provider.</p>

            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">
                  Key Name
                </label>
                <input
                  type="text"
                  value={newKeyName}
                  onChange={e => setNewKeyName(e.target.value)}
                  placeholder="e.g., CrowdStrike Production"
                  className="w-full px-3 py-2 text-sm rounded-md border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-slate-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">
                  Integration Type
                </label>
                <div className="flex gap-2">
                  {(['edr', 'backup', 'psa'] as const).map(type => (
                    <button
                      key={type}
                      onClick={() => {
                        setNewKeyIntegration(type);
                        setNewKeyProvider('');
                      }}
                      className={`flex-1 px-3 py-2 text-sm rounded-md border transition-colors ${
                        newKeyIntegration === type
                          ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-400'
                          : 'border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800'
                      }`}
                    >
                      {INTEGRATION_LABELS[type]}
                    </button>
                  ))}
                </div>
              </div>

              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">
                  Provider
                </label>
                <select
                  value={newKeyProvider}
                  onChange={e => setNewKeyProvider(e.target.value)}
                  className="w-full px-3 py-2 text-sm rounded-md border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-slate-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
                >
                  <option value="">Select a provider...</option>
                  {(newKeyIntegration === 'edr'
                    ? ['crowdstrike', 'sentinelone', 'defender'] as const
                    : newKeyIntegration === 'backup'
                      ? ['veeam', 'commvault', 'druva'] as const
                      : ['autotask', 'connectwise', 'freshservice', 'zendesk'] as const
                  ).map(provider => (
                    <option key={provider} value={provider}>
                      {PROVIDER_LABELS[provider]}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="flex justify-end gap-3 mt-6">
              <button
                onClick={() => { setShowCreateKey(false); setNewKeyName(''); setNewKeyProvider(''); }}
                className="px-4 py-2 text-sm rounded-md border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800"
              >
                Cancel
              </button>
              <button
                onClick={handleCreateKey}
                className="px-4 py-2 text-sm rounded-md bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
              >
                Create Key
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Revoke Key Confirmation */}
      {confirmDeleteKey && (
        <ConfirmDialog
          open={true}
          title="Revoke API Key"
          message="Revoking this key will immediately invalidate it. All authenticated requests using this key will fail. This action can be undone by creating a new key."
          confirmLabel="Revoke Key"
          onConfirm={() => handleRevokeKey(confirmDeleteKey)}
          onCancel={() => setConfirmDeleteKey(null)}
        />
      )}

      {/* Delete Key Confirmation */}
      {confirmDeleteKey && null}

      {/* Delete Webhook Confirmation */}
      {confirmDeleteWebhook && (
        <ConfirmDialog
          open={true}
          title="Delete Webhook"
          message="Deleting this webhook will stop receiving events from this integration. You will need to update the provider configuration with a new webhook URL."
          confirmLabel="Delete Webhook"
          onConfirm={() => handleDeleteWebhook(confirmDeleteWebhook)}
          onCancel={() => setConfirmDeleteWebhook(null)}
        />
      )}
    </div>
  );
}
