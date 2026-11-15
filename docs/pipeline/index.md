# Data Pipeline – Overview

The pipeline consists of two tools with a clear division of labor:

- **[dlt](dlt.md)** (`pipeline/dlt/`) – Extract & Load: reads raw data (CSV/GeoJSON)
  and writes it **unchanged** into PostgreSQL, schema `loaded_data`.
- **[dbt](dbt.md)** (`pipeline/dbt/socioeconomic_data_pipeline/`) – Transform:
  historizes, cleans, harmonizes, and aggregates the data into views/tables
  in the `transformed_data` schema, which are read by the backend.

```mermaid
flowchart LR
    raw["Raw data\n(data-exchange/, geo-mapper-output/)"] --> dlt["dlt\n(Extract & Load)"]
    dlt --> loaded[("loaded_data")]
    loaded --> snap["dbt snapshots\n(historization)"]
    loaded --> stg["dbt staging\n(views, 1:1 cleaned)"]
    snap --> stg
    stg --> intermediate["dbt intermediate\n(building blocks, e.g. crosswalks)"]
    intermediate --> marts["dbt marts\n(tables, finished facts)"]
    stg --> marts
    marts --> api["Backend API"]
```

## Order of a full run

See [`pipeline/dlt/run_all_pipelines.sh`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dlt/run_all_pipelines.sh):

1. All dlt scripts run (unemployment figures, geodata, generic
   socioeconomic datasets) → populate `loaded_data`.
2. `dbt deps` – install packages (`dbt_utils`).
3. `dbt test --select source:loaded_data --indirect-selection=cautious` – source tests (e.g. unique
   `(geodata_id_nuts, year)`); a failure aborts the run before the snapshot,
   which would otherwise hide key collisions.
4. `dbt snapshot` – advance historized snapshots from `loaded_data`.
5. `dbt run` – rebuild staging/intermediate/marts.

In production this happens inside the `pipeline` container (see
[Backend deployment](../backend/deployment.md#manually-running-a-changed-pipeline-on-the-server)).
