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
mkdir -p "${rclone_dest}"

cmd=(rclone sync "${RCLONE_SOURCE}" "${rclone_dest}" --delete-during --checksum)

if [[ -n "${RCLONE_CONFIG_FILE:-}" ]]; then
  cmd+=("--config" "${RCLONE_CONFIG_FILE}")
fi

echo "Starte Spiegelung von ${RCLONE_SOURCE} nach ${rclone_dest}"
exec "${cmd[@]}"
