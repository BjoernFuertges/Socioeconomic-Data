# Local Development

!!! tip "Full local stack via Docker Compose"
    To run the pipeline, database, backend, and frontend together with a
    single command instead of setting each one up individually, see
    `compose.local.yml` at the repo root (documented in the root
    [README.md](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/README.md#lokal-testen-docker-compose)).
    The rest of this page covers running the backend standalone.

## Prerequisites

- Go (see [`go.mod`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/backend/go.mod) for the version)
- A running PostgreSQL/PostGIS instance with a populated `transformed_data` schema
  (see [Data Pipeline](../pipeline/index.md))
- A `.env` file in the `backend/` directory (see `.env.example`)

## Configuration (environment variables)

From [`backend/.env.example`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/backend/.env.example):

| Variable | Default (in code) | Purpose |
|---|---|---|
| `API_HOST` | `localhost` | Host the server listens on |
| `PORT` | `8081` | Port of the API server |
| `DB_HOST` | `localhost` | Postgres host |
| `DB_PORT` | `5432` | Postgres port |
| `DB_USER` | `sozio_data_compose_user` | Postgres user (in production the read-only role `api_ro`; locally the owner may stay) |
| `DB_PASSWORD` | – | Postgres password |
| `DB_NAME` | `soziooekonomische-daten` | Database name |

Optional variables, commented out in `.env.example` and not required at startup:

| Variable | Default | Purpose |
|---|---|---|
| `UNLIMITED_ROWS_CAP` | 1.25 × largest table, at least 300000 | Row cap for `limit=null` (see [API](api.md)) |
| `TRUSTED_PROXIES` | loopback, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16` | Comma-separated IPs/CIDRs whose `X-Forwarded-For` header is trusted for the client IP (Traefik reaches the backend over the private compose network); an invalid value stops the server at startup |

`internal/config.Load()` checks at startup that every variable listed in
`.env.example` is actually set (from `.env` or the environment) and fails
otherwise.

## Running

```bash
cd backend
go run cmd/soziodata/main.go
```

The API is then available at `http://localhost:8081/api/v1`, and the
Swagger UI at `http://localhost:8081/api/docs/index.html`.

## Git hooks (recommended)

The hooks are version-controlled in `backend/githooks/`. Enable them once,
from the repository root:

```bash
git config core.hooksPath backend/githooks
```

The `pre-commit` hook automatically runs:

- Generating the Swagger documentation (`swag init`)
- Regenerating Go models when the dbt schema YAMLs have changed
  (`go generate ./internal/models/...`)
- Go tests (`go test ./...`)

| Purpose | Command |
|---|---|
| Skip the hook temporarily | `git commit --no-verify` |
| Run the hook manually | `bash backend/githooks/pre-commit` |

Details: [`backend/githooks/README.md`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/backend/githooks/README.md).

## Code generation from dbt schemas

New socioeconomic datasets are described in dbt via
`models/marts/*_schema.yml`. `cmd/gen-models` reads these schema files and
generates:

- `internal/models/models_soziodata_generated.go`
- `internal/handlers/handlers_soziodata_generated.go`

This means every new dataset automatically gets a generic REST endpoint
(see [API](api.md#generic-socioeconomic-data-routes)), without having to
hand-write handler code for each new dataset.

## Tests

```bash
cd backend
go test ./...
```

## Regenerating the Swagger docs

```bash
cd backend
swag init -g cmd/soziodata/main.go -o docs --parseDependency --parseInternal
```

Use the same flags as the pre-commit hook: the committed spec is generated
with them, and swag v1.16.4 without them renames every model definition
(large, meaningless diff). Verified with swag v1.16.4 on Go 1.26.

CI additionally checks via `backend/scripts/check_swagger_router_annotations.sh`
whether all routes carry matching Swagger annotations.

## Code documentation (godoc)

The Swagger/OpenAPI spec above documents the **public HTTP API** consumed by
clients. For browsing the **Go source code** itself (package/function-level
doc comments), use the standard Go tooling instead of a separately hosted
site:

```bash
cd backend
go doc ./internal/handlers        # package-level doc comment + exported symbols
go doc ./internal/handlers.GetLAURegions

# or browse interactively in the browser:
go install golang.org/x/pkgsite/cmd/pkgsite@latest
pkgsite -http=:8080   # then open http://localhost:8080/github.com/BjoernFuertges/Socioeconomic-Data/backend
```

!!! note "Why not a hosted godoc site?"
    A dedicated, CI-published godoc/pkgsite site was considered but is not
    set up, since it would add limited value here: almost all backend code
    lives under `internal/` (not importable outside this module anyway),
    the module isn't published to `pkg.go.dev`,
    and the actual API contract that matters to consumers is already fully
    documented via Swagger. If doc comments on exported Go symbols become
    more extensive, `pkgsite` can always be run locally as shown above
    without any additional infrastructure.
