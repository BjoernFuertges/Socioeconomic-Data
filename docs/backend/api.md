# API Endpoints

All endpoints live under the `/api/v1` prefix. The full, generated
OpenAPI/Swagger specification is available at `/api/docs/index.html` (see
[`backend/docs/swagger.yaml`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/backend/docs/swagger.yaml)).

Defined in [`internal/routes/routes.go`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/backend/internal/routes/routes.go).

## Response formats (JSON and CSV)

All list-shaped endpoints (the generic dataset routes, `/timeseries`, the
dataset-aggregate routes, the Agenturbezirk series and the region export
below) serve **either JSON or CSV**, negotiated via the HTTP `Accept`
header:

```bash
curl -H 'Accept: text/csv' '<host>/api/v1/bevoelkerung/timeseries?geodata_id_nuts=DE300'
```

- No `Accept` header (or `*/*`) yields JSON — the wrapped
  `{success, data, meta}` envelope, unchanged from before.
- `Accept: text/csv` yields a flat CSV whose column names are the JSON
  field names of the same response. Nullable fields (`sql.Null*`, pointers)
  become empty fields; an empty result is a header row on its own.
- An `Accept` header that matches neither yields `406`.
- `?format=csv|json` overrides the header, for browser links (`<a href>`
  cannot send headers); `?download=1` adds `Content-Disposition:
  attachment`. `?limit=null` lifts the row limit for a full export.

### Pagination

- `limit` / `offset`: default `limit` is 1000 on the generated dataset
  routes and 100 on the region lists (`/lau`, `/nuts`, `/geo`). Maximum
  `limit` is 10,000 on the dataset routes and `/nuts`, 1,000 on `/lau`,
  `/geo` and `POST /geo/query`.
- `limit=null` returns all rows, up to a cap of at least 300,000 rows
  (set at startup to 1.25× the largest table, so the largest dataset,
  `beschaeftigungsquote-gemeinden` with ~252,000 rows, stays downloadable). A larger
  result is rejected with `400` and a hint to page with `chunk_size` —
  never silently truncated.
- `chunk_size` (max. 10,000, otherwise `400`) + `chunk` (0-based) is an
  alternative to `limit`/`offset` and takes precedence when set — also over
  `limit=null`, whose cap then does not apply (the response is paged).
- JSON responses carry pagination info in `meta` (`meta.total` is the full
  row count, so a client can compute the number of chunks).

### Errors

Errors use the same envelope with `success: false` and a German `error`
text. Invalid parameters yield `400` with a specific message. Database
failures yield `500` with only the generic text "Interner Fehler beim
Datenbankzugriff" — the real error is logged server-side, never returned.
Timeouts and cancelled requests yield `503`: every statement is capped by
`statement_timeout` (60 s, set on the DB connection), and queries are
cancelled when the client disconnects.

Implemented once in
[`internal/handlers/negotiate.go`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/backend/internal/handlers/negotiate.go)
(`respondList`/`respondNegotiated`), so no endpoint carries its own CSV code.

## Combined region export

| Method | Path | Purpose |
|---|---|---|
| GET | `/regions/{region_type}/{region_id}/data` | Datasets of one region (`nuts`, `lau` or `agentur`) in one long-format table |

Assembles the existing marts for one region instead of requiring several
calls, and returns one uniform row shape (`region_type`, `region_id`,
`dataset_slug`, `year`, `month`, `value`, `is_computed`,
`is_interpolated`, `aggregation_method`, `value_kind`,
`source_region_count`). The backend only selects and concatenates here; all
aggregation itself still happens in dbt.

- `scope=raw` returns the region's own source values (NUTS-3: the
  interpolated Kreis series; LAU: the interpolated Gemeinde series;
  Agentur: the monthly BA figures as `ba_arbeitslose` /
  `ba_unterbeschaeftigung`).
- `scope=aggregated` returns computed values (NUTS-0/1/2 and Agenturbezirke
  aggregated from Kreise; NUTS-3 aggregated from Gemeinden, as
  `lau_population_sum` / `lau_beschaeftigungsquote_weighted_avg`).
- `scope=both` (the default) returns both in one file; `is_computed`
  separates them.
