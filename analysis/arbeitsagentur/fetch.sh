#!/usr/bin/env bash
# Ruft die aktuellen Arbeitslosenzahlen der drei Berliner Agenturen für Arbeit
# über die öffentliche Statistik-API der Bundesagentur für Arbeit ab und
# speichert sie als CSV in diesem Ordner.
#
# API-Doku: https://statistik.arbeitsagentur.de/DE/Statischer-Content/Service/API/API-ALO.html
set -euo pipefail

cd "$(dirname "$0")"

BASE_URL="https://statistik-dr.arbeitsagentur.de/bifrontend/bids-api/ct/v1/tableFetch/csv/EckwerteTabelleALOAA"

# DESC-Werte der Berliner Agenturen für Arbeit (aus der offiziellen Auswahlliste)
declare -A AGENTUREN=(
  [berlin-mitte]="AA Berlin Mitte"
  [berlin-nord]="AA Berlin Nord"
  [berlin-sued]="AA Berlin Süd"
)

for slug in "${!AGENTUREN[@]}"; do
  name="${AGENTUREN[$slug]}"
  encoded_name=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$name")
  url="${BASE_URL}?Agentur%20f%C3%BCr%20Arbeit=${encoded_name}"
  outfile="arbeitslose_${slug}.csv"
  echo "Lade ${name} -> ${outfile}"
  curl -sS "$url" -o "$outfile"
done

echo "Fertig."
