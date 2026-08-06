export type IntegrationType = 'edr' | 'backup' | 'psa';

export type IntegrationProvider =
  | 'crowdstrike'
  | 'sentinelone'
  | 'defender'
  | 'veeam'
  | 'commvault'
  | 'druva'
  | 'autotask'
  | 'connectwise'
  | 'freshservice'
  | 'zendesk';

export type ProviderHealth = 'healthy' | 'degraded' | 'error' | 'not_configured';

export type IntegrationEvent = {
  id: string;
  provider: IntegrationProvider;
  integration: IntegrationType;
  eventType: string;
  severity: 'info' | 'warning' | 'critical';
  deviceCount: number;
  message: string;
  timestamp: string;
  acknowledged: boolean;
};

export type ProviderStats = {
  provider: IntegrationProvider;
  integration: IntegrationType;
  health: ProviderHealth;
  totalDevices: number;
  alertsToday: number;
  alertsLast7Days: number;
  lastSyncAt: string;
  apiLatencyMs: number;
  uptime: number;
};

export type IntegrationDashboardData = {
  providers: ProviderStats[];
  events: IntegrationEvent[];
  summary: {
    totalProviders: number;
    healthyProviders: number;
    totalDevicesMonitored: number;
    activeAlerts: number;
    lastSyncAll: string;
  };
};

export const PROVIDER_LABELS: Record<IntegrationProvider, string> = {
  crowdstrike: 'CrowdStrike',
  sentinelone: 'SentinelOne',
  defender: 'Microsoft Defender',
  veeam: 'Veeam',
  commvault: 'Commvault',
  druva: 'Druva',
  autotask: 'Autotask',
  connectwise: 'ConnectWise',
  freshservice: 'Freshservice',
  zendesk: 'Zendesk',
};

export const PROVIDER_ICONS: Record<IntegrationProvider, string> = {
  crowdstrike: '🛡️',
  sentinelone: '🔵',
  defender: '🟢',
  veeam: '💾',
  commvault: '📦',
  druva: '☁️',
  autotask: '📋',
  connectwise: '🔗',
  freshservice: '🆘',
  zendesk: '🎧',
};

export const HEALTH_COLORS: Record<ProviderHealth, string> = {
  healthy: 'text-green-600 dark:text-green-400',
  degraded: 'text-amber-600 dark:text-amber-400',
  error: 'text-red-600 dark:text-red-400',
  not_configured: 'text-slate-400 dark:text-slate-500',
};

export const HEALTH_BG: Record<ProviderHealth, string> = {
  healthy: 'bg-green-100 dark:bg-green-900/30 border-green-300 dark:border-green-700',
  degraded: 'bg-amber-100 dark:bg-amber-900/30 border-amber-300 dark:border-amber-700',
  error: 'bg-red-100 dark:bg-red-900/30 border-red-300 dark:border-red-700',
  not_configured: 'bg-slate-100 dark:bg-slate-800/50 border-slate-200 dark:border-slate-700',
};

export const EVENT_SEVERITY_COLORS: Record<IntegrationEvent['severity'], string> = {
  info: 'text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/20',
  warning: 'text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-900/20',
  critical: 'text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-900/20',
};

export function createMockDashboardData(): IntegrationDashboardData {
  return {
    providers: [
      {
        provider: 'crowdstrike',
        integration: 'edr',
        health: 'healthy',
        totalDevices: 420,
        alertsToday: 12,
        alertsLast7Days: 67,
        lastSyncAt: '2026-08-06T02:00:00Z',
        apiLatencyMs: 45,
        uptime: 99.97,
      },
      {
        provider: 'sentinelone',
        integration: 'edr',
        health: 'healthy',
        totalDevices: 156,
        alertsToday: 5,
        alertsLast7Days: 23,
        lastSyncAt: '2026-08-06T01:45:00Z',
        apiLatencyMs: 62,
        uptime: 99.95,
      },
      {
        provider: 'defender',
        integration: 'edr',
        health: 'degraded',
        totalDevices: 89,
        alertsToday: 28,
        alertsLast7Days: 145,
        lastSyncAt: '2026-08-06T01:30:00Z',
        apiLatencyMs: 230,
        uptime: 98.5,
      },
      {
        provider: 'veeam',
        integration: 'backup',
        health: 'healthy',
        totalDevices: 35,
        alertsToday: 0,
        alertsLast7Days: 3,
        lastSyncAt: '2026-08-06T00:00:00Z',
        apiLatencyMs: 120,
        uptime: 99.99,
      },
      {
        provider: 'autotask',
        integration: 'psa',
        health: 'not_configured',
        totalDevices: 0,
        alertsToday: 0,
        alertsLast7Days: 0,
        lastSyncAt: '',
        apiLatencyMs: 0,
        uptime: 0,
      },
    ],
    events: [
      {
        id: 'evt-001',
        provider: 'crowdstrike',
        integration: 'edr',
        eventType: 'threat_detected',
        severity: 'critical',
        deviceCount: 3,
        message: 'Ransomware threat detected on 3 endpoints',
        timestamp: '2026-08-06T02:15:00Z',
        acknowledged: false,
      },
      {
        id: 'evt-002',
        provider: 'sentinelone',
        integration: 'edr',
        eventType: 'malware_quarantined',
        severity: 'warning',
        deviceCount: 1,
        message: 'Trojan quarantined on dev-003',
        timestamp: '2026-08-06T01:50:00Z',
        acknowledged: false,
      },
      {
        id: 'evt-003',
        provider: 'defender',
        integration: 'edr',
        eventType: 'policy_violation',
        severity: 'warning',
        deviceCount: 12,
        message: 'Firewall policy violation across 12 endpoints',
        timestamp: '2026-08-06T01:30:00Z',
        acknowledged: true,
      },
      {
        id: 'evt-004',
        provider: 'veeam',
        integration: 'backup',
        eventType: 'backup_completed',
        severity: 'info',
        deviceCount: 35,
        message: 'Daily backup completed for all 35 devices',
        timestamp: '2026-08-06T00:05:00Z',
        acknowledged: true,
      },
      {
        id: 'evt-005',
        provider: 'crowdstrike',
        integration: 'edr',
        eventType: 'device_offline',
        severity: 'info',
        deviceCount: 2,
        message: '2 devices offline for >24 hours',
        timestamp: '2026-08-05T22:00:00Z',
        acknowledged: false,
      },
    ],
    summary: {
      totalProviders: 5,
      healthyProviders: 3,
      totalDevicesMonitored: 680,
      activeAlerts: 2,
      lastSyncAll: '2026-08-06T02:15:00Z',
    },
  };
}
