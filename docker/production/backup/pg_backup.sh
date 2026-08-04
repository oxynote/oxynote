#!/usr/bin/env bash
set -euo pipefail

COMPOSE_FILE="/srv/docker-compose.yaml"
SERVICE="postgres"

DB_NAME="db"
DB_USER="root"

BACKUP_DIR="/srv/backups/postgres"
RETENTION_LOCAL_DAYS=14

# Restic repository on Hetzner Storage Box via SFTP
export RESTIC_REPOSITORY="sftp:USER@USER.your-storagebox.de:restic/postgres"
export RESTIC_PASSWORD_FILE="/root/.restic_pg_password"

TS="$(date -u +%Y-%m-%dT%H%M%SZ)"
OUT="${BACKUP_DIR}/${DB_NAME}_${TS}.dump"

mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"

# Create dump (compressed)
docker compose -f "$COMPOSE_FILE" exec -T "$SERVICE" \
  pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc -Z 6 > "$OUT"

sha256sum "$OUT" > "${OUT}.sha256"

# Local retention
find "$BACKUP_DIR" -type f -name "${DB_NAME}_*.dump" -mtime "+$RETENTION_LOCAL_DAYS" -delete
find "$BACKUP_DIR" -type f -name "${DB_NAME}_*.sha256" -mtime "+$RETENTION_LOCAL_DAYS" -delete

# Offsite backup with restic (encrypted + retention)
restic -o "sftp.args=-p 23 -o BatchMode=yes -o PreferredAuthentications=publickey" snapshots >/dev/null 2>&1 || restic -o "sftp.args=-p 23 -o BatchMode=yes -o PreferredAuthentications=publickey" init

# Back up only the latest dump + checksum (fast)
restic -o "sftp.args=-p 23 -o BatchMode=yes -o PreferredAuthentications=publickey" backup "$OUT" "${OUT}.sha256" --tag postgres --tag "$DB_NAME"

# Keep 14 daily
restic -o "sftp.args=-p 23 -o BatchMode=yes -o PreferredAuthentications=publickey" forget --keep-daily 14 --prune