- NUTS-0/1/2 have no raw rows, LAU has no aggregated ones.
- `datasets=a,b` restricts the dataset slugs (empty = all available),
  `from_year`/`to_year` the period.

```bash
curl -H 'Accept: text/csv' \
  '<host>/api/v1/regions/nuts/DE300/data?scope=both&from_year=2015'
```

The frontend's `DownloadDialog.vue` fires this alongside a second request to
`GET /datasets?datasets=<same slugs>` (see below) to export the matching
provenance/license/description metadata as a separate file at the same
time - CSV is a flat format and can't mix the two row shapes into one file,
so this stays two coordinated downloads/requests rather than a single
combined one.

## Health

| Method | Path | Purpose |
|---|---|---|
| GET | `/health` | Health check; also returns `version` and `build_time` of the running build |

The release version (from Semantic Release, e.g. `1.4.2`; builds without a new
release report `<last tag>+sha.<commit>`, local builds `dev`) is also the
`info.version` of the OpenAPI spec and is shown next to the title in the
Swagger UI (`/api/docs/index.html`). It is injected at build time via
`-ldflags` into `backend/internal/version`; the committed `docs/swagger.*`
files only carry the placeholder `dev`.

## LAU regions (municipalities)

| Method | Path | Purpose |
|---|---|---|
| GET | `/lau` | List of all LAU regions |
| GET | `/lau/{gisco_id}` | A single LAU region |
| GET | `/lau/{gisco_id}/versions` | Available geometry versions (years) of an LAU region |
| GET | `/lau/{gisco_id}/datasets` | Datasets available for the region |

## NUTS regions

| Method | Path | Purpose |
|---|---|---|
| GET | `/nuts` | List of all NUTS regions |
| GET | `/nuts/{nuts_id}` | All indicators for a NUTS region |
| GET | `/nuts/{nuts_id}/versions` | Geometry periods of a region (NUTS vintages 2003/2006/2010/2013/2016/2021/2024) |
| GET | `/nuts/{nuts_id}/datasets` | Datasets available for the region |
| GET | `/nuts/{nuts_id}/lau-aggregate` | NUTS-3 only: values aggregated from its municipalities (LAU) |
| GET | `/nuts/{nuts_id}/dataset-aggregate/{slug}` | NUTS-0/1/2 only: time series of a Kreis dataset aggregated from NUTS-3 (version 2024) |

## Geodata (generic, LAU + NUTS)

| Method | Path | Purpose |
|---|---|---|
| GET | `/geo` | List of all geo regions |
| GET | `/geo/{geo_id}` | A single geo region |
| GET | `/geo/{geo_id}/versions` | Available versions of a geo region |
| POST | `/geo/query` | Spatial query by geometry (e.g. point/polygon) |

`POST /geo/query` accepts any GeoJSON geometry (points and lines too, for
`intersects`/`contains`). Same input limits and geometry preparation as
custom regions below: 5 MiB body (`413` above), 200,000 vertices, WGS84
`[lon, lat]`. Polygons/MultiPolygons (e.g. self-intersecting "bowties") and
GeometryCollections are repaired with `ST_MakeValid` instead of rejected
(`400` only if no area is left); points and lines are used unchanged and
must be OGC-valid. Input PostGIS cannot parse or transform yields `400` with
a fixed message ("Ungültiges GeoJSON" / "Geometrie nicht transformierbar"),
never the raw PostGIS error; other database failures are `500`/`503`.

### Custom regions (GeoJSON)

| Method | Path | Purpose |
|---|---|---|
| GET | `/custom-regions/datasets` | Datasets that can be queried for a custom region |
| POST | `/custom-regions/dataset-aggregate` | Time series of the requested datasets for a user-defined Polygon/MultiPolygon (GeoJSON, WGS84) |

Request body: `geometry` (GeoJSON), `datasets` (slugs, max. 30), optional
`from_year`, `to_year`, `min_share`. Limits: 5 MiB body (`413` above),
200,000 vertices.

