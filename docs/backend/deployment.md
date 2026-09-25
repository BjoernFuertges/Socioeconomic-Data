# Deployment

In production, the stack runs via Docker Compose on a VM
(`backend/deployments/docker-compose.yml`): Traefik (TLS/routing), PostGIS,
a backend container, and a `pipeline` container for dlt/dbt.

The Postgres password is not stored in the compose file: `docker compose`
reads `DB_PASSWORD` from an untracked `backend/deployments/.env` on the VM
(template: `backend/deployments/.env.example`) and refuses to start without it.

## CI/CD (GitLab CI)

On push to `main` (`.gitlab-ci.yml`):

1. **validate-backend-swagger** – checks Swagger router annotations on
   changes under `backend/**`
2. **build-backend** – builds and pushes the backend image
   (`$DOCKER_HUB_USERNAME/backend:latest` + versioned tags) to Docker Hub
3. **deploy-production** – connects via SSH to the VM and runs:
   - `git pull --ff-only origin main`
   - Nextcloud data sync (`backend/scripts/sync_nextcloud_data_exchange.sh`)
   - `docker compose pull backend`
   - Backend restart on changes under `backend/`
   - Pipeline restart on changes under `pipeline/dlt/`, `pipeline/dbt/`, `geo-mapper/`, etc.

Required GitLab CI variables: `DOCKER_HUB_USERNAME`, `DOCKER_HUB_TOKEN`,
`SSH_USER`, `SSH_HOST`, `SSH_PRIVATE_KEY`, `SSH_KNOWN_HOSTS`.

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
cd /root/soziooekonomische-daten
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
