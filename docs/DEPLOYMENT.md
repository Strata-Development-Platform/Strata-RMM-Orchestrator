# Strata RMM — Deployment Reference

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Deployment Models

| Model | Status | Description |
|-------|--------|-------------|
| Docker Compose | ✅ Complete | Local development, simple deployments |
| Kubernetes (Helm) | ✅ Complete | Production SaaS, self-hosted |
| Linux systemd | ✅ Complete | Bare metal, VPS |
| Windows service | ✅ Complete | Windows servers |

---

## 2. Docker Compose (Development)

### 2.1 Quick Start

```bash
# Clone and start
git clone https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator.git
cd Strata-RMM-Orchestrator
docker-compose up -d
```

### 2.2 docker-compose.yml

```yaml
services:
  nats:
    image: nats:2.10
    ports:
      - "4222:4222"
      - "8222:8222"  # JetStream dashboard
    volumes:
      - nats-data:/var/lib/nats

  postgres:
    image: timescale/timescaledb:latest-pg16
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: strata_rmm
      POSTGRES_USER: strata
      POSTGRES_PASSWORD: strata
    volumes:
      - postgres-data:/var/lib/postgresql/data

  minio:
    image: minio/minio:latest
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      MINIO_ROOT_USER: minio
      MINIO_ROOT_PASSWORD: minio123
    command: server /data --console-address ":9001"
    volumes:
      - minio-data:/data

  orchestrator:
    build: .
    ports:
      - "8080:8080"
    environment:
      STRATA_RUNTIME_MODE: development
      JWT_SECRET: dev-secret-that-is-at-least-32-chars-long
      STRATA_SEED_DEV: "true"
      STRATA_DEV_ADMIN_EMAIL: admin@localhost
      STRATA_DEV_ADMIN_PASSWORD_HASH: hash
      TIMESCALE_DSN: postgres://strata:strata@postgres:5432/strata_rmm?sslmode=disable
      NATS_URL: nats://nats:4222
      STORAGE_BACKEND: minio
      STORAGE_ENDPOINT: http://minio:9000
      STORAGE_ACCESS_KEY: minio
      STORAGE_SECRET_KEY: minio123
    depends_on:
      - nats
      - postgres
      - minio

volumes:
  nats-data:
  postgres-data:
  minio-data:
```

### 2.3 Access

| Service | URL |
|---------|-----|
| API | `http://localhost:8080` |
| Health | `http://localhost:8080/health` |
| NATS Dashboard | `http://localhost:8222` |
| MinIO Console | `http://localhost:9001` |
| Grafana | `http://localhost:3000` (if configured) |

---

## 3. Kubernetes (Helm)

### 3.1 Install

```bash
helm install strata deploy/helm/strata/ -f deploy/helm/strata/values.yaml
```

### 3.2 Values

```yaml
replicaCount: 1

image:
  repository: strata-rmm/orchestrator
  tag: latest
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080

ingress:
  enabled: true
  hosts:
    - host: strata.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: strata-tls
      hosts:
        - strata.example.com

resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi

autoscaling:
  enabled: true
  minReplicas: 1
  maxReplicas: 5
  targetCPUUtilizationPercentage: 80

nats:
  enabled: true  # Use embedded NATS or external

postgres:
  enabled: true  # Use embedded PostgreSQL or external
  # For external:
  # external:
  #   host: postgres.example.com
  #   port: 5432
  #   database: strata_rmm

redis:
  enabled: true  # Use embedded Redis or external
  # For external:
  # external:
  #   host: redis.example.com
  #   port: 6379

storage:
  backend: minio  # local, minio, s3
  # For S3:
  # backend: s3
  # bucket: strata-recordings
  # endpoint: https://s3.amazonaws.com

env:
  JWT_SECRET: your-32-char-jwt-secret-here
  STRATA_RUNTIME_MODE: production
  STRATA_PUBLIC_URL: https://strata.example.com
```

### 3.3 Air-Gapped

```yaml
# deploy/helm/strata/values-airgapped.yaml
image:
  repository: strata-rmm/orchestrator
  tag: v1.0.0
  pullPolicy: Never  # Image pre-pulled

# Private registry
imagePullSecrets:
  - name: registry-secret

# All dependencies bundled
nats:
  enabled: true
postgres:
  enabled: true
redis:
  enabled: true
```

---

## 4. Linux systemd

### 4.1 Install

```bash
# Download binary
wget https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/releases/latest/strata-rmm-linux-amd64 -O /usr/local/bin/strata
chmod +x /usr/local/bin/strata

# Create systemd unit
cat > /etc/systemd/system/strata.service << EOF
[Unit]
Description=Strata RMM Orchestrator
After=network.target postgresql.service nats-server.service

[Service]
Type=simple
User=strata
Group=strata
WorkingDirectory=/opt/strata
ExecStart=/usr/local/bin/strata
Restart=on-failure
RestartSec=5
Environment=STRATA_RUNTIME_MODE=production
Environment=TIMESCALE_DSN=postgres://strata:strata@localhost:5432/strata_rmm?sslmode=require
Environment=NATS_URL=nats://localhost:4222
Environment=JWT_SECRET=your-32-char-jwt-secret-here

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

# Start
systemctl daemon-reload
systemctl enable strata
systemctl start strata
systemctl status strata
```

