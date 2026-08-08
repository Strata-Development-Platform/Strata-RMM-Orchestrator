# Strata RMM — Route Registry

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Overview

All routes are registered in `internal/platform/api.go` using Go 1.22+ `net/http` method-based routing. Total: 310+ routes.

---

## 2. Health & Observability

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/` | `handleRoot` | Public | Root endpoint |
| GET | `/health` | `handleHealth` | Public | Health check |
| GET | `/health/live` | `handleHealthLive` | Public | Liveness check |
| GET | `/health/ready` | `handleHealthReady` | Public | Readiness check |
| GET | `/metrics` | `handleMetrics` | Metrics token | Prometheus metrics |

---

## 3. Agent Registration

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| POST | `/api/v1/enroll` | `handleEnroll` | Public | Agent enrollment |
| POST | `/api/v1/agent/register` | `handleAgentRegister` | Agent | Agent registration |
| POST | `/api/v1/agent/config` | `handleAgentConfig` | Agent | Agent config fetch |
| GET | `/install.sh` | `handleInstallScript` | Public | Install script |
| GET | `/releases/latest/agent/{os}/{arch}` | `handleReleaseBinary` | Public | Agent binary |
| GET | `/releases/latest/agent/{os}/{arch}/sha256` | `handleReleaseChecksum` | Public | Agent checksum |

---

## 4. Authentication

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| POST | `/api/v1/auth/login` | `handleLogin` | Public | User login |
| POST | `/api/v1/auth/invitations/inspect` | `handleInspectOwnerInvitation` | Public | Inspect invitation |
| POST | `/api/v1/auth/invitations/accept` | `handleAcceptOwnerInvitation` | Public | Accept invitation |
| GET | `/api/v1/auth/me` | `handleMe` | Authenticated | Current user |
| POST | `/api/v1/auth/logout` | `handleLogout` | Authenticated | Logout (blacklist jti) |
| POST | `/api/v1/mfa/enroll/{userID}` | `handleMFAEnroll` | Authenticated | MFA enrollment |
| POST | `/api/v1/mfa/verify/{userID}` | `handleMFAVerify` | Authenticated | MFA verification |
| GET | `/api/v1/mfa/status/{userID}` | `handleMFAStatus` | Authenticated | MFA status |
| DELETE | `/api/v1/mfa/{userID}` | `handleMFADelete` | Authenticated | MFA deletion |

---

## 5. Platform Overview

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/platform/overview` | `handlePlatformOverview` | MSP admin | Platform overview |
| GET | `/api/v1/platform/customers` | `handlePlatformCustomers` | MSP admin | Customer list |
| GET | `/api/v1/platform/customers/{tenantID}/devices` | `handleTenantDevices` | MSP admin | Tenant devices |
| GET | `/api/v1/platform/customers/{tenantID}/devices/{deviceID}` | `handleDeviceInventory` | MSP admin | Device inventory |
| GET | `/api/v1/platform/customers/{tenantID}/devices-with-versions` | `handleDeviceVersion` | MSP admin | Device versions |
| POST | `/api/v1/platform/customers/{tenantID}/update-source` | `handleSetUpdateSource` | MSP admin | Set update source |
| POST | `/api/v1/platform/customers/{tenantID}/devices/{deviceID}/update` | `handleDeviceUpdate` | MSP admin | Update device |
| POST | `/api/v1/platform/customers/{tenantID}/devices/update-all` | `handleDeviceUpdateAll` | MSP admin | Update all devices |

---

## 6. Admin

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/admin/users` | `handleAdminUsers` | MSP manage | User list |
| POST | `/api/v1/admin/users` | `handleAdminCreateUser` | MSP manage | Create user |
| PUT | `/api/v1/admin/users/{userID}/tenants` | `handleAdminUpdateUserTenants` | MSP manage | Update user tenants |
| PUT | `/api/v1/admin/users/{userID}/memberships` | `handleAdminUpdateUserMemberships` | MSP manage | Update user memberships |
| POST | `/api/v1/admin/customers` | `handleAdminCreateCustomer` | MSP manage | Create customer |
| GET | `/api/v1/admin/update/check` | `handleAdminUpdateCheck` | MSP manage | Check for updates |
| POST | `/api/v1/admin/update/apply` | `handleAdminUpdateApply` | MSP manage | Apply update |

---

## 7. Metrics & Telemetry

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/metrics` | `handleQueryMetrics` | MSP admin | Query metrics |
| GET | `/api/v1/devices/{tenantID}/{deviceID}/metrics/{metricName}` | `handleDeviceMetrics` | MSP admin | Device metrics |
| GET | `/api/v1/heartbeat/{tenantID}/{deviceID}` | `handleGetHeartbeat` | MSP admin | Get heartbeat |

---

