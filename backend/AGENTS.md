# AGENTS.md

This file provides guidance to AI coding agents and contributors working in this part of the repository.

Scope: this file covers the `backend/` directory only. See the repository
root [AGENTS.md](../AGENTS.md) for cross-component architecture, data-flow
rules (e.g. "the backend never transforms data"), and conventions shared by
all modules.

## Commands

To run the backend together with the pipeline, database, and frontend in
one go instead of setting each up by hand, use `compose.local.yml` at the
repo root (`docker compose -f compose.local.yml up --build`) — see the root
[README.md](../README.md#lokal-testen-docker-compose). The commands below
are for running the backend standalone against an already-running/external
Postgres.

```bash
# Run locally (needs backend/.env, copy from .env.example, and a reachable
# Postgres/PostGIS with transformed_data already populated by dbt)
go run cmd/soziodata/main.go

go build ./...
go test ./...   # internal/handlers (negotiation/CSV, pagination limits, DB error responses, GeoJSON validation, metrics SQL), no DB needed
# Opt-in DB tests (TestDB*: synthetic fixtures in a fresh, auto-dropped database;
# the user needs CREATE DATABASE, e.g. a throwaway postgis/postgis container):
SOZ_TEST_PG_DSN="host=127.0.0.1 port=5432 user=... password=... dbname=postgres sslmode=disable" go test ./internal/handlers/ -run DB
# (cmd/soziodata: SOZ_TEST_PG_HOST/PORT/USER/PASSWORD/DB for the DSN login test)

# Release version: internal/version (Version/BuildTime, default "dev") is set via -ldflags
# (Dockerfile build args VERSION/BUILD_TIME, filled by CI) and copied into
# docs.SwaggerInfo.Version at startup (OpenAPI info.version) and GET /health.
# The `@version dev` annotation in cmd/soziodata/main.go is only the placeholder.

# Regenerate Swagger docs (writes into docs/) - always with the same flags as
# the pre-commit hook:
swag init -g cmd/soziodata/main.go -o docs --parseDependency --parseInternal
# NOTE: the flags keep the output identical to the committed docs (definitions
# named github_com_..._internal_models.X). Without them swag v1.16.4 still
# produces a valid spec, but renames every definition to models.X (~1,800
# changed lines per file).
# Verified with swag v1.16.4 on Go 1.26. Not verified: the earlier claim that
# --parseDependency fails on Go >= 1.27 (stdlib generics). The hook has no
# fallback: if the flagged run fails, the commit stops (fix swag, or
# --no-verify and say so in the commit message).

# Regenerate models/handlers/routes from dbt schema YAMLs
go generate ./internal/models/...
```

Enable the pre-commit hook (repo root: `git config core.hooksPath backend/githooks`) to run swag + go generate + go test automatically before each commit; see [githooks/README.md](githooks/README.md).

## Database access

In production the backend connects as the read-only role `api_ro`
(`DB_USER` in `deployments/docker-compose.yml`, password `API_DB_PASSWORD`;
created by `scripts/ensure_api_role.sql` in `deploy_remote.sh apply`): `SELECT`
on `transformed_data` only, sessions read-only. Any new query must only touch
`transformed_data` (or `public`/PostGIS and catalogs); no writes, no temp
tables. Locally `DB_USER` may remain the owner.

## Architecture

- `cmd/soziodata/main.go` is the actual API entrypoint (Gin router, loads
  config, mounts routes).
- `cmd/gen-models/main.go` is a code generator, not a runtime service. It
  reads `pipeline/dbt/socioeconomic_data_pipeline/models/staging/*_schema.yml` and
  emits `internal/models/*_generated.go`,
  `internal/handlers/*_generated.go`, and
  `internal/routes/routes_soziodata_generated.go`. **Never hand-edit a
  `*_generated.go` file** — edit the source dbt schema YAML (or
  `cmd/gen-models` itself for template changes) and rerun `go generate
  ./internal/models/...`.
- `internal/handlers/`: `kreis_generic.go` and
  `handlers_soziodata_generated.go` back the generated, uniform
  per-dataset read endpoints; `geo.go`, `lau.go`, `nuts.go`,
  `agenturbezirk.go`, `metrics.go`, `metrics_catalog.go`,
  `socioeconomic_inventory.go`, and `dataset_catalog.go` are hand-written
  cross-cutting endpoints (region metadata, geometry, dataset
  inventory/catalog). `target_versions.go` holds the supported
  `nuts_version`/`lau_version` values (read at startup from the distinct
  `target_version` of the harmonized marts, default = newest, static
  fallback without DB) and `GET /versions`; never hard-code a version
  default or list elsewhere - use `defaultNutsVersion()` /
  `supportedNutsVersions()` (LAU analogous). `negotiate.go` holds the shared response layer:
  `respondList`/`respondNegotiated` pick JSON or CSV per the `Accept`
  header (`?format=csv|json` overrides it for browser links, `?download=1`
  adds `Content-Disposition`), and a gin `render.Render` implementation
  encodes any struct slice as CSV via `jszwec/csvutil` using the models'
  existing `json` tags — never add per-endpoint CSV code, route the
  response through `respondList` instead. `region_data.go` is the combined
  region export (`GET /regions/{region_type}/{region_id}/data`): it only
  selects from the existing marts and concatenates them into one
  long-format row type (`models.RegionDataRow`), `scope=raw|aggregated|both`
  separating a region's own values from computed ones. `agenturbezirk.go` (`GET /agenturbezirke`, ...)
  reads `transformed_data.mart_agenturbezirk_sociodata` — Agenturbezirke
  are a distinct geo-level from NUTS/LAU/Kreise (Bundesagentur-für-Arbeit-
  internal district partition), so this is hand-written rather than
  going through the dbt-schema-driven codegen (`cmd/gen-models`) that
  the Kreis-level datasets use. `GET /agenturbezirke/{ba_schl}/dataset-
  aggregate/{slug}` aggregates a harmonized Kreis-grain dataset onto an
  Agenturbezirk via the area-share crosswalk (`mart_agentur_dataset_
  aggregate`, fed by `mart_agentur_nuts3_crosswalk`) — the Agentur
  equivalent of `GET /nuts/{nuts_id}/dataset-aggregate/{slug}`. Counts are
  summed, rates/means/medians are population-weighted averages of the
  Kreis values - an approximation, flagged per point via
  `aggregation_method`/`value_kind` (also `GET /agenturbezirke/
  dataset-aggregate[/{slug}?year=]` for the map). `GET /agenturbezirke/
  {ba_schl}/nuts-hierarchy` and `GET /nuts/{nuts_id}/agenturbezirke`
  (`mart_agentur_nuts_membership`) link Agenturbezirke into the NUTS
  hierarchy in both directions. `geo.go`'s generic `/geo` endpoint
  (`geo_type=nuts|lau|agentur`) also covers Agenturbezirke for list/
  detail/spatial-query (`ba_schl` detected via `detectGeoType` from a
  purely numeric ID) by delegating single-region reads into
  `agenturbezirk.go`'s handlers, same pattern as its NUTS/LAU delegation
  — the dedicated `/agenturbezirke/*` routes stay the canonical source for
  the monthly time series and period-based map coloring, which `/geo`
  does not replicate (Agentur has no yearly time axis). `dataset_catalog.go`
  (`GET /datasets`, `GET /datasets/{slug}`) reads
  `transformed_data.mart_dataset_catalog` — provenance/license/
  description per dataset, not to be confused with `metrics_catalog.go`
  (derived-metric formulas) or `socioeconomic_inventory.go` (per-year
  record counts). Goes through `respondList`/`respondNegotiated` like the
  rest (CSV/`?format=`/`?download=1`); `GET /datasets` also accepts
  `?datasets=a,b` (reuses `region_data.go`'s `parseDatasetList`) so the
  frontend can fetch metadata for exactly the slugs in a region/dataset
  download alongside it. `GET /datasets/{slug}/source` (`GetDatasetSource`,
  same file) serves the source values from
  `transformed_data.mart_dataset_source` - unharmonized, as delivered by
  geo-mapper (FAIR/provenance); 404 for slugs without source rows (derived
  slugs, Agenturbezirke).
- `custom_region.go` (`POST /custom-regions/dataset-aggregate`) aggregates
  datasets over a GeoJSON polygon sent per request (nothing is stored). Only
  the polygon∩unit overlap and the weighting run in SQL; the values come from
  `mart_custom_region_unit_values`, per-indicator level/measure type from
  `mart_dataset_aggregation_meta`. If the polygon equals one unit of a level
  (area IoU ≥ 0.999, `customRegionExactMatchSQL`) only that unit counts and
  `matched_unit` is set on the rows. Goes through `respondList` (JSON/CSV).
  Body/vertex limits and the GeoJSON checks (`bindGeoJSONBody`,
  `validateGeoJSONGeometry`, `checkGeoJSONInDB`) live in `geojson_input.go`
  and are shared with `POST /geo/query` - reuse them for any new GeoJSON
  input instead of duplicating limits. `geoJSONGeometrySQL`/`geoJSONInputCTE`
  are the one definition of the input geometry (polygons repaired with
  `ST_MakeValid`, points/lines unchanged); `/geo/query` runs the check and
  its count in one statement (`geoJSONCheckSQL`). PostGIS errors become a
  400 only via the whitelist in `geometryInputErrorMessage` (fixed text),
  everything else goes through `respondDBError`.
- Database errors go through `respondDBError(c, op, err)` (`common.go`): it
  logs the real error and answers with a generic message (503 on
  timeout/cancellation, else 500) - never put a raw `err` into a response.
  DB calls use the `*Context(c.Request.Context(), ...)` variants, and the
  connection sets `statement_timeout=60000` (`cmd/soziodata/main.go`).
  Pagination limits (`chunk_size` ≤ 10000, `limit=null` ≤ the cap from `row_cap.go`, else
  400) are enforced in `resolvePagination` (= `parsePagination` before any
  DB access + `paginationRequest.resolve(total)` with the pre-counted
  `total`; the cap only applies to unpaged `limit=null`, not with
  `chunk_size`). The `limit=null` cap is sized at startup:
  `max(300000, 1.25 × largest stg_*/snap_results_* table)` from
  `pg_class.reltuples`.
- `internal/routes/routes.go` wires the hand-written routes;
  `routes_soziodata_generated.go` wires the generated per-dataset routes.
  Both are mounted together in `cmd/soziodata/main.go`.
- `configs/metrics_catalog.yaml` is hand-maintained metadata (formulas/
  labels for the *derived* metrics, e.g. `bevoelkerungsdichte`) served by
  `internal/handlers/metrics_catalog.go` — independent of the codegen, so
  it is edited by hand when a derived metric is added or changes. Formula
  types: `ratio` (numerator / denominator × scale) and `product` (numerator ×
  factor × scale, e.g. `auszubildende-absolut` = rate per 1,000 × count /
  1,000); an optional `note` is appended to `meta.note`.
  `metrics.go`/`metrics_catalog.go` read harmonized datasets from
  `mart_nuts3_sociodata_harmonized` for `?nuts_version=` (default: newest
  version, see `target_versions.go`);
  columns without a harmonized counterpart (`harmonizedMetricsColumns`)
  fall back to the staging tables. Every `metricsSource` keeps invalid rows
  but marks them (`excluded_reason`: `invalid`, `no_geometry`,
  `no_denominator` for a ratio, `no_factor` for a product), so the
  statistics use only valid values and the response counts the rest
  (`excluded_invalid_count`, `excluded_regions`,
  `regions_without_geometry`/`_denominator`/`_factor`); `meta.note` of
  `GET /metrics/derived/{slug}` carries only the rule of its formula type. `GET /metrics` runs its
  columns via `runIndexedLimited` (≤ 4 parallel, request order kept).
- `api/bruno/` is a Bruno HTTP-request collection for manually exercising
  the API against a running instance.
- `deployments/` holds the production `docker-compose.yml`
  (Traefik-fronted); see the root [README.md](../README.md) for the
  operational runbooks (image updates, pipeline container restarts).
- `README.md` plus the per-directory `cmd/README.md` / `api/README.md` /
  `scripts/README.md` / `init/README.md` / `internal/README.md` are mostly
  `golang-standards/project-layout` template boilerplate; the project-specific
  docs are [docs/backend/](../docs/backend/index.md). `scripts/` holds the
  deploy/backup scripts (`deploy_remote.sh`, `backup_db.sh`, `restore_db.sh`)
  `sync_nextcloud_data_exchange.sh` (timer in `init/systemd/`), and
  `check_swagger_router_annotations.sh` (run in CI after `go test`).
