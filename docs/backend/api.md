# API Endpoints

All endpoints live under the `/api/v1` prefix. The full, generated
OpenAPI/Swagger specification is available at `/api/docs/index.html` (see
[`backend/docs/swagger.yaml`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/backend/docs/swagger.yaml)).

Defined in [`internal/routes/routes.go`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/backend/internal/routes/routes.go).

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

## Unemployment figures

| Method | Path | Purpose |
|---|---|---|
| GET | `/unemployment` | Raw data |
| GET | `/unemployment/aggregated` | Aggregated values |
| GET | `/unemployment/geodata/{geodata_id}` | Values for a specific geodata ID |

## Socioeconomic inventory

| Method | Path | Purpose |
|---|---|---|
| GET | `/socioeconomic/inventory` | Overview of all available socioeconomic datasets |

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
exist in the interpolated mart). `beschaeftigungsquote-gemeinden` has no
`/timeseries` endpoint.

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
