# Strata RMM — Restore Reference

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Restore Overview

Restore recovers a Strata deployment from an encrypted backup. The restore process decrypts the backup and restores the PostgreSQL/TimescaleDB database.

---

## 2. Prerequisites

### 2.1 Required Infrastructure

| Component | Requirement |
|-----------|-------------|
| PostgreSQL/TimescaleDB | Installed and running |
| NATS | Installed and running (or configured for recovery) |
| Object storage | Access to backup storage (same as backup) |
| Encryption key | Key provider file matching the backup |

### 2.2 Required Variables

```bash
STRATA_RECOVERY_STORAGE_BACKEND=s3
STRATA_RECOVERY_STORAGE_BUCKET=strata-backups
STRATA_RECOVERY_STORAGE_REGION=us-east-1
STRATA_RECOVERY_STORAGE_ACCESS_KEY=AKIA...
STRATA_RECOVERY_STORAGE_SECRET_KEY=wJalr...
STRATA_RECOVERY_NATS_URL=nats://recovery-nats:4222
```

---

## 3. Restore Process

### 3.1 Automated Restore

```bash
# Set recovery environment variables
export STRATA_RECOVERY_STORAGE_BACKEND=s3
export STRATA_RECOVERY_STORAGE_BUCKET=strata-backups
export STRATA_RECOVERY_NATS_URL=nats://localhost:4222

# Start orchestrator in recovery mode
./strata recovery restore
```

### 3.2 Manual Restore

```bash
# Step 1: Download backup
aws s3 cp s3://strata-backups/latest.enc /tmp/backup.enc

# Step 2: Decrypt backup
./strata recovery decrypt \
  --input /tmp/backup.enc \
  --key-provider /path/to/key-provider \
  --output /tmp/backup.sql

# Step 3: Restore database
psql -h localhost -U strata -d strata_rmm -f /tmp/backup.sql

# Step 4: Verify
psql -h localhost -U strata -d strata_rmm -c "SELECT count(*) FROM tenants;"
```

---

## 4. Recovery Modes

### 4.1 Full Restore

Restores entire database to backup state:

```bash
# Drop and recreate database
psql -U strata -c "DROP DATABASE strata_rmm;"
psql -U strata -c "CREATE DATABASE strata_rmm;"
psql -U strata -d strata_rmm -f /tmp/backup.sql
```

### 4.2 Point-in-Time Recovery (Future)

Not yet implemented. Requires WAL archiving.

---

## 5. Post-Restep Validation

### 5.1 Database Integrity

```bash
# Check table counts
psql -U strata -d strata_rmm -c "\dt"

# Verify tenant data
psql -U strata -d strata_rmm -c "SELECT count(*) FROM tenants;"
psql -U strata -d strata_rmm -c "SELECT count(*) FROM devices;"
```

### 5.2 Service Health

```bash
# Check orchestrator health
curl http://localhost:8080/health

# Check NATS connectivity
nats-server -config /etc/nats/nats.conf
```

---

## 6. Troubleshooting

### 6.1 Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| Restore fails | Wrong encryption key | Verify key provider file |
| Restore fails | Storage access denied | Check storage credentials |
| Restore fails | Database schema mismatch | Upgrade schema to current version |
| Restore fails | NATS connection error | Verify NATS URL and connectivity |

---

## 7. Rollback

If restore fails:

1. **Stop** all services
2. **Revert** to pre-restore state (if available)
3. **Investigate** failure cause
4. **Retry** restore with corrected configuration

---

## 8. Limitations

- **No partial restore:** Full database restore only
- **No cross-version restore:** Restore to matching or newer schema version
- **No encryption bypass:** Must use matching encryption key

---

*Last Updated: 2026-08-08*
