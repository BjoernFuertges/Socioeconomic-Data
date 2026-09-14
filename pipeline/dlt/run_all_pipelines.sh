#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

echo "=== dlt: Geodaten ==="
python load_geodata_to_postgis.py
python load_agenturbezirke_geodata.py

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
python load_soziodata_generic.py wohngeldhaushalte/results_wohngeldhaushalte_kreise

echo "=== dlt: Arbeitslosenzahlen (Agenturbezirke, AfA Lange Zeitreihen) ==="
python load_arbeitslosenzahlen_afa.py

echo "=== dbt ==="
cd /app/dbt/socioeconomic_data_pipeline
dbt deps
dbt snapshot

echo "=== dbt run ==="
dbt run

echo "=== Pipeline abgeschlossen ==="