The region is sent per request and never stored. The polygon is intersected
with the source geometries (reference state 2024) and the values are
aggregated per dataset on the finest available level: LAU before NUTS-3;
the monthly Agentur figures exist only per Agenturbezirk and are always
aggregated on that level. Counts (`sum`) are distributed by area share,
rates/means (`weighted_avg`) are population-weighted where every
contributing unit has a population, otherwise area-weighted; values per
employed person (`weighted_avg_employees`, e.g. `arbeitsvolumen`,
`bruttoverdienst`) are weighted by the number of employed persons, with the
same area fallback. Every row
carries `level_used`, `aggregation_method`, `value_kind` (approximation
flag, e.g. medians) and `coverage` (share of the polygon area that has
data). Supports JSON/CSV like the other endpoints. Details in Swagger.

## Agenturbezirke (AfA unemployment figures)

Agenturbezirke are a Bundesagentur-für-Arbeit-internal district partition,
distinct from and not congruent with NUTS-3/LAU/Kreise — complements
(does not replace) the Kreis-level `arbeitslose` dataset available through
the generic socioeconomic-data routes below.

| Method | Path | Purpose |
|---|---|---|
| GET | `/agenturbezirke` | List of all Agenturbezirke, with geometry |
| GET | `/agenturbezirke/latest` | All Agenturbezirke with their latest reporting month (map view) |
| GET | `/agenturbezirke/periods` | Available reporting months |
| GET | `/agenturbezirke/period?year=&month=` | All Agenturbezirke for one reporting month |
| GET | `/agenturbezirke/dataset-aggregate` | Kreis datasets/years available aggregated to Agenturbezirke |
| GET | `/agenturbezirke/dataset-aggregate/{slug}?year=` | One Kreis dataset aggregated to all Agenturbezirke for one year |
| GET | `/agenturbezirke/{ba_schl}` | A single Agenturbezirk |
| GET | `/agenturbezirke/{ba_schl}/arbeitslosigkeit` | Monthly unemployment/underemployment series (since Jan 2007) |
| GET | `/agenturbezirke/{ba_schl}/nuts3` | Overlapping NUTS-3 regions with area shares |
| GET | `/agenturbezirke/{ba_schl}/dataset-aggregate/{slug}` | Time series of a Kreis dataset aggregated to this Agenturbezirk |
| GET | `/agenturbezirke/{ba_schl}/nuts-hierarchy` | Overlapping NUTS-1/2/3 regions; `is_parent` marks the smallest one that fully contains the Agenturbezirk (none if it spans several Bundesländer) |
| GET | `/nuts/{nuts_id}/agenturbezirke` | Reverse direction (NUTS-1 to 3): Agenturbezirke in a region, `is_fully_contained` separates real children from partial overlaps |

