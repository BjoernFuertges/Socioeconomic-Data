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
| [`load_arbeitslosenzahlen_afa.py`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/load_arbeitslosenzahlen_afa.py) | Unemployment figures from the Agentur für Arbeit (transposed/normalized) | `arbeitslosenzahlen_agentur_arbeit`, `arbeitslosenzahlen_metadata` |
| [`load_geodata_to_postgis.py`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/load_geodata_to_postgis.py) | LAU/NUTS geometries (GeoJSON) + LAU population (CSV) | `lau_geometry`, `lau_population`, `nuts_geometry` |
| [`load_beschaeftigungsquote.py`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/load_beschaeftigungsquote.py) | Employment rate (special case: municipalities/LAU) | `results_beschaeftigungsquote_gemeinden` |
| [`load_soziodata_generic.py`](https://gitlab.htw-berlin.de/s0595954/soziooekonomische-daten/-/blob/main/dlt/load_soziodata_generic.py) | generic format `mapped_pairs.csv` + `meta.yaml` for all remaining county-level (Kreis) datasets (population, debtor quota, work volume, ...) | `results_<dataset>_kreise` |

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
- `meta.yaml` – `geodata_level`, `geodata_year`

The table name is derived from the last path segment
(`results_bevoelkerung_kreise` → table `results_bevoelkerung_kreise`).

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
