# Strata RMM — MSP Lifecycle Reference

**Version:** 2026-08-08
**Last Updated: 2026-08-08

---

## 1. MSP Lifecycle Overview

Strata supports MSP (Managed Service Provider) lifecycle with provider registration, client management, billing, and offboarding.

---

## 2. MSP Registration

### 2.1 Provider Setup

```bash
# Provider setup
curl -X POST https://strata.example.com/api/v2/platform/provider/setup \
  -H "Authorization: Bearer {token}" \
  -d '{"name": "My MSP", "domain": "mymsp.com"}'
```

### 2.2 Owner Invitation

```bash
# Invite MSP owner
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/owner-invitation \
  -H "Authorization: Bearer {token}" \
  -d '{"email": "owner@mymsp.com"}'
```

### 2.3 MSP Activation

```bash
# Activate MSP
curl -X POST https://strata.example.com/api/v2/platform/msps/{mspID}/activate \
  -H "Authorization: Bearer {token}"
```

---

## 3. Client Management

### 3.1 Create Client

```bash
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/clients \
  -H "Authorization: Bearer {token}" \
  -d '{"name": "My Client", "domain": "myclient.com"}'
```

### 3.2 Client Profile

```bash
# Update client profile
curl -X PATCH https://strata.example.com/api/v2/clients/{clientID}/profile \
  -H "Authorization: Bearer {token}" \
  -d '{"name": "Updated Client", "plan": "enterprise"}'
```

### 3.3 Client Sites

```bash
# Create site
curl -X POST https://strata.example.com/api/v2/clients/{clientID}/sites \
  -H "Authorization: Bearer {token}" \
  -d '{"name": "Main Office", "address": "123 Main St"}'
```

### 3.4 Client Archive

```bash
# Archive client
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/clients/{clientID}/archive \
  -H "Authorization: Bearer {token}"
```

---

## 4. Billing

### 4.1 Billing Account

```bash
# Create billing account
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/billing/account \
  -H "Authorization: Bearer {token}"

# Get billing account
curl -X GET https://strata.example.com/api/v2/msps/{mspID}/billing/account \
  -H "Authorization: Bearer {token}"

# Delete billing account
curl -X DELETE https://strata.example.com/api/v2/msps/{mspID}/billing/account \
  -H "Authorization: Bearer {token}"
```

### 4.2 Subscriptions

```bash
# Create subscription
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/billing/subscriptions \
  -H "Authorization: Bearer {token}" \
  -d '{"plan": "enterprise", "quantity": 10}'

# List subscriptions
curl -X GET https://strata.example.com/api/v2/msps/{mspID}/billing/subscriptions \
  -H "Authorization: Bearer {token}"

# Delete subscription
curl -X DELETE https://strata.example.com/api/v2/msps/{mspID}/billing/subscriptions/{subscriptionID} \
  -H "Authorization: Bearer {token}"
```

### 4.3 Payment Methods

```bash
# Create payment method
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/billing/payment-methods \
  -H "Authorization: Bearer {token}" \
  -d '{"type": "credit_card", "last4": "1234"}'

# Update payment method
curl -X PATCH https://strata.example.com/api/v2/msps/{mspID}/billing/payment-methods/{paymentMethodID} \
  -H "Authorization: Bearer {token}"

# Delete payment method
curl -X DELETE https://strata.example.com/api/v2/msps/{mspID}/billing/payment-methods/{paymentMethodID} \
  -H "Authorization: Bearer {token}"
```

### 4.4 Usage Meters

```bash
# Report usage
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/billing/usage \
  -H "Authorization: Bearer {token}" \
  -d '{"meterName": "device_count", "value": 100}'

# Get usage
curl -X GET https://strata.example.com/api/v2/msps/{mspID}/billing/usage/{meterName} \
  -H "Authorization: Bearer {token}"

# Get revenue report
curl -X GET https://strata.example.com/api/v2/msps/{mspID}/billing/reports/revenue \
  -H "Authorization: Bearer {token}"
```

