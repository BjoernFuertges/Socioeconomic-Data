# dbt – Transformation

Project: [`dbt/socioeconomic_data_pipeline/`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/tree/main/dbt/socioeconomic_data_pipeline).
Reads from the `loaded_data` schema (populated by dlt), writes to
`transformed_data`.

```bash
cd dbt/socioeconomic_data_pipeline
dbt deps      # once / when packages change (dbt_utils)
dbt snapshot  # advance historization
dbt run       # build staging/intermediate/marts
dbt test      # data-quality tests from the *_schema.yml files
```

The connection is configured via `profiles.yml` (see `profiles.example.yml`)
or the `DBT_PROFILES_DIR` environment variable.

## Layers

| Layer | Directory | Materialization | Purpose |
|---|---|---|---|
| Snapshots | `snapshots/` | `snapshot` (table with history) | historizes `loaded_data` tables over time (SCD-like) |
| Staging | `models/staging/` | View | 1:1 cleaned view of `loaded_data`/snapshots, type conversion, column naming |
| Intermediate | `models/intermediate/` | View/Table (depends on model) | building blocks that aren't consumed directly (e.g. crosswalks) |
| Marts | `models/marts/` | Table | finished fact/dimension tables consumed by the backend |

### Snapshots

Each `snap_results_*` snapshot typically uses the
[`soziodata_snapshot_query`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dbt/socioeconomic_data_pipeline/macros/soziodata_snapshot_query.sql)
macro, which returns the latest value per `(geodata_id_nuts, year)`
(`distinct on` + `order by ... loaded_at desc`). Since different dlt load
runs may bring different metadata columns (`loaded_at` vs. `_dlt_load_id`),
the macro detects the available column at runtime via
`adapter.get_columns_in_relation`.

Datasets without `geodata_id_nuts` (e.g. LAU/municipality data such as
`snap_results_beschaeftigungsquote_gemeinden`) cannot use the shared macro
and define their own, adapted snapshot query instead.

## NUTS version harmonization and time-series interpolation

NUTS-3 boundaries change over time (e.g. 2013 → 2016 → 2021 → 2024 due to
territorial reforms). To allow time series to be compared consistently
across multiple years, there is a multi-stage harmonization pipeline in
`models/intermediate/` and `models/marts/`:

```mermaid
flowchart TB
    stg_nuts["stg_nuts_geometry"] --> spatial["int_nuts_spatial_crosswalk\n(area shares per version pair)"]
    stg_lau_geo["stg_lau_geometry"] --> lau_geo["int_lau_population_geo\n(municipality population + geometry)"]
    stg_lau_pop["stg_lau_population"] --> lau_geo
    stg_nuts --> membership["int_lau_nuts3_membership\n(municipality → NUTS3 per version)"]
    lau_geo --> membership
    membership --> popweight["int_lau_nuts3_population_weight\n(real population weight per region pair)"]
    spatial --> xwalk["mart_nuts_version_crosswalk\n(population-weighted, 1-/2-hop)"]
    popweight --> xwalk
    xwalk --> harmonized["mart_nuts3_sociodata_harmonized\n(all datasets on target NUTS version)"]
    harmonized --> interpolated["mart_nuts3_timeseries_interpolated\n(gap-free yearly series)"]
```

- **`int_nuts_spatial_crosswalk`** – pure area intersections between NUTS-3
  regions of two versions (`ST_Intersection`/`ST_Area`), including forward
  and backward version pairs.
- **`int_lau_population_geo`** / **`int_lau_nuts3_membership`** /
  **`int_lau_nuts3_population_weight`** – determine the **real** population
  weight per (source, target) region pair based on municipality (LAU)
  population. Weighting by the total population of the NUTS-3 source region
  itself would always cancel out during normalization (see the comment in
  `mart_nuts_version_crosswalk.sql`) – only the finer LAU level makes the
  population weighting actually effective.
- **`mart_nuts_version_crosswalk`** – combines area and population weight
  (falls back to area if no LAU population can be determined), including
  2-hop compositions (e.g. 2013 → 2016 → 2021).
- **`mart_nuts3_sociodata_harmonized`** – converts all county (Kreis)
  datasets onto a common target NUTS version (default:
  `var('target_nuts_version', 2024)`, configurable via
  `dbt run --vars 'target_nuts_version: 2021'`).
- **`mart_nuts3_timeseries_interpolated`** – fills missing years per region
  and dataset via linear interpolation between known values (no
  extrapolation beyond the known time range).

## Conventions

- Surrogate keys via `dbt_utils.generate_surrogate_key(...)`.
- Marts read directly from the outside (backend) document their columns in
  an adjacent `*_schema.yml` (including tests like `not_null`,
  `accepted_values`, `dbt_utils.accepted_range`).
- New county (Kreis) datasets follow the existing pattern
  `stg_<name>_kreise.sql` → `results_<name>_kreise` in
  `mart_nuts3_sociodata_harmonized` (see the `source_datasets` list there).
