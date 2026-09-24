# API Endpoints

All endpoints live under the `/api/v1` prefix. The full, generated
OpenAPI/Swagger specification is available at `/api/docs/index.html` (see
[`backend/docs/swagger.yaml`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/backend/docs/swagger.yaml)).

Defined in [`internal/routes/routes.go`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/backend/internal/routes/routes.go).

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

Implemented once in
[`internal/handlers/negotiate.go`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/backend/internal/handlers/negotiate.go)
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
| GET | `/health` | Health check |

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
| GET | `/nuts/{nuts_id}/versions` | Available NUTS versions (2013/2016/2021/2024) of a region |
| GET | `/nuts/{nuts_id}/datasets` | Datasets available for the region |

## Geodata (generic, LAU + NUTS)

| Method | Path | Purpose |
|---|---|---|
| GET | `/geo` | List of all geo regions |
| GET | `/geo/{geo_id}` | A single geo region |
| GET | `/geo/{geo_id}/versions` | Available versions of a geo region |
| POST | `/geo/query` | Spatial query by geometry (e.g. point/polygon) |

## Agenturbezirke (AfA unemployment figures)

Agenturbezirke are a Bundesagentur-für-Arbeit-internal district partition,
distinct from and not congruent with NUTS-3/LAU/Kreise — complements
(does not replace) the Kreis-level `arbeitslose` dataset available through
the generic socioeconomic-data routes below.

| Method | Path | Purpose |
|---|---|---|
| GET | `/agenturbezirke` | List of all Agenturbezirke, with geometry |
| GET | `/agenturbezirke/{ba_schl}` | A single Agenturbezirk |
| GET | `/agenturbezirke/{ba_schl}/arbeitslosigkeit` | Monthly unemployment/underemployment series (since Jan 2007) |
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
[`mart_dataset_catalog.sql`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dbt/socioeconomic_data_pipeline/models/marts/mart_dataset_catalog.sql).

Reads `transformed_data.mart_dataset_catalog` (name, description, unit,
provenance, license/license_url, geodata level/year, year coverage, and a
persistent `dataset_uri` of the form `urn:soziodata:dataset:<slug>`).
`license` is the literal string `"unbekannt"` rather than a bare `null`
for any dataset whose usage terms couldn't be determined — see
[`stg_dataset_metadata.sql`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dbt/socioeconomic_data_pipeline/models/staging/stg_dataset_metadata.sql).
For BBSR-sourced datasets (currently all of them) the license is looked
up automatically and kept up to date by
[`dlt/bbsr_license.py`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/bbsr_license.py)
— no manual research needed, see [dlt.md](../pipeline/dlt.md).

Two additional fields make the license's temporal coverage honest:
`license_tracked_since` (date the license was first observed for this
dataset) and `license_backfilled` (`true` if the dataset has data years
older than `license_tracked_since` — the current license was then
retroactively applied to those older years without independent
verification for that period, since neither BBSR nor we kept a license
history before that date). Historized via
[`snap_dataset_license.sql`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dbt/socioeconomic_data_pipeline/snapshots/snap_dataset_license.sql)
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

## Generic socioeconomic-data routes

In addition to the hand-written routes above, `RegisterSoziodataRoutes`
registers **automatically generated** routes – one endpoint per dataset slug
from the dbt schema YAMLs (e.g. `bevoelkerung`, `schuldnerquote`,
`arbeitslose`, ...). These are produced by `cmd/gen-models`, see
[`handlers_soziodata_generated.go`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/backend/internal/handlers/handlers_soziodata_generated.go).

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
[ID decision](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/backend/docs/id-decision.md).
