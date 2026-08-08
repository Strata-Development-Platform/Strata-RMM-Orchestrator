# Strata RMM — Dashboard Architecture

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Dashboard Overview

Strata provides a React TypeScript frontend (React 19+, Vite, Tailwind CSS) with real API integration. All pages fetch live data from the orchestrator API — no mock data in production.

---

## 2. Tech Stack

| Layer | Technology |
|-------|------------|
| Framework | React 19+ with TypeScript |
| Build | Vite |
| Routing | React Router v6 (BrowserRouter) |
| State | React Context + useState (no external state library) |
| Data Fetching | Direct `fetch()` via singleton `ApiClient` (no React Query/SWR) |
| Styling | Tailwind CSS (dark mode via `dark:` variant, slate palette) |
| Icons | Lucide React (`lucide-react`) |
| Testing | Vitest (600+ frontend tests) |
| Path Aliases | `@/` maps to `src/` |

---

## 3. Application Structure

```
ui/src/
├── api/                          # API client, types, tests
│   ├── client.ts                 # Singleton ApiClient (fetch wrapper)
│   ├── types.ts                  # 24+ TypeScript types
│   └── client.test.ts
├── components/
│   ├── layout/                   # Shell layout, sidebar, branding
│   │   ├── Layout.tsx
│   │   └── ProductAttribution.tsx
│   ├── shared/                   # Reusable primitives
│   │   ├── StatusBadge.tsx       # Status indicator badges
│   │   ├── ThemeToggle.tsx       # Light/dark toggle
│   │   ├── Skeleton.tsx          # Loading placeholders
│   │   ├── Toast.tsx             # Notifications (4s auto-dismiss)
│   │   ├── EmptyState.tsx        # Empty data state
│   │   └── ConfirmDialog.tsx     # Confirmation modal
│   ├── smart-groups/             # Device group management
│   │   ├── DeviceGroupListPage.tsx
│   │   ├── SmartGroupDetailPage.tsx
│   │   ├── SmartGroupTypes.tsx
│   │   └── SmartGroupExpressionBuilder.tsx
│   ├── settings/                 # Integration configuration panels
│   │   ├── IntegrationSettingsPanel.tsx
│   │   ├── IntegrationDashboardPanel.tsx
│   │   └── related types
│   ├── policies/                 # Policy tree visualization
│   │   ├── PolicyTreeView.tsx
│   │   └── PolicyTreeTypes.tsx
│   └── provider/                 # MSP onboarding forms
│       ├── ProviderBusinessSettings.tsx
│       └── ProviderProfileFields.tsx
├── hooks/                        # Custom hooks
│   ├── useAuth.tsx               # Auth state: user, login, logout, refresh
│   └── useWorkspace.tsx          # Workspace context: scope, permissions
├── lib/                          # Utilities
│   └── providerProfile.ts        # Provider profile fields, validation
├── pages/                        # Page components (20 pages)
│   ├── LoginPage.tsx
│   ├── ActivateAccountPage.tsx
│   ├── DashboardPage.tsx
│   ├── LegalPage.tsx
│   ├── CustomersPage.tsx
│   ├── CustomerDetailPage.tsx
│   ├── UserManagementPage.tsx
│   ├── AdminSettingsPage.tsx
│   ├── SettingsPage.tsx
│   ├── ScriptsPage.tsx
│   ├── SoftwarePage.tsx
│   ├── ThirdPartyPage.tsx
│   ├── ReportsPage.tsx
│   ├── JobsPage.tsx
│   ├── JobHealthPage.tsx
│   ├── MSPListPage.tsx
│   ├── MSPWorkspacePage.tsx
│   ├── DeviceWorkspacePage.tsx
│   ├── DeviceRemotePage.tsx
│   └── ProviderSetupPage.tsx
├── App.tsx                       # Root component + routing
├── main.tsx                      # Entry point
├── index.css                     # Global styles
└── App.test.tsx                  # App-level tests
```

---

## 4. Routing (App.tsx)

Routes defined in `App.tsx` using React Router v6. Four route-guard patterns:

| Guard | Purpose |
|-------|---------|
| `ProtectedRoute` | Requires authentication + workspace |
| `PlatformRoute` | Requires auth + platform admin role |
| `CapabilityRoute` | Requires auth + specific API permission strings |
| `ProviderSetupRoute` | Requires auth + platform admin + incomplete setup |

