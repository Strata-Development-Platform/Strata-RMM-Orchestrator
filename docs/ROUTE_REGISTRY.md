# Route Authorization Registry

## Route Classification

| Method | Path | Auth | TokenUse | Permission | Resource | Scope |
|--------|------|------|----------|------------|----------|-------|
| GET | / | public | - | - | - | - |
| GET | /health | public | - | - | - | - |
| POST | /api/v1/auth/login | public | - | - | - | - |
| GET | /install.sh | public | - | - | - | - |
| GET | /releases/latest/agent/{os}/{arch} | public | - | - | - | - |
| GET | /api/v1/auth/me | required | user | authenticated | user | user |
| POST | /api/v1/enroll | required | user+agent | admin | msp | msp |
| POST | /api/v1/agent/register | required | agent | agent | device | tenant |
| POST | /api/v1/agent/config | required | agent | agent | device | tenant |
| GET | /api/v1/platform/overview | required | user | authenticated | platform | user |
| GET | /api/v1/platform/customers | required | user | msp_admin | msp | msp |
| GET | /api/v1/platform/customers/{id}/devices | required | user | msp_admin | client | client |
| GET | /api/v1/admin/users | required | user | platform_admin | platform | platform |
| POST | /api/v1/admin/users | required | user | platform_admin | platform | platform |
| GET/POST/PUT/DELETE | /api/v1/scripts/* | required | user | msp_technician | client | client |
| GET/POST/PUT/DELETE | /api/v1/software/* | required | user | msp_technician | client | client |
| GET/POST/DELETE | /api/v1/alerts/* | required | user | msp_technician | client | client |
| GET/POST | /api/v1/remote/* | required | user | msp_technician | client | client |
| GET/POST/DELETE | /api/v1/keys/* | required | user | msp_admin | client | client |
| GET/POST | /api/v1/branding | required | user | msp_admin | msp | msp |
| GET/POST | /api/v1/domains | required | user | msp_admin | msp | msp |
| GET/POST | /api/v2/platform/msps | required | user | platform_admin | platform | platform |
| POST | /api/v2/platform/msps/{id}/suspend | required | user | platform_admin | msp | platform |
| POST | /api/v2/platform/msps/{id}/activate | required | user | platform_admin | msp | platform |
| GET/POST | /api/v2/msps/{id}/clients | required | user | msp_admin | msp | msp |
| GET/POST | /api/v2/clients/{id}/sites | required | user | client_admin | client | client |
| GET/POST | /api/v2/msps/{id}/memberships | required | user | msp_admin | msp | msp |
