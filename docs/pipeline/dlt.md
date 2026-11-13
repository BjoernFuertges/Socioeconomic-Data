# dlt – Extract & Load

Python scripts based on [dlt (data load tool)](https://dlthub.com/) that
load raw data unchanged into PostgreSQL (schema `loaded_data`). Each script
can be run standalone:

```bash
cd pipeline/dlt
python load_<name>.py [args...]
```

Configuration (target DB connection, etc.) via `pipeline/dlt/config.toml` or
`pipeline/dlt/.dlt/secrets.toml` (generate locally from `.secrets.toml.example`).

## Scripts

| Script | Purpose | Target tables (`loaded_data`) |
|---|---|---|
| [`load_arbeitslosenzahlen_afa.py`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dlt/load_arbeitslosenzahlen_afa.py) | Monthly unemployment/underemployment figures per Agenturbezirk (Bundesagentur-für-Arbeit district partition, distinct from NUTS-3/LAU/Kreise), pulled live from the official "Lange Zeitreihen" REST API on every run | `results_arbeitslosenzahlen_afa_agenturbezirke`, `dataset_metadata` |
| [`load_agenturbezirke_geodata.py`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dlt/load_agenturbezirke_geodata.py) | Agenturbezirk boundaries (official BA-Gebietsstruktur shapefile, all ~146 districts) | `agenturbezirk_geometry` |
| [`fetch_lau_population.py`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dlt/fetch_lau_population.py) | Not a dlt pipeline: downloads Eurostat's yearly LAU-NUTS correspondence tables and writes the German sheet as `lau_population_<year>.csv` (input for the next script). One snapshot per publication year, used as LAU population weight, not as an official population series | – (files only) |
| [`fetch_ba_sv_beschaeftigte.py`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dlt/fetch_ba_sv_beschaeftigte.py) | Not a dlt pipeline: builds the Kreis dataset `sv_beschaeftigte` (employees subject to social insurance contributions at place of work, June value per year from 2007) from the BA's auth-free `EckwerteZeitreiheBST` API and writes it in the generic `mapped_pairs.csv`/`meta.yaml` format (see "BA employees" below) | – (files only, loaded by `load_soziodata_generic.py`) |
| [`load_geodata_to_postgis.py`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dlt/load_geodata_to_postgis.py) | LAU/NUTS geometries (GeoJSON) + LAU population (CSV) | `lau_geometry`, `lau_population`, `nuts_geometry` |
| [`load_beschaeftigungsquote.py`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dlt/load_beschaeftigungsquote.py) | Employment rate (municipalities/LAU), standalone alternative loader. `run_all_pipelines.sh` loads this dataset with `load_soziodata_generic.py` instead | `results_beschaeftigungsquote_gemeinden` |
| [`load_soziodata_generic.py`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dlt/load_soziodata_generic.py) | generic format `mapped_pairs.csv` + `meta.yaml` for all remaining county-level (Kreis) datasets (population, debtor quota, work volume, ...) | `results_<dataset>_kreise`, `dataset_metadata` |

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
  (all except `sv_beschaeftigte`, which sets its BA license explicitly),
  [`bbsr_license.py`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dlt/bbsr_license.py)
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
[`stg_dataset_metadata.sql`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dbt/socioeconomic_data_pipeline/models/staging/stg_dataset_metadata.sql)
/ `mart_dataset_catalog.sql` downstream and the `/datasets` API endpoint
in [../backend/api.md](../backend/api.md).

All datasets loaded in production are listed in
[`run_all_pipelines.sh`](https://github.com/BjoernFuertges/Socioeconomic-Data/blob/main/pipeline/dlt/run_all_pipelines.sh).

### BA employees (`fetch_ba_sv_beschaeftigte.py`)

Unlike the BBSR datasets, which are prepared by hand (geo-mapper) and
synced from Nextcloud, `sv_beschaeftigte` is built by a script: it reads
the BA Kreis list, fetches each Kreis' monthly series (~400 requests) and
keeps the June value of every complete year. NUTS-3 IDs come from a lookup
`geodata_id_ars` → `geodata_id_nuts` in an existing NUTS-2024 Kreis
dataset (`--nuts-lookup`), not from name matching. The BA publishes its
series on its current territorial status: Kreise that did not exist in
NUTS 2024 are added to the Kreis they belonged to (currently Hanau,
kreisfrei since 2026, into Main-Kinzig-Kreis; marked as own calculation
in `meta.yaml`); anything else without a match goes to `unmapped.csv`.
This merge is valid for NUTS 2024 only: NUTS 2027 makes Hanau its own
NUTS-3 region, so with a 2027 lookup the merge map must be emptied (the
script refuses the combination).
`meta.yaml` sets the BA's license/attribution explicitly, so the BBSR
license lookup does not apply.

`run_all_pipelines.sh` calls it before the loads, but it skips when the
output already exists (`--force` rebuilds, `--cache-dir` reuses raw
responses), so a normal pipeline run makes no requests. Without the call
a missing folder would make `load_soziodata_generic.py` exit 1 and abort
the whole run. On the server, `data-exchange/1_friedrich-bjoern` is an
`rclone sync` mirror of Nextcloud (deletes files not in Nextcloud, every
10 minutes), so the generated folder should also be uploaded to Nextcloud
there; otherwise every pipeline run fetches it again.

### Geodata: Geodaten folder with legacy fallbacks

`load_geodata_to_postgis.py` prefers the `Geodaten` folder of the data
exchange (`geojson/LAU/<year>/lau_<year>_level_0.geojson`,
`geojson/NUTS_<level>/<year>/nuts_<year>_level_<level>.geojson`). In
production it is mirrored to `data-exchange/geodata`; locally it may sit in
`<DATA_EXCHANGE_PATH>/Geodaten` (override: `GEODATEN_PATH`). Those files are
WGS84 and only carry ids and names, so the loader reprojects them to
EPSG:3035 and derives the columns dbt expects (`gisco_id = DE_<AGS>`,
`levl_code` from the file name, `cntr_code`). The folder holds Germany only,
LAU 2011-2024, NUTS levels 2 and 3 only (no levels 0/1).

If the folder is absent, the loader looks for pre-processed files in the
configured `GEODATA_BASE_PATH` output (compressed, `*.geojson.gz`) and
otherwise falls back to uncompressed files in local `geo-mapper/` or
`geo-importer/` checkouts next to the repo (environment variables
`LAU_GEODATA_PATH`, `LAU_POPULATION_PATH`, `NUTS_GEODATA_PATH`); these are
external tools, not directories of this repository. Geodata and population data run
as separate dlt pipelines to avoid stale state between runs.

## write_disposition

All dataset-content resources load with `write_disposition="replace"` –
each run replaces the entire table content. The only exception is the
shared `dataset_metadata` table, which is upserted (`"merge"` on
`dataset_slug`) by `load_soziodata_generic.py` and
`load_arbeitslosenzahlen_afa.py`. Historization over time is handled by **dbt**
via snapshots (see [dbt](dbt.md#snapshots)), not by dlt itself.