## 8. Alerts

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/alerts/{tenantID}` | `handleListActiveAlerts` | MSP admin | List active alerts |
| GET | `/api/v1/alerts/{tenantID}/history` | `handleAlertHistory` | MSP admin | Alert history |
| POST | `/api/v1/alerts/{tenantID}/{alertID}/acknowledge` | `handleAcknowledgeAlert` | MSP admin | Acknowledge alert |
| GET | `/api/v1/alerts/{tenantID}/groups` | `handleListAlertGroups` | MSP admin | List alert groups |
| GET | `/api/v1/alerts/{tenantID}/groups/severity/{severity}` | `handleGetGroupsBySeverity` | MSP admin | Groups by severity |
| GET | `/api/v1/alerts/{tenantID}/groups/device/{deviceID}` | `handleGetGroupsByDevice` | MSP admin | Groups by device |
| POST | `/api/v1/alerts/{tenantID}/groups/device/{deviceID}/resolve` | `handleResolveGroupsByDevice` | MSP admin | Resolve groups by device |
| POST | `/api/v1/alerts/{tenantID}/groups/resolve-all` | `handleResolveAllGroups` | MSP admin | Resolve all groups |
| GET | `/api/v1/alerts/{tenantID}/groups/cascade` | `handleGetCascadeGroups` | MSP admin | Cascade groups |
| GET | `/api/v1/alerts/{tenantID}/groups/time-window/{duration}` | `handleGetTimeWindowGroups` | MSP admin | Time-window groups |
| POST | `/api/v1/rules/{tenantID}` | `handleCreateRule` | MSP admin | Create alert rule |
| GET | `/api/v1/rules/{tenantID}` | `handleListRules` | MSP admin | List alert rules |
| DELETE | `/api/v1/rules/{tenantID}/{ruleID}` | `handleDeleteRule` | MSP admin | Delete alert rule |

---

## 9. Maintenance Windows

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| POST | `/api/v1/tenants/{tenantID}/maintenance-windows` | `handleCreateMaintenanceWindow` | MSP admin | Create maintenance window |
| GET | `/api/v1/tenants/{tenantID}/maintenance-windows` | `handleListMaintenanceWindows` | MSP admin | List maintenance windows |
| DELETE | `/api/v1/tenants/{tenantID}/maintenance-windows/{windowID}` | `handleDeleteMaintenanceWindow` | MSP admin | Delete maintenance window |
| POST | `/api/v1/maintenance-windows` | `handleCreateMaintenanceWindow` | MSP admin | Create maintenance window |
| GET | `/api/v1/maintenance-windows` | `handleListMaintenanceWindows` | MSP admin | List maintenance windows |
| DELETE | `/api/v1/maintenance-windows/{windowID}` | `handleDeleteMaintenanceWindow` | MSP admin | Delete maintenance window |

---

## 10. Retention

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/tenants/{tenantID}/retention` | `handleGetRetention` | Client access | Get tenant retention |
| PATCH | `/api/v1/tenants/{tenantID}/retention` | `handleUpdateRetention` | Client access | Update tenant retention |
| GET | `/api/v1/retention/policies` | `handleListRetentionPolicies` | MSP admin | List retention policies |

---

## 11. Vulnerability Management

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/vulnerabilities/device/{deviceID}` | `handleDeviceVulnerabilities` | MSP admin | Device vulnerabilities |
| GET | `/api/v1/vulnerabilities/tenant/{tenantID}` | `handleTenantVulnerabilities` | MSP admin | Tenant vulnerabilities |
| GET | `/api/v1/vulnerabilities/tenant/{tenantID}/summary` | `handleVulnerabilitySummary` | MSP admin | Vulnerability summary |
| POST | `/api/v1/vulnerabilities/{vulnID}/resolve` | `handleResolveVulnerability` | MSP admin | Resolve vulnerability |
| POST | `/api/v1/vulnerabilities/{vulnID}/ignore` | `handleIgnoreVulnerability` | MSP admin | Ignore vulnerability |
| GET | `/api/v1/cve/stats` | `handleCVEDBStats` | MSP admin | CVE DB stats |
| POST | `/api/v1/cve/sync` | `handleCVESync` | MSP admin | Trigger CVE sync |
| GET | `/api/v1/cve/packages` | `handleCVEPackages` | MSP admin | List CVE packages |
| POST | `/api/v1/cve/packages` | `handleCVEAddPackage` | MSP admin | Add CVE package |
| DELETE | `/api/v1/cve/packages/{name}/{ecosystem}` | `handleCVEDeletePackage` | MSP admin | Delete CVE package |
| GET | `/api/v1/cve/sync/status` | `handleCVESyncStatus` | MSP admin | CVE sync status |
| GET | `/api/v1/cve/package/{name}` | `handleCVEPackage` | MSP admin | Get CVE package |

---

## 12. Third-Party Patch Catalog

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/thirdparty/apps` | `handleThirdPartyApps` | MSP admin | List third-party apps |
| GET | `/api/v1/thirdparty/packages` | `handleThirdPartyPackages` | MSP admin | List third-party packages |
| POST | `/api/v1/thirdparty/sync` | `handleThirdPartySync` | MSP admin | Sync all third-party |
| POST | `/api/v1/thirdparty/sync/{app}` | `handleThirdPartySyncApp` | MSP admin | Sync specific app |
| GET | `/api/v1/thirdparty/vendors` | `handleThirdPartyVendors` | MSP admin | List vendors |
| POST | `/api/v1/thirdparty/vendors/{vendor}/sync` | `handleThirdPartySyncVendor` | MSP admin | Sync specific vendor |
| GET | `/api/v1/thirdparty/vendors/status` | `handleThirdPartyVendorStatus` | MSP admin | Vendor sync status |

