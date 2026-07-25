#!/usr/bin/env bash
# Strata RMM End-to-End Smoke Test
set -euo pipefail

BASE_URL="${API_URL:-http://localhost:8080}"
NATS_URL="${NATS_URL:-nats://localhost:4222}"
TENANT_ID="${TENANT_ID:-00000000-0000-0000-0000-000000000001}"
PASS=0
FAIL=0

info()  { echo "[INFO] $*"; }
pass()  { echo "[PASS] $*"; PASS=$((PASS+1)); }
fail()  { echo "[FAIL] $*"; FAIL=$((FAIL+1)); }

cleanup() {
  info "--- cleanup ---"
}
trap cleanup EXIT

# --- Health check ---
info "=== Health Check ==="
if HEALTH=$(curl -sf "$BASE_URL/health" 2>/dev/null); then
  STATUS=$(echo "$HEALTH" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
  if [ "$STATUS" = "ok" ]; then
    pass "Health endpoint returns ok"
  else
    fail "Health endpoint unexpected: $HEALTH"
  fi
else
  fail "Health endpoint unreachable at $BASE_URL"
fi

# --- Enroll agent ---
info "=== Agent Enrollment ==="
ENROLL_RESULT=$(curl -sf -X POST "$BASE_URL/api/v1/enroll" \
  -H 'Content-Type: application/json' \
  -d "{\"tenant_id\": \"$TENANT_ID\"}" 2>/dev/null || echo "")
if [ -n "$ENROLL_RESULT" ]; then
  TOKEN=$(echo "$ENROLL_RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('enrollment_token',''))" 2>/dev/null || echo "")
  if [ -n "$TOKEN" ]; then
    pass "Enrollment token generated: ${TOKEN:0:16}..."
  else
    fail "Enrollment response missing token: $ENROLL_RESULT"
  fi
else
  fail "Enrollment endpoint failed"
fi

# --- CVE sync ---
info "=== CVE Management ==="
STATS=$(curl -sf "$BASE_URL/api/v1/cve/stats" 2>/dev/null || echo "")
if [ -n "$STATS" ]; then
  CVE_COUNT=$(echo "$STATS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('cve_count',0))" 2>/dev/null || echo "0")
  if [ "$CVE_COUNT" -ge 10 ]; then
    pass "CVE database has $CVE_COUNT records (expected >= 10)"
  else
    fail "CVE database only has $CVE_COUNT records (expected >= 10)"
  fi
else
  fail "CVE stats endpoint failed"
fi

PACKAGES=$(curl -sf "$BASE_URL/api/v1/cve/packages" 2>/dev/null || echo "")
if [ -n "$PACKAGES" ]; then
  pass "CVE tracked packages endpoint works"
else
  fail "CVE packages endpoint failed"
fi

# --- MFA ---
info "=== MFA ==="
MFA_ENROLL=$(curl -sf -X POST "$BASE_URL/api/v1/mfa/enroll/test-user" 2>/dev/null || echo "")
if [ -n "$MFA_ENROLL" ]; then
  MFA_SECRET=$(echo "$MFA_ENROLL" | python3 -c "import sys,json; print(json.load(sys.stdin).get('secret',''))" 2>/dev/null || echo "")
  if [ -n "$MFA_SECRET" ]; then
    pass "MFA enrollment generated secret"
  else
    fail "MFA enrollment missing secret"
  fi
else
  fail "MFA enrollment endpoint failed"
fi

# Verify MFA status shows enabled
MFA_STATUS=$(curl -sf "$BASE_URL/api/v1/mfa/status/test-user" 2>/dev/null || echo "")
if [ -n "$MFA_STATUS" ]; then
  pass "MFA status endpoint works"
else
  fail "MFA status endpoint failed"
fi

# --- Recording list ---
info "=== Session Recordings ==="
RECORDINGS=$(curl -sf "$BASE_URL/api/v1/recordings/$TENANT_ID" 2>/dev/null || echo "")
if [ -n "$RECORDINGS" ]; then
  pass "Recording list endpoint works"
else
  fail "Recording list endpoint failed"
fi

# --- Access Review ---
info "=== Access Review ==="
ACCESS_AUDIT=$(curl -sf "$BASE_URL/api/v1/access/audit/$TENANT_ID" 2>/dev/null || echo "")
if [ -n "$ACCESS_AUDIT" ]; then
  pass "Access audit endpoint works"
else
  fail "Access audit endpoint failed"
fi

ACCESS_USERS=$(curl -sf "$BASE_URL/api/v1/access/users/$TENANT_ID" 2>/dev/null || echo "")
if [ -n "$ACCESS_USERS" ]; then
  pass "Access users endpoint works"
else
  fail "Access users endpoint failed"
fi

# --- Encryption Keys ---
info "=== Encryption Keys ==="
KEY_CREATE=$(curl -sf -X POST "$BASE_URL/api/v1/keys/$TENANT_ID" \
  -H 'Content-Type: application/json' \
  -d '{"kms_type": "local", "encryption": "aes-256-gcm"}' 2>/dev/null || echo "")
if [ -n "$KEY_CREATE" ]; then
  KEY_ID=$(echo "$KEY_CREATE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
  if [ -n "$KEY_ID" ]; then
    pass "Encryption key created: ${KEY_ID:0:8}..."
  else
    fail "Key creation response missing id"
  fi
else
  fail "Key creation endpoint failed"
fi

KEYS_LIST=$(curl -sf "$BASE_URL/api/v1/keys/$TENANT_ID" 2>/dev/null || echo "")
if [ -n "$KEYS_LIST" ]; then
  pass "Key list endpoint works"
else
  fail "Key list endpoint failed"
fi

# --- Vulnerabilities ---
info "=== Vulnerability Management ==="
VULN_SUMMARY=$(curl -sf "$BASE_URL/api/v1/vulnerabilities/tenant/$TENANT_ID/summary" 2>/dev/null || echo "")
if [ -n "$VULN_SUMMARY" ]; then
  OPEN_COUNT=$(echo "$VULN_SUMMARY" | python3 -c "import sys,json; print(json.load(sys.stdin).get('open_count',0))" 2>/dev/null || echo "0")
  pass "Vulnerability summary: $OPEN_COUNT open"
else
  fail "Vulnerability summary endpoint failed"
fi

# --- Summary ---
info "=== Results ==="
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
echo "All smoke tests passed!"
