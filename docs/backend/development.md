# Local Development

## Prerequisites

- Go (see [`go.mod`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/backend/go.mod) for the version)
- A running PostgreSQL/PostGIS instance with a populated `transformed_data` schema
  (see [Data Pipeline](../pipeline/index.md))
- A `.env` file in the `backend/` directory (see `.env.example`)

## Configuration (environment variables)

From [`backend/.env.example`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/backend/.env.example):

| Variable | Default (in code) | Purpose |
|---|---|---|
| `API_HOST` | `localhost` | Host the server listens on |
| `PORT` | `8081` | Port of the API server |
| `DB_HOST` | `localhost` | Postgres host |
| `DB_PORT` | `5432` | Postgres port |
| `DB_USER` | `sozio_data_compose_user` | Postgres user |
| `DB_PASSWORD` | – | Postgres password |
| `DB_NAME` | `soziooekonomische-daten` | Database name |

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

Details: [`backend/githooks/README.md`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/backend/githooks/README.md).

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
swag init -g cmd/soziodata/main.go -o docs
```

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
pkgsite -http=:8080   # then open http://localhost:8080/gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/backend
```

!!! note "Why not a hosted godoc site?"
    A dedicated, CI-published godoc/pkgsite site was considered but is not
    set up, since it would add limited value here: almost all backend code
    lives under `internal/` (not importable outside this module anyway),
    the module isn't published to `pkg.go.dev` (private GitLab instance),
    and the actual API contract that matters to consumers is already fully
    documented via Swagger. If doc comments on exported Go symbols become
    more extensive, `pkgsite` can always be run locally as shown above
    without any additional infrastructure.
