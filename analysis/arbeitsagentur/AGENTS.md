# AGENTS.md

This file provides guidance to AI coding agents and contributors working in this part of the repository.

Scope: this file covers `analysis/arbeitsagentur/` — a standalone,
manually-run data-fetching directory, not wired into the dlt/dbt pipeline
or backend. It fetches current unemployment figures for the three Berlin
Arbeitsagentur districts plus their district boundary geodata. See the
repository root [AGENTS.md](../../AGENTS.md) for how the production pipeline
(`pipeline/dlt/`, `pipeline/dbt/`) differs from this ad-hoc script.

## Commands

```bash
./fetch.sh              # re-fetch current-month CSV eckwerte for all 3 Berlin agencies (no auth needed)

./fetch_geodata.sh       # download BA-Gebietsstruktur shapefile -> geodata_raw/ (gitignored)
python3 convert_geodata.py   # extract the 3 Berlin polygons -> agenturbezirk_berlin-*.geojson
```

`convert_geodata.py` needs `pyshp`/`pyproj`, which are not part of the
project `.venv`:

```bash
pip install --target /tmp/pylibs pyshp pyproj
PYTHONPATH=/tmp/pylibs python3 convert_geodata.py
```

## Notes

- The Arbeitsagentur statistics API only exposes the *current* eckwerte
  table (current month, previous month, same month last year) — there is
  no historical time-series endpoint. Building a longer series here would
  require running `fetch.sh` repeatedly over time and keeping every
  snapshot, which nothing currently does.
- `arbeitslose_berlin-{mitte,nord,sued}.csv` are the fetched data
  (regenerated in place by `fetch.sh`); `agenturbezirk_berlin-*.geojson`
  are the derived district boundaries in WGS84 (source shapefile is
  ETRS_1989_UTM_Zone_32N, reprojected by `convert_geodata.py`).
  `geodata_raw/` (the raw shapefile) is gitignored — only the converted
  GeoJSON is versioned.
