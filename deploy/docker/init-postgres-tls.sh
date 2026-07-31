#!/bin/sh
set -eu

install -m 0600 /run/secrets/postgres_tls_key "$PGDATA/server.key"
install -m 0644 /run/secrets/postgres_tls_cert "$PGDATA/server.crt"
