#!/usr/bin/env bash
# Runs on the Hetzner server (invoked via SSH from .github/workflows/main.yml).
#
#   deploy_remote.sh apply <git-sha> [true|false]   Roll out configuration, update the stack
#                                                   (3rd argument = initial data load in the background)
#   deploy_remote.sh initial-load-status            Show the state of the background load
#   deploy_remote.sh mark-good <git-sha>            Remember the state as "last good" after a green E2E
#   deploy_remote.sh rollback                       Reset to the last good state
#
# Backups: see backup_db.sh / restore_db.sh (cron is set up by "apply").
#
# The workflow puts the configuration (.env, .env.nextcloud-sync, rclone.conf)
# in ${DEPLOY_INCOMING_DIR} beforehand; this keeps secrets out of process lists.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
deploy_dir="${repo_root}/backend/deployments"
state_dir="${repo_root}/.deploy-state"
incoming_dir="${DEPLOY_INCOMING_DIR:-${HOME}/deploy-incoming}"

mkdir -p "${state_dir}"

compose() {
  (cd "${deploy_dir}" && docker compose "$@")
}

# Nightly backup (02:17 server time) via the deploy user's cron; idempotent via the marker.
install_backup_cron() {
  local marker="# soziodata-backup"
  if ! command -v crontab > /dev/null 2>&1; then
    echo "Warnung: crontab nicht installiert - Backup-Cron nicht eingerichtet." >&2
    return 0
  fi
  {
    crontab -l 2> /dev/null | grep -vF "${marker}" || true
    echo "17 2 * * * bash '${repo_root}/backend/scripts/backup_db.sh' >> \"\$HOME/backup.log\" 2>&1 ${marker}"
  } | crontab -
  echo "Backup-Cron eingerichtet (täglich 02:17, Log: ~/backup.log)"
}

install_config() {
  test -f "${incoming_dir}/.env" || { echo "${incoming_dir}/.env fehlt" >&2; exit 1; }
  install -m 600 "${incoming_dir}/.env" "${deploy_dir}/.env"

  if [[ -f "${incoming_dir}/.env.nextcloud-sync" ]]; then
    install -m 600 "${incoming_dir}/.env.nextcloud-sync" "${deploy_dir}/.env.nextcloud-sync"
    echo "RCLONE_CONFIG_FILE=${HOME}/.config/rclone/rclone.conf" >> "${deploy_dir}/.env.nextcloud-sync"
  fi
  if [[ -f "${incoming_dir}/rclone.conf" ]]; then
    install -m 700 -d "${HOME}/.config/rclone"
    install -m 600 "${incoming_dir}/rclone.conf" "${HOME}/.config/rclone/rclone.conf"
  fi

  if [[ -f "${incoming_dir}/.env.backup" ]]; then
    install -m 700 -d "${HOME}/.config"
    install -m 600 "${incoming_dir}/.env.backup" "${HOME}/.config/soziodata-backup.env"
    install_backup_cron
  else
    echo "Hinweis: .env.backup fehlt - nächtliches DB-Backup ist NICHT eingerichtet."
  fi

  # Values are overridden in the compose service via "environment:"; the file only needs to exist.
  if [[ ! -f "${deploy_dir}/.env.backend" ]]; then
    install -m 600 "${deploy_dir}/.env.backend.example" "${deploy_dir}/.env.backend"
  fi

  mkdir -p "${deploy_dir}/letsencrypt"
  touch "${deploy_dir}/letsencrypt/acme.json"
  chmod 600 "${deploy_dir}/letsencrypt/acme.json"
}

wait_for_pipeline_healthy() {
  echo "Warte auf Healthcheck des pipeline-Containers (pip install läuft beim Start) ..."
  for _ in $(seq 1 60); do
    status="$(docker inspect -f '{{.State.Health.Status}}' pipeline 2>/dev/null || echo missing)"
    if [[ "${status}" == "healthy" ]]; then
      return 0
    fi
    sleep 10
  done
  echo "pipeline-Container wurde nicht healthy (Status: ${status})" >&2
  compose logs --tail=50 pipeline >&2 || true
  return 1
}

