# Strata RMM — Backup Reference

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Backup Overview

Strata provides encrypted database backup with AES-256-GCM encryption. Backups support filesystem and S3-compatible storage repositories.

---

## 2. Backup Configuration

### 2.1 Required Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `STRATA_BACKUP_ENABLED` | Yes (if backup enabled) | `true` to enable backup engine |
| `STRATA_BACKUP_ENVIRONMENT_ID` | Yes (if backup enabled) | Unique environment identifier |
| `STRATA_BACKUP_KEY_PROVIDER_PATH` | Yes (if backup enabled) | Path to encryption key provider file |

### 2.2 Repository Configuration

**Filesystem Repository:**
```bash
STRATA_BACKUP_REPOSITORY_TYPE=filesystem
STRATA_BACKUP_DIRECTORY=/var/backups/strata
```

**S3 Repository:**
```bash
STRATA_BACKUP_REPOSITORY_TYPE=s3
STRATA_BACKUP_EXTERNAL_BUCKET=strata-backups
STRATA_BACKUP_EXTERNAL_REGION=us-east-1
STRATA_BACKUP_EXTERNAL_ENDPOINT=https://s3.amazonaws.com
STRATA_BACKUP_EXTERNAL_ACCESS_KEY=AKIA...
STRATA_BACKUP_EXTERNAL_SECRET_KEY=wJalr...
```

### 2.3 Optional Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `STRATA_BACKUP_DATABASE_TYPE` | `timescaledb` | `postgresql` or `timescaledb` |
| `STRATA_BACKUP_ENCRYPTION_SCHEME` | `aes-256-gcm` | Only `aes-256-gcm` allowed |

---

## 3. Encryption

### 3.1 Algorithm

- **Scheme:** AES-256-GCM (Galois/Counter Mode)
- **Key derivation:** Key provider file (operator-provided)
- **IV:** Random per backup, stored with encrypted data
- **Authentication:** GCM tag for integrity verification

### 3.2 Key Provider

The key provider file contains the encryption key material. Format:
```
[encryption]
key = <base64-encoded-32-byte-key>
```

**Security:** File must be:
- Absolute path
- Canonical (no traversal)
- Regular file
- Max 16 KiB
- Not empty

---

## 4. Backup Process

### 4.1 Engine

`pkg/backup/engine.go` orchestrates the backup:

1. **Quiesce** — Graceful service shutdown
2. **Snapshot** — Database backup (pg_dump/pg_basebackup)
3. **Encrypt** — AES-256-GCM encryption
4. **Upload** — Repository storage (filesystem or S3)
5. **Resume** — Service restart

### 4.2 Components

| Component | Description |
|-----------|-------------|
| Coordinator | Manages backup lifecycle |
| Quiescer | Graceful shutdown/resume |
| PostgresComponent | PostgreSQL-specific backup logic |
| OfflineQuiescer | Offline backup mode |

---

## 5. Recovery

### 5.1 Recovery Configuration

| Variable | Description |
|----------|-------------|
| `STRATA_RECOVERY_STORAGE_BACKEND` | `local`, `minio`, `s3` |
| `STRATA_RECOVERY_STORAGE_BUCKET` | Storage bucket name |
| `STRATA_RECOVERY_STORAGE_REGION` | Storage region |
| `STRATA_RECOVERY_STORAGE_ENDPOINT` | Custom endpoint |
| `STRATA_RECOVERY_STORAGE_ACCESS_KEY` | Access key |
| `STRATA_RECOVERY_STORAGE_SECRET_KEY` | Secret key |
| `STRATA_RECOVERY_STORAGE_USE_SSL` | Use SSL for storage |
| `STRATA_RECOVERY_NATS_URL` | NATS URL for recovery |
| `STRATA_RECOVERY_NATS_TOKEN` | NATS token |
| `STRATA_RECOVERY_NATS_TLS_CA` | NATS CA certificate |
| `STRATA_RECOVERY_NATS_TLS_CERT` | NATS client certificate |
| `STRATA_RECOVERY_NATS_TLS_KEY` | NATS client key |

### 5.2 Recovery Process

1. **Download** — Retrieve encrypted backup from storage
2. **Decrypt** — AES-256-GCM decryption
3. **Restore** — Database restore (pg_restore)
4. **Verify** — Integrity check

---

## 6. Scheduling

Backups are triggered via the backup engine. For automated scheduling, integrate with cron/systemd timer:

```bash
# Example: Daily backup at 2am
0 2 * * * /usr/local/bin/strata backup run
```

---

## 7. Retention

Configure retention policies based on compliance requirements:

| Policy | Duration |
|--------|----------|
| Daily backups | 7 days |
| Weekly backups | 4 weeks |
| Monthly backups | 12 months |

**Note:** Retention cleanup must be implemented by the operator.

---

## 8. Verification

### 8.1 Post-Backup Verification

```bash
# Check backup exists
ls -la /var/backups/strata/

# Verify encryption
# (decrypt and compare with original)
```

### 8.2 Recovery Testing

**Recommended:** Test recovery quarterly:

1. Create test environment
2. Restore backup to test environment
3. Verify data integrity
4. Document results

---

## 9. Security

### 9.1 Key Management

- Store key provider file in secure location (K8s Secret, HashiCorp Vault)
- Rotate keys periodically
- Never commit keys to version control

### 9.2 Storage Security

- Use encrypted storage (S3 SSE-KMS, encrypted filesystem)
- Restrict access to backup storage
- Enable access logging

---

## 10. Troubleshooting

### 10.1 Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| Backup fails | Key provider missing | Verify `STRATA_BACKUP_KEY_PROVIDER_PATH` |
| Backup fails | Storage connection error | Check `STRATA_BACKUP_EXTERNAL_*` variables |
| Backup fails | Disk full | Increase backup directory space |
| Recovery fails | Wrong key | Verify key provider matches backup |
| Recovery fails | Storage access denied | Check storage credentials |

---

## 11. Limitations

- **Encryption:** Only `aes-256-gcm` supported
- **Database:** Only PostgreSQL/TimescaleDB
- **Repository:** Filesystem or S3-compatible only
- **No incremental:** Full backup only
- **No compression:** Compress externally if needed

---

*Last Updated: 2026-08-08*
