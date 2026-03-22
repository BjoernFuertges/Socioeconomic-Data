#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
env_file="${repo_root}/backend/deployments/.env.nextcloud-sync"

if [[ -f "${env_file}" ]]; then
  set -a
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

if [[ -n "${RCLONE_CONFIG_FILE:-}" ]]; then
  common_args+=("--config" "${RCLONE_CONFIG_FILE}")
fi

echo "Starte Spiegelung von ${RCLONE_SOURCE} nach ${rclone_dest} (ohne Geodaten)"
rclone sync "${RCLONE_SOURCE}" "${rclone_dest}" "${common_args[@]}" --exclude "/Geodaten/**"

if [[ -d "${rclone_dest}/Geodaten" ]]; then
  echo "Entferne veraltetes Zielverzeichnis ${rclone_dest}/Geodaten"
  rm -rf "${rclone_dest}/Geodaten"
fi

echo "Starte Spiegelung von ${RCLONE_SOURCE}/Geodaten nach ${rclone_geodata_dest}"
exec rclone sync "${RCLONE_SOURCE}/Geodaten" "${rclone_geodata_dest}" "${common_args[@]}"