# Initial data load in the background (dlt + dbt, may take long): the deploy job does not wait for it
# and so uses no Actions minutes. The scheduler is stopped meanwhile so the hourly run does not write
# the same tables in parallel; the wrapper restarts it at the end.
start_initial_load() {
  local status_file="${state_dir}/initial_load.status" log_file="${HOME}/initial_load.log"
  if [[ "$(cat "${status_file}" 2> /dev/null)" == running* ]]; then
    echo "Ein initialer Load läuft bereits (siehe: deploy_remote.sh initial-load-status) - kein zweiter Start."
    return 0
  fi
  echo "running since $(date -u +%Y-%m-%dT%H:%M:%SZ)" > "${status_file}"
  compose stop scheduler
  # shellcheck disable=SC2016 # script deliberately in single quotes, it is only evaluated in the wrapper shell
  DEPLOY_DIR="${deploy_dir}" setsid nohup bash -c '
    docker exec pipeline bash /app/dlt/run_all_pipelines.sh
    rc=$?
    if [ "$rc" -eq 0 ]; then
      echo "done at $(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$1"
    else
      echo "failed (exit $rc) at $(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$1"
    fi
    cd "$DEPLOY_DIR" && docker compose start scheduler
  ' _ "${status_file}" > "${log_file}" 2>&1 < /dev/null &
  echo "Initialer Daten-Load im Hintergrund gestartet. Log: ${log_file}"
  echo "Status: bash ${repo_root}/backend/scripts/deploy_remote.sh initial-load-status"
}

cmd_initial_load_status() {
  echo "Status: $(cat "${state_dir}/initial_load.status" 2> /dev/null || echo 'kein initialer Load gestartet')"
  if [[ -f "${HOME}/initial_load.log" ]]; then
    echo "--- letzte Zeilen von ~/initial_load.log ---"
    tail -n 15 "${HOME}/initial_load.log"
  fi
}

cmd_apply() {
  local sha="${1:?git-sha fehlt}" initial_load="${2:-false}"
  local prev_sha=""
  [[ -f "${state_dir}/current_sha" ]] && prev_sha="$(cat "${state_dir}/current_sha")"

  install_config

  # Pull images first: if the registry fails, the deploy aborts before anything on the running stack was changed.
  compose pull

  if [[ -f "${deploy_dir}/.env.nextcloud-sync" ]]; then
    bash "${repo_root}/backend/scripts/sync_nextcloud_data_exchange.sh"
  else
    echo "Hinweis: .env.nextcloud-sync fehlt, Nextcloud-Sync wird übersprungen."
  fi

  # --wait: waits for the healthchecks (backend, postgis, pipeline) and fails if a container does not become healthy
  compose up -d --remove-orphans --wait --wait-timeout "${WAIT_TIMEOUT:-600}"

  # The pipeline container installs its Python packages on start; code changes
  # are visible immediately via the bind mount, only new requirements need a restart.
  if [[ -n "${prev_sha}" ]] && git -C "${repo_root}" diff --name-only "${prev_sha}" "${sha}" 2>/dev/null \
      | grep -Eq '^pipeline/dlt/requirements\.txt$'; then
    echo "requirements.txt geändert -> pipeline-Container wird neu gestartet"
    compose up -d --force-recreate --no-deps --wait --wait-timeout "${WAIT_TIMEOUT:-600}" pipeline
  fi

  if [[ "${initial_load}" == "true" ]]; then
    wait_for_pipeline_healthy
    start_initial_load
  fi

  echo "${sha}" > "${state_dir}/current_sha"
  compose ps
}

cmd_mark_good() {
  echo "${1:?git-sha fehlt}" > "${state_dir}/last_good_sha"
  echo "Letzter guter Stand: $1"
}

cmd_rollback() {
  test -f "${state_dir}/last_good_sha" || { echo "Kein letzter guter Stand gespeichert - Rollback nicht möglich." >&2; exit 1; }
  local sha tag
  sha="$(cat "${state_dir}/last_good_sha")"
  tag="sha-${sha:0:7}"
  echo "Rollback auf ${sha} (Image-Tag ${tag})"

  GIT_LFS_SKIP_SMUDGE=1 git -C "${repo_root}" checkout --quiet --detach "${sha}"
  sed -i "s/^IMAGE_TAG=.*/IMAGE_TAG=${tag}/" "${deploy_dir}/.env"

  compose pull
  compose up -d --remove-orphans --wait --wait-timeout "${WAIT_TIMEOUT:-600}"
  echo "${sha}" > "${state_dir}/current_sha"
  compose ps
}

case "${1:-}" in
  apply) shift; cmd_apply "$@" ;;
  mark-good) shift; cmd_mark_good "$@" ;;
  rollback) cmd_rollback ;;
  initial-load-status) cmd_initial_load_status ;;
  *) echo "Usage: $0 {apply <sha> [initial_load]|mark-good <sha>|rollback|initial-load-status}" >&2; exit 2 ;;
esac
