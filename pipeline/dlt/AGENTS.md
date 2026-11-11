# AGENTS.md

This file provides guidance to AI coding agents and contributors working in this part of the repository.

Scope: this file covers the `pipeline/dlt/` directory only. See the repository
root [AGENTS.md](../../AGENTS.md) for cross-component architecture and the
"dlt never historizes" rule, and
[docs/pipeline/dlt.md](../../docs/pipeline/dlt.md) for the per-script writeup.

## Commands

To run the pipeline together with the database, backend, and frontend in
one go instead of setting each up by hand, use `compose.local.yml` at the
repo root (`docker compose -f compose.local.yml up --build`, then
`docker compose -f compose.local.yml exec pipeline bash /app/dlt/run_all_pipelines.sh`)
— see the root [README.md](../../README.md#lokal-testen-docker-compose). The
commands below are for running dlt scripts standalone against an
already-running/external Postgres.

```bash
# Needs pipeline/dlt/.dlt/secrets.toml (copy .secrets.toml.example, which
# sits in pipeline/dlt/) and a reachable Postgres. Every script sets
# dataset_name="loaded_data" in code, so loads land in the loaded_data
# schema (config.toml's `schema = "store"` is not the target).
python load_arbeitslosenzahlen_afa.py
python fetch_lau_population.py       # before load_geodata_to_postgis.py
python load_geodata_to_postgis.py
python load_agenturbezirke_geodata.py
python load_soziodata_generic.py <dataset>/results_<dataset>_kreise

# Run all loaders except load_beschaeftigungsquote.py, then dbt deps/snapshot/run
# (deployed pipeline container; dbt dir defaults to /app/dbt/..., override via DBT_PROJECT_DIR):
./run_all_pipelines.sh
```

## Architecture

- `load_soziodata_generic.py` is the workhorse: a single generic
  loader, parameterized by a `<dataset>/results_<dataset>_kreise`-style
  path argument, that reads `mapped_pairs.csv` + `meta.yaml` from
  `data-exchange/.../<dataset>/results_<dataset>_kreise/` and loads it
  into `loaded_data.results_<dataset>_kreise`. Adding a new Kreis-level
  dataset means adding the right files under `data-exchange/` and calling
  this script with the new path — not writing a new loader script (see
  `run_all_pipelines.sh` for the full list of currently registered
  datasets, and the "Adding a new Kreis-level dataset" recipe in
  [AGENTS.md](../../AGENTS.md)); a new dataset also needs a line in
  `run_all_pipelines.sh`. It also upserts (`write_disposition="merge"`
  on `dataset_slug`, unlike the data resource's `"replace"`) one row per
  dataset into the shared `loaded_data.dataset_metadata` table from
  `meta.yaml`'s provenance/license/description fields — downstream in
  dbt: `stg_dataset_metadata` / `mart_dataset_catalog`, exposed via the
  backend's `GET /datasets`.
- `load_arbeitslosenzahlen_afa.py`, `load_beschaeftigungsquote.py`,
  `load_geodata_to_postgis.py` are special-purpose loaders for sources that
  don't fit the generic `mapped_pairs.csv`/`meta.yaml` shape (Arbeitsagentur
  API, employment-quota data, geodata into PostGIS).
  `load_beschaeftigungsquote.py` is not called by `run_all_pipelines.sh`.
- `load_arbeitslosenzahlen_afa.py` pulls monthly unemployment figures per
  **Agenturbezirk** (Bundesagentur-für-Arbeit-internal district partition,
  not NUTS-3/LAU/ARS) from the official, auth-free "Lange Zeitreihen" REST
  API (`EckwerteZeitreiheALO?Agentur für Arbeit=<name>`, see
  `API-ALO-Zeitreihe.html`). It fetches the current Agenturbezirk list
  (name + `BA_SCHL`) from `DR-LZR-persistListenfelderJSON.js` on every run,
  then requests each district's full monthly series (back to Jan 2007) -
  no manual data-exchange preparation needed, unlike the generic
  Kreis-dataset recipe. Individual districts that fail to fetch are
  skipped and logged, not fatal. Complements (does not replace) the
  Kreis-level `arbeitslose` dataset (INKAR/BBSR, loaded via
  `load_soziodata_generic.py`) - both stay side by side, since
  Agenturbezirke and Kreise are different, non-overlapping geographies.
  Also upserts its own `dataset_metadata` row (same mechanism as
  `load_soziodata_generic.py`), with an explicit, hand-set license (the
  source states its own reuse terms directly in every response, not a
  named license like the BBSR ones).
- `fetch_lau_population.py` downloads Eurostat's yearly LAU-NUTS
  correspondence tables (one EU-wide workbook per year, "DE" sheet
  extracted) into `<GEODATA_BASE_PATH>/statistics/lokale_verwaltungseinheiten_eu/output/lau_population_<year>.csv`,
  the exact shape `load_geodata_to_postgis.py`'s primary LAU-population
  path expects. Idempotent (skips years whose output CSV already exists,
  `--force` to redo) and part of `run_all_pipelines.sh`, run before
  `load_geodata_to_postgis.py` - without it (e.g. on a fresh checkout with
  no cached downloads), `load_geodata_to_postgis.py` silently falls back
  to a `geo-importer`-sourced CSV (external sibling repo, not part of this repo) that only has `ID,Name` columns, leaving
  `gisco_id`/`nuts_3_code`/`population` NULL for every LAU row.
- `load_agenturbezirke_geodata.py` loads the official BA-Gebietsstruktur
  shapefile (all ~146 Agenturbezirke, same source ZIP as
  `analysis/arbeitsagentur/fetch_geodata.sh` used for its 3-district
  proof of concept) into `loaded_data.agenturbezirk_geometry`, reprojected
  to WGS84 at load time (unlike `load_geodata_to_postgis.py`'s NUTS/LAU
  geometry, which stays in EPSG:3035 and is transformed in dbt staging).
- `write_disposition` is `"replace"` for all data resources and `"merge"`
  (key `dataset_slug`) for `dataset_metadata`; `"append"` is a bug, not a
  pattern (see [AGENTS.md](../../AGENTS.md), "dlt never historizes"). Because
  `"replace"` swaps the whole table, loaders must fail (not skip) on
  missing/partial input, otherwise a partial extract overwrites good data.
  A missing `mapped_pairs.csv`/`meta.yaml` makes `load_soziodata_generic.py`
  exit 1.
- `bbsr_license.py` supplies `license`/`license_url` for the metadata
  resource automatically — no meta.yaml maintenance needed. Every BBSR-
  sourced dataset (currently all of them, detected via `source` mentioning
  "BBSR") has its license looked up via `bbsr_license.get_license()`
  inside `load_soziodata_generic.py`'s metadata resource on every run: a
  fresh (<24h) on-disk cache (`pipeline/dlt/.dlt/bbsr_license_cache.json`,
  gitignored) is used as-is, otherwise it scrapes the current value from
  BBSR's INKAR page and refreshes the cache; if that live fetch fails it
  falls back to a stale cache rather than failing the load, and only
  falls back to `"unbekannt"` if there's no cache at all yet. Setting
  `license`/`license_url` explicitly in a dataset's `meta.yaml` overrides
  the lookup (for a future non-BBSR source). Run
  `python bbsr_license.py` standalone to see the current live value
  without touching any pipeline data.
- `run_all_pipelines.sh` is the single source of truth for pipeline
  ordering: geodata + all `results_*_kreise` datasets load first, then
  `dbt deps && dbt snapshot && dbt run` runs against the freshly loaded
  data. Any macro relying on `adapter.get_columns_in_relation` (e.g. the
  snapshot query macro) requires this ordering — dbt must never run before
  the corresponding dlt load in a given pipeline invocation.
