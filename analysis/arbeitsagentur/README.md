# Arbeitslosenzahlen Berlin

Aktuelle Arbeitslosen-Eckwerte der drei Berliner Agenturen für Arbeit, abgerufen
über die öffentliche Statistik-API der Bundesagentur für Arbeit.

Quelle / API-Doku: https://statistik.arbeitsagentur.de/DE/Statischer-Content/Service/API/API-ALO.html

## Abruf

```bash
./fetch.sh
```

Ruft für jede Agentur die CSV-Eckwertetabelle ab:

```bash
curl "https://statistik-dr.arbeitsagentur.de/bifrontend/bids-api/ct/v1/tableFetch/csv/EckwerteTabelleALOAA?Agentur%20f%C3%BCr%20Arbeit=AA%20Berlin%20Mitte"
```

Keine Authentifizierung nötig.

## Dateien

| Datei | Agentur für Arbeit |
|---|---|
| `arbeitslose_berlin-mitte.csv` | AA Berlin Mitte |
| `arbeitslose_berlin-nord.csv` | AA Berlin Nord |
| `arbeitslose_berlin-sued.csv` | AA Berlin Süd |

Jede Datei enthält die Eckwerte (Arbeitslose insgesamt, nach Rechtskreis SGB
III/SGB II, Arbeitslosenquote, Unterbeschäftigung) für den aktuellen und
vorherigen Berichtsmonat sowie den Vorjahresmonat.

## Hinweis

Die API liefert nur die aktuelle Eckwertetabelle (kein Zeitreihen-Endpoint für
historische Monatswerte). Für eine längere Zeitreihe muss `fetch.sh` regelmäßig
erneut ausgeführt werden, oder es müsste ein anderer Tabellentyp der
Statistik-API recherchiert werden.

## Geodaten der Agenturbezirke

Die Grenzen der drei Berliner Agenturbezirke als GeoJSON (WGS84 / EPSG:4326):

| Datei | Agentur für Arbeit | BA-ID |
|---|---|---|
| `agenturbezirk_berlin-mitte.geojson` | Berlin Mitte | 962 |
| `agenturbezirk_berlin-nord.geojson` | Berlin Nord | 955 |
| `agenturbezirk_berlin-sued.geojson` | Berlin Süd | 922 |

Quelle: offizielle BA-Gebietsstruktur (Regionaldirektions- und
Agenturbezirke) als Shapefile, veröffentlicht unter
https://statistik.arbeitsagentur.de/DE/Navigation/Grundlagen/Klassifikationen/Regionale-Gliederungen/BA-Gebietsstruktur-Nav.html.
Das Shapefile liegt ursprünglich in ETRS_1989_UTM_Zone_32N (EPSG:25832) vor
und wurde für die GeoJSON-Ausgabe nach WGS84 (EPSG:4326) reprojiziert.

Neu erzeugen:

```bash
./fetch_geodata.sh          # lädt Shapefile nach geodata_raw/
python3 convert_geodata.py  # extrahiert Berlin-Polygone -> GeoJSON
```

`convert_geodata.py` benötigt `pyshp` und `pyproj`, die nicht Teil der
Projekt-`.venv` sind, z. B.:

```bash
pip install --target /tmp/pylibs pyshp pyproj
PYTHONPATH=/tmp/pylibs python3 convert_geodata.py
```

`geodata_raw/` (Rohdaten-Shapefile) ist per `.gitignore` von der
Versionierung ausgeschlossen, die fertigen GeoJSON-Dateien werden versioniert.
