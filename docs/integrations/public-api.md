openapi: 3.0.3
info:
  title: Strata RMM — Third-Party Integration API
  description: >
    Public webhook endpoints for ingesting events from external EDR, backup, and
    PSA providers. All routes are HMAC-SHA256 signed; no JWT or bearer token is
    required.

    The orchestrator validates each request against the shared webhook secret
    (`INTEGRATION_WEBHOOK_SECRET`). The signature is computed as `hmac-sha256(secret, raw-body)` and
    transmitted in the `X-Signature` header as a lowercase hex digest.
  version: 1.0.0
  contact:
    name: Strata Platform Engineering

servers:
  - url: https://{host}/api/v1
    variables:
      host:
        default: orchestrator.example.com
        description: Public hostname of the Strata orchestrator.

security:
  - webhook_signature: []

paths:
  /integrations/edr/alerts:
    post:
      summary: Ingest EDR alert
      description: >
        Accepts a normalized alert payload from any EDR provider (CrowdStrike,
        SentinelOne, Microsoft Defender, etc.). The orchestrator logs the alert,
        normalizes the severity field, and publishes a notification to the NATS
        JetStream alert bus (`tenant.{tenant_id}.edrs.alert`).

        If the alert severity is `critical` or `high` the orchestrator may
        optionally trigger automated security isolation via the
        `POST /api/v1/integrations/isolate` route.
      operationId: ingestEdrAlert
      tags:
        - EDR
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/EdrAlert'
            example:
              provider: crowdstrike
              alert_id: CS-20260805-00142
              device_id: dev-a1b2c3d4
              tenant_id: tenant-xyz
              severity: critical
              title: Lateral movement detected on endpoint
              description: >
                Multiple failed RDP logins followed by successful admin session
                from an unusual source IP.
              timestamp: "2026-08-05T08:30:00Z"
              actions:
                - name: "Quarantine endpoint"
                  requires_app: "CrowdStrike Falcon"
                - name: "Collect memory dump"
              network_info:
                - direction: inbound
                  protocol: TCP
                  local_addr: "10.0.1.5:445"
                  remote_addr: "203.0.113.42:54321"
        description: >
          HMAC-SHA256 signed JSON payload. The entire request body (before any
          parsing) is used for signature verification.
      responses:
        "200":
          description: Alert received and processed.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AlertReceipt'
              example:
                status: received
                alert_id: CS-20260805-00142
                provider: crowdstrike
        "400":
          description: >
            Invalid JSON or missing required fields (`device_id`, `alert_id`).
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                error: missing required fields: device_id, alert_id
        "401":
          description: >
            Missing or invalid `X-Signature` header, or `X-Webhook-Timestamp`
            older than the 5-minute clock skew window.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                error: invalid signature

  /integrations/backup/sync:
    post:
      summary: Ingest backup sync event
      description: >
        Accepts a backup status event from any backup provider (Veeam, Commvault,
        Druva, etc.). The orchestrator logs the event and may update device-level
        backup health indicators.

        Accepted `status` values: `success`, `failed`, `in_progress`. Values
        not matching these are normalized to `unknown`.
      operationId: ingestBackupSync
      tags:
        - Backup
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/BackupSync'
            example:
              provider: veeam
              tenant_id: tenant-xyz
              device_id: dev-a1b2c3d4
              status: failed
              message: "Backup job completed with warnings. 2 of 5 files skipped."
              timestamp: "2026-08-05T02:00:00Z"
        description: >
          HMAC-SHA256 signed JSON payload. The entire request body is used for
          signature verification.
      responses:
        "200":
          description: Backup sync event received.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/BackupReceipt'
              example:
                status: received
                device_id: dev-a1b2c3d4
                backup_status: failed
        "400":
          description: >
            Invalid JSON or missing required field (`device_id`).
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                error: missing required field: device_id
        "401":
          description: >
            Missing or invalid `X-Signature` header, or expired timestamp.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                error: invalid signature

  /integrations/psa/webhooks:
    post:
      summary: Ingest PSA ticket event
      description: >
        Accepts a ticket lifecycle event from any PSA provider (Autotask,
        ConnectWise, Freshservice, Zendesk). The orchestrator logs the event
        and may correlate the ticket with existing alerts or devices.

        Accepted `action` values: `created`, `updated`, `closed`. Values not
        matching these are normalized to `unknown`.
      operationId: ingestPsaWebhook
      tags:
        - PSA
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/PsaWebhook'
            example:
              provider: autotask
              tenant_id: tenant-xyz
              action: created
              ticket_id: AT-98765
              subject: "Printer offline at Branch Office 3"
              device_id: dev-e5f6a7b8
              owner: support@example.com
              severity: high
              timestamp: "2026-08-05T09:15:00Z"
        description: >
          HMAC-SHA256 signed JSON payload. The entire request body is used for
          signature verification.
      responses:
        "200":
          description: PSA webhook event received.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/PsaReceipt'
              example:
                status: received
                ticket_id: AT-98765
                action: created
        "400":
          description: >
            Invalid JSON or missing required field (`tenant_id`).
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                error: missing required field: tenant_id
        "401":
          description: >
            Missing or invalid `X-Signature` header, or expired timestamp.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                error: invalid signature

  /integrations/isolate:
    post:
      summary: Trigger automated security isolation
      description: >
        Accepts an isolation request from any EDR provider. The orchestrator
        publishes an isolation command to NATS JetStream on the subject
        `tenant.{tenant_id}.cmd.isolate`, instructing the target device's agent
        to disconnect from the network.

        If the device is already isolated the command is idempotent — the
        orchestrator acknowledges receipt regardless.
      operationId: triggerIsolation
      tags:
        - Isolation
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/IsolationAction'
            example:
              device_id: dev-a1b2c3d4
              tenant_id: tenant-xyz
              reason: "Lateral movement detected — automated response"
              severity: critical
              alert_id: CS-20260805-00142
              provider: crowdstrike
        description: >
          HMAC-SHA256 signed JSON payload. The entire request body is used for
          signature verification.
      responses:
        "200":
          description: >
            Isolation command dispatched to NATS JetStream. The command may have
            already been applied to the device.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/IsolationReceipt'
              example:
                status: isolated
                event_id: iso-CS-20260805-00142
                device_id: dev-a1b2c3d4
        "400":
          description: >
            Invalid JSON or missing required fields (`device_id`, `tenant_id`).
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                error: missing required fields: device_id, tenant_id
        "401":
          description: >
            Missing or invalid `X-Signature` header, or expired timestamp.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                error: invalid signature
        "503":
          description: >
            NATS JetStream is not configured or the publish failed.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                error: nats not configured