The Agenturbezirk ↔ NUTS link is a spatial estimate (area share, "fully
contained" = ≥ 99 %), not an official assignment.

## Socioeconomic inventory

| Method | Path | Purpose |
|---|---|---|
| GET | `/socioeconomic/inventory` | Overview of all available socioeconomic datasets |

## Dataset catalog

| Method | Path | Purpose |
|---|---|---|
| GET | `/datasets` | Provenance/license/description catalog for all datasets |
| GET | `/datasets/{slug}` | Catalog entry for a single dataset |
| GET | `/datasets/{slug}/source` | Source values of a dataset, unharmonized |

Both serve JSON or CSV like the endpoints above (`respondList`/
`respondNegotiated`, `?format=`/`?download=1`). `GET /datasets?datasets=a,b`
restricts the list to those slugs (empty = all) - used to export metadata
for exactly the datasets included in a data download, see "Combined region
export" above.

`GET /datasets/{slug}/source` returns the dataset's values as they came out
of the upstream [geo-mapper](https://pypi.org/project/geo-mapper/) step
(`mapped_pairs.csv`): the values are unchanged, only the region names are
mapped to geodata IDs (`geodata_id_*`, `mapper`, `parameter` document how).
There is no NUTS harmonization, interpolation or aggregation - use the
dataset endpoints for that. Together with `GET /datasets/{slug}`
(provenance, license) this lets any harmonized value be traced back to its
input (FAIR: reusability/provenance). Derived slugs (`beschaeftigungsquote-
nuts3`, `lau_*`, `ba_*`) and the Agenturbezirk figures have no source
rows here (404); the latter are available via `/agenturbezirke`. Backed by
the dbt model `mart_dataset_source`.

Includes four synthetic entries with no own `loaded_data.dataset_metadata`
row: `ba_arbeitslose`/`ba_unterbeschaeftigung` (provenance/license borrowed
from `arbeitslosenzahlen-afa-agenturbezirke`) and
`lau_beschaeftigungsquote_weighted_avg` (borrowed from
`beschaeftigungsquote-gemeinden`) plus `lau_population_sum` (`license`
`"unbekannt"` - sourced from `fetch_lau_population.py`/Eurostat, not a
`meta.yaml`-driven loader) - these only appear as `dataset_slug` values in
the combined region export above, not as their own routes, so without these
rows the metadata export would have nothing for them. Same synthetic
pattern already used for `beschaeftigungsquote-nuts3`, see
[`mart_dataset_catalog.sql`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dbt/socioeconomic_data_pipeline/models/marts/mart_dataset_catalog.sql).

Reads `transformed_data.mart_dataset_catalog` (name, description, unit,
provenance, license/license_url, geodata level/year, year coverage, and a
persistent `dataset_uri` of the form `urn:soziodata:dataset:<slug>`).
`license` is the literal string `"unbekannt"` rather than a bare `null`
for any dataset whose usage terms couldn't be determined — see
[`stg_dataset_metadata.sql`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dbt/socioeconomic_data_pipeline/models/staging/stg_dataset_metadata.sql).
For BBSR-sourced datasets (currently all `meta.yaml`-driven ones) the license is looked
up automatically and kept up to date by
[`pipeline/dlt/bbsr_license.py`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dlt/bbsr_license.py)
— no manual research needed, see [dlt.md](../pipeline/dlt.md).

Two additional fields make the license's temporal coverage honest:
`license_tracked_since` (date the license was first observed for this
dataset) and `license_backfilled` (`true` if the dataset has data years
older than `license_tracked_since` — the current license was then
retroactively applied to those older years without independent
verification for that period, since neither BBSR nor we kept a license
history before that date). Historized via
[`snap_dataset_license.sql`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dbt/socioeconomic_data_pipeline/snapshots/snap_dataset_license.sql)
/ `stg_dataset_license_history`.

Covers every dataset whose loader upserts a `loaded_data.dataset_metadata`
row — the `meta.yaml`-driven Kreis/Gemeinden datasets as well as
`arbeitslosenzahlen-afa-agenturbezirke` (own upsert in
`load_arbeitslosenzahlen_afa.py`).

## Metrics

| Method | Path | Purpose |
|---|---|---|
| GET | `/metrics` | Statistical aggregations over socioeconomic datasets |
| GET | `/metrics/derived` | Catalog of derived metrics (from `configs/metrics_catalog.yaml`) |
| GET | `/metrics/derived/{slug}` | A single entry from the derived-metrics catalog |

Both statistics endpoints read harmonized datasets from
`mart_nuts3_sociodata_harmonized` for one NUTS-3 version, selected with
`?nuts_version=` (2003–2024, default 2024, like `/{slug}/timeseries`), so
every region counts once per year and `geodata_id_nuts` refers to that
version. Derived metrics with an area denominator use the area of the same
NUTS version. The response states the version in `nuts_version`; it is
absent where a column has no harmonized counterpart
(`beschaeftigungsquote-gemeinden`, and the median/unweighted/count columns
of `beschaeftigungsquote-nuts3`), which are still computed from the staging
tables as before.

Only valid values enter the statistics and the ranking: `has_valid_value`
of the staging models, and for harmonized values only when every
contributing source value was valid (`mart_nuts3_sociodata_harmonized.
has_valid_value`). Each result reports what was left out instead of
dropping it silently: `excluded_invalid_count` per year and in total,
`excluded_regions` (distinct regions with at least one excluded value) and,
for derived metrics (numerator and denominator must both be valid),
`regions_without_geometry` (area denominator without a matching NUTS-3
geometry) and `regions_without_denominator`. `meta.note` states this rule;
years with only invalid values are missing from `by_year` but counted in the
totals. `GET /metrics` computes its dataset columns in parallel (at most 4
at a time) and keeps the request order.

## Generic socioeconomic-data routes

In addition to the hand-written routes above, `RegisterSoziodataRoutes`
registers **automatically generated** routes – one endpoint per dataset slug
from the dbt schema YAMLs (e.g. `bevoelkerung`, `schuldnerquote`,
`arbeitslose`, ...). These are produced by `cmd/gen-models`, see
[`handlers_soziodata_generated.go`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/backend/internal/handlers/handlers_soziodata_generated.go).

### Gap-free time series (`fill_gaps` and `/timeseries`)

13 of the single-value generated dataset endpoints fed into
[`mart_nuts3_sociodata_harmonized`](../pipeline/dbt.md#interpolation-methods-and-evaluation),
expose interpolated values directly on the standard route via `fill_gaps=true`:

```
GET /api/v1/wohngeldhaushalte?geodata_id_nuts=DE226&fill_gaps=true
```

The endpoint keeps the usual dataset-specific response shape and adds the
optional interpolation metadata fields `is_interpolated`, `value_linear`,
`value_peer_trend` and `n_peer_regions` when the fill-gaps output view is
used. This works through per-dataset dbt views (`api_*_fill_gaps`) that join
the generic timeseries mart back onto API-shaped region metadata.

For consumers that want the reduced, explicitly timeseries-shaped payload,
harmonized datasets also expose a dedicated endpoint:

```
GET /api/v1/wohngeldhaushalte/timeseries?geodata_id_nuts=DE226
```

The `/timeseries` endpoint switches from the raw per-dataset table to
`transformed_data.mart_nuts3_timeseries_interpolated` and returns **one
entry per year in the known range**, including years that were originally
missing, transparently flagged instead of a single, silently-chosen number:

```json
{
  "nuts_id": "DE226",
  "dataset_slug": "wohngeldhaushalte",
  "year": 2021,
  "value": 8.775,
  "is_interpolated": true,
  "value_linear": 8.775,
  "value_peer_trend": 8.500951924915944,
  "n_peer_regions": 11
}
```

- `value` is the value actually used downstream (original value, or the
  linear-interpolation estimate if the year was missing).
- `is_interpolated` flags whether the row was estimated rather than an
  original observation.
- `value_linear` / `value_peer_trend` are only populated for interpolated
  rows and expose **both** estimation methods side by side (linear
  interpolation vs. peer-region trend carry-forward, see
  [dbt: Interpolation methods and evaluation](../pipeline/dbt.md#interpolation-methods-and-evaluation))
  so consumers can decide which one to trust rather than the API silently
  picking one. `value_peer_trend` can be `null` if it wasn't computable.
- `n_peer_regions` reports how many regions of the same NUTS-2 group
  contributed to `value_peer_trend`.

`/timeseries` only supports the `geodata_id_nuts`, `nuts_parent_id`,
`year`, `from_year` and `to_year` filters (not `geodata_name_contains` or
`point`, since those require the original per-dataset columns that don't
exist in the interpolated mart).

`beschaeftigungsquote-gemeinden` is the LAU-grain counterpart: it is fed by
[`mart_lau_sociodata_harmonized`](../pipeline/dbt.md#lau-version-harmonization)
instead and exposes the same `fill_gaps=true` / `/timeseries` mechanism, but
with `lau_version` (2011–2024) instead of `nuts_version`, `geodata_id_lau`
instead of `geodata_id_nuts` for a direct region filter (`nuts_parent_id`
still works, filtering via the canonical LAU→NUTS-3 link in
`lau_integrated.nuts3_id`), and `gisco_id`/Kreis-peer-group fields instead
of `nuts_id`/NUTS-2-peer-group ones in the timeseries response.

`beschaeftigungsquote-nuts3` remains a special case: its route still has a
dedicated `/timeseries` endpoint, but `fill_gaps=true` is not supported on the
standard route because the generic interpolation mart only models the primary
weighted percentage and not the additional columns
`beschaeftigungsquote_median`, `beschaeftigungsquote_ungewichtet` and
`gemeinden_count`.

## ID conventions

There is deliberately **no** additional artificial "socio ID" – instead, the
existing geodata references serve as the domain keys (`nuts_id`, `gisco_id`,
`geodata_id_nuts`/`geodata_id_lau`/`geodata_id_ars`). Details and rationale:
[ID decision](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/backend/docs/id-decision.md).
