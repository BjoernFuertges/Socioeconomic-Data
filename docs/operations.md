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

**Docker volumes:** the GitLab runner creates temporary volumes for cache
and build directories on every CI job. These are **not deleted
automatically** after the job finishes and can take up several GB.

Identify and remove unused runner volumes:

```bash
# List and inspect volumes
docker volume inspect $(docker volume ls -q)

# Remove unused runner volumes (do NOT remove pgdata!)
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

## CI deployment to the VM – prerequisites

The deploy job runs on the existing GitLab Docker runner and connects via
SSH to the VM (`/root/soziooekonomische-daten`). Details: see
[Backend deployment](backend/deployment.md).

Required GitLab CI variables: `DOCKER_HUB_USERNAME`, `DOCKER_HUB_TOKEN`,
`SSH_USER`, `SSH_HOST`, `SSH_PRIVATE_KEY`, `SSH_KNOWN_HOSTS`.

In addition, `backend/deployments/.env.nextcloud-sync` (copied from
`.env.nextcloud-sync.example`) must exist on the VM with valid `rclone`
values so that `backend/scripts/sync_nextcloud_data_exchange.sh` works.