components:
  securitySchemes:
    webhook_signature:
      type: http
      scheme: custom
      description: >
        HMAC-SHA256 signature validation.

        | Header                  | Required | Description                                                  |
        |-------------------------|----------|--------------------------------------------------------------|
        | `X-Signature`           | Yes      | SHA-256 hex digest of `hmac(secret, raw-request-body)`       |
        | `X-Webhook-Timestamp`   | No       | RFC 3339 datetime; rejected if > 5 minutes old               |
        | `Content-Type`          | Yes      | Must be `application/json`                                   |

        The orchestrator computes the expected HMAC using the shared secret
        configured in `INTEGRATION_WEBHOOK_SECRET`. Request body is read in
        full before signature verification; the body is then re-wound for
        JSON parsing by the handler.

  schemas:
    EdrAlert:
      type: object
      required:
        - provider
        - alert_id
        - device_id
      properties:
        provider:
          type: string
          description: >
            Provider name. Must match one of the registered providers in the
            integration settings panel.
          example: crowdstrike
        alert_id:
          type: string
          description: >
            Unique alert identifier from the EDR provider. Used for deduplication.
          example: CS-20260805-00142
        device_id:
          type: string
          description: >
            Strata device ID matching the enrolled agent.
          example: dev-a1b2c3d4
        tenant_id:
          type: string
          description: >
            Strata tenant ID. Required for routing the alert to the correct NATS
            subject (`tenant.{tenant_id}.edrs.alert`).
          example: tenant-xyz
        severity:
          type: string
          description: >
            Severity from the provider. Automatically normalized to one of:
            `critical`, `high`, `medium`, `low`, `informational`. Provider-specific
            synonyms (e.g. `critical_severity`, `serious`, `moderate`) are mapped
            to the canonical value.
          enum:
            - critical
            - high
            - medium
            - low
            - informational
          example: critical
        title:
          type: string
          description: >
            Brief, human-readable alert title.
          example: Lateral movement detected on endpoint
        description:
          type: string
          description: >
            Detailed description of the alert. May contain markdown or plain text.
          example: >
            Multiple failed RDP logins followed by successful admin session
            from an unusual source IP.
        timestamp:
          type: string
          format: date-time
          description: >
            Event timestamp in RFC 3339 format, as reported by the provider.
          example: "2026-08-05T08:30:00Z"
        actions:
          type: array
          items:
            $ref: '#/components/schemas/Action'
          description: >
            Remediation actions suggested by the EDR provider.
          example:
            - name: "Quarantine endpoint"
              requires_app: "CrowdStrike Falcon"
        network_info:
          type: array
          items:
            $ref: '#/components/schemas/NetworkInfo'
          description: >
            Network connection details associated with the alert.
          example:
            - direction: inbound
              protocol: TCP
              local_addr: "10.0.1.5:445"
              remote_addr: "203.0.113.42:54321"

    Action:
      type: object
      properties:
        name:
          type: string
          description: >
            Human-readable name of the remediation action.
          example: Quarantine endpoint
        command:
          type: string
          nullable: true
          description: >
            Shell command to execute (when applicable). May be empty.
          example: "crowdstrike-cli quarantine --device dev-a1b2c3d4"
        requires_app:
          type: string
          nullable: true
          description: >
            Name of the EDR application required to execute this action.
          example: CrowdStrike Falcon

    NetworkInfo:
      type: object
      properties:
        direction:
          type: string
          description: >
            Connection direction.
          enum:
            - inbound
            - outbound
          example: inbound
        protocol:
          type: string
          description: >
            Transport protocol.
          example: TCP
        local_addr:
          type: string
          description: >
            Local endpoint address and port.
          example: "10.0.1.5:445"
        remote_addr:
          type: string
          description: >
            Remote endpoint address and port.
          example: "203.0.113.42:54321"

    BackupSync:
      type: object
      required:
        - device_id
      properties:
        provider:
          type: string
          description: >
            Provider name. Must match one of the registered providers in the
            integration settings panel.
          example: veeam
        tenant_id:
          type: string
          description: >
            Strata tenant ID.
          example: tenant-xyz
        device_id:
          type: string
          description: >
            Strata device ID matching the enrolled agent.
          example: dev-a1b2c3d4
        status:
          type: string
          description: >
            Backup status from the provider. Non-standard values are normalized
            to `unknown`.
          enum:
            - success
            - failed
            - in_progress
            - unknown
          example: failed
        message:
          type: string
          nullable: true
          description: >
            Human-readable status message from the provider.
          example: "Backup job completed with warnings. 2 of 5 files skipped."
        timestamp:
          type: string
          format: date-time
          description: >
            Event timestamp in RFC 3339 format.
          example: "2026-08-05T02:00:00Z"

    PSAWebhook:
      type: object
      required:
        - tenant_id
      properties:
        provider:
          type: string
          description: >
            Provider name. Must match one of the registered providers in the
            integration settings panel.
          example: autotask
        tenant_id:
          type: string
          description: >
            Strata tenant ID. Required for routing the event.
          example: tenant-xyz
        action:
          type: string
          description: >
            Ticket lifecycle action. Non-standard values are normalized to
            `unknown`.
          enum:
            - created
            - updated
            - closed
            - unknown
          example: created
        ticket_id:
          type: string
          description: >
            Unique ticket identifier from the PSA provider.
          example: AT-98765
        subject:
          type: string
          description: >
            Ticket subject / title.
          example: "Printer offline at Branch Office 3"
        device_id:
          type: string
          nullable: true
          description: >
            Optional device ID correlated with the ticket.
          example: dev-e5f6a7b8
        owner:
          type: string
          nullable: true
          description: >
            Owner email or user ID.
          example: support@example.com
        severity:
          type: string
          nullable: true
          description: >
            Ticket severity as defined by the PSA provider.
          example: high
        timestamp:
          type: string
          format: date-time
          description: >
            Event timestamp in RFC 3339 format.
          example: "2026-08-05T09:15:00Z"

    IsolationAction:
      type: object
      required:
        - device_id
        - tenant_id
      properties:
        device_id:
          type: string
          description: >
            Strata device ID to isolate.
          example: dev-a1b2c3d4
        tenant_id:
          type: string
          description: >
            Strata tenant ID. Used to route the NATS command.
          example: tenant-xyz
        reason:
          type: string
          nullable: true
          description: >
            Human-readable reason for isolation.
          example: "Lateral movement detected — automated response"
        severity:
          type: string
          nullable: true
          description: >
            Severity of the triggering event. Defaults to `high` if omitted.
          enum:
            - critical
            - high
            - medium
            - low
            - informational
          example: critical
        alert_id:
          type: string
          nullable: true
          description: >
            Optional reference to the originating alert ID.
          example: CS-20260805-00142
        provider:
          type: string
          nullable: true
          description: >
            Provider that triggered the isolation request.
          example: crowdstrike

    # --- Response schemas ---

    AlertReceipt:
      type: object
      properties:
        status:
          type: string
          enum: [received]
        alert_id:
          type: string
        provider:
          type: string

    BackupReceipt:
      type: object
      properties:
        status:
          type: string
          enum: [received]
        device_id:
          type: string
        backup_status:
          type: string

    PsaReceipt:
      type: object
      properties:
        status:
          type: string
          enum: [received]
        ticket_id:
          type: string
        action:
          type: string

    IsolationReceipt:
      type: object
      properties:
        status:
          type: string
          enum: [isolated]
        event_id:
          type: string
        device_id:
          type: string

    ErrorResponse:
      type: object
      properties:
        error:
          type: string
          description: >
            Human-readable error message. Always a short, single-line string.
            Never exposes internal stack traces, database details, or secrets.
