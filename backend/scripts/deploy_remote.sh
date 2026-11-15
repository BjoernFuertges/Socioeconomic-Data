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

# Nightly backup (02:17 server time) via the deploy user's cron; idempotent via the markers.
# ~/backup.log is rotated by a user-level logrotate run (the deploy user cannot write
# /etc/logrotate.d): weekly, 8 compressed generations, own state file.
install_backup_cron() {
  local marker="# soziodata-backup" lr_marker="# soziodata-logrotate"
  local lr_conf="${HOME}/.config/soziodata-logrotate.conf" lr_state="${HOME}/.local/state/soziodata-logrotate.status"
  local lr_bin lr_line=""
  if ! command -v crontab > /dev/null 2>&1; then
    echo "Warnung: crontab nicht installiert - Backup-Cron nicht eingerichtet." >&2
    return 0
  fi
  # cron's PATH usually lacks /usr/sbin, so resolve the absolute path now
  lr_bin="$(PATH="${PATH}:/usr/sbin:/sbin" command -v logrotate || true)"
  if [[ -n "${lr_bin}" ]]; then
    install -m 700 -d "${HOME}/.config" "$(dirname "${lr_state}")"
    printf '%s\n' \
      "${HOME}/backup.log {" \
      "  weekly" \
      "  rotate 8" \
      "  compress" \
      "  delaycompress" \
      "  missingok" \
      "  notifempty" \
      "  copytruncate" \
      "}" > "${lr_conf}"
    lr_line="5 3 * * * '${lr_bin}' -s '${lr_state}' '${lr_conf}' ${lr_marker}"
  else
    echo "Warnung: logrotate nicht installiert - ~/backup.log wird nicht rotiert." >&2
  fi
  {
    crontab -l 2> /dev/null | grep -vF -e "${marker}" -e "${lr_marker}" || true
    echo "17 2 * * * bash '${repo_root}/backend/scripts/backup_db.sh' >> \"\$HOME/backup.log\" 2>&1 ${marker}"
    if [[ -n "${lr_line}" ]]; then echo "${lr_line}"; fi
  } | crontab -
  echo "Backup-Cron eingerichtet (täglich 02:17, Log: ~/backup.log${lr_bin:+, wöchentlich rotiert})"
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

# Creates/updates the backend's read-only login role api_ro (idempotent). Needs the running
# postgis container; the password comes from .env (API_DB_PASSWORD) and is passed via the
# environment (docker exec -e NAME without value), so it never shows up in a process list.
ensure_api_role() {
  local api_pw db_user db_name
  api_pw="$(sed -n 's/^API_DB_PASSWORD=//p' "${deploy_dir}/.env" | tail -n 1)"
  [[ -n "${api_pw}" ]] || { echo "API_DB_PASSWORD fehlt in ${deploy_dir}/.env" >&2; return 1; }
  db_user="$(compose exec -T postgis printenv POSTGRES_USER | tr -d '\r')"
  db_name="$(compose exec -T postgis printenv POSTGRES_DB | tr -d '\r')"
  echo "Stelle DB-Rolle api_ro sicher ..."
  API_DB_PASSWORD="${api_pw}" docker exec -i -e API_DB_PASSWORD postgis \
    psql -q -v ON_ERROR_STOP=1 -U "${db_user}" -d "${db_name}" \
    < "${repo_root}/backend/scripts/ensure_api_role.sql"
}

# True while the background initial load (start_initial_load) is active: the status file says
# "running" and a host process of its wrapper (bash -c '... docker exec pipeline bash
# /app/dlt/run_all_pipelines.sh ...') or of that docker exec still exists. Without pgrep only the
# status file counts. A stale "running" (wrapper killed, e.g. by a reboot) thus does not keep the
# scheduler stopped forever.
initial_load_active() {
  [[ "$(cat "${state_dir}/initial_load.status" 2> /dev/null)" == running* ]] || return 1
  command -v pgrep > /dev/null 2>&1 || return 0
  pgrep -f 'docker exec pipeline bash /app/dlt/run_all_pipelines\.sh' > /dev/null
}

# Stops the scheduler and waits for a running run_all_pipelines.sh (hourly ofelia job, initial
# load or a manual docker exec) to finish, so the Nextcloud sync, a postgis recreate or
# ensure_pipeline_role (owners/password) do not happen under a running dlt/dbt. No-op without a
# running pipeline container (fresh server). compose_up_stack starts the scheduler again.
quiesce_pipeline() {
  local timeout="${PIPELINE_WAIT_TIMEOUT:-3600}" waited=0 running
  compose stop scheduler || true
  running="$(docker inspect -f '{{.State.Running}}' pipeline 2> /dev/null || echo false)"
  [[ "${running}" == "true" ]] || return 0
  # docker top lists the container's processes from the host (python:slim has no pgrep)
  # grep without -q reads all input: an early exit could SIGPIPE docker top (pipefail -> false)
  while docker top pipeline 2> /dev/null | grep 'run_all_pipelines\.sh' > /dev/null; do
    if (( waited >= timeout )); then
      echo "Abbruch: run_all_pipelines.sh läuft seit über ${timeout}s weiter (initialer Load?)." >&2
      echo "Später erneut deployen oder den Lauf prüfen: docker top pipeline / deploy_remote.sh initial-load-status" >&2
      # Leave the stack as it was: restart the scheduler unless the initial-load wrapper restarts it itself
      if ! initial_load_active; then
        compose start scheduler || true
      fi
      return 1
    fi
    echo "Warte auf laufenden Pipeline-Lauf (${waited}s/${timeout}s) ..."
    sleep 30
    waited=$((waited + 30))
  done
}

# Creates/updates the pipeline's non-superuser write role pipeline_rw (dlt + dbt) and hands it
# ownership of the pipeline schemas and everything in them (idempotent, after ensure_api_role:
# it also sets api_ro's default privileges for objects pipeline_rw creates).
ensure_pipeline_role() {
  local pipeline_pw db_user db_name
  pipeline_pw="$(sed -n 's/^PIPELINE_DB_PASSWORD=//p' "${deploy_dir}/.env" | tail -n 1)"
  [[ -n "${pipeline_pw}" ]] || { echo "PIPELINE_DB_PASSWORD fehlt in ${deploy_dir}/.env" >&2; return 1; }
  db_user="$(compose exec -T postgis printenv POSTGRES_USER | tr -d '\r')"
  db_name="$(compose exec -T postgis printenv POSTGRES_DB | tr -d '\r')"
  echo "Stelle DB-Rolle pipeline_rw sicher ..."
  PIPELINE_DB_PASSWORD="${pipeline_pw}" docker exec -i -e PIPELINE_DB_PASSWORD postgis \
    psql -q -v ON_ERROR_STOP=1 -U "${db_user}" -d "${db_name}" \
    < "${repo_root}/backend/scripts/ensure_pipeline_role.sql"
}

# dbt reads its connection from the gitignored profiles.yml next to dbt_project.yml (bind mount,
# DBT_PROFILES_DIR). Only a profile that takes user/password from DB_USER/DB_PASSWORD uses pipeline_rw.
# Default: warn only (the deploy goes on, dbt keeps whatever credentials the profile has).
# STRICT_DBT_PROFILE=1 (repo variable, passed through by main.yml) turns the warning into an
# abort; cmd_apply runs this check before anything on the stack is touched.
check_dbt_profile() {
  local profile="${repo_root}/pipeline/dbt/socioeconomic_data_pipeline/profiles.yml" problem=""
  if [[ ! -f "${profile}" ]]; then
    problem="${profile} fehlt - dbt im pipeline-Container schlägt fehl (Vorlage: profiles.example.yml)."
  elif ! grep -q "env_var('DB_USER')" "${profile}" || ! grep -q "env_var('DB_PASSWORD')" "${profile}"; then
    problem="${profile} liest user/password nicht aus DB_USER/DB_PASSWORD - dbt läuft dann NICHT als pipeline_rw (siehe profiles.example.yml)."
  fi
  [[ -n "${problem}" ]] || return 0
  if [[ "${STRICT_DBT_PROFILE:-0}" == "1" ]]; then
    echo "Abbruch (STRICT_DBT_PROFILE=1): ${problem}" >&2
    return 1
  fi
  echo "Warnung: ${problem}" >&2
}

# EXIT trap of apply/rollback after quiesce_pipeline: if the command fails later on, do not leave the
# hourly job stopped (unless the initial load is active; its wrapper restarts the scheduler).
restart_scheduler_on_failure() {
  local rc=$?
  if (( rc != 0 )) && ! initial_load_active; then
    echo "Abgebrochen (exit ${rc}) - starte scheduler wieder." >&2
    compose start scheduler || true
  fi
}

# Brings the whole stack up (--wait: healthchecks; fails if a container does not become healthy).
# While the initial load is active its wrapper restarts the scheduler at the end, so it is left out
# here instead of running the hourly job in parallel (normally quiesce_pipeline has already waited
# for the load; this covers the moment between its end and the wrapper's restart).
compose_up_stack() {
  if initial_load_active; then
    local services
    services="$(compose config --services | grep -vx scheduler | tr '\n' ' ')"
    echo "Initialer Load aktiv - scheduler wird nicht gestartet (das übernimmt der Load-Wrapper)."
    # shellcheck disable=SC2086 # word splitting of the service list is intended
    compose up -d --remove-orphans --wait --wait-timeout "${WAIT_TIMEOUT:-600}" ${services}
  else
    compose up -d --remove-orphans --wait --wait-timeout "${WAIT_TIMEOUT:-600}"
  fi
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
  check_dbt_profile

  # No pipeline run may be active from here on: the Nextcloud sync rewrites data-exchange/ (read by
  # dlt), "compose up postgis" may recreate the database container, and ensure_pipeline_role takes
  # over table ownership. Stops the scheduler, waits for a running run (PIPELINE_WAIT_TIMEOUT) and
  # aborts the deploy on timeout (scheduler restarted unless the initial load is active).
  quiesce_pipeline
  # If the deploy aborts from here on, do not leave the hourly job stopped (covers e.g. a failing
  # registry or sync with an otherwise unchanged stack; the rollback step would also start it).
  trap restart_scheduler_on_failure EXIT

  # Pull images before touching containers: if the registry fails, the deploy aborts with the running stack unchanged.
  compose pull

  if [[ -f "${deploy_dir}/.env.nextcloud-sync" ]]; then
    bash "${repo_root}/backend/scripts/sync_nextcloud_data_exchange.sh"
  else
    echo "Hinweis: .env.nextcloud-sync fehlt, Nextcloud-Sync wird übersprungen."
  fi

  # The backend logs in as api_ro, the pipeline as pipeline_rw; both must exist before the
  # containers start: bring up postgis first, then create/update the roles (idempotent).
  compose up -d --wait --wait-timeout "${WAIT_TIMEOUT:-600}" postgis
  ensure_api_role
  ensure_pipeline_role

  # Starts the scheduler again; waits for the healthchecks (backend, postgis, pipeline)
  compose_up_stack

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
  trap - EXIT
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

  # Do not swap code/bind mounts under a running dlt/dbt (e.g. the initial load): stop the scheduler
  # and wait; on timeout the rollback aborts (scheduler restarted unless the initial load is active).
  quiesce_pipeline
  trap restart_scheduler_on_failure EXIT

  GIT_LFS_SKIP_SMUDGE=1 git -C "${repo_root}" checkout --quiet --detach "${sha}"
  sed -i "s/^IMAGE_TAG=.*/IMAGE_TAG=${tag}/" "${deploy_dir}/.env"

  compose pull
  # Leaves the scheduler stopped while the initial load is active (its wrapper restarts it)
  compose_up_stack
  echo "${sha}" > "${state_dir}/current_sha"
  trap - EXIT
  compose ps
}

case "${1:-}" in
  apply) shift; cmd_apply "$@" ;;
  mark-good) shift; cmd_mark_good "$@" ;;
  rollback) cmd_rollback ;;
  initial-load-status) cmd_initial_load_status ;;
  *) echo "Usage: $0 {apply <sha> [initial_load]|mark-good <sha>|rollback|initial-load-status}" >&2; exit 2 ;;
esac
