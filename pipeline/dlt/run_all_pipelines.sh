#!/usr/bin/env bash
set -euo pipefail

# Only one run at a time: the hourly ofelia job (its no-overlap only covers ofelia itself), the
# initial load (deploy_remote.sh) and manual `docker exec` runs share this lock. Two concurrent dbt
# snapshots could corrupt the snapshots schema, the one part the pipeline cannot rebuild.
#   PIPELINE_LOCK_FILE       lock file (default: /tmp inside the container, shared by all docker exec runs)
#   PIPELINE_LOCK_BUSY_EXIT  exit code if another run holds the lock (default 75 = EX_TEMPFAIL, so the
#                            initial-load wrapper and manual callers see "not run"; the ofelia job sets 0
#                            in docker-compose.yml, a skipped hourly run is not an error)
lock_file="${PIPELINE_LOCK_FILE:-${TMPDIR:-/tmp}/soziodata_run_all_pipelines.lock}"
if command -v flock > /dev/null 2>&1; then
  exec 9> "${lock_file}"
  if ! flock -n 9; then
    echo "run_all_pipelines.sh läuft bereits (Lock ${lock_file} belegt) - dieser Lauf wird übersprungen." >&2
    exit "${PIPELINE_LOCK_BUSY_EXIT:-75}"
  fi
else
  echo "Warnung: flock nicht gefunden - kein Schutz gegen parallele Läufe." >&2
fi

cd "$(dirname "$0")"

echo "=== dlt: Eurostat LAU-Bevölkerung (Korrespondenztabellen) ==="
python fetch_lau_population.py

echo "=== dlt: Geodaten ==="
python load_geodata_to_postgis.py
python load_agenturbezirke_geodata.py

# Builds data-exchange/.../sv_beschaeftigte/ itself from the BA API (~400
# requests); skipped when the output already exists, so usually offline.
echo "=== dlt: SV-Beschäftigte (BA-Zeitreihe, nur falls noch nicht vorhanden) ==="
python fetch_ba_sv_beschaeftigte.py

echo "=== dlt: Sozioökonomische Daten ==="
python load_soziodata_generic.py arbeitslose/results_arbeitslose_kreise
python load_soziodata_generic.py arbeitsvolumen/results_arbeitsvolumen_kreise
python load_soziodata_generic.py auszubildende/results_auszubildende_kreise
python load_soziodata_generic.py beschaeftigungsquote/results_beschaeftigungsquote_gemeinden
python load_soziodata_generic.py bevoelkerung/results_bevoelkerung_kreise
python load_soziodata_generic.py bruttoverdienst/results_bruttoverdienst_kreise
python load_soziodata_generic.py durchschnittsalter/results_durchschnittsalter_kreise
python load_soziodata_generic.py erwerbstaetige_pro_1000_einwohner/results_erwerbstaetige_pro_1000_einwohner_kreise
python load_soziodata_generic.py kinderarmut/results_kinderarmut_kreise
python load_soziodata_generic.py medianeinkommen/results_medianeinkommen_kreise
python load_soziodata_generic.py schuldnerquote/results_schuldnerquote_kreise
python load_soziodata_generic.py sgb_ii_quote/results_sgb_ii_quote_kreise
python load_soziodata_generic.py studierende/results_studierende_kreise
python load_soziodata_generic.py sv_beschaeftigte/results_sv_beschaeftigte_kreise
python load_soziodata_generic.py wohngeldhaushalte/results_wohngeldhaushalte_kreise

echo "=== dlt: Arbeitslosenzahlen (Agenturbezirke, AfA Lange Zeitreihen) ==="
python load_arbeitslosenzahlen_afa.py

echo "=== dbt ==="
cd "${DBT_PROJECT_DIR:-/app/dbt/socioeconomic_data_pipeline}"
dbt deps
dbt snapshot

echo "=== dbt run ==="
dbt run

echo "=== Pipeline abgeschlossen ==="
