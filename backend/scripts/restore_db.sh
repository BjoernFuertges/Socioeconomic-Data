#!/usr/bin/env bash
# Restores a backup from backup_db.sh into a FRESH database
# (disaster recovery, e.g. new server). Afterwards: run the pipeline,
# dbt snapshots continue the history.
#
#   restore_db.sh [latest|<filename>]      Restore snapshots
#   RESTORE_METADATA=1 restore_db.sh ...    additionally loaded_data.dataset_metadata
#
# The script stops the scheduler container (SCHEDULER_CONTAINER, default "scheduler") itself and
# starts it again at the end if it was running (after a failed restore it stays stopped); it aborts
# while run_all_pipelines.sh is running in the pipeline container (PIPELINE_CONTAINER).
#
# The script aborts on a database that already has snapshots: dbt views depend on the
# snapshot tables, so overwriting would fail or lose history
#
# On a NEW server: run `deploy_remote.sh apply` (or backend/scripts/ensure_api_role.sql +
# ensure_pipeline_role.sql) once before the pipeline run, so the roles api_ro and pipeline_rw exist.
# If pipeline_rw already exists, the restored tables are created as pipeline_rw (else as the
# superuser; the next ensure_pipeline_role.sql run hands them to pipeline_rw).
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
scheduler="${SCHEDULER_CONTAINER:-scheduler}"
pipeline="${PIPELINE_CONTAINER:-pipeline}"
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

tmp=""
restore_started=0
scheduler_was_running=0
# Restarts the scheduler only if it ran before. After a failure *during* the restore it stays
# stopped: the next hourly dbt snapshot would build on half-restored snapshot tables.
cleanup() {
  local rc=$?
  [[ -n "${tmp}" ]] && rm -f "${tmp}"
  if [[ "${scheduler_was_running}" == "1" ]]; then
    if [[ "${rc}" -eq 0 || "${restore_started}" == "0" ]]; then
      if docker start "${scheduler}" > /dev/null; then
        echo "Scheduler ${scheduler} wieder gestartet."
      else
        echo "Warnung: Scheduler ${scheduler} konnte nicht gestartet werden: docker start ${scheduler}" >&2
      fi
    else
      echo "Restore fehlgeschlagen: Scheduler ${scheduler} bleibt gestoppt. Nach Klärung: docker start ${scheduler}" >&2
    fi
  fi
}
trap cleanup EXIT

# Keep the hourly pipeline from creating (empty) snapshot tables before/while restoring
if [[ "$(docker inspect -f '{{.State.Running}}' "${scheduler}" 2> /dev/null || echo false)" == "true" ]]; then
  echo "Stoppe Scheduler ${scheduler} für die Dauer des Restores ..."
  docker stop "${scheduler}" > /dev/null
  scheduler_was_running=1
fi
# A run that is already in progress is not stopped by that; do not race it
# (grep without -q: an early exit could SIGPIPE docker top under pipefail)
if docker top "${pipeline}" 2> /dev/null | grep 'run_all_pipelines\.sh' > /dev/null; then
  echo "Abbruch: im Container ${pipeline} läuft gerade run_all_pipelines.sh. Ende abwarten und erneut starten." >&2
  exit 1
fi

if [[ "${which_backup}" == "latest" ]]; then
  which_backup="$(rclone lsf "${BACKUP_TARGET}" --include 'soziodata_*.dump' | sort | tail -n1)"
  [[ -n "${which_backup}" ]] || { echo "Kein Backup in ${BACKUP_TARGET} gefunden." >&2; exit 1; }
fi
echo "Stelle ${which_backup} wieder her"

# shellcheck disable=SC2310  # the failure is handled explicitly here
if ! existing="$(psql_q "select count(*) from information_schema.tables where table_schema = 'snapshots'")"; then
  echo "Abbruch: Datenbank nicht erreichbar (Container ${container}, DB ${db_name})." >&2
  exit 1
fi
if [[ "${existing}" != "0" ]]; then
  echo "Abbruch: Schema snapshots enthält bereits ${existing} Tabellen. Restore nur in eine frische Datenbank." >&2
  exit 1
fi

tmp="$(mktemp)"
rclone copyto "${BACKUP_TARGET}/${which_backup}" "${tmp}"

# dlt/dbt run as pipeline_rw (non-superuser) and must own what they write
owner_args=() schema_auth=""
if [[ "$(psql_q "select count(*) from pg_roles where rolname = 'pipeline_rw'")" == "1" ]]; then
  owner_args=(--role=pipeline_rw)
  schema_auth=" authorization pipeline_rw"
fi

restore() {
  docker exec -i "${container}" pg_restore -U "${db_user}" -d "${db_name}" --no-owner "${owner_args[@]}" --exit-on-error "$@" < "${tmp}"
}

# The archive contains only tables (pg_dump --table), no schema definitions
restore_started=1
psql_q "create schema if not exists snapshots${schema_auth}" > /dev/null
restore --schema=snapshots

if [[ "${RESTORE_METADATA:-0}" == "1" ]]; then
  psql_q "create schema if not exists loaded_data${schema_auth}" > /dev/null
  restore --schema=loaded_data --table=dataset_metadata
fi

echo "Fertig. Jetzt deploy_remote.sh apply erneut ausführen (oder backend/scripts/ensure_api_role.sql + ensure_pipeline_role.sql):"
echo "legt api_ro/pipeline_rw an bzw. übereignet die wiederhergestellten Tabellen pipeline_rw."
echo "Dann die Pipeline ausführen: docker exec pipeline bash /app/dlt/run_all_pipelines.sh"
