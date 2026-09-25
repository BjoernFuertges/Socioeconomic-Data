# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository.
Full documentation lives in the [MkDocs site](docs/index.md) (`mkdocs serve`)
— this file is intentionally short and links to it instead of duplicating
content. Update this file whenever a practice described here changes.

## Keeping documentation in sync

If a change affects architecture, data flow, API surface, deployment, or a
convention described in the [MkDocs site](docs/index.md), update the
relevant `docs/*.md` page(s) as part of the same change — high level only
(what changed and why it matters), not a line-by-line rewrite. Do not
duplicate implementation detail that already lives in code, code comments,
or Swagger; link to it instead. Skip doc updates for purely internal
refactors that don't change behavior or structure visible to the docs.

The same applies to the per-directory `CLAUDE.md` files (`backend/`,
`frontend/`, `dbt/`, `dlt/`, `analysis/`, `arbeitslosenzahlen-berlin/`):
if a change alters commands, generated-file boundaries, model/mart
layering, or another fact one of them states, update that `CLAUDE.md` in
the same change rather than letting it drift out of date.

## Project overview

Monorepo that loads socioeconomic/geographic data (unemployment, population,
debtor quota, LAU/NUTS geodata, ...) from various sources, maps it onto
common regions (LAU/NUTS), historizes it, and serves it via a Go REST API.
See [docs/architecture.md](docs/architecture.md) for the full data flow
diagram across all modules.

| Component | Language | Purpose |
|---|---|---|
| `backend/` | Go | REST API, reads only from `transformed_data` (read-only) |
| `dlt/` | Python | Extract & Load raw data into `loaded_data` (unchanged) |
| `dbt/socioeconomic_data_pipeline/` | SQL (dbt) | Transform `loaded_data` → `transformed_data` (staging/intermediate/marts/snapshots) |
| `geo-importer/`, `geo-mapper/` | Python | Prepare geodata and map raw datasets onto geodata IDs, upstream of `dlt/` |
| `data-exchange/` | data only | Exchange area for intermediate raw-data states |
| `docs/`, `mkdocs.yml` | Markdown | This documentation site |

## Build and test

```bash
# Backend (Go)
cd backend && go run cmd/soziodata/main.go   # run locally (needs Postgres + .env)
cd backend && go test ./...                  # tests
cd backend && swag init -g cmd/soziodata/main.go -o docs  # regenerate Swagger

# dbt (needs a configured profiles.yml / DBT_PROFILES_DIR + reachable Postgres/PostGIS)
cd dbt/socioeconomic_data_pipeline && dbt deps && dbt snapshot && dbt run && dbt test

# dlt (needs dlt/.dlt/secrets.toml + reachable Postgres)
cd dlt && python load_<name>.py [args...]

# Docs site
pip install -r requirements-docs.txt && mkdocs build --strict   # must pass with zero warnings
```

There is no top-level "build everything" command — each component is
built/tested independently as shown above.

## Architecture rules agents must respect

- **The backend never transforms data.** It only reads finished
  views/tables from the `transformed_data` schema. Any data cleaning,
  joining, or aggregation logic belongs in dbt, not in Go handlers.
- **dlt never historizes.** History over time is exclusively built by dbt
  snapshots (`snapshots/snap_results_*.sql`). Do not add historization
  logic to a dlt script. `write_disposition` is now uniformly `"replace"`
  for every dataset-content resource across `dlt/` (`load_soziodata_generic.py`
  was the one exception - it used `"append"` with no primary key/dedup
  until this was fixed; reruns had been silently accumulating duplicate
  `loaded_data` rows, which inflated every "sum"-type indicator in
  `mart_nuts3_sociodata_harmonized`/`mart_nuts3_timeseries_interpolated`
  and downstream fill_gaps views by however many times the pipeline had
  been rerun locally - "weighted_avg" indicators happened to cancel the
  duplication out in their ratio and were unaffected). `load_arbeitslosenzahlen_afa.py`
  still legitimately mixes `"merge"` (upserting `dataset_metadata` by
  `dataset_slug`, same pattern as `load_soziodata_generic.py`'s own
  metadata resource) and `"replace"` (the actual Agenturbezirk data) -
  that split is intentional, not the same bug. Treat `"append"` as a red
  flag, not a pattern to copy into a new loader.
