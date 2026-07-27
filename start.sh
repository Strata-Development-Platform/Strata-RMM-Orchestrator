#!/bin/bash
export JWT_SECRET=b02088ce0a77671a2790d8ac6f3c6b0923af35d45b5b34fdb2024338f79038ab
exec /home/administrator/strata-rmm/bin/strata-rmm orchestrator \
  --nats-url nats://127.0.0.1:4222 \
  --timescale-dsn "postgres://strata:strata_dev_2026@localhost:5432/strata_rmm?sslmode=disable" \
  --api-addr 127.0.0.1:8080
