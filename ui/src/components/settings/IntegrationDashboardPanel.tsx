import { useState } from 'react';
import {
  Activity,
  Shield,
  Server,
  AlertTriangle,
  CheckCircle2,
  AlertCircle,
  Wifi,
  WifiOff,
  Clock,
  RefreshCw,
  Eye,
  EyeOff,
  Filter,
  X,
} from 'lucide-react';
import { useToast } from '@/components/shared/Toast';
import {
  type IntegrationDashboardData,
  type IntegrationEvent,
  type ProviderStats,
  type IntegrationProvider,
  createMockDashboardData,
  PROVIDER_LABELS,
  PROVIDER_ICONS,
  HEALTH_COLORS,
  HEALTH_BG,
  EVENT_SEVERITY_COLORS,
} from './IntegrationDashboardTypes';

type FilterType = 'all' | 'critical' | 'warning' | 'info';

export default function IntegrationDashboardPanel() {
  const { showToast } = useToast();
  const [data] = useState<IntegrationDashboardData>(createMockDashboardData);
  const [activeFilter, setActiveFilter] = useState<FilterType>('all');
  const [expandedEvent, setExpandedEvent] = useState<string | null>(null);
  const [showOnlyUnacknowledged, setShowOnlyUnacknowledged] = useState(false);
  const [loading, setLoading] = useState(false);

  const handleRefresh = () => {
    setLoading(true);
    showToast('info', 'Refreshing integrations...');
    setTimeout(() => {
      setLoading(false);
      showToast('success', 'Integrations refreshed');
    }, 1000);
  };

  const handleAcknowledge = (eventId: string) => {
    showToast('success', 'Event acknowledged');
    setExpandedEvent(null);
  };

  const filteredEvents = data.events.filter((event) => {
    if (showOnlyUnacknowledged && event.acknowledged) return false;
    if (activeFilter === 'all') return true;
    return event.severity === activeFilter;
  });

  const criticalEvents = data.events.filter(
    (e) => e.severity === 'critical' && !e.acknowledged
  ).length;

  const totalAlertsToday = data.providers.reduce(
    (sum, p) => sum + p.alertsToday, 0
  );

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Activity className="h-6 w-6 text-blue-600 dark:text-blue-400" />
          <h2 className="text-lg font-bold text-slate-900 dark:text-slate-100">
            Integration Dashboard
          </h2>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleRefresh}
            disabled={loading}
            className="flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-slate-600 dark:text-slate-400 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-md hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50"
          >
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </button>
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-5 gap-4">
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <Shield className="h-5 w-5 text-blue-600 dark:text-blue-400" />
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {data.summary.totalProviders}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            Total Providers
          </p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <CheckCircle2 className="h-5 w-5 text-green-600 dark:text-green-400" />
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {data.summary.healthyProviders}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            Healthy
          </p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <Server className="h-5 w-5 text-purple-600 dark:text-purple-400" />
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {data.summary.totalDevicesMonitored.toLocaleString()}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            Devices Monitored
          </p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-red-600 dark:text-red-400" />
            <span className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              {data.summary.activeAlerts}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            Active Alerts
          </p>
        </div>
        <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
          <div className="flex items-center gap-2">
            <Clock className="h-5 w-5 text-amber-600 dark:text-amber-400" />
            <span className="text-sm font-bold text-slate-900 dark:text-slate-100">
              {totalAlertsToday}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            Alerts Today
          </p>
        </div>
      </div>

      {/* Active Alerts Banner */}
      {criticalEvents > 0 && (
        <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 p-4">
          <div className="flex items-center gap-3">
            <AlertTriangle className="h-5 w-5 text-red-600 dark:text-red-400" />
            <div>
              <p className="text-sm font-semibold text-red-900 dark:text-red-100">
                {criticalEvents} Unacknowledged Critical Alert{criticalEvents > 1 ? 's' : ''}
              </p>
              <p className="text-xs text-red-700 dark:text-red-300 mt-0.5">
                Immediate attention required — check the events feed below
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Provider Health Cards */}
      <div>
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">
          Provider Health
        </h3>
        <div className="grid grid-cols-2 lg:grid-cols-3 gap-4">
          {data.providers.map((provider) => (
            <div
              key={provider.provider}
              className={`rounded-lg border ${HEALTH_BG[provider.health]} p-4`}
            >
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-2">
                  <span className="text-xl">{PROVIDER_ICONS[provider.provider]}</span>
                  <div>
                    <p className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                      {PROVIDER_LABELS[provider.provider]}
                    </p>
                    <p className="text-xs text-slate-500 dark:text-slate-400 capitalize">
                      {provider.integration}
                    </p>
                  </div>
                </div>
                <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${HEALTH_BG[provider.health]} ${HEALTH_COLORS[provider.health]}`}>
                  {provider.health === 'not_configured' ? 'Not Configured' : provider.health === 'degraded' ? 'Degraded' : provider.health === 'error' ? 'Error' : 'Healthy'}
                </span>
              </div>

              <div className="mt-3 space-y-2">
                <div className="flex justify-between text-xs">
                  <span className="text-slate-500 dark:text-slate-400">Devices</span>
                  <span className="font-medium text-slate-900 dark:text-slate-100">
                    {provider.totalDevices.toLocaleString()}
                  </span>
                </div>
                <div className="flex justify-between text-xs">
                  <span className="text-slate-500 dark:text-slate-400">Alerts (24h)</span>
                  <span className="font-medium text-slate-900 dark:text-slate-100">
                    {provider.alertsToday}
                  </span>
                </div>
                <div className="flex justify-between text-xs">
                  <span className="text-slate-500 dark:text-slate-400">Last Sync</span>
                  <span className="font-medium text-slate-900 dark:text-slate-100">
                    {provider.lastSyncAt ? new Date(provider.lastSyncAt).toLocaleTimeString() : 'Never'}
                  </span>
                </div>
                <div className="flex justify-between text-xs">
                  <span className="text-slate-500 dark:text-slate-400">Latency</span>
                  <span className={`font-medium ${provider.apiLatencyMs > 200 ? 'text-amber-600 dark:text-amber-400' : 'text-slate-900 dark:text-slate-100'}`}>
                    {provider.apiLatencyMs > 0 ? `${provider.apiLatencyMs}ms` : 'N/A'}
                  </span>
                </div>
                <div className="flex justify-between text-xs">
                  <span className="text-slate-500 dark:text-slate-400">Uptime</span>
                  <span className="font-medium text-slate-900 dark:text-slate-100">
                    {provider.uptime > 0 ? `${provider.uptime}%` : 'N/A'}
                  </span>
                </div>
              </div>

              <div className="mt-3 flex items-center gap-1 text-xs">
                {provider.health !== 'not_configured' ? (
                  <>
                    <Wifi className="h-3 w-3 text-green-500" />
                    <span className="text-green-600 dark:text-green-400">Connected</span>
                  </>
                ) : (
                  <>
                    <WifiOff className="h-3 w-3 text-slate-400" />
                    <span className="text-slate-500 dark:text-slate-400">Not configured</span>
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Recent Events Feed */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
            Recent Events ({filteredEvents.length})
          </h3>
          <div className="flex items-center gap-2">
            <select
              value={activeFilter}
              onChange={(e) => setActiveFilter(e.target.value as FilterType)}
              className="text-xs rounded-md border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 text-slate-900 dark:text-slate-100 px-2 py-1"
            >
              <option value="all">All Severity</option>
              <option value="critical">Critical</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
            <button
              onClick={() => setShowOnlyUnacknowledged(!showOnlyUnacknowledged)}
              className={`flex items-center gap-1 px-2 py-1 text-xs rounded-md border ${
                showOnlyUnacknowledged
                  ? 'bg-red-100 dark:bg-red-900/30 border-red-300 dark:border-red-700 text-red-700 dark:text-red-300'
                  : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-400'
              }`}
            >
              <EyeOff className="h-3 w-3" />
              Unacknowledged Only
            </button>
          </div>
        </div>

        <div className="space-y-2">
          {filteredEvents.map((event) => (
            <div
              key={event.id}
              className={`rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 ${
                !event.acknowledged && event.severity === 'critical'
                  ? 'border-red-300 dark:border-red-700'
                  : ''
              }`}
            >
              <div
                className="flex items-start gap-3 p-3 cursor-pointer hover:bg-slate-50 dark:hover:bg-slate-700/50"
                onClick={() => setExpandedEvent(expandedEvent === event.id ? null : event.id)}
              >
                <div className={`flex-shrink-0 w-2 h-2 rounded-full mt-2 ${
                  event.severity === 'critical' ? 'bg-red-500' :
                  event.severity === 'warning' ? 'bg-amber-500' : 'bg-blue-500'
                }`} />

                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-medium text-slate-900 dark:text-slate-100">
                      {PROVIDER_LABELS[event.provider]}
                    </span>
                    <span className={`text-xs px-2 py-0.5 rounded ${EVENT_SEVERITY_COLORS[event.severity]}`}>
                      {event.severity}
                    </span>
                    <span className="text-xs text-slate-400 dark:text-slate-500">
                      {event.eventType}
                    </span>
                    {!event.acknowledged && (
                      <span className="text-xs text-amber-600 dark:text-amber-400 font-medium">
                        Unacknowledged
                      </span>
                    )}
                  </div>
                  <p className="text-sm text-slate-700 dark:text-slate-300 mt-0.5">
                    {event.message}
                  </p>
                  <div className="flex items-center gap-3 mt-1 text-xs text-slate-400 dark:text-slate-500">
                    <span>{new Date(event.timestamp).toLocaleString()}</span>
                    <span>{event.deviceCount} device{event.deviceCount !== 1 ? 's' : ''}</span>
                  </div>
                </div>

                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    setExpandedEvent(expandedEvent === event.id ? null : event.id);
                  }}
                  className="flex-shrink-0 p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
                >
                  {expandedEvent === event.id ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>

              {/* Expanded details */}
              {expandedEvent === event.id && (
                <div className="px-3 pb-3 pt-1 border-t border-slate-100 dark:border-slate-700">
                  <div className="grid grid-cols-2 gap-2 text-xs">
                    <div>
                      <span className="text-slate-500 dark:text-slate-400">Event ID:</span>{' '}
                      <code className="text-slate-900 dark:text-slate-100">{event.id}</code>
                    </div>
                    <div>
                      <span className="text-slate-500 dark:text-slate-400">Type:</span>{' '}
                      <span className="text-slate-900 dark:text-slate-100">{event.eventType}</span>
                    </div>
                    <div>
                      <span className="text-slate-500 dark:text-slate-400">Provider:</span>{' '}
                      <span className="text-slate-900 dark:text-slate-100">{PROVIDER_LABELS[event.provider]}</span>
                    </div>
                    <div>
                      <span className="text-slate-500 dark:text-slate-400">Devices:</span>{' '}
                      <span className="text-slate-900 dark:text-slate-100">{event.deviceCount}</span>
                    </div>
                  </div>
                  {!event.acknowledged && (
                    <div className="mt-3">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          handleAcknowledge(event.id);
                        }}
                        className="px-3 py-1 text-xs font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700"
                      >
                        Acknowledge
                      </button>
                    </div>
                  )}
                </div>
              )}
            </div>
          ))}

          {filteredEvents.length === 0 && (
            <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-8 text-center">
              <Filter className="h-8 w-8 text-slate-400 mx-auto mb-2" />
              <p className="text-sm text-slate-500 dark:text-slate-400">
                No events match your filters
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