- **NUTS-3 region IDs are not stable over time** (2003/2006/2010/2013/
  2016/2021/2024 boundary versions exist). Any new model that aggregates
  or compares NUTS-3 data across years must go through
  `mart_nuts3_sociodata_harmonized` / `mart_nuts3_timeseries_interpolated`
  (see [docs/pipeline/dbt.md](docs/pipeline/dbt.md)) rather than joining raw
  year-over-year data directly. The same applies to LAU (gisco_id) data
  across the 2011–2024 vintages: use `mart_lau_sociodata_harmonized` /
  `mart_lau_timeseries_interpolated` instead of joining raw year-over-year
  LAU data directly.
- **No artificial "socio ID".** Existing geodata references
  (`geodata_id_nuts`, `geodata_id_lau`, `geodata_id_ars`, `nuts_id`,
  `gisco_id`) are the domain keys everywhere — see
  [backend/docs/id-decision.md](backend/docs/id-decision.md). Do not
  introduce a new synthetic identifier for a dataset.

## Conventions

- **Adding a new "Kreis"-level socioeconomic dataset** follows an existing,
  repeated pattern — copy it rather than inventing a new one:
  1. Raw data as `mapped_pairs.csv` + `meta.yaml` under
     `data-exchange/.../<dataset>/results_<dataset>_kreise/`
  2. `dlt/load_soziodata_generic.py <dataset>/results_<dataset>_kreise` →
     table `loaded_data.results_<dataset>_kreise`
  3. `dbt/.../models/staging/stg_<dataset>_kreise.sql` (+ matching
     `*_schema.yml` with `not_null`/`accepted_range` tests)
  4. Add an entry to the `source_datasets` list in
     `mart_nuts3_sociodata_harmonized.sql`
  5. `backend/cmd/gen-models` regenerates the Go models/handlers/routes for
     the new dataset automatically — do not hand-write a handler for it.
- **Go**: `internal/` packages are private to this module; only put code in
  `pkg/` if it is genuinely meant to be imported elsewhere. Files ending in
  `_generated.go` are produced by `cmd/gen-models` from dbt schema YAMLs —
  edit the source YAML, not the generated file.
- **dbt macros that use `adapter.get_columns_in_relation`** (e.g.
  `soziodata_snapshot_query`) require the source relation to already exist
  in the target database — the dlt load must run before `dbt snapshot`/`dbt run`.
- **Commit hooks**: `git config core.hooksPath backend/githooks` enables a
  `pre-commit` hook that regenerates Swagger docs, regenerates Go models
  (`go generate ./internal/models/...`), and runs `go test ./...`. Do not
  bypass it silently — if a change requires `--no-verify`, say so.
- **CI is GitLab CI** (`.gitlab-ci.yml`), not GitHub Actions. Pipeline
  stages: `validate` → `build` → `deploy` → `pages`. Deploys happen on push
  to `main` only.

## Verifying non-trivial changes

Whenever possible, do not just review dbt/SQL changes statically — spin up
a local `postgis/postgis` Docker container, load a minimal synthetic
fixture into `loaded_data`, run the affected `dbt run --select +<model>`
subgraph, and inspect the resulting rows before considering a change to a
model verified. This project has no automated dbt CI run yet, so this is
currently the only way to catch logic errors (e.g. weights cancelling out
in a normalization step) before merge.

## Where to look for more detail

- [docs/architecture.md](docs/architecture.md) — full component & data flow diagrams
- [docs/backend/](docs/backend/index.md) — API endpoints, local dev, deployment
- [docs/pipeline/](docs/pipeline/index.md) — dlt scripts and dbt model layers in detail
- [docs/operations.md](docs/operations.md) — disk space / Docker maintenance
- [README.md](README.md) — root-level operational runbooks (Traefik, deployment, disk cleanup)
