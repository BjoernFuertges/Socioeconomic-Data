# Deployment

In production, the stack runs via Docker Compose on a Hetzner Cloud server
(`backend/deployments/docker-compose.yml`): Traefik (TLS/routing), PostGIS,
a backend container, and a `pipeline` container for dlt/dbt.

The Postgres password is not stored in the compose file: `docker compose`
reads `DB_PASSWORD`, `APP_HOST`, `LE_EMAIL`, `GHCR_OWNER` and `IMAGE_TAG` from an untracked `backend/deployments/.env` on the server (written by the deploy job)
(template: `backend/deployments/.env.example`) and refuses to start without it.

## CI/CD (GitHub Actions)

The workflow `.github/workflows/main.yml` runs on every push/PR to `main`; the
build and deploy stages only run on push to `main` (or manually via
`workflow_dispatch`):

1. **Tests** – Go build/tests plus the Swagger router-annotation check
   (`backend/scripts/check_swagger_router_annotations.sh`), frontend
   type-check/build, `mkdocs build --strict`, a syntax check of
   `pipeline/dlt/`, and `docker compose config`.
2. **Semantic Release dry run** – determines the next version from the
   Conventional Commit messages.
3. **Image builds** – backend, frontend and docs images are pushed to GHCR
   (`ghcr.io/<owner>/soziodata-<component>`) with the tags `sha-<commit>`,
   `latest` and, if a release is due, the version.
4. **Semantic Release** – tag, `CHANGELOG.md` and GitHub release.
5. **Deploy to Hetzner** – over SSH: check out the built commit on the
   server, write `.env`/`.env.nextcloud-sync` from GitHub secrets,
   `docker compose pull` (first, so a registry failure changes nothing),
   run the Nextcloud sync, then `docker compose up -d --wait` (all in
   `backend/scripts/deploy_remote.sh`). `--wait` uses the healthchecks of
   `postgis`, `backend` and `pipeline`; if the rollout or the subsequent
   health request fails, the job rolls the server back to the last state
   that passed E2E. Compose only recreates containers whose
   image changed; the `pipeline` container is restarted when
   `pipeline/dlt/requirements.txt` changed. The manual workflow input
   `initial_load` additionally starts `run_all_pipelines.sh` once *in the
   background on the server* (fresh server, rebuild of all data): the job does
   not wait for it, so it uses no Actions minutes, the hourly scheduler is
   paused until the load has finished, and the E2E data check is skipped.
   Progress: `bash backend/scripts/deploy_remote.sh initial-load-status` on the
   server (log: `~/initial_load.log`).
6. **E2E** – Playwright smoke test (`frontend/web/app/e2e`) against the
   deployed instance. On failure the server is rolled back to the last state
   that passed E2E.

The Hetzner server itself is provisioned with Terraform (`infra/`, workflow
`infra.yml`); the repository's `infra/README.md` lists the one-time setup
(state bucket, secrets, DNS) and all required GitHub secrets and variables.

## Manually deploying a new backend image

```bash
cd backend/deployments

# Pull the new backend image
docker compose pull backend

# Recreate/restart only the backend container with the new image
docker compose up -d --no-deps backend

# Check status and logs
docker compose ps backend
docker compose logs --tail=80 backend
```

Alternative, in one step:

```bash
docker compose up -d --pull always --no-deps backend
```

## Manually running a changed pipeline on the server

Since `pipeline/dlt/` and `pipeline/dbt/` are mounted as volumes into the `pipeline`
container, restarting the container after a `git pull` on the server is
enough for changes to take effect:

```bash
cd /opt/soziooekonomische-daten
git pull --ff-only
cd backend/deployments
docker compose up -d --no-deps pipeline
```

Manually run all dlt pipelines + dbt inside the running container:

```bash
docker exec pipeline bash /app/dlt/run_all_pipelines.sh
docker compose logs --tail=80 pipeline
```

## Traefik & TLS (initial setup)

```bash
mkdir -p letsencrypt && touch letsencrypt/acme.json && chmod 600 letsencrypt/acme.json
```
