#!/usr/bin/env bash
# Stellt ein Backup von backup_db.sh in eine FRISCHE Datenbank wieder her
# (Disaster Recovery, z. B. neuer Server). Ablauf danach: Pipeline laufen lassen,
# dbt-Snapshots setzen die Historie fort.
#
#   restore_db.sh [latest|<dateiname>]      Snapshots wiederherstellen
#   RESTORE_METADATA=1 restore_db.sh ...    zusätzlich loaded_data.dataset_metadata
#
# Auf einer Datenbank mit vorhandenen Snapshots bricht das Skript ab: dbt-Views hängen an den
# Snapshot-Tabellen, ein Überschreiben würde scheitern oder Historie verlieren
set -euo pipefail

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
which_backup="${1:-latest}"

if [[ -z "${BACKUP_TARGET:-}" ]]; then
  : "${BACKUP_S3_BUCKET:?BACKUP_S3_BUCKET fehlt (${env_file})}"
  : "${BACKUP_S3_REGION:?BACKUP_S3_REGION fehlt (${env_file})}"
  : "${BACKUP_S3_ACCESS_KEY:?BACKUP_S3_ACCESS_KEY fehlt (${env_file})}"
  : "${BACKUP_S3_SECRET_KEY:?BACKUP_S3_SECRET_KEY fehlt (${env_file})}"
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

if [[ "${which_backup}" == "latest" ]]; then
  which_backup="$(rclone lsf "${BACKUP_TARGET}" --include 'soziodata_*.dump' | sort | tail -n1)"
  [[ -n "${which_backup}" ]] || { echo "Kein Backup in ${BACKUP_TARGET} gefunden." >&2; exit 1; }
fi
echo "Stelle ${which_backup} wieder her"

existing="$(psql_q "select count(*) from information_schema.tables where table_schema = 'snapshots'" || echo 0)"
if [[ "${existing}" != "0" ]]; then
  echo "Abbruch: Schema snapshots enthält bereits ${existing} Tabellen. Restore nur in eine frische Datenbank." >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT
rclone copyto "${BACKUP_TARGET}/${which_backup}" "${tmp}"

restore() {
  docker exec -i "${container}" pg_restore -U "${db_user}" -d "${db_name}" --no-owner --exit-on-error "$@" < "${tmp}"
}

# Das Archiv enthält nur Tabellen (pg_dump --table), keine Schema-Definitionen
psql_q "create schema if not exists snapshots" > /dev/null
restore --schema=snapshots

if [[ "${RESTORE_METADATA:-0}" == "1" ]]; then
  psql_q "create schema if not exists loaded_data" > /dev/null
  restore --schema=loaded_data --table=dataset_metadata
fi

echo "Fertig. Jetzt die Pipeline ausführen: docker exec pipeline bash /app/dlt/run_all_pipelines.sh"
