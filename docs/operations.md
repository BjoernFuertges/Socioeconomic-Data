# Operations

## Disk space management

### Diagnostics

```bash
# Filesystem overview
df -h

# Find the largest directories
du -sh /* 2>/dev/null | sort -rh | head -20

# Docker disk usage
docker system df
docker volume ls
du -sh /var/lib/docker/volumes/* | sort -rh
```

### Known disk hogs

**Docker images and volumes:** every deployment pulls new images
(`sha-<commit>` tags); old image versions stay on the server and can take up
several GB. Unused volumes can pile up as well.

Identify and remove unused volumes:

```bash
# List and inspect volumes
docker volume inspect $(docker volume ls -q)

# Remove unused volumes (do NOT remove pgdata!)
docker volume rm <volume-name>

# Alternative: remove all unused volumes at once
docker volume prune
```

!!! warning
    The `deployments_pgdata` volume contains the PostgreSQL database data
    and must **not** be deleted.

**Clean up unused Docker images:**

```bash
docker system prune -a --volumes
```

## Server hardening and Adminer

The server is hardened by cloud-init on first boot (`infra/hetzner/cloud-init.yaml.tftpl`,
applies to newly created servers only):

- SSH: key login for the `deploy` user only (`PermitRootLogin no`,
  `PasswordAuthentication no`, `AllowUsers deploy`, `MaxAuthTries 3`); the drop-in is
  named `00-hardening.conf` because sshd uses the first value it finds per option.
- `fail2ban` for SSH (5 failures in 10 min → 1 h ban), automatic security updates
  (`unattended-upgrades`, no automatic reboot), Hetzner firewall allowing only 22/80/443.
- Emergency access if SSH is locked out (per Hetzner docs): in the Hetzner Console open the server →
  *Rescue* → *Reset Root Password*, then log in as root on the *Console* (VNC); password login on the console
  works even though SSH password login is disabled. Alternatively boot the *Rescue* system (SSH as root, valid
  for 60 minutes) or simply rebuild the server (`terraform taint hcloud_server.main`; the database volume is kept).
  `infra.yml` refuses to apply if `SSH_PRIVATE_KEY` and `DEPLOY_SSH_PUBLIC_KEY` do not match.

The `deploy` user is in the `docker` group, which is effectively root on the
server: treat the `SSH_PRIVATE_KEY` secret accordingly.

Traefik and the ofelia scheduler do not mount the Docker socket. Each talks to its own
`tecnativa/docker-socket-proxy` on an internal network (`traefik-socket`, `scheduler-socket`):
Traefik's proxy only allows reading containers (`CONTAINERS=1`, no `POST`), ofelia's additionally
`POST` + `EXEC` for its `job-exec` (that still allows creating containers, so it narrows rather
than removes ofelia's privileges). Traefik sends HSTS (`Strict-Transport-Security: max-age=31536000`,
no `includeSubDomains`/`preload`) on all routers via the `hsts` middleware.

**Adminer is not publicly reachable.** It is a Compose profile (`tools`) that
only listens on `127.0.0.1:8083` of the server and is started on demand:

```bash
ssh -L 8083:localhost:8083 deploy@<SSH_HOST>
# on the server:
cd /opt/soziooekonomische-daten/backend/deployments && docker compose up -d adminer
# browser on your machine: http://localhost:8083  (System PostgreSQL, Server: postgis)
docker compose stop adminer            # when done
```

## DB roles

Only `postgis` (init, healthcheck), Adminer and the role/backup scripts use the superuser
`sozio_data_compose_user` (`DB_PASSWORD`). The application containers log in as two
non-superuser roles, both created/updated by `deploy_remote.sh apply` after `postgis` is up
and before the other containers start (idempotent SQL files in `backend/scripts/`):

- **`api_ro`** (backend; `DB_USER` of the `backend` service, password `API_DB_PASSWORD`,
  `ensure_api_role.sql`): may only `SELECT` in schema `transformed_data` (plus PostGIS in
  `public`), has `default_transaction_read_only=on`, a 60 s statement timeout and a limit of
  40 connections; `loaded_data` and `snapshots` are not readable. dbt re-grants `SELECT` after
  every run (`on-run-end` hook `grant_api_read`, no-op if the role is missing).
