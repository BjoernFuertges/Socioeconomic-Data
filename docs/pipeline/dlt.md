# dlt – Extract & Load

Python scripts based on [dlt (data load tool)](https://dlthub.com/) that
load raw data unchanged into PostgreSQL (schema `loaded_data`). Each script
can be run standalone:

```bash
cd dlt
python load_<name>.py [args...]
```

Configuration (target DB connection, etc.) via `dlt/config.toml` or
`dlt/.dlt/secrets.toml` (generate locally from `.secrets.toml.example`).

## Scripts

| Script | Purpose | Target tables (`loaded_data`) |
|---|---|---|
| [`load_arbeitslosenzahlen_afa.py`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/load_arbeitslosenzahlen_afa.py) | Monthly unemployment/underemployment figures per Agenturbezirk (Bundesagentur-für-Arbeit district partition, distinct from NUTS-3/LAU/Kreise), pulled live from the official "Lange Zeitreihen" REST API on every run | `results_arbeitslosenzahlen_afa_agenturbezirke`, `dataset_metadata` |
| [`load_agenturbezirke_geodata.py`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/load_agenturbezirke_geodata.py) | Agenturbezirk boundaries (official BA-Gebietsstruktur shapefile, all ~146 districts) | `agenturbezirk_geometry` |
| [`load_geodata_to_postgis.py`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/load_geodata_to_postgis.py) | LAU/NUTS geometries (GeoJSON) + LAU population (CSV) | `lau_geometry`, `lau_population`, `nuts_geometry` |
| [`load_beschaeftigungsquote.py`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/load_beschaeftigungsquote.py) | Employment rate (special case: municipalities/LAU) | `results_beschaeftigungsquote_gemeinden` |
| [`load_soziodata_generic.py`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/load_soziodata_generic.py) | generic format `mapped_pairs.csv` + `meta.yaml` for all remaining county-level (Kreis) datasets (population, debtor quota, work volume, ...) | `results_<dataset>_kreise`, `dataset_metadata` |

### Generic format (`load_soziodata_generic.py`)

Most socioeconomic datasets follow the same layout, so a single script
covers all of them:

```bash
python load_soziodata_generic.py <dataset_subdir>
# e.g.
python load_soziodata_generic.py bevoelkerung/results_bevoelkerung_kreise
```

Expected input structure under `<DATA_EXCHANGE_PATH>/<dataset_subdir>/`:

- `mapped_pairs.csv` – columns `original_id_1`, `original_name`,
  `geodata_name`, `geodata_id_nuts`, `geodata_id_ars`, `mapper`, `parameter`,
  plus one column per year
- `meta.yaml` – `geodata_level`, `geodata_year`, `name`, `description`,
  `description_long`, `unit`, `source`, `algorithm`, `number_type`, and
  optionally `license`/`license_url` as a manual override. Normally these
  two are left unset: for any dataset whose `source` mentions BBSR
  (currently all of them),
  [`bbsr_license.py`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/bbsr_license.py)
  looks up the license BBSR currently states for "Laufende
  Raumbeobachtung"/INKAR automatically on every pipeline run (cached for
  24h, falls back to a stale cache or `"unbekannt"` rather than failing
  the load if BBSR is unreachable) - no manual research or maintenance
  required.

The table name is derived from the last path segment
(`results_bevoelkerung_kreise` → table `results_bevoelkerung_kreise`).

Every run also upserts (merge on `dataset_slug`) one row into the shared
`loaded_data.dataset_metadata` table with the provenance/license/
description fields from `meta.yaml`, so that information reaches the
database instead of staying on disk only - see
[`stg_dataset_metadata.sql`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dbt/socioeconomic_data_pipeline/models/staging/stg_dataset_metadata.sql)
/ `mart_dataset_catalog.sql` downstream and the `/datasets` API endpoint
in [../backend/api.md](../backend/api.md).

All datasets loaded in production are listed in
[`run_all_pipelines.sh`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/run_all_pipelines.sh).

### Geodata with fallback paths

`load_geodata_to_postgis.py` first looks for pre-processed files in the
configured `GEODATA_BASE_PATH` output (compressed, `*.geojson.gz`) and
otherwise falls back to uncompressed files from `geo-mapper/` or
`geo-importer/` (environment variables `LAU_GEODATA_PATH`,
`LAU_POPULATION_PATH`, `NUTS_GEODATA_PATH`). Geodata and population data run
as separate dlt pipelines to avoid stale state between runs.

## write_disposition

All resources load with `write_disposition="replace"` – each run replaces
the entire table content. Historization over time is handled by **dbt**
via snapshots (see [dbt](dbt.md#snapshots)), not by dlt itself.