### Public Routes
| Path | Page |
|------|------|
| `/login` | LoginPage |
| `/activate-account` | ActivateAccountPage |
| `/legal` | LegalPage |

### Protected Routes (Layout Shell)
| Path | Page | Required Permissions |
|------|------|---------------------|
| `/` | DashboardPage | device:read |
| `/customers` | CustomersPage | client:read, client:manage, msp:manage |
| `/customers/:id` | CustomerDetailPage | client:read, client:manage, msp:manage |
| `/admin/users` | UserManagementPage | platform:admin only |
| `/admin/settings` | AdminSettingsPage | msp:manage, platform:manage |
| `/settings` | SettingsPage | protected |
| `/scripts` | ScriptsPage | device:manage, job:manage |
| `/software` | SoftwarePage | device:read, device:manage |
| `/thirdparty` | ThirdPartyPage | device:manage, job:manage |
| `/reports` | ReportsPage | device:read, device:manage |
| `/jobs` | JobsPage | job:read, job:manage |
| `/jobs/health` | JobHealthPage | job:read, job:manage, platform:manage |
| `/msp` | MSPWorkspacePage | msp:manage, client:read, client:manage |
| `/devices/:deviceID` | DeviceWorkspacePage | protected |
| `/remote/:tid/:did` | DeviceRemotePage | protected |
| `/groups` | DeviceGroupListPage | device:manage, job:manage |
| `/groups/:groupId` | SmartGroupDetailPage | device:manage, job:manage |

### Platform Admin Routes
| Path | Page |
|------|------|
| `/platform/msps` | MSPListPage |
| `/provider/setup` | ProviderSetupPage |

---

## 5. Layout Shell

`components/layout/Layout.tsx` wraps all protected routes with:

- **Collapsible sidebar navigation** with permission-based filtering
- **Workspace scope switcher** dropdown (platform/MSP level)
- **User panel** (avatar, email, role)
- **Theme toggle** (light/dark mode)
- **Sign out button**
- **Product attribution** footer
- **Main content area** (max-w-7xl, p-6)

---

## 6. State Management

No external state management library. Three React Context providers:

| Context | Provider | Wraps |
|---------|----------|-------|
| `AuthContext` | `hooks/useAuth.tsx` | Entire app |
| `WorkspaceState` | `hooks/useWorkspace.tsx` | Entire app (inside AuthProvider) |
| `ToastContext` | `components/shared/Toast.tsx` | Entire app (inside BrowserRouter) |

**Token persistence:** `localStorage` key `strata_auth_token` (managed in `api/client.ts`)

**Page-local state:** Each page uses `useState` and `useEffect` for data fetching. No form library (React Hook Form, etc.) — all forms are hand-built with controlled inputs.

---

## 7. API Layer

`api/client.ts` — Singleton `ApiClient` class wrapping `fetch()`:
- Auth token handling (from localStorage)
- 401 redirect to login
- Error mapping
- Typed request/response methods

### Domain Areas Covered

| Domain | API Methods |
|--------|-------------|
| Auth | login, logout, me, invitation inspect/accept |
| Platform | overview, customers, workspace context, provider profile, MSP CRUD, branding, domains, enrollment tokens, entitlements, usage, audit |
| Admin | user CRUD, customer creation |
| Devices | device listing, detail, alerts, vulnerabilities, recordings, keys |
| Scripts | CRUD, execution, execution history |
| Software | package CRUD, deployments |
| Access | users, audit, permissions per tenant |
| Device Groups | CRUD, smart groups, members, evaluation status, script bindings |

---

## 8. Shared Components

| Component | Purpose |
|-----------|---------|
| `StatusBadge` | Status indicator badges |
| `ThemeToggle` | Light/dark mode toggle |
| `Skeleton` | Loading skeleton placeholders |
| `Toast` | Toast notifications (success/error/info, 4s auto-dismiss) |
| `EmptyState` | Empty data state component |
| `ConfirmDialog` | Confirmation modal |

---

## 9. Frontend Test Coverage

| Area | Tests |
|------|-------|
| API client | `api/client.test.ts` |
| Provider profile validation | `lib/providerProfile.test.ts` |
| Pages | 20 page test files |
| Components | 22 component test files |
| Hooks | `useAuth.test.tsx`, `useWorkspace.test.tsx` |
| **Total** | **600+ frontend tests** |

---

*Last Updated: 2026-08-08*