---

## 13. Patch Management

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/patch-policies` | `handleListPatchPolicies` | MSP admin | List patch policies |
| GET | `/api/v1/patch-policies/{policyID}` | `handleGetPatchPolicy` | MSP admin | Get patch policy |
| POST | `/api/v1/patch-policies` | `handleCreatePatchPolicy` | MSP admin | Create patch policy |
| DELETE | `/api/v1/patch-policies/{policyID}` | `handleDeletePatchPolicy` | MSP admin | Delete patch policy |
| GET | `/api/v1/patch-deployments` | `handleListPatchDeployments` | MSP admin | List patch deployments |
| GET | `/api/v1/patch-inventory/{tenantID}/{deviceID}` | `handleGetPatchInventory` | MSP admin | Get patch inventory |
| GET | `/api/v1/patch-inventory/{tenantID}/{deviceID}/latest` | `handleGetLatestPatchInventory` | MSP admin | Get latest patch inventory |

---

## 14. Reports

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/reports/{tenantID}` | `handleListReports` | MSP admin | List reports |
| POST | `/api/v1/reports/{tenantID}/schedules` | `handleCreateSchedule` | MSP admin | Create report schedule |
| GET | `/api/v1/reports/{tenantID}/schedules` | `handleListSchedules` | MSP admin | List report schedules |
| DELETE | `/api/v1/reports/{tenantID}/schedules/{scheduleID}` | `handleDeleteSchedule` | MSP admin | Delete report schedule |
| PATCH | `/api/v1/reports/{tenantID}/schedules/{scheduleID}` | `handleUpdateSchedule` | MSP admin | Update report schedule |
| PATCH | `/api/v1/reports/{tenantID}/schedules/{scheduleID}/enable` | `handleToggleSchedule` | MSP admin | Toggle report schedule |
| POST | `/api/v1/reports/{tenantID}/schedules/{scheduleID}/trigger` | `handleTriggerSchedule` | MSP admin | Trigger report schedule |
| POST | `/api/v1/reports/{tenantID}/generate` | `handleGenerateReport` | MSP admin | Generate report |
| GET | `/api/v1/reports/{tenantID}/{reportID}/download` | `handleDownloadReport` | MSP admin | Download report |
| POST | `/api/v1/reports/{tenantID}/compliance` | `handleGenerateComplianceReport` | MSP admin | Generate compliance report |
| GET | `/api/v1/reports/{tenantID}/compliance` | `handleListComplianceReports` | MSP admin | List compliance reports |
| GET | `/api/v1/reports/{tenantID}/compliance/{reportID}` | `handleGetComplianceReport` | MSP admin | Get compliance report |
| GET | `/api/v1/reports/{tenantID}/compliance/{reportID}/export/csv` | `handleExportComplianceReportCSV` | MSP admin | Export compliance CSV |
| GET | `/api/v1/reports/{tenantID}/compliance/{reportID}/export/json` | `handleExportComplianceReportJSON` | MSP admin | Export compliance JSON |

---

## 15. Remediation

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/remediation/{tenantID}/attempts/{vulnID}` | `handleGetRemediationHistory` | MSP admin | Get remediation history |
| GET | `/api/v1/remediation/summary/{tenantID}` | `handleGetRemediationSummary` | MSP admin | Get remediation summary |
| GET | `/api/v1/remediation/policy/{tenantID}` | `handleGetRemediationPolicy` | MSP admin | Get remediation policy |
| PATCH | `/api/v1/remediation/policy/{tenantID}` | `handleUpdateRemediationPolicy` | MSP admin | Update remediation policy |

---

## 16. Policies

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/policies` | `handleListPolicies` | MSP manage | List policies |
| GET | `/api/v1/policies/{policyID}` | `handleGetPolicy` | MSP manage | Get policy |
| POST | `/api/v1/policies` | `handleCreatePolicy` | MSP manage | Create policy |
| PUT | `/api/v1/policies/{policyID}` | `handleUpdatePolicy` | MSP manage | Update policy |
| DELETE | `/api/v1/policies/{policyID}` | `handleDeletePolicy` | MSP manage | Delete policy |
| POST | `/api/v1/policies/{policyID}/validate` | `handleValidatePolicy` | MSP manage | Validate policy |
| POST | `/api/v1/policies/{policyID}/preview` | `handlePreviewPolicy` | MSP manage | Preview policy |
| POST | `/api/v1/policies/{policyID}/publish` | `handlePublishPolicy` | MSP manage | Publish policy |
| POST | `/api/v1/policies/{policyID}/rollback` | `handleRollbackPolicy` | MSP manage | Rollback policy |
| POST | `/api/v1/policies/{policyID}/effective` | `handleEffectivePolicy` | MSP manage | Compute effective policy |
| POST | `/api/v1/policies/{policyID}/diff` | `handlePolicyDiff` | MSP manage | Policy diff |
| GET | `/api/v1/policies/{policyID}/revisions` | `handleGetPolicyRevisions` | MSP manage | Get policy revisions |