### 4.5 Invoices

```bash
# List invoices
curl -X GET https://strata.example.com/api/v2/msps/{mspID}/billing/invoices \
  -H "Authorization: Bearer {token}"

# Get invoice
curl -X GET https://strata.example.com/api/v2/msps/{mspID}/billing/invoices/{invoiceID} \
  -H "Authorization: Bearer {token}"
```

---

## 5. Entitlements

```bash
# Get entitlement
curl -X GET https://strata.example.com/api/v2/msps/{mspID}/entitlement \
  -H "Authorization: Bearer {token}"

# Update entitlement
curl -X PATCH https://strata.example.com/api/v2/msps/{mspID}/entitlement \
  -H "Authorization: Bearer {token}" \
  -d '{"features": ["remote_access", "patch_management"]}'
```

---

## 6. Offboarding

### 6.1 Initiate Offboarding

```bash
# Offboard MSP
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/offboarding \
  -H "Authorization: Bearer {token}"
```

### 6.2 Platform Offboarding

```bash
# Platform-initiated offboarding
curl -X POST https://strata.example.com/api/v2/platform/msps/{mspID}/offboarding \
  -H "Authorization: Bearer {token}"

# Approve deletion
curl -X POST https://strata.example.com/api/v2/platform/msps/{mspID}/offboarding/approve-deletion \
  -H "Authorization: Bearer {token}"
```

### 6.3 Client Offboarding

```bash
# Archive client during offboarding
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/clients/{clientID}/archive \
  -H "Authorization: Bearer {token}"
```

---

## 7. Memberships

```bash
# List memberships
curl -X GET https://strata.example.com/api/v2/msps/{mspID}/memberships \
  -H "Authorization: Bearer {token}"

# Create membership
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/memberships \
  -H "Authorization: Bearer {token}" \
  -d '{"email": "user@mymsp.com", "role": "admin"}'

# Delete membership
curl -X DELETE https://strata.example.com/api/v2/msps/{mspID}/memberships/{membershipID} \
  -H "Authorization: Bearer {token}"
```

---

## 8. Branding

```bash
# Get branding
curl -X GET https://strata.example.com/api/v1/branding \
  -H "Authorization: Bearer {token}"

# Update branding
curl -X PUT https://strata.example.com/api/v1/branding \
  -H "Authorization: Bearer {token}" \
  -d '{"logo": "base64-encoded-logo", "primaryColor": "#000000"}'
```

---

## 9. Domains

```bash
# List domains
curl -X GET https://strata.example.com/api/v1/domains \
  -H "Authorization: Bearer {token}"

# Create domain
curl -X POST https://strata.example.com/api/v1/domains \
  -H "Authorization: Bearer {token}" \
  -d '{"domain": "mymsp.com"}'

# Verify domain
curl -X POST https://strata.example.com/api/v1/domains/{domainID}/verify \
  -H "Authorization: Bearer {token}"

# Update certificate
curl -X PATCH https://strata.example.com/api/v2/platform/domains/{domainID}/certificate \
  -H "Authorization: Bearer {token}"

# Delete domain
curl -X DELETE https://strata.example.com/api/v1/domains/{domainID} \
  -H "Authorization: Bearer {token}"
```

---

## 10. Usage Analytics

```bash
# Get usage
curl -X GET https://strata.example.com/api/v2/msps/{mspID}/usage \
  -H "Authorization: Bearer {token}"

# Get billing analytics
curl -X GET https://strata.example.com/api/v2/platform/billing/analytics \
  -H "Authorization: Bearer {token}"
```

---

## 11. Audit

```bash
# Get MSP audit log
curl -X GET https://strata.example.com/api/v2/msps/{mspID}/audit \
  -H "Authorization: Bearer {token}"
```

---

## 12. Suspension

```bash
# Suspend MSP
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/suspend \
  -H "Authorization: Bearer {token}"
```

---

*Last Updated: 2026-08-08*
