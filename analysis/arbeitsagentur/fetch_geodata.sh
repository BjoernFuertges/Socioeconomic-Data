#!/usr/bin/env bash
# Lädt die offizielle BA-Gebietsstruktur (Regionaldirektions- und
# Agenturbezirke) als Shapefile herunter und entpackt sie nach geodata_raw/.
#
# Quelle: https://statistik.arbeitsagentur.de/DE/Navigation/Grundlagen/Klassifikationen/Regionale-Gliederungen/BA-Gebietsstruktur-Nav.html
set -euo pipefail

cd "$(dirname "$0")"

URL="https://statistik.arbeitsagentur.de/DE/Statischer-Content/Grundlagen/Klassifikationen/Regionale-Gliederungen/BA-Gebietsstruktur/Generische-Publikationen/BA-Gebietsstrukturen-shp.zip"

mkdir -p geodata_raw
curl -sSL "$URL" -o geodata_raw/BA-Gebietsstrukturen-shp.zip
unzip -o geodata_raw/BA-Gebietsstrukturen-shp.zip -d geodata_raw

echo "Shapefile liegt in geodata_raw/. Weiter mit: python3 convert_geodata.py"