---

## 17. Smart Groups (Device Groups)

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/device-groups` | `handleListDeviceGroups` | MSP manage | List device groups |
| POST | `/api/v1/device-groups` | `handleCreateDeviceGroup` | MSP manage | Create device group |
| POST | `/api/v1/device-groups/smart` | `handleCreateSmartGroup` | MSP manage | Create smart group |
| GET | `/api/v1/device-groups/{groupID}` | `handleGetDeviceGroup` | MSP manage | Get device group |
| PUT | `/api/v1/device-groups/{groupID}` | `handleUpdateDeviceGroup` | MSP manage | Update device group |
| DELETE | `/api/v1/device-groups/{groupID}` | `handleDeleteDeviceGroup` | MSP manage | Delete device group |
| GET | `/api/v1/device-groups/{groupID}/detail` | `handleGetDeviceGroupDetail` | MSP manage | Get device group detail |
| GET | `/api/v1/device-groups/{groupID}/members` | `handleGetDeviceGroupMembers` | MSP manage | Get device group members |
| POST | `/api/v1/device-groups/{groupID}/evaluate` | `handleEvaluateDeviceGroup` | MSP manage | Evaluate device group |
| GET | `/api/v1/device-groups/{groupID}/evaluation-status` | `handleGetDeviceGroupEvalStatus` | MSP manage | Get eval status |
| GET | `/api/v1/device-groups/{groupID}/script-bindings` | `handleGetScriptBindings` | MSP manage | Get script bindings |
| POST | `/api/v1/device-groups/{groupID}/script-bindings` | `handleCreateScriptBinding` | MSP manage | Create script binding |
| DELETE | `/api/v1/device-groups/{groupID}/script-bindings/{bindingID}` | `handleDeleteScriptBinding` | MSP manage | Delete script binding |

---

## 18. Scripts

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/scripts/{tenantID}` | `handleListScripts` | MSP manage | List scripts |
| POST | `/api/v1/scripts/{tenantID}` | `handleCreateScript` | MSP manage | Create script |
| GET | `/api/v1/scripts/{tenantID}/{scriptID}` | `handleGetScript` | MSP manage | Get script |
| DELETE | `/api/v1/scripts/{tenantID}/{scriptID}` | `handleDeleteScript` | MSP manage | Delete script |
| POST | `/api/v1/scripts/{tenantID}/{scriptID}/run` | `handleRunScript` | MSP manage | Run script |
| GET | `/api/v1/scripts/{tenantID}/executions` | `handleListScriptExecutions` | MSP manage | List script executions |
| GET | `/api/v1/scripts/{tenantID}/executions/{execID}` | `handleGetScriptExecution` | MSP manage | Get script execution |

---

## 19. Script Schedules

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| POST | `/api/v1/tenants/{tenantID}/scripts/schedule` | `handleCreateScriptSchedule` | MSP manage | Create script schedule |
| GET | `/api/v1/tenants/{tenantID}/scripts/schedules` | `handleListScriptSchedules` | MSP manage | List script schedules |
| PUT | `/api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}` | `handleUpdateScriptSchedule` | MSP manage | Update script schedule |
| DELETE | `/api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}` | `handleDeleteScriptSchedule` | MSP manage | Delete script schedule |
| GET | `/api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}` | `handleGetScriptSchedule` | MSP manage | Get script schedule |
| GET | `/api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}/devices` | `handleGetScheduleDevices` | MSP manage | Get schedule devices |
| GET | `/api/v1/tenants/{tenantID}/scripts/schedules/executions` | `handleListScheduleExecutions` | MSP manage | List schedule executions |
| POST | `/api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}/preview` | `handlePreviewSchedule` | MSP manage | Preview schedule |
| POST | `/api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}/devices/{execID}/retry` | `handleRetryScheduleExecution` | MSP manage | Retry schedule execution |

---

