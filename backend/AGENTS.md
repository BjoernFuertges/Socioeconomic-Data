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
go test ./...   # currently covers internal/handlers (content negotiation, CSV encoding, export params)

# Release version: internal/version (Version/BuildTime, default "dev") is set via -ldflags
# (Dockerfile build args VERSION/BUILD_TIME, filled by CI) and copied into
# docs.SwaggerInfo.Version at startup (OpenAPI info.version) and GET /health.
# The `@version dev` annotation in cmd/soziodata/main.go is only the placeholder.

# Regenerate Swagger docs (writes into docs/)
swag init -g cmd/soziodata/main.go -o docs
# NOTE: the pre-commit hook adds --parseDependency --parseInternal, which
# fails with swag <=1.16.4 on Go >= 1.27 (it cannot parse the generics in
# the stdlib's math/rand/v2). Without the flags the output is equivalent;
# with them, commits need --no-verify until swag is updated.

# Regenerate models/handlers/routes from dbt schema YAMLs
go generate ./internal/models/...
```

Enable the pre-commit hook (repo root: `git config core.hooksPath backend/githooks`) to run swag + go generate + go test automatically before each commit; see [githooks/README.md](githooks/README.md).

## Architecture

- `cmd/soziodata/main.go` is the actual API entrypoint (Gin router, loads
  config, mounts routes). The `backend/soziodata` file in this directory is
  a stray compiled binary, not source — ignore it.
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
  inventory/catalog). `negotiate.go` holds the shared response layer:
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
- `internal/routes/routes.go` wires the hand-written routes;
  `routes_soziodata_generated.go` wires the generated per-dataset routes.
  Both are mounted together in `cmd/soziodata/main.go`.
- `configs/metrics_catalog.yaml` is hand-maintained metadata (display
  labels/units/descriptions per metric) served by
  `internal/handlers/metrics_catalog.go` — independent of the
  dbt-schema-driven codegen, so it does not get regenerated and must be
  updated manually when a dataset is added or changes.
- `api/bruno/` is a Bruno HTTP-request collection for manually exercising
  the API against a running instance.
- `deployments/` holds the production `docker-compose.yml`
  (Traefik-fronted); see the root [README.md](../README.md) for the
  operational runbooks (image updates, pipeline container restarts).
- `README.md`, `project-layout-master/`, and the per-directory
  `cmd/README.md` / `pkg/README.md` / `api/README.md` / etc. are the
  unmodified `golang-standards/project-layout` template boilerplate, not
  project-specific documentation — for real docs see
  [docs/backend/](../docs/backend/index.md).
- `pkg/_your_public_lib_/` and `internal/pkg/_your_private_lib_/` are
  unused template scaffolding (`.keep` files only); nothing in this
  project currently lives there.
