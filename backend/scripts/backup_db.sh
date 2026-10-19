#!/usr/bin/env bash
# Sichert die nicht rekonstruierbaren Daten aus PostgreSQL in Hetzner Object Storage:
#   - Schema "snapshots"            (dbt-Snapshots = historisierte Werte)
#   - Tabelle loaded_data.dataset_metadata (Herkunft/Lizenz der Datensätze)
# Alles andere (loaded_data.*, transformed_data.*) baut die Pipeline aus den Quellen neu.
#
# Läuft auf dem Server (Host, nicht im Container) per Cron, eingerichtet von deploy_remote.sh.
# Konfiguration: ~/.config/soziodata-backup.env (vom Deploy-Job aus GitHub-Secrets geschrieben)
#   BACKUP_S3_BUCKET, BACKUP_S3_REGION, BACKUP_S3_ACCESS_KEY, BACKUP_S3_SECRET_KEY
# Optional: BACKUP_RETENTION_DAYS (30), POSTGRES_CONTAINER (postgis), DB_USER, DB_NAME,
#           BACKUP_TARGET (beliebiges rclone-Ziel, z. B. ein lokaler Pfad für Tests).
# Wiederherstellung: backend/scripts/restore_db.sh
set -euo pipefail

# rclone meldet sonst bei jedem Aufruf, dass keine rclone.conf existiert (Remote kommt aus der Umgebung)
export RCLONE_LOG_LEVEL=ERROR

env_file="${BACKUP_ENV_FILE:-${HOME}/.config/soziodata-backup.env}"
if [[ -f "${env_file}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${env_file}"
  set +a
fi

container="${POSTGRES_CONTAINER:-postgis}"
db_user="${DB_USER:-sozio_data_compose_user}"
db_name="${DB_NAME:-soziooekonomische-daten}"
retention_days="${BACKUP_RETENTION_DAYS:-30}"

if [[ -z "${BACKUP_TARGET:-}" ]]; then
  : "${BACKUP_S3_BUCKET:?BACKUP_S3_BUCKET fehlt (${env_file})}"
  : "${BACKUP_S3_REGION:?BACKUP_S3_REGION fehlt (${env_file})}"
  : "${BACKUP_S3_ACCESS_KEY:?BACKUP_S3_ACCESS_KEY fehlt (${env_file})}"
  : "${BACKUP_S3_SECRET_KEY:?BACKUP_S3_SECRET_KEY fehlt (${env_file})}"
  # rclone-Remote "sozio" komplett über Umgebungsvariablen, keine rclone.conf nötig
  export RCLONE_CONFIG_SOZIO_TYPE=s3
  export RCLONE_CONFIG_SOZIO_PROVIDER=Other
  export RCLONE_CONFIG_SOZIO_ACCESS_KEY_ID="${BACKUP_S3_ACCESS_KEY}"
  export RCLONE_CONFIG_SOZIO_SECRET_ACCESS_KEY="${BACKUP_S3_SECRET_KEY}"
  export RCLONE_CONFIG_SOZIO_REGION="${BACKUP_S3_REGION}"
  export RCLONE_CONFIG_SOZIO_ENDPOINT="https://${BACKUP_S3_REGION}.your-objectstorage.com"
  BACKUP_TARGET="sozio:${BACKUP_S3_BUCKET}/db"
fi

psql_q() {
  docker exec "${container}" psql -U "${db_user}" -d "${db_name}" -tAc "$1"
}

# Beim allerersten Deploy existiert noch nichts (Pipeline lief noch nicht)
dump_args=()
if [[ -n "$(psql_q "select 1 from information_schema.schemata where schema_name = 'snapshots'")" ]]; then
  # --table statt --schema: pg_dump ignoriert --schema, sobald --table angegeben ist
  dump_args+=(--table='snapshots.*')
fi
if [[ -n "$(psql_q "select to_regclass('loaded_data.dataset_metadata')")" ]]; then
  dump_args+=(--table=loaded_data.dataset_metadata)
fi
if [[ ${#dump_args[@]} -eq 0 ]]; then
  echo "Noch keine Snapshots/Metadaten vorhanden - nichts zu sichern."
  exit 0
fi

tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

name="soziodata_$(date -u +%Y%m%dT%H%M%SZ).dump"
docker exec "${container}" pg_dump -U "${db_user}" -d "${db_name}" --format=custom --no-owner "${dump_args[@]}" > "${tmp}"

# Archiv auf Lesbarkeit prüfen, bevor es hochgeladen wird
docker exec -i "${container}" pg_restore --list < "${tmp}" > /dev/null

rclone copyto "${tmp}" "${BACKUP_TARGET}/${name}"
echo "Backup ${name} ($(stat -c %s "${tmp}") Bytes) -> ${BACKUP_TARGET}"

# Aufbewahrung
rclone delete "${BACKUP_TARGET}" --min-age "${retention_days}d" --include "soziodata_*.dump"