- **`pipeline_rw`** (dlt + dbt in the `pipeline` container, incl. the hourly ofelia job;
  `DB_USER`/`DESTINATION__POSTGRES__CREDENTIALS__USERNAME` of the `pipeline` service, password
  `PIPELINE_DB_PASSWORD`, `ensure_pipeline_role.sql`): owns the schemas `loaded_data`,
  `loaded_data_staging` (dlt staging for the `dataset_metadata` merge), `transformed_data`,
  `snapshots` and everything in them (dbt/dlt need ownership for `DROP`/`ALTER`/`RENAME`).
  Database privileges are only `CONNECT` + `TEMPORARY` (dbt snapshots use temp tables) - no
  `CREATE`, no superuser/`CREATEDB`/`CREATEROLE`, so no `COPY ... PROGRAM`, server file access
  or new extensions. Role settings: `statement_timeout=2h` (long spatial joins),
  `idle_in_transaction_session_timeout=30min`, `lock_timeout=5min` (a dbt table swap gives up
  instead of blocking the API for long), `CONNECTION LIMIT 10`. The script also creates the
  extensions `postgis`/`pg_trgm` (so `lau_integrated`'s `create extension if not exists pg_trgm`
  is a no-op), hands every existing relation in those schemas to `pipeline_rw`, and sets
  `ALTER DEFAULT PRIVILEGES FOR ROLE pipeline_rw ... GRANT SELECT ON TABLES TO api_ro`.
  **A new pipeline schema must be added to `ensure_pipeline_role.sql`** (`pipeline_rw` cannot
  create schemas). Right after installing the configuration, before the Nextcloud sync,
  `compose pull` and starting `postgis`, `deploy_remote.sh apply` stops the scheduler and waits for a
  running `run_all_pipelines.sh` (max. `PIPELINE_WAIT_TIMEOUT`, default 60 min, then the deploy
  aborts - e.g. during a long initial load; deploy again later). If the deploy fails after that
  point, the scheduler is started again; `deploy_remote.sh rollback` waits the same way. While an
  initial load is active, neither starts the scheduler (the load's wrapper does when it ends).

dbt takes its credentials from `profiles.yml` next to `dbt_project.yml`
(`pipeline/dbt/socioeconomic_data_pipeline/profiles.yml`, gitignored, bind-mounted into the
container via `DBT_PROFILES_DIR`). It only uses `pipeline_rw` if it reads
`user`/`password` from `DB_USER`/`DB_PASSWORD` like `profiles.example.yml`; check this on the
server before the first deploy with the role (`deploy_remote.sh apply` warns otherwise; with the
repo variable `STRICT_DBT_PROFILE=1` it aborts before touching the stack instead). dlt
reads the `DESTINATION__POSTGRES__CREDENTIALS__*` env vars, which take precedence over
`.dlt/secrets.toml`.

**Manual pipeline runs** on the server must go through the container
(`docker exec pipeline bash /app/dlt/run_all_pipelines.sh`), i.e. as `pipeline_rw` - not as
the superuser (e.g. dbt with the owner credentials): objects it creates would belong to the
superuser, `pipeline_rw` could no longer drop/replace them, and later runs fail until the next
`deploy_remote.sh apply` (or `ensure_pipeline_role.sql`) hands them back. On a new server
(e.g. after `restore_db.sh`), run `deploy_remote.sh apply` (or both SQL files, API role first)
once before the pipeline.

Check (as superuser, `docker exec -it postgis psql -U sozio_data_compose_user -d soziooekonomische-daten`):

```sql
SELECT rolname, rolcanlogin, rolsuper, rolconnlimit, rolconfig FROM pg_roles WHERE rolname IN ('api_ro', 'pipeline_rw');
SELECT has_schema_privilege('api_ro', 'transformed_data', 'USAGE'),
       has_schema_privilege('api_ro', 'loaded_data', 'USAGE'),
       has_schema_privilege('api_ro', 'snapshots', 'USAGE');   -- t, f, f
-- owners: only pipeline_rw expected
SELECT nspname, pg_get_userbyid(nspowner) FROM pg_namespace
 WHERE nspname IN ('loaded_data', 'loaded_data_staging', 'transformed_data', 'snapshots');
SELECT n.nspname, pg_get_userbyid(c.relowner) AS owner, count(*)
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE n.nspname IN ('loaded_data', 'loaded_data_staging', 'transformed_data', 'snapshots')
   AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
 GROUP BY 1, 2 ORDER BY 1, 2;
```

Manual rollback, backend back to the superuser: set `DB_USER: sozio_data_compose_user` and
`DB_PASSWORD: ${DB_PASSWORD:?}` in the `backend` service, redeploy, then optionally drop the
role: `REVOKE ALL ON SCHEMA transformed_data FROM api_ro; REVOKE ALL ON ALL TABLES IN SCHEMA transformed_data FROM api_ro; ALTER DEFAULT PRIVILEGES FOR ROLE sozio_data_compose_user IN SCHEMA transformed_data REVOKE SELECT ON TABLES FROM api_ro; ALTER DEFAULT PRIVILEGES FOR ROLE pipeline_rw IN SCHEMA transformed_data REVOKE SELECT ON TABLES FROM api_ro; REVOKE CONNECT ON DATABASE "soziooekonomische-daten" FROM api_ro; DROP ROLE api_ro;`

Manual rollback, pipeline back to the superuser: set `DB_USER`/`DB_PASSWORD` and
`DESTINATION__POSTGRES__CREDENTIALS__USERNAME`/`PASSWORD` of the `pipeline` service back to
`sozio_data_compose_user`/`${DB_PASSWORD:?}` and redeploy. The superuser can replace
`pipeline_rw`-owned objects, and the old `FOR ROLE sozio_data_compose_user` default privileges
for `api_ro` are still in place, so nothing else is needed (a `deploy_remote.sh rollback` to a
commit before the role does exactly this). To drop the role afterwards, first remove
`ensure_pipeline_role` from `deploy_remote.sh`, then: `REASSIGN OWNED BY pipeline_rw TO sozio_data_compose_user; DROP OWNED BY pipeline_rw; DROP ROLE pipeline_rw;`

Adminer can log in as `api_ro` as well (System PostgreSQL, Server `postgis`) for read-only browsing.

## Rate limits and memory limits

The API is public on a 1 vCPU / 2 GB server, so Traefik protects it via labels
in `backend/deployments/docker-compose.yml` (values and reasons are commented
there):

- Three API routers: `backend` (normal requests, per IP 10 req/s, burst 50,
  16 concurrent), `backend-heavy` (geometries, `/geo`, `/regions/.../data`,
  `/metrics`, `/datasets/{slug}/source`, CSV/`download`, unfiltered
  `limit=null`: per IP 20/min, burst 15, 4 concurrent, plus a global cap of 5
  concurrent) and `backend-post` (POST, per IP like heavy, global cap 2, bodies
  over 6 MiB rejected with 413). Frontend and docs have a generous per-IP limit.
  Exceeding a limit returns **429**.
- Entrypoint timeouts: read 30 s, idle 90 s, write 150 s (above the backend's
  120 s `WriteTimeout`).
- Clients behind one NAT address (e.g. a university network, the CI E2E runner)
  share the per-IP limits.
- IPv6: if Docker does not pass IPv6 through to the containers, Traefik sees the
  gateway address and all IPv6 clients share one per-IP bucket. Before setting
  an AAAA record, check with `curl -6 https://<APP_HOST>/api/v1/health` that the
  access log shows the real client address.
- Known limits of the protection: the "heavy" classification is based on path
  and query parameters and can be bypassed with a meaningless filter (e.g.
  `limit=null&year=x` on an endpoint that ignores `year`); then only the normal
  per-IP limits, the `limit=null` row cap, the 60 s `statement_timeout` and the
  `api_ro` connection limit apply. Request bodies must be fully received within
  the 30 s read timeout (a 6 MiB GeoJSON needs about 1.6 Mbit/s).
- Diagnose: `docker logs traefik` (access log, rotated) shows status and router
  name per request, e.g. `docker logs traefik 2>&1 | grep '" 429 '`.
- `mem_limit`: `postgis` 1 GB, `pipeline` 1 GB, `backend` 512 MB (caps against one
  runaway container, plus swap). An OOM-killed container is restarted
  (`docker inspect <name> --format '{{.State.OOMKilled}}'`).
- The hourly ofelia pipeline job has `no-overlap`, so a run that takes longer
  than an hour is not started twice. In addition `run_all_pipelines.sh` takes a
  `flock` (`PIPELINE_LOCK_FILE`, default `/tmp/soziodata_run_all_pipelines.lock`
  in the container), so the hourly job, the initial load and manual
  `docker exec` runs never overlap: a second run exits immediately with code 75
  (`PIPELINE_LOCK_BUSY_EXIT`; the ofelia job sets it to 0, so a skipped hourly
  run is logged but not counted as failed).

To adjust or roll back, edit the labels/limits in the Compose file and run
`docker compose up -d` in `backend/deployments` (Traefik picks up label
changes when the container is recreated).

## Database backup and restore

Only data the pipeline cannot rebuild is backed up: the `snapshots` schema
(dbt snapshot history) and `loaded_data.dataset_metadata`. Everything else is
recomputed from the sources by `run_all_pipelines.sh`.

- `backend/scripts/backup_db.sh` runs nightly (02:17 server time) from the
  `deploy` user's crontab, set up by the deploy job. It dumps both parts with
  `pg_dump`, verifies the archive, uploads it to the Hetzner Object Storage
  bucket (`soziodata_<UTC timestamp>.dump`) and deletes dumps older than
  30 days (`BACKUP_RETENTION_DAYS`; skipped if the new dump has no `snapshots` tables). It exits non-zero if the database is unreachable and pings `HEALTHCHECK_URL` on success, if set (GitHub secret or variable `BACKUP_HEALTHCHECK_URL`; the deploy rewrites `~/.config/soziodata-backup.env`, so do not add it there by hand). Log: `~/backup.log`, rotated weekly by a user-level `logrotate` cron entry (8 generations). It needs the
  variables/secrets `BACKUP_S3_BUCKET`, `BACKUP_S3_REGION`,
  `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY`; without them no backup is
  configured.
- Run a backup right now: `bash backend/scripts/backup_db.sh` on the server.
- Backups are listed in the Hetzner Console (Object Storage → bucket →
  folder `db/`).

**Restore (fresh database only, e.g. new server).** Restore *before* the
first pipeline run, otherwise dbt creates empty snapshot tables and the
history is lost; the script refuses to run if `snapshots` already contains
tables. It stops the `scheduler` container itself and starts it again at the
end if it was running (after a failed restore it stays stopped, so the hourly
dbt snapshot does not build on half-restored tables); it aborts while a
`run_all_pipelines.sh` is running.

```bash
cd /opt/soziooekonomische-daten
bash backend/scripts/restore_db.sh                 # latest backup, snapshots only
# bash backend/scripts/restore_db.sh soziodata_<timestamp>.dump   # a specific backup
# RESTORE_METADATA=1 bash backend/scripts/restore_db.sh           # also dataset_metadata
docker exec pipeline bash /app/dlt/run_all_pipelines.sh           # dbt continues the history
```

If the hourly job happens to start first, the manual run exits with code 75
(lock held); just wait for the hourly run instead.

On a fresh server, deploy first *without* `initial_load`, restore, then run
the pipeline as shown (or re-run the workflow with `initial_load`). The
metadata restore is optional because the pipeline rewrites
`dataset_metadata` from the `meta.yaml` files on every run. The restore
creates the tables as `pipeline_rw` if that role exists (it does after the
first deploy); otherwise run `deploy_remote.sh apply` (or
`ensure_pipeline_role.sql`) again before the pipeline, so `pipeline_rw` owns
them.

Test a restore into a scratch database occasionally; an untested backup is
not a backup.

## Git LFS

This project uses [Git Large File Storage](https://git-lfs.com/).

```bash
git lfs install
```

Add files to LFS:

```bash
git lfs track "*.psd"
git add .gitattributes
```

## CI deployment to the Hetzner server – prerequisites

The deploy job of `.github/workflows/main.yml` connects via SSH to the
server (`/opt/soziooekonomische-daten`, user `deploy`). The server is
provisioned by Terraform (`infra/`); the one-time setup, DNS record, and the
full list of required GitHub secrets and variables are in `infra/README.md`.
Details of the pipeline: see [Backend deployment](backend/deployment.md).

To make `backend/scripts/sync_nextcloud_data_exchange.sh` work, set the
variable `RCLONE_SOURCE` and the secret `RCLONE_CONF` (contents of
`rclone.conf`); the deploy job writes them to the server. Without them the
sync is skipped.

After the first deployment (or a rebuild of all data), start `main.yml`
manually with `initial_load = true`. The load then runs in the background on
the server (the workflow does not wait for it; the scheduler is stopped until
it finishes, then started again). Check progress on the server:

```bash
bash /opt/soziooekonomische-daten/backend/scripts/deploy_remote.sh initial-load-status
tail -f ~/initial_load.log
```

Without the workflow you can also run
`docker exec pipeline bash /app/dlt/run_all_pipelines.sh` directly.

Connect to the database from your machine through an SSH tunnel (port 5432
is not publicly reachable):

```bash
ssh -L 5432:localhost:5432 deploy@<SSH_HOST>
```

Manual rollback to the last state that passed E2E:
`bash backend/scripts/deploy_remote.sh rollback` on the server (waits for a
running pipeline run first, see [DB roles](#db-roles)). The workflow does this
automatically when `e2e-production` fails, times out or is cancelled.
