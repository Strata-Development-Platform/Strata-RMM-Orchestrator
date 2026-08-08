# Strata RMM — Information Architecture

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Navigation Structure

### 1.1 Sidebar Navigation

The sidebar shows menu items filtered by:
- User permissions
- Workspace scope (platform/MSP)
- Feature availability

### 1.2 Menu Items

| Menu Item | Route | Page | Permission |
|-----------|-------|------|------------|
| Dashboard | `/` | DashboardPage | device:read |
| Devices | `/devices/:deviceID` | DeviceWorkspacePage | protected |
| Remote | `/remote/:tid/:did` | DeviceRemotePage | protected |
| Scripts | `/scripts` | ScriptsPage | device:manage, job:manage |
| Software | `/software` | SoftwarePage | device:read, device:manage |
| Groups | `/groups` | DeviceGroupListPage | device:manage, job:manage |
| Policies | (via groups) | SmartGroupDetailPage | device:manage, job:manage |
| Jobs | `/jobs` | JobsPage | job:read, job:manage |
| Reports | `/reports` | ReportsPage | device:read, device:manage |
| Jobs Health | `/jobs/health` | JobHealthPage | job:read, job:manage, platform:manage |
| Third-Party | `/thirdparty` | ThirdPartyPage | device:manage, job:manage |
| Customers | `/customers` | CustomersPage | client:read, client:manage, msp:manage |
| Admin | `/admin/users` | UserManagementPage | platform:admin |
| Admin Settings | `/admin/settings` | AdminSettingsPage | msp:manage, platform:manage |
| Settings | `/settings` | SettingsPage | protected |
| Platform | `/platform/msps` | MSPListPage | platform:admin only |
| Provider Setup | `/provider/setup` | ProviderSetupPage | platform:admin, incomplete setup |

---

## 2. Workspace Scopes

Three workspace scopes with different views:

### 2.1 Platform Scope
- Access to all MSPs
- User management
- Provider setup
- Platform settings

### 2.2 MSP Scope
- Access to MSP clients
- Device management
- Script execution
- Software deployment
- Reports
- Settings

### 2.3 Client Scope (via workspace switcher)
- Access to client devices
- Client support requests
- Client settings

---

## 3. Page Hierarchy

### 3.1 Dashboard (Root)
```
Dashboard
├── Overview panels (devices, alerts, patches)
├── Telemetry charts
└── Quick actions
```

### 3.2 Device Workspace
```
Devices → :deviceID
├── Device details (OS, hardware, status)
├── Telemetry (CPU, RAM, disk, net)
├── Alerts
├── Vulnerabilities
├── Packages
├── Jobs history
└── Remote access
```

### 3.3 Remote Access
```
Remote → :tid/:did
├── WebRTC session controls
├── Screen capture view
├── Input controls
├── Recording controls
└── Transcription status
```

### 3.4 Scripts
```
Scripts
├── Script list
├── Create script
├── Script detail
│   ├── Script content
│   ├── Execution history
│   └── Schedule management
├── Schedule list
├── Create schedule
└── Schedule detail
    ├── Schedule config
    ├── Device targets
    └── Execution results
```

### 3.5 Software
```
Software
├── Package list
├── Create package
├── Package detail
├── Deployments
├── Create deployment
└── Deployment detail
```

### 3.6 Groups
```
Groups
├── Device group list
├── Create group (standard/smart)
├── Smart group detail
│   ├── Expression builder
│   ├── Evaluation status
│   ├── Members list
│   └── Script bindings
└── Standard group detail
    ├── Members list
    └── Script bindings
```

### 3.7 Jobs
```
Jobs
├── Job list (filter by status)
├── Job detail
│   ├── Job info
│   ├── Events timeline
│   └── Actions (cancel, retry)
└── Job health dashboard
    ├── Success rate
    ├── Failure analysis
    └── Performance metrics
```

### 3.8 Reports
```
Reports
├── Report list
├── Generate report
├── Schedule list
├── Create schedule
├── Schedule detail
│   ├── Schedule config
│   └── Actions (trigger, enable/disable)
├── Report download
└── Compliance reports
    ├── List
    ├── Detail
    └── Export (CSV/JSON)
```

### 3.9 Third-Party
```
Third-Party
├── Apps list
├── Packages list
├── Vendors list
├── Sync controls
│   ├── Sync all
│   └── Sync by app/vendor
└── Sync status
```

### 3.10 Customers
```
Customers
├── Customer list
├── Customer detail
│   ├── Client info
│   ├── Site list
│   ├── Device list
│   ├── Support requests
│   └── Settings
└── Create customer
```

### 3.11 Admin
```
Admin
├── User management
│   ├── User list
│   ├── Create user
│   ├── Edit user
│   └── Tenant/membership management
├── Settings
│   ├── Branding
│   ├── Domains
│   ├── Enrollment tokens
│   ├── API keys
│   ├── Maintenance windows
│   └── Retention policies
├── Platform overview
├── Provider profile
├── MSP management
│   ├── MSP list
│   ├── MSP detail
│   ├── Billing
│   ├── Entitlements
│   └── Offboarding
└── System health
```

### 3.12 MSP Workspace
```
MSP
├── Overview
├── Clients
├── Devices
├── Billing
│   ├── Account
│   ├── Subscriptions
│   ├── Payment methods
│   ├── Invoices
│   └── Usage
├── Memberships
├── Entitlements
└── Audit log
```

### 3.13 Settings
```
Settings
├── Profile
├── MFA/OTP
├── API keys
├── Branding (if MSP admin)
├── Domains (if MSP admin)
└── Notifications (future)
```

---

## 4. Authentication Flow

```
Login Page → Auth API → JWT Token → localStorage
    │
    ├── Success → Dashboard (redirected from /login)
    ├── 401 → Stay on login page
    └── Error → Error message

Token Refresh:
    ├── On API 401 → Redirect to login
    └── Token stored in localStorage (strata_auth_token)
```

---

## 5. Data Flow

```
Page Component
    ├── useWorkspace() → Workspace context (scope, permissions)
    └── ApiClient.get/post() → Orchestrator API
            │
            ├── Success → Page state update
            ├── 401 → Redirect to login
            └── Error → Error toast
```

---

## 6. Permission Model

| Permission | Pages Accessed |
|------------|----------------|
| `device:read` | Dashboard, Devices, Reports, Software |
| `device:manage` | Scripts, Software, Groups, Third-Party, Reports |
| `job:read` | Jobs, Jobs Health |
| `job:manage` | Scripts, Jobs, Groups, Third-Party |
| `client:read` | Customers, MSP Workspace |
| `client:manage` | Customers, MSP Workspace |
| `msp:manage` | Admin Settings, MSP Workspace |
| `platform:manage` | User Management, MSP List, Job Health |
| `platform:admin` | User Management, MSP List, Provider Setup |

---

*Last Updated: 2026-08-08*
