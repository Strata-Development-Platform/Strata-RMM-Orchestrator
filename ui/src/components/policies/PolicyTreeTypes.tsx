import type { ReactNode } from 'react';

// PolicyTree types — mirrors the policy engine hierarchy and inheritance model.

export type PolicyCategory =
  | 'patch'
  | 'alerting'
  | 'monitoring'
  | 'software'
  | 'scripts'
  | 'maintenance';

export type PolicyScope = 'platform' | 'msp' | 'client' | 'site' | 'device';

export interface PolicyField {
  name: string;
  value: string | number | boolean | null;
  overridden: boolean;
  inheritedFrom?: PolicyScope;
}

export interface PolicyLevel {
  scope: PolicyScope;
  level: number; // 0=platform, 1=mso, 2=client, 3=site, 4=device
  label: string;
  fields: PolicyField[];
}

export interface PolicyTree {
  tenantId: string;
  targetDeviceId?: string;
  categories: Record<PolicyCategory, PolicyLevel[]>;
}

// Policy tree data types — what the API would return.

export type PatchPolicy = {
  approval_mode: 'auto' | 'manual' | string;
  severity: string;
  platforms: string[];
  max_retries: number;
  maintenance_window?: string;
};

export type AlertingRule = {
  metric_name: string;
  condition: string;
  threshold: number;
  severity: string;
  cooldown: number;
};

export type MonitoringConfig = {
  collection_interval: number;
  retention_days: number;
  enabled_metrics: string[];
};

export type SoftwarePolicy = {
  approval_mode: 'auto' | 'manual' | string;
  allowed_types: string[];
  allowed_sources: string[];
};

export type ScriptPolicy = {
  timeout_max: number;
  allowed_languages: string[];
  requires_approval: boolean;
};

export type MaintenanceWindow = {
  start_time: string;
  end_time: string;
  days_of_week: string[];
  timezone: string;
};

// Mock data for testing — a complete policy tree for a single device.

export function createMockPolicyTree(): PolicyTree {
  return {
    tenantId: 'tenant-123',
    targetDeviceId: 'device-456',
    categories: {
      patch: [
        {
          scope: 'platform',
          level: 0,
          label: 'Platform Level',
          fields: [
            { name: 'approval_mode', value: 'auto', overridden: false },
            { name: 'severity', value: 'critical', overridden: false },
            { name: 'max_retries', value: 3, overridden: false },
          ],
        },
        {
          scope: 'msp',
          level: 1,
          label: 'MSP Level',
          fields: [
            { name: 'severity', value: 'important', overridden: true, inheritedFrom: 'platform' },
            { name: 'max_retries', value: 5, overridden: true, inheritedFrom: 'platform' },
          ],
        },
        {
          scope: 'client',
          level: 2,
          label: 'Client Level',
          fields: [
            { name: 'approval_mode', value: 'manual', overridden: true, inheritedFrom: 'platform' },
          ],
        },
        {
          scope: 'site',
          level: 3,
          label: 'Site Level',
          fields: [], // No overrides — inherits from Client
        },
        {
          scope: 'device',
          level: 4,
          label: 'Device Level',
          fields: [], // No overrides — inherits from Site
        },
      ],
      alerting: [
        {
          scope: 'platform',
          level: 0,
          label: 'Platform Level',
          fields: [
            { name: 'severity', value: 'critical', overridden: false },
            { name: 'cooldown', value: 300, overridden: false },
          ],
        },
        {
          scope: 'msp',
          level: 1,
          label: 'MSP Level',
          fields: [
            { name: 'severity', value: 'high', overridden: true, inheritedFrom: 'platform' },
          ],
        },
        {
          scope: 'client',
          level: 2,
          label: 'Client Level',
          fields: [],
        },
        {
          scope: 'site',
          level: 3,
          label: 'Site Level',
          fields: [],
        },
        {
          scope: 'device',
          level: 4,
          label: 'Device Level',
          fields: [
            { name: 'cooldown', value: 60, overridden: true, inheritedFrom: 'platform' },
          ],
        },
      ],
      monitoring: [
        {
          scope: 'platform',
          level: 0,
          label: 'Platform Level',
          fields: [
            { name: 'collection_interval', value: 60, overridden: false },
            { name: 'retention_days', value: 90, overridden: false },
          ],
        },
        {
          scope: 'msp',
          level: 1,
          label: 'MSP Level',
          fields: [],
        },
        {
          scope: 'client',
          level: 2,
          label: 'Client Level',
          fields: [],
        },
        {
          scope: 'site',
          level: 3,
          label: 'Site Level',
          fields: [],
        },
        {
          scope: 'device',
          level: 4,
          label: 'Device Level',
          fields: [],
        },
      ],
      software: [
        {
          scope: 'platform',
          level: 0,
          label: 'Platform Level',
          fields: [
            { name: 'approval_mode', value: 'auto', overridden: false },
          ],
        },
        {
          scope: 'msp',
          level: 1,
          label: 'MSP Level',
          fields: [
            { name: 'approval_mode', value: 'manual', overridden: true, inheritedFrom: 'platform' },
          ],
        },
        {
          scope: 'client',
          level: 2,
          label: 'Client Level',
          fields: [],
        },
        {
          scope: 'site',
          level: 3,
          label: 'Site Level',
          fields: [],
        },
        {
          scope: 'device',
          level: 4,
          label: 'Device Level',
          fields: [],
        },
      ],
      scripts: [
        {
          scope: 'platform',
          level: 0,
          label: 'Platform Level',
          fields: [
            { name: 'timeout_max', value: 300, overridden: false },
            { name: 'requires_approval', value: true, overridden: false },
          ],
        },
        {
          scope: 'msp',
          level: 1,
          label: 'MSP Level',
          fields: [
            { name: 'timeout_max', value: 600, overridden: true, inheritedFrom: 'platform' },
          ],
        },
        {
          scope: 'client',
          level: 2,
          label: 'Client Level',
          fields: [],
        },
        {
          scope: 'site',
          level: 3,
          label: 'Site Level',
          fields: [],
        },
        {
          scope: 'device',
          level: 4,
          label: 'Device Level',
          fields: [],
        },
      ],
      maintenance: [
        {
          scope: 'platform',
          level: 0,
          label: 'Platform Level',
          fields: [
            { name: 'start_time', value: '02:00', overridden: false },
            { name: 'end_time', value: '06:00', overridden: false },
          ],
        },
        {
          scope: 'msp',
          level: 1,
          label: 'MSP Level',
          fields: [
            { name: 'start_time', value: '01:00', overridden: true, inheritedFrom: 'platform' },
            { name: 'end_time', value: '05:00', overridden: true, inheritedFrom: 'platform' },
          ],
        },
        {
          scope: 'client',
          level: 2,
          label: 'Client Level',
          fields: [],
        },
        {
          scope: 'site',
          level: 3,
          label: 'Site Level',
          fields: [],
        },
        {
          scope: 'device',
          level: 4,
          label: 'Device Level',
          fields: [],
        },
      ],
    },
  };
}
