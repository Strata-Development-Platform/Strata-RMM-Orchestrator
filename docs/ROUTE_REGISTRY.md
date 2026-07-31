# Route Authorization Registry

## Route Classification

| Method | Path | Auth | TokenUse | Permission | Resource | Scope |
|--------|------|------|----------|------------|----------|-------|
| GET | / | public | - | - | - | - |
| GET | /health | public | - | - | - | - |
| GET | /health/live | public | - | - | - | - |
| GET | /health/ready | public | - | - | - | - |
| POST | /api/v1/auth/login | public | - | - | - | - |
| GET | /install.sh | public | - | - | - | - |
| GET | /releases/latest/agent/{os}/{arch} | public | - | - | - | - |
| GET | /api/v1/auth/me | required | user | authenticated | user | user |
| POST | /api/v1/enroll | required | user | authorized client operator | client | client |
| POST | /api/v1/agent/register | public bootstrap | enrollment token | single-use enrollment | device | enrolled scope |
| POST | /api/v1/agent/config | required | agent | agent | device | tenant |
| GET | /api/v1/platform/overview | required | user | authenticated | platform | user |
| GET | /api/v1/platform/customers | required | user | msp_admin | msp | msp |
| GET | /api/v1/platform/customers/{id}/devices | required | user | msp_admin | client | client |
| GET | /api/v1/admin/users | required | user | platform_admin | platform | platform |
| POST | /api/v1/admin/users | required | user | platform_admin | platform | platform |
| PUT | /api/v1/admin/users/{id}/tenants | required | user | platform_admin | user membership | platform |
| POST | /api/v1/admin/customers | required | user | platform_admin | customer | platform |
| GET | /api/v1/admin/update/check | required | user | platform_admin | release | platform |
| POST | /api/v1/admin/update/apply | required | user | platform_admin | release | platform |
| GET/POST/PUT/DELETE | /api/v1/scripts/* | required | user | msp_technician | client | client |
| GET/POST/PUT/DELETE | /api/v1/software/* | required | user | msp_technician | client | client |
| GET/POST/DELETE | /api/v1/alerts/* | required | user | msp_technician | client | client |
| GET/POST | /api/v1/remote/* | required | user | msp_technician | client | client |
| GET/POST/DELETE | /api/v1/keys/* | required | user | msp_admin | client | client |
| GET/POST | /api/v1/branding | required | user | msp_admin | msp | msp |
| GET/POST | /api/v1/domains | required | user | msp_admin | msp | msp |
| GET/POST | /api/v2/platform/msps | required | user | platform_admin | platform | platform |
| GET | /api/v2/platform/msps/{id} | required | user | platform_admin | msp | platform |
| POST | /api/v2/platform/msps/{id}/suspend | required | user | platform_admin | msp | platform |
| POST | /api/v2/platform/msps/{id}/activate | required | user | platform_admin | msp | platform |
| GET/POST | /api/v2/platform/msps/{id}/offboarding | required | user | platform_admin | msp | platform |
| POST | /api/v2/platform/msps/{id}/offboarding/approve-deletion | required | user | platform_admin | msp | platform |
| GET | /api/v2/platform/msps/{id}/export | required | user | platform_admin | msp | platform |
| PATCH | /api/v2/platform/msps/{id}/entitlement | required | user | platform_admin | msp | platform |
| PATCH | /api/v2/platform/domains/{id}/certificate | required | user | platform_admin | domain | platform |
| POST/DELETE | /api/v2/platform/support-grants/* | required | user | platform_admin | support grant | platform |
| GET | /api/v2/deployment/state | required | user | platform_admin | deployment | platform |
| GET | /api/v2/deployment/history | required | user | platform_admin | deployment | platform |
| GET/POST | /api/v2/msps/{id}/clients | required | user | msp_admin | msp | msp |
| GET/POST | /api/v2/clients/{id}/sites | required | user | client_admin | client | client |
| GET/POST | /api/v2/msps/{id}/memberships | required | user | msp_admin | msp | msp |

The table documents registered privileged operations. Independently of the
table, `/api/v1/admin/*`, `/api/v2/platform/*`, and `/api/v2/deployment/*` fail
closed to platform-admin access so a future handler cannot inherit ordinary user
access when an inventory entry is missed. Handler-level authorization and scoped
database transactions remain mandatory; route classification is not a substitute
for resource ownership checks.
