#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
env_file="${repo_root}/backend/deployments/.env.nextcloud-sync"

if [[ -f "${env_file}" ]]; then
  set -a
  # shellcheck source=/dev/null
  source "${env_file}"
  set +a
fi

if ! command -v rclone >/dev/null 2>&1; then
  echo "rclone nicht gefunden. Bitte rclone installieren." >&2
  exit 1
fi

: "${RCLONE_SOURCE:?RCLONE_SOURCE ist nicht gesetzt (backend/deployments/.env.nextcloud-sync).}"

rclone_dest="${RCLONE_DEST:-${repo_root}/data-exchange/1_friedrich-bjoern}"
rclone_geodata_dest="${RCLONE_GEODATA_DEST:-${repo_root}/data-exchange/geodata}"
mkdir -p "${rclone_dest}"
mkdir -p "${rclone_geodata_dest}"

common_args=(--delete-during --checksum)

# Locally generated paths that do not exist in Nextcloud: without --exclude, --delete-during
# would remove them on every sync (excluded paths are neither copied nor deleted).
#  - sv_beschaeftigte: written by pipeline/dlt/fetch_ba_sv_beschaeftigte.py (cache of ~400 BA requests).
#    Remove this exclude once the folder has been uploaded to Nextcloud (data-exchange/TODO.md).
#  - lau_population_<year>.csv: written by pipeline/dlt/fetch_lau_population.py into
#    $GEODATA_BASE_PATH/statistics/lokale_verwaltungseinheiten_eu/output/ (= the geodata destination).
#  - _downloads/: download cache of fetch_lau_population.py ($GEODATA_BASE_PATH/_downloads/lau_<year>.xlsx),
#    written by the pipeline container (root), so the deploy user could not delete it anyway
#    ("permission denied", sync aborts).
local_excludes=(--exclude "/sv_beschaeftigte/results_sv_beschaeftigte_kreise/**")
geodata_excludes=(--exclude "/statistics/lokale_verwaltungseinheiten_eu/output/lau_population_*.csv" --exclude "/_downloads/**")

if [[ -n "${RCLONE_CONFIG_FILE:-}" ]]; then
  common_args+=("--config" "${RCLONE_CONFIG_FILE}")
fi

echo "Starte Spiegelung von ${RCLONE_SOURCE} nach ${rclone_dest} (ohne Geodaten)"
rclone sync "${RCLONE_SOURCE}" "${rclone_dest}" "${common_args[@]}" "${local_excludes[@]}" --exclude "/Geodaten/**"

if [[ -d "${rclone_dest}/Geodaten" ]]; then
  echo "Entferne veraltetes Zielverzeichnis ${rclone_dest}/Geodaten"
  rm -rf "${rclone_dest}/Geodaten"
fi

echo "Starte Spiegelung von ${RCLONE_SOURCE}/Geodaten nach ${rclone_geodata_dest}"
exec rclone sync "${RCLONE_SOURCE}/Geodaten" "${rclone_geodata_dest}" "${common_args[@]}" "${geodata_excludes[@]}"
