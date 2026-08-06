import type { ReactNode } from 'react';

// Smart Group DSL expression types — mirrors the Go DSL evaluator.

export type Operator =
  | 'eq'
  | 'neq'
  | 'gt'
  | 'gte'
  | 'lt'
  | 'lte'
  | 'contains'
  | 'startswith'
  | 'in'
  | 'contains_any'
  | 'is_null'
  | 'not_null'
  | 'regex';

export type LogicOperator = 'AND' | 'OR';

export interface SmartGroupFilter {
  field: string;
  op: Operator;
  value: string | string[] | number;
  caseInsensitive?: boolean;
}

export interface SmartGroupExpression {
  condition: LogicOperator;
  filters?: SmartGroupFilter[];
  children?: SmartGroupExpression[];
}

export type SmartGroupStatus = 'active' | 'paused' | 'error';

export type SmartGroupType = 'smart' | 'static';

export interface SmartGroupMember {
  id: string;
  hostname: string;
  os: string;
  lastSeen: string;
  ip: string;
  tags: string[];
}

export interface SmartGroupScriptBinding {
  id: string;
  scheduleId: string;
  scheduleName: string;
  bindingType: string;
  priority: number;
  enabled: boolean;
  createdAt: string;
}

export interface SmartGroup {
  id: string;
  name: string;
  description: string;
  isSmart: boolean;
  filterExpression: SmartGroupExpression | null;
  memberCount: number;
  status: SmartGroupStatus;
  lastEvaluated: string | null;
  clientId: string;
  createdAt: string;
  updatedAt: string;
  bindings: SmartGroupScriptBinding[];
}

export interface SmartGroupListData {
  groups: SmartGroup[];
  total: number;
  page: number;
  pageSize: number;
}

export type FieldOption = {
  label: string;
  value: string;
};

export const FIELD_OPTIONS: FieldOption[] = [
  { label: 'Hostname', value: 'hostname' },
  { label: 'OS Name', value: 'os' },
  { label: 'OS Version', value: 'os_version' },
  { label: 'Platform', value: 'platform' },
  { label: 'Architecture', value: 'arch' },
  { label: 'IP Address', value: 'ip_address' },
  { label: 'CPU Usage', value: 'cpu_usage' },
  { label: 'Memory Usage', value: 'memory_usage' },
  { label: 'Disk Usage', value: 'disk_usage' },
  { label: 'Tags', value: 'tags' },
  { label: 'Last Seen', value: 'last_seen' },
  { label: 'Agent Version', value: 'agent_version' },
];

export const OPERATOR_LABELS: Record<Operator, string> = {
  eq: 'Equals',
  neq: 'Not equals',
  gt: 'Greater than',
  gte: 'Greater than or equals',
  lt: 'Less than',
  lte: 'Less than or equals',
  contains: 'Contains',
  startswith: 'Starts with',
  in: 'In list',
  contains_any: 'Contains any',
  is_null: 'Is null',
  not_null: 'Is not null',
  regex: 'Matches regex',
};

export function createMockSmartGroups(): SmartGroup[] {
  return [
    {
      id: 'group-001',
      name: 'Linux Servers',
      description: 'All Linux-based servers',
      isSmart: true,
      filterExpression: {
        condition: 'AND',
        filters: [
          { field: 'os', op: 'startswith', value: 'Linux' },
          { field: 'cpu_usage', op: 'lt', value: 90 },
        ],
      },
      memberCount: 42,
      status: 'active',
      lastEvaluated: '2026-08-06T01:00:00Z',
      clientId: 'client-001',
      createdAt: '2026-06-01T10:00:00Z',
      updatedAt: '2026-08-05T08:30:00Z',
      bindings: [
        {
          id: 'binding-001',
          scheduleId: 'sched-001',
          scheduleName: 'Security Patch Scan',
          bindingType: 'scheduled',
          priority: 50,
          enabled: true,
          createdAt: '2026-07-01T12:00:00Z',
        },
      ],
    },
    {
      id: 'group-002',
      name: 'Windows Workstations',
      description: 'Windows 10/11 workstations for technicians',
      isSmart: true,
      filterExpression: {
        condition: 'AND',
        filters: [
          { field: 'os', op: 'contains', value: 'Windows', caseInsensitive: true },
        ],
      },
      memberCount: 156,
      status: 'active',
      lastEvaluated: '2026-08-06T01:00:00Z',
      clientId: 'client-001',
      createdAt: '2026-05-15T09:00:00Z',
      updatedAt: '2026-08-04T14:20:00Z',
      bindings: [],
    },
    {
      id: 'group-003',
      name: 'Backup Agents',
      description: 'Devices running backup agent',
      isSmart: false,
      filterExpression: null,
      memberCount: 15,
      status: 'active',
      lastEvaluated: null,
      clientId: 'client-002',
      createdAt: '2026-07-10T16:00:00Z',
      updatedAt: '2026-08-03T11:45:00Z',
      bindings: [],
    },
  ];
}

export function createMockMembers(): SmartGroupMember[] {
  return [
    {
      id: 'dev-001',
      hostname: 'web-server-01',
      os: 'Ubuntu 22.04',
      lastSeen: '2026-08-06T01:30:00Z',
      ip: '10.0.1.10',
      tags: ['web', 'production'],
    },
    {
      id: 'dev-002',
      hostname: 'db-server-01',
      os: 'CentOS 8',
      lastSeen: '2026-08-06T01:28:00Z',
      ip: '10.0.1.20',
      tags: ['database', 'production'],
    },
    {
      id: 'dev-003',
      hostname: 'app-server-01',
      os: 'Ubuntu 20.04',
      lastSeen: '2026-08-06T01:25:00Z',
      ip: '10.0.1.30',
      tags: ['application', 'production'],
    },
  ];
}
