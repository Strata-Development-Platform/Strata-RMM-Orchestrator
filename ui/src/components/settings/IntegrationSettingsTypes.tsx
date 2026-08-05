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

export type ApiKeyStatus = 'active' | 'revoked' | 'expired';

export type ApiKey = {
  id: string;
  name: string;
  integration: IntegrationType;
  provider: IntegrationProvider;
  key: string; // Full key shown on creation, masked after
  maskedKey: string;
  status: ApiKeyStatus;
  createdAt: string;
  lastUsedAt?: string;
  expiresAt?: string;
};

export type WebhookConfig = {
  id: string;
  name: string;
  integration: IntegrationType;
  provider: IntegrationProvider;
  url: string;
  secret: string; // HMAC secret
  enabled: boolean;
  createdAt: string;
};

export type IntegrationStatus = {
  integration: IntegrationType;
  provider: IntegrationProvider;
  alertsReceived: number;
  alertsProcessed: number;
  lastAlertAt?: string;
  health: 'healthy' | 'degraded' | 'error' | 'not_configured';
};

export type IntegrationSettingsData = {
  apiKeys: ApiKey[];
  webhooks: WebhookConfig[];
  statuses: IntegrationStatus[];
};

export function createMockApiKeys(): ApiKey[] {
  return [
    {
      id: 'key-001',
      name: 'CrowdStrike Production',
      integration: 'edr',
      provider: 'crowdstrike',
      key: 'csapi-abc123def456ghi789jkl012',
      maskedKey: 'csapi-abc••••••••••••••••••••',
      status: 'active',
      createdAt: '2026-06-01T10:00:00Z',
      lastUsedAt: '2026-08-05T08:30:00Z',
      expiresAt: '2027-06-01T10:00:00Z',
    },
    {
      id: 'key-002',
      name: 'SentinelOne Staging',
      integration: 'edr',
      provider: 'sentinelone',
      key: 's1-xyz789uvw456rst123',
      maskedKey: 's1-xyz••••••••••••••',
      status: 'active',
      createdAt: '2026-07-15T14:00:00Z',
      lastUsedAt: '2026-08-04T16:45:00Z',
    },
    {
      id: 'key-003',
      name: 'Veeam Backup Primary',
      integration: 'backup',
      provider: 'veeam',
      key: 'veeam-pbk-mno345pqr678',
      maskedKey: 'veeam-pbk••••••••••',
      status: 'active',
      createdAt: '2026-05-20T09:00:00Z',
      lastUsedAt: '2026-08-05T07:00:00Z',
      expiresAt: '2026-12-31T23:59:59Z',
    },
    {
      id: 'key-004',
      name: 'Autotask Legacy',
      integration: 'psa',
      provider: 'autotask',
      key: 'at-legacy-abc000def111',
      maskedKey: 'at-legacy-abc••••••',
      status: 'revoked',
      createdAt: '2025-11-01T12:00:00Z',
      lastUsedAt: '2026-03-15T10:00:00Z',
    },
  ];
}

export function createMockWebhooks(): WebhookConfig[] {
  return [
    {
      id: 'wh-001',
      name: 'CrowdStrike Alert Ingestion',
      integration: 'edr',
      provider: 'crowdstrike',
      url: '/api/v1/integrations/edr/alerts',
      secret: 'whsec_cs••••••••••••••••••••••••••••',
      enabled: true,
      createdAt: '2026-06-01T10:30:00Z',
    },
    {
      id: 'wh-002',
      name: 'Veeam Backup Sync',
      integration: 'backup',
      provider: 'veeam',
      url: '/api/v1/integrations/backup/sync',
      secret: 'whsec_vbm••••••••••••••••••••••••••••',
      enabled: true,
      createdAt: '2026-05-20T09:30:00Z',
    },
    {
      id: 'wh-003',
      name: 'Autotask Ticket Events',
      integration: 'psa',
      provider: 'autotask',
      url: '/api/v1/integrations/psa/webhooks',
      secret: 'whsec_at••••••••••••••••••••••••••••',
      enabled: false,
      createdAt: '2025-11-01T12:30:00Z',
    },
  ];
}

export function createMockStatuses(): IntegrationStatus[] {
  return [
    {
      integration: 'edr',
      provider: 'crowdstrike',
      alertsReceived: 1247,
      alertsProcessed: 1247,
      lastAlertAt: '2026-08-05T08:30:00Z',
      health: 'healthy',
    },
    {
      integration: 'edr',
      provider: 'sentinelone',
      alertsReceived: 83,
      alertsProcessed: 81,
      lastAlertAt: '2026-08-04T16:45:00Z',
      health: 'degraded',
    },
    {
      integration: 'backup',
      provider: 'veeam',
      alertsReceived: 0,
      alertsProcessed: 0,
      health: 'healthy',
    },
    {
      integration: 'psa',
      provider: 'autotask',
      alertsReceived: 0,
      alertsProcessed: 0,
      health: 'not_configured',
    },
  ];
}

export function createMockSettingsData(): IntegrationSettingsData {
  return {
    apiKeys: createMockApiKeys(),
    webhooks: createMockWebhooks(),
    statuses: createMockStatuses(),
  };
}

export const INTEGRATION_LABELS: Record<IntegrationType, string> = {
  edr: 'Endpoint Detection & Response',
  backup: 'Backup & Recovery',
  psa: 'Professional Services Automation',
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

export const STATUS_COLORS: Record<ApiKeyStatus, { bg: string; text: string; dot: string }> = {
  active: {
    bg: 'bg-green-100 dark:bg-green-900/30',
    text: 'text-green-700 dark:text-green-400',
    dot: 'bg-green-500',
  },
  revoked: {
    bg: 'bg-red-100 dark:bg-red-900/30',
    text: 'text-red-700 dark:text-red-400',
    dot: 'bg-red-500',
  },
  expired: {
    bg: 'bg-amber-100 dark:bg-amber-900/30',
    text: 'text-amber-700 dark:text-amber-400',
    dot: 'bg-amber-500',
  },
};

export const HEALTH_COLORS: Record<IntegrationStatus['health'], { bg: string; text: string; dot: string }> = {
  healthy: {
    bg: 'bg-green-100 dark:bg-green-900/30',
    text: 'text-green-700 dark:text-green-400',
    dot: 'bg-green-500',
  },
  degraded: {
    bg: 'bg-amber-100 dark:bg-amber-900/30',
    text: 'text-amber-700 dark:text-amber-400',
    dot: 'bg-amber-500',
  },
  error: {
    bg: 'bg-red-100 dark:bg-red-900/30',
    text: 'text-red-700 dark:text-red-400',
    dot: 'bg-red-500',
  },
  not_configured: {
    bg: 'bg-slate-100 dark:bg-slate-800',
    text: 'text-slate-500 dark:text-slate-400',
    dot: 'bg-slate-400',
  },
};
