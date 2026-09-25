# AGENTS.md

This file provides guidance to AI coding agents and contributors working in this part of the repository.

Scope: this file covers the `analysis/data-analysis/` directory — ad-hoc/exploratory
analysis scripts, not part of the production pipeline (backend/dbt/dlt).
See the repository root [AGENTS.md](../AGENTS.md) for the overall
architecture these scripts read data from.

## Commands

```bash
# Needs a local Postgres/PostGIS with loaded_data populated (see pipeline/dlt/run_all_pipelines.sh)
source pipeline/dlt/.venv/bin/activate   # reuses the dlt venv (pandas/numpy/psycopg2), run from repo root
python analysis/data-analysis/impute_missing_values.py

# Notebooks additionally need matplotlib + a Jupyter kernel, not part of
# pipeline/dlt/requirements.txt (that file is the pinned production pipeline install
# list) - install once into the same venv:
pip install matplotlib ipykernel nbformat nbconvert
jupyter nbconvert --to notebook --execute --inplace analysis/data-analysis/agentur_nuts3_crosswalk_shares.ipynb
jupyter nbconvert --to notebook --execute --inplace analysis/data-analysis/impute_missing_values_evaluation.ipynb
```

Connection defaults to `localhost:5432` / db `soziooekonomische-daten` /
user+password `sozio_data_compose_user`; override via
`PGHOST`/`PGPORT`/`PGDATABASE`/`PGUSER`/`PGPASSWORD`. The script's own
docstring includes a `docker run postgis/postgis:17-3.5` one-liner for
spinning up a matching local DB.

## Architecture

`impute_missing_values.py` compares gap-filling strategies for missing
values in the Kreis-level time series (`results_*_kreise`, keyed by
`geodata_id_nuts`) read directly from `loaded_data` (raw, pre-dbt) — it
evaluates candidates for the logic that
`pipeline/dbt/socioeconomic_data_pipeline/macros/render_nuts3_fill_gaps_view.sql`
implements in production, it does not run inside the pipeline itself. Four
methods are evaluated side by side: linear interpolation, naive
carry-forward (LOCF, a trivial baseline), NUTS-2 peer-based extrapolation,
and OLS regression over the other 12 Kreis indicators. Two modes, both in
the same file:

- `demo_known_gaps()` — demonstrates the two actually-occurring gaps in
  the dataset (Kelheim/wohngeldhaushalte/2021,
  Neitersen/beschaeftigungsquote_gemeinden/2020).
- `run_evaluation()` — leave-one-out evaluation: hides a real known value,
  estimates it with all four methods, then scores each
  (MAE/median AE/RMSE/std/MAPE/median APE/bias/P90/P95/max/hit-rate). Only
  runs against the 13 Kreis datasets (the Gemeinden dataset has no NUTS-3
  code to peer-group or regress on; naive carry-forward and linear
  interpolation still apply to it). Regression matches
  `beschaeftigungsquote_gemeinden` differently since it lacks a NUTS-3
  key.

`results/*.csv` are the last-generated evaluation outputs (checked in as
reference, not regenerated automatically — rerun the script to refresh
them).

`impute_missing_values_evaluation.ipynb` re-runs the same leave-one-out
evaluation (`imp.run_evaluation`/`imp.summarize_evaluation`, imported
directly from `impute_missing_values.py` — no separate estimation logic)
and visualizes it: MAPE vs. median APE per method, a log-scale boxplot of
the per-sample percentage errors, a per-dataset heatmap of MAE relative to
linear interpolation, and the hit-rate chart. Checked in with outputs
already executed — rerun via `jupyter nbconvert --to notebook --execute
--inplace` (see Commands above) to refresh against current data.

`agentur_nuts3_crosswalk_shares.ipynb` visualizes the distribution of
`area_share_of_agentur`/`area_share_of_nuts3` in
`transformed_data.mart_agentur_nuts3_crosswalk`
(`pipeline/dbt/socioeconomic_data_pipeline/models/marts/mart_agentur_nuts3_crosswalk.sql`)
— histograms + a log-log scatter of the two share dimensions, plus a
threshold-sensitivity table (rows kept/dropped and whether any
Agenturbezirk would lose all its NUTS-3 links) for candidate cutoffs. Used
to size the sliver-filter threshold in that dbt model (small
boundary-digitization mismatches between the BA Agenturbezirk shapefile
and NUTS-3/GISCO create tiny non-zero overlaps with neighboring Kreise
that aren't real membership). Checked in with outputs already executed —
rerun via `jupyter nbconvert --to notebook --execute --inplace` (see
Commands above) to refresh against current data, not regenerated
automatically.