## 20. Jobs

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/jobs` | `handleListJobs` | MSP manage | List jobs |
| GET | `/api/v1/jobs/{jobID}` | `handleGetJob` | MSP manage | Get job |
| GET | `/api/v1/jobs/{jobID}/events` | `handleGetJobEvents` | MSP manage | Get job events |
| POST | `/api/v1/jobs` | `handleCreateJob` | MSP manage | Create job |
| POST | `/api/v1/jobs/{jobID}/cancel` | `handleCancelJob` | MSP manage | Cancel job |
| POST | `/api/v1/jobs/{jobID}/retry` | `handleRetryJob` | MSP manage | Retry job |

---

## 21. Devices

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v2/devices` | `handleListDevices` | MSP manage | List all devices |
| GET | `/api/v2/devices/{deviceID}` | `handleGetDevice` | MSP manage | Get device |
| POST | `/api/v2/devices/bulk-action` | `handleBulkDeviceAction` | MSP manage | Bulk device action |
| POST | `/api/v2/devices/{deviceID}/action` | `handleDeviceAction` | MSP manage | Device action |
| GET | `/api/v2/devices/{deviceID}/capabilities` | `handleDeviceCapabilities` | MSP manage | Get device capabilities |
| GET | `/api/v2/devices/{deviceID}/inventory` | `handleDeviceInventory` | MSP manage | Get device inventory |
| GET | `/api/v1/devices/addresses` | `handleGetDeviceAddresses` | MSP manage | Get device addresses |
| GET | `/api/v1/devices/relationships` | `handleListDeviceRelationships` | MSP manage | List device relationships |
| POST | `/api/v1/devices/relationships` | `handleCreateDeviceRelationship` | MSP manage | Create device relationship |
| DELETE | `/api/v1/devices/relationships/{relationshipID}` | `handleDeleteDeviceRelationship` | MSP manage | Delete device relationship |
| GET | `/api/v1/devices/{deviceID}/dependencies` | `handleGetDeviceDependencies` | MSP manage | Get device dependencies |
| GET | `/api/v1/devices/{deviceID}/impact` | `handleGetDeviceImpact` | MSP manage | Get device impact |
| GET | `/api/v1/devices/{deviceID}/jobs` | `handleGetDeviceJobs` | MSP manage | Get device jobs |
| GET | `/api/v1/devices/{deviceID}/packages` | `handleGetDevicePackages` | MSP manage | Get device packages |
| GET | `/api/v1/devices/{deviceID}/services` | `handleGetDeviceServices` | MSP manage | Get device services |
| POST | `/api/v1/devices/{deviceID}/packages` | `handleAddDevicePackage` | MSP manage | Add device package |

---

## 22. Access Control

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/access/audit/{tenantID}` | `handleAccessAudit` | MSP admin | Access audit log |
| GET | `/api/v1/access/permissions/{tenantID}` | `handleAccessPermissions` | MSP admin | Access permissions |
| GET | `/api/v1/access/users/{tenantID}` | `handleAccessUsers` | MSP admin | Access users |

---

## 23. Enrollment Tokens

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/enrollment/tokens` | `handleListEnrollmentTokens` | MSP admin | List enrollment tokens |
| POST | `/api/v1/enrollment/tokens` | `handleCreateEnrollmentToken` | MSP admin | Create enrollment token |
| DELETE | `/api/v1/enrollment/tokens/{tokenID}` | `handleDeleteEnrollmentToken` | MSP admin | Delete enrollment token |
| POST | `/api/v1/enrollment/validate` | `handleValidateEnrollment` | MSP admin | Validate enrollment |

---

## 24. Domains

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/domains` | `handleListDomains` | MSP admin | List domains |
| POST | `/api/v1/domains` | `handleCreateDomain` | MSP admin | Create domain |
| DELETE | `/api/v1/domains/{domainID}` | `handleDeleteDomain` | MSP admin | Delete domain |
| POST | `/api/v1/domains/{domainID}/verify` | `handleVerifyDomain` | MSP admin | Verify domain |

---

## 25. Keys

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/keys/{tenantID}` | `handleListKeys` | MSP admin | List keys |
| GET | `/api/v1/keys/{tenantID}/active` | `handleGetActiveKeys` | MSP admin | Get active keys |
| POST | `/api/v1/keys/{tenantID}` | `handleCreateKey` | MSP admin | Create key |
| DELETE | `/api/v1/keys/{tenantID}/{keyID}` | `handleDeleteKey` | MSP admin | Delete key |
| POST | `/api/v1/keys/{tenantID}/rotate` | `handleRotateKey` | MSP admin | Rotate key |

---

## 26. Remote Access

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| POST | `/api/v1/remote/{tenantID}/session` | `handleCreateRemoteSession` | MSP manage | Create remote session |
| DELETE | `/api/v1/remote/{tenantID}/session/{sessionID}` | `handleDeleteRemoteSession` | MSP manage | Delete remote session |
| GET | `/api/v1/remote/{tenantID}/interactive` | `handleListInteractiveSessions` | MSP manage | List interactive sessions |
| POST | `/api/v1/remote/{tenantID}/interactive` | `handleCreateInteractiveSession` | MSP manage | Create interactive session |
| DELETE | `/api/v1/remote/{tenantID}/interactive/{sessionID}` | `handleDeleteInteractiveSession` | MSP manage | Delete interactive session |
| POST | `/api/v1/remote/{tenantID}/interactive/{sessionID}/input` | `handleInputInteractiveSession` | MSP manage | Input to interactive session |
| POST | `/api/v1/remote/{tenantID}/interactive/{sessionID}/recording` | `handleRecordInteractiveSession` | MSP manage | Record interactive session |
| POST | `/api/v1/remote/{tenantID}/session/{sessionID}/input` | `handleInputSession` | MSP manage | Input to session |
| GET | `/api/v1/remote/{tenantID}/recordings` | `handleListRemoteRecordings` | MSP manage | List recordings |
| DELETE | `/api/v1/recordings/{id}` | `handleDeleteRecording` | MSP manage | Delete recording |
| GET | `/api/v1/recordings/{id}/playback` | `handlePlaybackRecording` | MSP manage | Playback recording |
| GET | `/api/v1/recordings/{tenantID}` | `handleListTenantRecordings` | MSP manage | List tenant recordings |
| POST | `/api/v1/remote/{tenantID}/recording/{recordingID}/stop` | `handleStopRecording` | MSP manage | Stop recording |

