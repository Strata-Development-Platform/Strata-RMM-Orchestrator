#!/bin/bash
set -euo pipefail

: "${JWT_SECRET:?JWT_SECRET must be supplied by the deployment secret store}"
: "${TIMESCALE_DSN:?TIMESCALE_DSN must be supplied without embedding it in this script}"

NATS_URL="${NATS_URL:-nats://127.0.0.1:4222}"
API_ADDR="${API_ADDR:-127.0.0.1:8080}"

exec /home/administrator/strata-rmm/bin/strata-rmm orchestrator \
  --nats-url "$NATS_URL" \
  --timescale-dsn "$TIMESCALE_DSN" \
  --api-addr "$API_ADDR"