### 4.2 Log Journal

```bash
# View logs
journalctl -u strata -f

# View recent
journalctl -u strata --since "1 hour ago"
```

---

## 5. Windows Service

### 5.1 Install

```powershell
# Download binary
Invoke-WebRequest -Uri "https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/releases/latest/strata-rmm-windows-amd64.exe" -OutFile "C:\Strata\strata.exe"

# Install as service
& "C:\Strata\strata.exe" install

# Start service
Start-Service Strata

# View status
Get-Service Strata
```

### 5.2 Uninstall

```powershell
# Stop service
Stop-Service Strata

# Uninstall
& "C:\Strata\strata.exe" uninstall
```

---

## 6. Environment Variables

All configuration via environment variables. See `docs/CONFIGURATION.md` for full reference.

**Required:**
- `TIMESCALE_DSN` (or `STRATA_DB_DSN` or `DATABASE_URL`)
- `NATS_URL`
- `JWT_SECRET` (min 32 chars)

**Optional:**
- `REDIS_URL` (for token blacklisting)
- `STORAGE_BACKEND` (local, minio, s3)
- `STORAGE_*` (bucket, endpoint, credentials)
- `NATS_*` (TLS, token)
- `STRATA_*` (SMTP, alerts, etc.)

---

## 7. Multi-Region Deployment

### 7.1 Region Configuration

```yaml
# deploy/helm/strata/values-us-east.yaml
region: us-east-1
postgres:
  host: us-east-postgres.example.com
nats:
  url: nats://us-east-nats.example.com:4222
  advertise_urls:
    - tls://us-east-nats1.example.com:4222
    - tls://us-east-nats2.example.com:4222
```

```yaml
# deploy/helm/strata/values-eu-west.yaml
region: eu-west-1
postgres:
  host: eu-west-postgres.example.com
nats:
  url: nats://eu-west-nats.example.com:4222
  advertise_urls:
    - tls://eu-west-nats1.example.com:4222
    - tls://eu-west-nats2.example.com:4222
```

### 7.2 NATS Supercluster

NATS JetStream supercluster for cross-region replication:

```bash
# Configure NATS supercluster
# See: https://docs.nats.io/nats-concepts/jetstream/superclusters
```

---

## 8. Backup & Restore

### 8.1 Backup

```yaml
# Enable backup
STRATA_BACKUP_ENABLED=true
STRATA_BACKUP_ENVIRONMENT_ID=my-environment
STRATA_BACKUP_DATABASE_TYPE=timescaledb
STRATA_BACKUP_REPOSITORY_TYPE=s3
STRATA_BACKUP_EXTERNAL_BUCKET=strata-backups
STRATA_BACKUP_EXTERNAL_REGION=us-east-1
```

### 8.2 Restore

```bash
# Restore from backup
STRATA_RECOVERY_STORAGE_BACKEND=s3
STRATA_RECOVERY_STORAGE_BUCKET=strata-backups
STRATA_RECOVERY_NATS_URL=nats://recovery-nats:4222
```

---

## 9. Monitoring

### 9.1 Health Checks

| Endpoint | Purpose |
|----------|---------|
| `/health` | Ready (DB, NATS, storage) |
| `/health/live` | Liveness (process alive) |

### 9.2 Prometheus Metrics

| Metric | Description |
|--------|-------------|
| `strata_db_connections_open` | Open DB connections |
| `strata_db_connections_in_use` | In-use DB connections |
| `strata_nats_connected` | NATS connection status |
| `strata_nats_reconnects_total` | NATS reconnect count |

---

## 10. Scaling

### 10.1 Horizontal Pod Autoscaler

```yaml
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70
```

### 10.2 NATS Scaling

```yaml
nats:
  cluster:
    replicas: 3
  jetstream:
    maxMemoryStore: 4GB
    maxFileStore: 100GB
```

---

## 11. Rollback

### 11.1 Application Rollback

```bash
# Helm rollback
helm rollback strata 1

# Docker rollback
docker-compose up -d --force-recreate --no-deps orchestrator

# Systemd rollback
cp /opt/strata/strata-v1.0.0 /opt/strata/strata
systemctl restart strata
```

### 11.2 Database Rollback

```bash
# Rollback schema migrations
# See: docs/ROLLBACK.md
```

---

## 12. Upgrade

### 12.1 Application Upgrade

```bash
# Helm upgrade
helm upgrade strata deploy/helm/strata/ -f values.yaml

# Docker upgrade
docker-compose pull
docker-compose up -d --force-recreate --no-deps orchestrator

# Systemd upgrade
cp /opt/strata/strata-v1.1.0 /opt/strata/strata
systemctl restart strata
```

### 12.2 Database Upgrade

```bash
# Schema migrations run automatically on startup
# See: pkg/postgres/upgrade.go
```

---

*Last Updated: 2026-08-08*