---

## 27. WebRTC

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| POST | `/api/v1/webrtc/sessions` | `handleCreateWebRTCSesion` | MSP manage | Create WebRTC session |
| GET | `/api/v1/webrtc/sessions` | `handleListWebRTCSessions` | MSP manage | List WebRTC sessions |
| GET | `/api/v1/webrtc/sessions/{sessionID}` | `handleGetWebRTCSesion` | MSP manage | Get WebRTC session |
| POST | `/api/v1/webrtc/sessions/{sessionID}/offer` | `handleWebRTCSessionOffer` | MSP manage | WebRTC session offer |
| POST | `/api/v1/webrtc/sessions/{sessionID}/answer` | `handleWebRTCSessionAnswer` | MSP manage | WebRTC session answer |
| POST | `/api/v1/webrtc/sessions/{sessionID}/ice-candidate` | `handleWebRTCSessionICECandidate` | MSP manage | WebRTC ICE candidate |
| POST | `/api/v1/webrtc/sessions/{sessionID}/end` | `handleWebRTCSessionEnd` | MSP manage | End WebRTC session |
| GET | `/api/v1/webrtc/sessions/{sessionID}/relay-config` | `handleWebRTCRelayConfig` | MSP manage | WebRTC relay config |
| GET | `/api/v1/webrtc/sessions/{sessionID}/recordings` | `handleGetWebRTCRecordings` | MSP manage | Get WebRTC recordings |
| POST | `/api/v1/webrtc/sessions/{sessionID}/record` | `handleStartWebRTCRecording` | MSP manage | Start WebRTC recording |
| POST | `/api/v1/webrtc/recordings/{recordingID}/stop` | `handleStopWebRTCRecording` | MSP manage | Stop WebRTC recording |
| GET | `/api/v1/webrtc/sessions/{sessionID}/transcriptions` | `handleGetWebRTCTranscriptions` | MSP manage | Get WebRTC transcriptions |
| POST | `/api/v1/webrtc/sessions/{sessionID}/transcribe` | `handleStartWebRTCTranscription` | MSP manage | Start WebRTC transcription |
| POST | `/api/v1/webrtc/transcriptions/{transcriptionID}/stop` | `handleStopWebRTCTranscription` | MSP manage | Stop WebRTC transcription |

---

