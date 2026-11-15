#!/usr/bin/env bash
# Backs up the non-reproducible data from PostgreSQL to Hetzner Object Storage:
#   - schema "snapshots"            (dbt snapshots = historized values)
#   - table loaded_data.dataset_metadata (origin/license of the datasets)
# Everything else (loaded_data.*, transformed_data.*) is rebuilt by the pipeline from the sources.
#
# Runs on the server (host, not in the container) via cron, set up by deploy_remote.sh.
# Configuration: ~/.config/soziodata-backup.env (written by the deploy job from GitHub secrets)
#   BACKUP_S3_BUCKET, BACKUP_S3_REGION, BACKUP_S3_ACCESS_KEY, BACKUP_S3_SECRET_KEY
# Optional: BACKUP_RETENTION_DAYS (30), POSTGRES_CONTAINER (postgis), DB_USER, DB_NAME,
#           BACKUP_TARGET (any rclone target, e.g. a local path for tests),
#           HEALTHCHECK_URL (curl -fsS on success, e.g. healthchecks.io ping URL).
# Restore: backend/scripts/restore_db.sh
set -euo pipefail

# Otherwise rclone complains on every call that no rclone.conf exists (the remote comes from the environment)
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
  # rclone remote "sozio" configured entirely via environment variables, no rclone.conf needed
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

# Fail loudly if the DB is unreachable (container down, wrong credentials): inside
# [[ -n "$(psql_q ...)" ]] a failure would be swallowed (set -e does not apply there, SC2310)
# and the script would report "nothing to back up" with exit 0.
# shellcheck disable=SC2310  # the failure is handled explicitly here
if ! psql_q 'select 1' > /dev/null; then
  echo "Fehler: Datenbank nicht erreichbar (Container ${container}, DB ${db_name}) - kein Backup." >&2
  exit 1
fi

# On the very first deploy nothing exists yet (the pipeline has not run)
dump_args=()
# Plain assignments: a psql failure aborts via set -e
has_snapshots="$(psql_q "select 1 from information_schema.schemata where schema_name = 'snapshots'")"
has_metadata="$(psql_q "select to_regclass('loaded_data.dataset_metadata')")"
if [[ -n "${has_snapshots}" ]]; then
  # --table instead of --schema: pg_dump ignores --schema once --table is given
  dump_args+=(--table='snapshots.*')
fi
if [[ -n "${has_metadata}" ]]; then
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

# Verify the archive is readable before uploading it
docker exec -i "${container}" pg_restore --list < "${tmp}" > /dev/null

# The snapshots are the irreplaceable part: never prune old backups on the strength of a dump without them
# (the retention below would otherwise eventually delete the last usable backups).
if [[ "${dump_args[*]}" != *snapshots* ]]; then
  echo "Warnung: Dump enthält keine snapshots.*-Tabellen - Backup hochgeladen, aber Retention übersprungen." >&2
  skip_retention=1
fi

rclone copyto "${tmp}" "${BACKUP_TARGET}/${name}"
echo "Backup ${name} ($(stat -c %s "${tmp}") Bytes) -> ${BACKUP_TARGET}"

# Retention
if [[ "${skip_retention:-0}" != "1" ]]; then
  rclone delete "${BACKUP_TARGET}" --min-age "${retention_days}d" --include "soziodata_*.dump"
fi

# Optional success ping (dead-man's switch), only if configured. Not sent for a dump without the
# snapshots (that is not a healthy backup), so a missing ping alerts the operator.
if [[ -n "${HEALTHCHECK_URL:-}" && "${skip_retention:-0}" != "1" ]]; then
  curl -fsS -m 10 --retry 3 -o /dev/null "${HEALTHCHECK_URL}" || echo "Warnung: HEALTHCHECK_URL nicht erreichbar." >&2
fi
