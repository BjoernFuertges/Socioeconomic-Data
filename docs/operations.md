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

**Adminer is not publicly reachable.** It is a Compose profile (`tools`) that
only listens on `127.0.0.1:8083` of the server and is started on demand:

```bash
ssh -L 8083:localhost:8083 deploy@<SSH_HOST>
# on the server:
cd /opt/soziooekonomische-daten/backend/deployments && docker compose up -d adminer
# browser on your machine: http://localhost:8083  (System PostgreSQL, Server: postgis)
docker compose stop adminer            # when done
```

## Database backup and restore

Only data the pipeline cannot rebuild is backed up: the `snapshots` schema
(dbt snapshot history) and `loaded_data.dataset_metadata`. Everything else is
recomputed from the sources by `run_all_pipelines.sh`.

- `backend/scripts/backup_db.sh` runs nightly (02:17 server time) from the
  `deploy` user's crontab, set up by the deploy job. It dumps both parts with
  `pg_dump`, verifies the archive, uploads it to the Hetzner Object Storage
  bucket (`soziodata_<UTC timestamp>.dump`) and deletes dumps older than
  30 days (`BACKUP_RETENTION_DAYS`). Log: `~/backup.log`. It needs the
  variables/secrets `BACKUP_S3_BUCKET`, `BACKUP_S3_REGION`,
  `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY`; without them no backup is
  configured.
- Run a backup right now: `bash backend/scripts/backup_db.sh` on the server.
- Backups are listed in the Hetzner Console (Object Storage → bucket →
  folder `db/`).

**Restore (fresh database only, e.g. new server).** Restore *before* the
first pipeline run, otherwise dbt creates empty snapshot tables and the
history is lost; the script refuses to run if `snapshots` already contains
tables.

```bash
cd /opt/soziooekonomische-daten/backend/deployments
docker compose stop scheduler          # keep the hourly pipeline from running first
cd ../..
bash backend/scripts/restore_db.sh                 # latest backup, snapshots only
# bash backend/scripts/restore_db.sh soziodata_<timestamp>.dump   # a specific backup
# RESTORE_METADATA=1 bash backend/scripts/restore_db.sh           # also dataset_metadata
docker exec pipeline bash /app/dlt/run_all_pipelines.sh           # dbt continues the history
cd backend/deployments && docker compose start scheduler
```

On a fresh server, deploy first *without* `initial_load`, restore, then run
the pipeline as shown (or re-run the workflow with `initial_load`). The
metadata restore is optional because the pipeline rewrites
`dataset_metadata` from the `meta.yaml` files on every run.

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
`bash backend/scripts/deploy_remote.sh rollback` on the server.