## 28. Software Deployment

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/software/packages/{tenantID}` | `handleListSoftwarePackages` | MSP admin | List software packages |
| POST | `/api/v1/software/packages/{tenantID}` | `handleCreateSoftwarePackage` | MSP admin | Create software package |
| GET | `/api/v1/software/deployments/{tenantID}` | `handleListSoftwareDeployments` | MSP admin | List software deployments |
| GET | `/api/v1/software/deployments/{tenantID}/{deployID}` | `handleGetSoftwareDeployment` | MSP admin | Get software deployment |
| POST | `/api/v1/software/deployments/{tenantID}` | `handleCreateSoftwareDeployment` | MSP admin | Create software deployment |
| DELETE | `/api/v1/software/packages/{tenantID}/{pkgID}` | `handleDeleteSoftwarePackage` | MSP admin | Delete software package |

---

## 29. Integrations (Webhooks)

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| POST | `/api/v1/integrations/edr/alerts` | `handleEDRAlerts` | HMAC auth | EDR alert ingestion |
| POST | `/api/v1/integrations/backup/sync` | `handleBackupSync` | HMAC auth | Backup sync |
| POST | `/api/v1/integrations/isolate` | `handleIsolateDevice` | HMAC auth | Device isolation |
| POST | `/api/v1/integrations/psa/webhooks` | `handlePSAWebhooks` | HMAC auth | PSA webhooks |
| POST | `/api/v1/integrations/psa/feedback` | `handlePSAFeedback` | MSP manage | PSA feedback |
| POST | `/api/v1/integrations/psa/tickets` | `handleCreatePSATicket` | MSP manage | Create PSA ticket |
| GET | `/api/v1/integrations/psa/tickets/{ticketID}` | `handleGetPSATicket` | MSP manage | Get PSA ticket |
| PUT | `/api/v1/integrations/psa/tickets/{ticketID}` | `handleUpdatePSATicket` | MSP manage | Update PSA ticket |
| DELETE | `/api/v1/integrations/psa/tickets/{ticketID}` | `handleDeletePSATicket` | MSP manage | Delete PSA ticket |
| GET | `/api/v1/integrations/psa/tickets/device/{deviceID}` | `handleListPSATicketsByDevice` | MSP manage | List PSA tickets by device |

---

## 30. MSP (v2)

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v2/msps/{mspID}/audit` | `handleMSPAudit` | MSP owner | MSP audit log |
| GET | `/api/v2/msps/{mspID}/clients` | `handleListMSPClients` | MSP owner | List MSP clients |
| GET | `/api/v2/msps/{mspID}/clients/{clientID}` | `handleGetMSPClient` | MSP owner | Get MSP client |
| POST | `/api/v2/msps/{mspID}/clients` | `handleCreateMSPClient` | MSP owner | Create MSP client |
| POST | `/api/v2/msps/{mspID}/clients/{clientID}/archive` | `handleArchiveMSPClient` | MSP owner | Archive MSP client |
| GET | `/api/v2/msps/{mspID}/devices` | `handleListMSPDevices` | MSP owner | List MSP devices |
| GET | `/api/v2/msps/{mspID}/memberships` | `handleListMSPMemberships` | MSP owner | List MSP memberships |
| POST | `/api/v2/msps/{mspID}/memberships` | `handleCreateMSPMembership` | MSP owner | Create MSP membership |
| DELETE | `/api/v2/msps/{mspID}/memberships/{membershipID}` | `handleDeleteMSPMembership` | MSP owner | Delete MSP membership |
| GET | `/api/v2/msps/{mspID}/billing/account` | `handleGetBillingAccount` | MSP owner | Get billing account |
| POST | `/api/v2/msps/{mspID}/billing/account` | `handleCreateBillingAccount` | MSP owner | Create billing account |
| DELETE | `/api/v2/msps/{mspID}/billing/account` | `handleDeleteBillingAccount` | MSP owner | Delete billing account |
| GET | `/api/v2/msps/{mspID}/billing/invoices` | `handleListBillingInvoices` | MSP owner | List billing invoices |
| GET | `/api/v2/msps/{mspID}/billing/invoices/{invoiceID}` | `handleGetBillingInvoice` | MSP owner | Get billing invoice |
| GET | `/api/v2/msps/{mspID}/billing/payment-methods` | `handleListBillingPaymentMethods` | MSP owner | List payment methods |
| POST | `/api/v2/msps/{mspID}/billing/payment-methods` | `handleCreateBillingPaymentMethod` | MSP owner | Create payment method |
| PATCH | `/api/v2/msps/{mspID}/billing/payment-methods/{paymentMethodID}` | `handleUpdateBillingPaymentMethod` | MSP owner | Update payment method |
| DELETE | `/api/v2/msps/{mspID}/billing/payment-methods/{paymentMethodID}` | `handleDeleteBillingPaymentMethod` | MSP owner | Delete payment method |
| GET | `/api/v2/msps/{mspID}/billing/reports/revenue` | `handleGetBillingRevenueReport` | MSP owner | Get billing revenue report |
| GET | `/api/v2/msps/{mspID}/billing/subscriptions` | `handleListBillingSubscriptions` | MSP owner | List subscriptions |
| POST | `/api/v2/msps/{mspID}/billing/subscriptions` | `handleCreateBillingSubscription` | MSP owner | Create subscription |
| DELETE | `/api/v2/msps/{mspID}/billing/subscriptions/{subscriptionID}` | `handleDeleteBillingSubscription` | MSP owner | Delete subscription |
| GET | `/api/v2/msps/{mspID}/billing/usage/{meterName}` | `handleGetBillingUsage` | MSP owner | Get billing usage |
| POST | `/api/v2/msps/{mspID}/billing/usage` | `handlePostBillingUsage` | MSP owner | Post billing usage |
| GET | `/api/v2/msps/{mspID}/entitlement` | `handleGetMSPEntitlement` | MSP owner | Get MSP entitlement |
| PATCH | `/api/v2/msps/{mspID}/entitlement` | `handleUpdateMSPEntitlement` | MSP owner | Update MSP entitlement |
| GET | `/api/v2/msps/{mspID}/usage` | `handleGetMSPUsage` | MSP owner | Get MSP usage |
| POST | `/api/v2/msps/{mspID}/offboarding` | `handleOffboardMSP` | MSP owner | Offboard MSP |
| POST | `/api/v2/msps/{mspID}/activate` | `handleActivateMSP` | Platform admin | Activate MSP |
| POST | `/api/v2/msps/{mspID}/suspend` | `handleSuspendMSP` | Platform admin | Suspend MSP |
| POST | `/api/v2/msps/{mspID}/owner-invitation` | `handleInviteMSPOwner` | Platform admin | Invite MSP owner |

---

