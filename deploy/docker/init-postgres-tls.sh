#!/bin/sh
set -eu

install -d -o postgres -g postgres -m 0700 /run/postgres-tls
install -o postgres -g postgres -m 0600 /run/secrets/postgres_tls_key /run/postgres-tls/server.key
install -o postgres -g postgres -m 0644 /run/secrets/postgres_tls_cert /run/postgres-tls/server.crt

exec /usr/local/bin/docker-entrypoint.sh "$@"
