# PostgreSQL Backups (Docker Compose + Hetzner Storage Box)

Production uses PostgreSQL running in Docker Compose.
Backups are taken **daily at midnight** and are:

- created using `pg_dump` from inside the container
- stored locally in `/srv/backups/postgres`
- encrypted and uploaded to **Hetzner Storage Box** using `restic`
- kept with automatic retention

This provides:
- fast local restores
- safe off-site encrypted backups
- automatic cleanup

## Requirements

- Docker + Docker Compose
- PostgreSQL container running from `postgres:16`
- Hetzner Storage Box with SSH access
- `restic` installed on the host
- systemd (Ubuntu)

## Local Backup Folder

Backups are written to:

```
/srv/backups/postgres
```

This directory is only readable by root.

## Restic Repository

Backups are stored encrypted in Hetzner Storage Box using:

```
sftp:USER@HOST.your-storagebox.de:/./restic/postgres
```

Restic encryption password is stored in:

```
/root/.restic_pg_password
```

## Backup Script

Location:

```
/usr/local/bin/pg_backup.sh
```

This script:

1. Runs `pg_dump` inside the postgres container
2. Writes a compressed dump to `/srv/backups/postgres`
3. Uploads the dump to Storage Box via restic
4. Enforces 14 days retention both locally and off-site

## Systemd Timer

Backups run automatically via:

```
pg-backup.timer
```

Schedule:

```
Every day at 00:00 (midnight)
```

If the server is offline, the job runs as soon as it boots (`Persistent=true`).

Check status:

```bash
systemctl list-timers | grep pg-backup
journalctl -u pg-backup.service
```

## Files Created

| File | Purpose |
|------|--------|
| `/usr/local/bin/pg_backup.sh` | Backup script |
| `/etc/systemd/system/pg-backup.service` | Runs the backup |
| `/etc/systemd/system/pg-backup.timer` | Daily scheduler |
| `/srv/backups/postgres` | Local backup cache |
| `/root/.restic_pg_password` | Restic encryption key |

## How to Restore

### Restore latest local backup

```bash
docker compose -f /srv/docker-compose.yaml exec -T postgres \
 pg_restore -U DBUSER -d DBNAME --clean --if-exists < /srv/backups/postgres/DBNAME_YYYY-MM-DD.dump
```

### Restore from Hetzner Storage Box

List snapshots:

```bash
restic -r sftp:USER@HOST.your-storagebox.de:restic/postgres snapshots
```

Restore a snapshot:

```bash
mkdir /tmp/pg-restore
restic -r sftp:USER@HOST.your-storagebox.de:restic/postgres restore SNAPSHOT_ID --target /tmp/pg-restore
```

Then restore into Postgres:

```bash
pg_restore -U DBUSER -d DBNAME --clean --if-exists < /tmp/pg-restore/srv/backups/postgres/DBNAME_*.dump
```

## Why This Is Safe

- Backups are **not stored inside Docker volumes**
- Storage Box is **off-server**
- Data is **encrypted before upload**
- Multiple restore points are available
- Local + remote copies exist

## To Check Last Backup

```bash
ls -lh /srv/backups/postgres
restic -r sftp:... snapshots
journalctl -u pg-backup.service -n 50
```

## Disaster Recovery Summary

If the server dies:

1. Create new server
2. Install Docker + PostgreSQL container
3. Install `restic`
4. Restore from Storage Box
5. Run `pg_restore`

Data loss window: **max 24 hours**