## 31. Client Portal (v2)

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v2/clients/{clientID}/profile` | `handleGetClientProfile` | Client admin | Get client profile |
| PATCH | `/api/v2/clients/{clientID}/profile` | `handleUpdateClientProfile` | Client admin | Update client profile |
| GET | `/api/v2/clients/{clientID}/settings` | `handleGetClientSettings` | Client admin | Get client settings |
| PATCH | `/api/v2/clients/{clientID}/settings` | `handleUpdateClientSettings` | Client admin | Update client settings |
| GET | `/api/v2/clients/{clientID}/sites` | `handleListClientSites` | Client admin | List client sites |
| POST | `/api/v2/clients/{clientID}/sites` | `handleCreateClientSite` | Client admin | Create client site |
| GET | `/api/v2/clients/{clientID}/sites/{siteID}` | `handleGetClientSite` | Client admin | Get client site |
| POST | `/api/v2/clients/{clientID}/sites/{siteID}/archive` | `handleArchiveClientSite` | Client admin | Archive client site |
| GET | `/api/v2/clients/{clientID}/sessions` | `handleListClientSessions` | Client admin | List client sessions |
| DELETE | `/api/v2/clients/{clientID}/sessions/{sessionID}` | `handleDeleteClientSession` | Client admin | Delete client session |
| POST | `/api/v2/clients/{clientID}/auth/providers` | `handleCreateAuthProvider` | Client admin | Create auth provider |
| GET | `/api/v2/clients/{clientID}/auth/providers` | `handleListAuthProviders` | Client admin | List auth providers |
| GET | `/api/v2/clients/{clientID}/support-requests` | `handleListSupportRequests` | Client admin | List support requests |
| POST | `/api/v2/clients/{clientID}/support-requests` | `handleCreateSupportRequest` | Client admin | Create support request |
| GET | `/api/v2/clients/{clientID}/support-requests/{requestID}` | `handleGetSupportRequest` | Client admin | Get support request |
| PATCH | `/api/v2/clients/{clientID}/support-requests/{requestID}/close` | `handleCloseSupportRequest` | Client admin | Close support request |
| PATCH | `/api/v2/clients/{clientID}/support-requests/{requestID}/reply` | `handleReplySupportRequest` | Client admin | Reply to support request |

---

## 32. Platform Admin (v2)

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v2/platform/msps` | `handleListPlatformMSPs` | Platform admin | List all MSPs |
| GET | `/api/v2/platform/msps/{mspID}` | `handleGetPlatformMSP` | Platform admin | Get MSP |
| GET | `/api/v2/platform/msps/{mspID}/export` | `handleExportMSP` | Platform admin | Export MSP |
| POST | `/api/v2/platform/msps/{mspID}/offboarding` | `handlePlatformOffboardMSP` | Platform admin | Platform offboard MSP |
| POST | `/api/v2/platform/msps/{mspID}/offboarding/approve-deletion` | `handleApproveMSPDeletion` | Platform admin | Approve MSP deletion |
| PATCH | `/api/v2/platform/provider/profile` | `handleUpdateProviderProfile` | Platform admin | Update provider profile |
| POST | `/api/v2/platform/provider/setup` | `handleProviderSetup` | Platform admin | Provider setup |
| POST | `/api/v2/platform/support-grants` | `handleCreateSupportGrant` | Platform admin | Create support grant |
| DELETE | `/api/v2/platform/support-grants/{grantID}` | `handleDeleteSupportGrant` | Platform admin | Delete support grant |
| PATCH | `/api/v2/platform/domains/{domainID}/certificate` | `handleUpdateDomainCertificate` | Platform admin | Update domain certificate |
| GET | `/api/v2/platform/billing/analytics` | `handleGetPlatformBillingAnalytics` | Platform admin | Get billing analytics |

---

## 33. Approvals

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v2/approvals` | `handleListApprovals` | MSP manage | List approvals |
| GET | `/api/v2/approvals/{approvalID}` | `handleGetApproval` | MSP manage | Get approval |
| GET | `/api/v2/approvals/{approvalID}/decisions` | `handleGetApprovalDecisions` | MSP manage | Get approval decisions |
| POST | `/api/v2/approvals` | `handleCreateApproval` | MSP manage | Create approval |
| POST | `/api/v2/approvals/{approvalID}/approve` | `handleApprove` | MSP manage | Approve |
| POST | `/api/v2/approvals/{approvalID}/reject` | `handleRejectApproval` | MSP manage | Reject approval |
| POST | `/api/v2/approvals/{approvalID}/cancel` | `handleCancelApproval` | MSP manage | Cancel approval |

---

## 34. LAN Cache

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/lancache/entries` | `handleListLANCacheEntries` | MSP admin | List cache entries |
| GET | `/api/v1/lancache/entries/{entryID}` | `handleGetLANCacheEntry` | MSP admin | Get cache entry |
| POST | `/api/v1/lancache/entries` | `handleCreateLANCacheEntry` | MSP admin | Create cache entry |
| DELETE | `/api/v1/lancache/entries/{entryID}` | `handleDeleteLANCacheEntry` | MSP admin | Delete cache entry |
| GET | `/api/v1/lancache/stats` | `handleGetLANCacheStats` | MSP admin | Get cache stats |

---

## 35. Context Switching

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v2/context` | `handleGetContext` | Authenticated | Get current context |
| POST | `/api/v2/context/switch` | `handleSwitchContext` | Authenticated | Switch context |

---

## 36. Deployment History

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v2/deployment/history` | `handleGetDeploymentHistory` | MSP manage | Get deployment history |
| GET | `/api/v2/deployment/state` | `handleGetDeploymentState` | MSP manage | Get deployment state |

---

## 37. Branding

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v1/branding` | `handleGetBranding` | Authenticated | Get branding |
| PUT | `/api/v1/branding` | `handleUpdateBranding` | Authenticated | Update branding |

---

## 38. Access Audit

| Method | Path | Handler | Access | Description |
|--------|------|---------|--------|-------------|
| GET | `/api/v2/audit/endpoint` | `handleAuditEndpoint` | MSP admin | Audit endpoint access |

---

*Last Updated: 2026-08-08*
*Source: `internal/platform/api.go`*
*Total routes: 310+*
