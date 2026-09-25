# AGENTS.md

This file provides guidance to AI coding agents and contributors working in this part of the repository.

Scope: this file covers `pipeline/dbt/` (the actual dbt project is nested one
level down at `pipeline/dbt/socioeconomic_data_pipeline/` — all commands below
assume you `cd` there first). See the repository root
[AGENTS.md](../AGENTS.md) for cross-component architecture and the
NUTS-3-versioning / historization rules that govern this project, and
[docs/pipeline/dbt.md](../docs/pipeline/dbt.md) for the full model-layer
writeup.

## Commands

To run dbt together with the database, backend, and frontend in one go
instead of setting each up by hand, use `compose.local.yml` at the repo
root (`docker compose -f compose.local.yml up --build`, then
`docker compose -f compose.local.yml exec pipeline bash /app/dlt/run_all_pipelines.sh`
— that runs the dlt loads before dbt, per the ordering rule below) — see
the root [README.md](../README.md#lokal-testen-docker-compose). The
commands below are for running dbt standalone against an
already-running/external Postgres.

```bash
cd socioeconomic_data_pipeline

# Needs DBT_PROFILES_DIR pointing at a profiles.yml (see profiles.example.yml)
# and a reachable Postgres/PostGIS with loaded_data already populated by dlt.
dbt deps            # install packages.yml dependencies
dbt snapshot         # build snap_results_*_kreise (history), must run before `dbt run`
dbt run              # build staging/intermediate/marts views & tables
dbt test             # schema tests (not_null/accepted_range/...) + tests/*.sql

# Iterate on a single model + its dependents:
dbt run --select +mart_nuts3_sociodata_harmonized
dbt test --select stg_arbeitslose_kreise
```

`profiles.example.yml` reads `DB_HOST`/`DB_PORT`/`DB_USER`/`DB_PASSWORD`/`DB_NAME`
env vars; copy it to wherever `DBT_PROFILES_DIR` points and set those, or
point `DBT_PROFILES_DIR` at the deployment's existing `profiles.yml`.

`README.md` in the project directory is the unmodified dbt-init
boilerplate, not project documentation.

## Architecture

Standard staging → intermediate → marts layering
(`dbt_project.yml` pins materializations per layer: staging/intermediate =
views, marts = tables, `marts/fill_gaps/*` = views):

- **snapshots/** — the only place historization happens. Two shapes:
  - `snap_results_<dataset>_kreise.sql` — one per Kreis-level dataset,
    `strategy: check` on the `value` column, unique key
    `geodata_id_nuts || '_' || year` (dlt always loads these with
    `write_disposition="replace"`).
  - `snap_dataset_license.sql` — historizes `license`/`license_url` from
    `loaded_data.dataset_metadata` (a `write_disposition="merge"`-upserted
    table, one row per `dataset_slug`, unlike the Kreis tables above),
    `strategy: check` on `[license, license_url]`, unique key
    `dataset_slug`. Feeds `stg_dataset_license_history` /
    `mart_dataset_catalog.license_tracked_since` /
    `.license_backfilled` — see that mart's description below.
- **models/staging/** (`stg_*.sql` + matching `*_schema.yml`) — one model
  per raw `loaded_data.results_*` table (or per snapshot), light
  renaming/typing plus the schema tests. Adding a new Kreis dataset means
  adding a `stg_<dataset>_kreise.sql`/`_schema.yml` pair here first — see
  the "Adding a new Kreis-level dataset" recipe in
  [AGENTS.md](../AGENTS.md).
- **models/intermediate/** — crosswalk building blocks, not dataset
  content: LAU↔NUTS3 membership and population weights
  (`int_lau_nuts3_membership`, `int_lau_nuts3_population_weight`), the
  NUTS boundary-version crosswalk (`int_nuts_spatial_crosswalk`) that
  `mart_nuts_version_crosswalk` / `mart_nuts3_sociodata_harmonized` build
  on to reconcile 2003/2006/2010/2013/2016/2021/2024 NUTS-3 boundary
  changes, its LAU-version counterpart (`int_lau_spatial_crosswalk`, feeds
  `mart_lau_version_crosswalk` / `mart_lau_sociodata_harmonized`), and the
  LAU→NUTS-3 spatial fallback link (`int_lau_nuts3_spatial_link`, feeds the
  canonical `nuts3_id` in `lau_integrated`). `int_lau_spatial_crosswalk`
  deliberately excludes same-`gisco_id` period pairs (handled cheaply as a
  trivial identity mapping directly in `mart_lau_version_crosswalk`
  instead) — LAU geometry vintages are re-generalized yearly by GISCO, so
  most municipalities have many geometry periods with no real
  administrative change behind them, and spatially intersecting all of
  them against each other does not finish in reasonable time.
- **models/marts/** — the tables the backend actually reads from
  (`transformed_data` schema):
  - `mart_nuts3_sociodata_harmonized` / `mart_lau_sociodata_harmonized` —
    Kreis- resp. LAU-grain datasets harmonized onto every available target
    boundary version simultaneously (`target_version` column) via
    `mart_nuts_version_crosswalk` / `mart_lau_version_crosswalk`; these are
    the dataset lists every new Kreis/LAU dataset must be registered in.
    Each entry carries a `measure_type` ('sum' for counts, 'weighted_avg'
    for rates/averages) that decides how values are combined when several
    source regions map onto one target region.
  - `mart_nuts3_timeseries_interpolated` / `mart_lau_timeseries_interpolated`
    — gap-filled time series built on top of the harmonized marts, one row
    per (region, dataset, target_version, year).
  - `marts/fill_gaps/api_*_fill_gaps.sql` — one thin per-dataset view over
    `render_nuts3_fill_gaps_view` / `render_lau_fill_gaps_view` (macros in
    `macros/`) exposed to the backend; a new dataset needs an entry here
    too, not just in the harmonized mart.
  - `mart_nuts0/1/2/3_summary`, `mart_region_datasets`,
    `mart_socioeconomic_inventory` — aggregation/catalog views consumed by
    backend inventory endpoints.
  - `lau_integrated` / `lau_data_quality` — the LAU-level counterpart to
    the NUTS-3 harmonization. `lau_integrated.nuts3_id` is the canonical
    LAU→NUTS-3 link (declared source column if present, otherwise the
    spatial fallback from `int_lau_nuts3_spatial_link`); `nuts3_link_source`
    ('declared'/'spatial_estimate') tells consumers which one it is.
  - `mart_nuts_geometry_historized` / `mart_lau_geometry_historized` —
    versioned geometry, joined by NUTS/LAU id + boundary version rather
    than a foreign key to a single "current" geometry table.
  - `mart_dataset_source` — source values of all `meta.yaml`-based
    Kreis/Gemeinde datasets in one long format (read from `loaded_data`,
    unharmonized, uninterpolated; only region names mapped to geodata IDs
    by geo-mapper). Backs `GET /datasets/{slug}/source`. New dataset -> add
    an entry to its `source_datasets` list.
  - `mart_dataset_catalog` — one row per dataset with provenance/license/
    description (from `stg_dataset_metadata`, itself fed by the
    `meta.yaml`-driven `load_soziodata_generic.py` via `loaded_data.
    dataset_metadata`) plus year-coverage aggregated from
    `mart_socioeconomic_inventory` and a persistent, slug-derived
    `dataset_uri` (`urn:soziodata:dataset:<slug>`). `license` is the
    literal string `"unbekannt"`, not `NULL`, for datasets whose actual
    usage terms haven't been researched. Also joins
    `stg_dataset_license_history` (fed by `snap_dataset_license`, see
    above) to expose `license_tracked_since` (date the license was first
    observed for this dataset) and `license_backfilled` (true if the
    dataset has data years older than `license_tracked_since` — the
    current license claim was then retroactively applied to those older
    years without independent verification for that period; BBSR itself
    keeps no license archive, so this can't be resolved further back,
    only flagged). Served by the backend's `GET /datasets` /
    `GET /datasets/{slug}` (`?datasets=a,b` filters the list; both support
    CSV via `respondList`). Covers every dataset whose loader upserts a
    `loaded_data.dataset_metadata` row, including `arbeitslosenzahlen-afa-
    agenturbezirke` (own upsert in `load_arbeitslosenzahlen_afa.py`, same
    mechanism as the generic Kreis/Gemeinden datasets). Also unions in
    synthetic rows (no own loader) for slugs that only exist inside
    `GET /regions/{region_type}/{region_id}/data`
    (`backend/internal/handlers/region_data.go`): `ba_arbeitslose`/
    `ba_unterbeschaeftigung` and `lau_beschaeftigungsquote_weighted_avg`
    borrow provenance/license from their real source dataset
    (`arbeitslosenzahlen-afa-agenturbezirke` resp.
    `beschaeftigungsquote-gemeinden`), same pattern as
    `beschaeftigungsquote-nuts3` below; `lau_population_sum` has no source
    dataset to borrow from (fed by `fetch_lau_population.py`/Eurostat, not
    a `meta.yaml`-driven loader) and gets `license = "unbekannt"` instead.
    Without these, the frontend's per-region metadata export
    (`DownloadDialog.vue`) would have nothing to show for these slugs.
  - `mart_agenturbezirk_sociodata` — monthly unemployment/underemployment
    figures per Agenturbezirk (Bundesagentur-für-Arbeit-internal district
    partition), joined with `stg_agenturbezirk_geometry`. Agenturbezirke
    are a distinct geo-level from NUTS-3/LAU/ARS, so this is not an entry
    in `mart_nuts3_sociodata_harmonized` and has no version crosswalk
    (only one current geometry vintage) or gap-filling yet - both
    explicitly deferred future work. Complements, not replaces, the
    Kreis-level `arbeitslose` dataset (`arbeitslose_kreise` column in
    `mart_nuts3_summary`).
  - `mart_agentur_nuts3_crosswalk` — n:m Agentur↔NUTS-3 area-share
    crosswalk (both directions), generalizes correctly to the Berlin case
    (3 Agenturbezirke sharing one NUTS-3 Kreis) via plain `ST_Intersects`,
    no special-cased logic. Feeds `mart_agentur_dataset_aggregate`
    (rollup of every harmonized Kreis-grain dataset onto Agenturbezirk
    level: counts are summed via `area_share_of_nuts3`, rates/means/
    medians are population-weighted averages of the Kreis values (area-
    weighted fallback if a Kreis lacks population that year) - an
    approximation, flagged via `aggregation_method`/`value_kind`; the Agentur counterpart to
    `mart_nuts012_dataset_aggregate` - served by `GET /agenturbezirke/
    {ba_schl}/dataset-aggregate/{slug}`) and `GET /agenturbezirke/
    {ba_schl}/nuts3`.
  - `mart_agentur_nuts_membership` — places each Agenturbezirk in the NUTS
    hierarchy: one row per (Agenturbezirk, overlapping NUTS-1/2/3 region),
    derived from `mart_agentur_nuts3_crosswalk` via NUTS-code prefixes (no
    extra geometry work). `is_fully_contained` = area share ≥ 99 %,
    `is_parent` = smallest fully containing level (absent for Agenturbezirke
    spanning several Bundesländer, e.g. Bremen/Osterholz). Served by
    `GET /agenturbezirke/{ba_schl}/nuts-hierarchy` and
    `GET /nuts/{nuts_id}/agenturbezirke`.
  - `mart_dataset_aggregation_meta` / `mart_custom_region_unit_values` —
    inputs for `POST /custom-regions/dataset-aggregate` (per-indicator level,
    `measure_type`, `kind`; long table of NUTS-3/LAU (target version 2024) and
    Agenturbezirk values plus population weight). The NUTS-3 list in the meta
    model duplicates `source_datasets` from
    `mart_nuts3_sociodata_harmonized` and must be kept in sync.
- **macros/** — `soziodata_snapshot_query.sql` (used by the
  `snap_results_*` snapshots, requires the source relation to already
  exist via `adapter.get_columns_in_relation` — dlt must have loaded
  before `dbt snapshot` runs), `render_nuts3_fill_gaps_view.sql` /
  `render_lau_fill_gaps_view.sql` (shared gap-filling logic behind
  `marts/fill_gaps/*`), `missing_data_summary.sql`,
  `generate_schema_name.sql` (schemas are taken verbatim from
  `+schema:` config, not prefixed with the target schema).
