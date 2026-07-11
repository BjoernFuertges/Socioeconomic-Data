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

## ID conventions

There is deliberately **no** additional artificial "socio ID" – instead, the
existing geodata references serve as the domain keys (`nuts_id`, `gisco_id`,
`geodata_id_nuts`/`geodata_id_lau`/`geodata_id_ars`). Details and rationale:
[ID decision](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/backend/docs/id-decision.md).
