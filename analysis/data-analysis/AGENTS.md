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
# nuts3_crosswalk_evaluation additionally needs pyarrow (parquet cache); run from
# analysis/data-analysis/ so its results/*.csv land there. CW_EVAL_CACHE (default
# <tmp>/nuts3_crosswalk_eval) caches the ~3-4 min geometry intersections; CW_EVAL_REFRESH=1 recomputes.
pip install pyarrow
cd analysis/data-analysis && jupyter nbconvert --to notebook --execute --inplace nuts3_crosswalk_evaluation.ipynb
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

`nuts3_crosswalk_evaluation.ipynb` evaluates the population weighting of the
NUTS-3 version crosswalk (`int_lau_nuts3_membership` /
`int_lau_nuts3_population_weight` / `mart_nuts_version_crosswalk`) empirically
before the method is fixed. It rebuilds the candidate methods itself in
SQL/pandas (read-only, temp tables only; it does not use or depend on the dbt
crosswalk models): A = whole Gemeinde to the region with the largest overlap
(old state, incl. per-pair area fallback), A0 = A without that fallback,
B = population split by Gemeinde area share with `lau_nuts3_snap_share`
0.75/0.90/0.95/0.99/1.0 (new state), C = pure NUTS area share, D = Gemeinde
point-on-surface. Truth: Gemeinde population 2019 (finer than the 2023
Gemeinden production uses) aggregated by the Eurostat-declared NUTS code
(targets 2016/2021) or by the historical AGS-5 Kreis of the LAU 2011/2015
geometry vintages (2003-2013); Kreise whose truth is ambiguous (Gemeinden
already split across old Kreise in 2011, e.g. Potsdam/Dessau/Sachsen-Anhalt
2003) only get bounds. Outputs MAPE/bias per target version, error
distribution, worst Kreise, the known examples, a synthetic and a real
(`sgb_ii_quote`) ratio test, and a German Fazit cell.
`results/nuts3_crosswalk_eval_*.csv` are its last outputs. Needs
`stg_lau_population`, `stg_lau_geometry`, `int_lau_population_geo`,
`mart_nuts_geometry_historized`, `stg_beschaeftigungsquote_gemeinden`,
`stg_sgb_ii_quote_kreise` built. Checked in with outputs already executed.
