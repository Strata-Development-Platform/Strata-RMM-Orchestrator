# Information Architecture

## MSP Navigation

The MSP-facing UI is organized into functional groups accessible from the left sidebar.

### Overview

| Route | Page | Description |
|-------|------|-------------|
| `/` | Dashboard | Platform-wide overview with stat cards (customers, devices, online, alerts, CVEs), priority issues panel, customer table with device/alert/CVE counts |

### Operations

| Route | Page | Description |
|-------|------|-------------|
| `/remote/:tid/:did` | Remote Control | Live screen view with quality/FPS controls for remote desktop sessions |
| `/scripts` | Scripts | Script library with editor, run dispatch, execution history viewer |
| `/software` | Software | Package library, deployment creation, deployment tracking |
| `/thirdparty` | Patch Mgmt | Third-party application list, sync controls, patch policy management |

### Assets

| Route | Page | Description |
|-------|------|-------------|
| `/customers` | Customers | Customer list with deployment IDs, device/alert/CVE counts, plan tier |
| `/customers/:id` | Customer Detail | Tabbed drill-down: Devices, Alerts, Vulnerabilities, Recordings, Settings |

### Management

| Route | Page | Description |
|-------|------|-------------|
| `/admin/users` | Users | User management with create, tenant scoping, role assignment |
| `/admin/settings` | Settings | Platform status, CVE sync management, API reference, system configuration |

### Customers (Drill-down tabs on `/customers/:id`)

| Tab | Content |
|-----|---------|
| **Devices** | Device list with status, last heartbeat, OS, hardware info |
| **Alerts** | Active and historical alerts with severity, acknowledgment, resolution |
| **Vulnerabilities** | Open CVEs with severity, package, device, remediation actions |
| **Recordings** | Session recording list with date, duration, playback (MFA-gated) |
| **Settings** | Customer configuration, deployment ID, plan, maintenance windows |

### Library (Planned)

- Script templates
- Software package repository
- Policy templates
- Report templates

### Administration (Planned)

- Billing and subscription management
- Audit log viewer
- API key management
- System health and monitoring
- Branding configuration

---

## Platform Navigation

The operator platform provides a higher-level view across all MSP tenants.

### Navigation Groups

| Group | Sections | Description |
|-------|----------|-------------|
| **MSP Tenants** | List, Detail, Billing | Manage MSP organizations, plans, usage |
| **Subscriptions** | Plans, Invoices, Usage | Subscription tier management and billing |
| **Infrastructure** | Services, Nodes, Status | Platform infrastructure health monitoring |
| **Agent Releases** | Channels, Rollouts, Manifests | Agent update management across tenants |
| **Usage** | Platform Stats, Tenant Stats | Aggregate platform utilization metrics |
| **Security Events** | Audit Log, Access Review | Cross-tenant security event monitoring |
| **Settings** | System Config, SSO, Integrations | Global platform configuration |

---

## Application Shell

### Layout Structure

```
┌─────────────────────────────────────────────────────────┐
│ ┌──────────┐  ┌────────────────────────────────────────┐ │
│ │          │  │  Main Content (max-w-7xl mx-auto)       │ │
│ │ Sidebar  │  │  ┌──────────────────────────────────┐  │ │
│ │ (w-56,   │  │  │  Children (page content)          │  │ │
│ │ collaps- │  │  │                                    │  │ │
│ │ ible to   │  │  │                                    │  │ │
│ │ w-16)    │  │  │                                    │  │ │
│ │          │  │  └──────────────────────────────────┘  │ │
│ │ nav items│  │                                        │ │
│ │          │  │                                        │ │
│ │ user     │  │                                        │ │
│ │ profile  │  │                                        │ │
│ └──────────┘  └────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

### Sidebar States

| State | Width | Behavior |
|-------|-------|----------|
| Expanded | 224px (w-56) | Full labels and icons, user profile visible |
| Collapsed | 64px (w-16) | Icons only, labels shown as title tooltips |

### Navigation Items

**Primary Nav**: Overview, Customers, Users, Scripts, Software, Patch Mgmt, Reports, Settings

**Bottom Nav**: My Settings

**User Section**: Avatar (first letter of email), email, role, sign out, theme toggle

### Theme Support

- Dark/light mode via `ThemeToggle` component
- localStorage persistence via `useTheme` hook
- Tailwind CSS dark mode (`dark:` prefix)

### Application States

| State | Component | Behavior |
|-------|-----------|----------|
| Loading | Inline text "Loading..." | Shown during auth check or data fetch |
| Empty | `EmptyState` component | Icon + title + description + CTA |
| Error | `Toast` (error variant) | 4s auto-dismiss notification |
| Success | `Toast` (success variant) | 4s auto-dismiss notification |
| Confirm | `ConfirmDialog` modal | Destructive action confirmation |

### UI Components

| Component | Purpose |
|-----------|---------|
| `Toast` | Success/error/info notifications, 4s auto-dismiss |
| `Skeleton` | Pulsing table/card/text loading placeholders |
| `ConfirmDialog` | Modal confirmation for destructive actions |
| `EmptyState` | Empty table/grid state with icon, message, CTA |
| `StatusBadge` | Colored pill with dot indicator for any status |
| `ThemeToggle` | Dark/light mode with localStorage persistence |
| `DataTable` | Sortable, filterable tables with row actions |

---

## Design Token Approach

### Color System

| Token | Light | Dark | Usage |
|-------|-------|------|-------|
| `bg-primary` | `slate-50` | `slate-950` | Page background |
| `bg-card` | `white` | `slate-900` | Card/panel backgrounds |
| `bg-sidebar` | `slate-900` | `slate-900` | Sidebar background |
| `text-primary` | `slate-900` | `white` | Primary text |
| `text-muted` | `slate-500` | `slate-400` | Secondary/muted text |
| `accent` | `blue-600` | `blue-600` | Active nav, links, CTAs |
| `danger` | `red-600` | `red-400` | Errors, critical alerts |
| `warning` | `amber-600` | `amber-400` | Warnings |
| `success` | `green-600` | `green-400` | Online, healthy states |
| `border` | `slate-200` | `slate-700` | Borders and dividers |

### Typography

| Token | Size | Weight | Usage |
|-------|------|--------|-------|
| `text-xs` | 12px | 400 | Labels, timestamps |
| `text-sm` | 14px | 400/500 | Body text, table cells |
| `text-base` | 16px | 500 | Default text |
| `text-lg` | 18px | 600 | Section headings |
| `text-2xl` | 24px | 700 | Page titles |

### Spacing

- Uses Tailwind spacing scale (4px base unit)
- Page padding: `p-6`
- Card padding: `p-4`
- Between sections: `space-y-6`
- Between cards: `gap-4`

### Shadow / Elevation

- Cards: `border` (no shadow, flat design)
- Active nav item: `bg-blue-600`
- Hover state: `hover:bg-slate-50` (light) / `hover:bg-slate-800` (dark)
