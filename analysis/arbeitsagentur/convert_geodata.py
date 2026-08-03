"""Extrahiert die Polygone der drei Berliner Agenturbezirke aus dem
BA-Gebietsstruktur-Shapefile (geodata_raw/) und schreibt sie als GeoJSON
(WGS84 / EPSG:4326) in dieses Verzeichnis.

Benötigt: pyshp, pyproj (nicht Teil der Projekt-.venv, z. B. per
`pip install --target /tmp/pylibs pyshp pyproj` und PYTHONPATH installierbar).

Ausführen: python3 convert_geodata.py
"""

import json
from pathlib import Path

import shapefile
from pyproj import Transformer

HERE = Path(__file__).parent
SHP_PATH = HERE / "geodata_raw" / "Deutschland_Agenturbezirke_gen.shp"

# Quell-CRS des Shapefiles laut .prj: ETRS_1989_UTM_Zone_32N
SOURCE_CRS = "EPSG:25832"
TARGET_CRS = "EPSG:4326"

# BA-interne IDs der Berliner Agenturen (Feld "ID" im Shapefile)
AGENTUREN = {
    "962": "berlin-mitte",
    "955": "berlin-nord",
    "922": "berlin-sued",
}


def reproject_coords(coords, transformer):
    if isinstance(coords[0][0], (int, float)):
        return [list(transformer.transform(x, y)) for x, y in coords]
    return [reproject_coords(c, transformer) for c in coords]


def main() -> None:
    transformer = Transformer.from_crs(SOURCE_CRS, TARGET_CRS, always_xy=True)
    sf = shapefile.Reader(str(SHP_PATH))

    for shape_record in sf.iterShapeRecords():
        record = shape_record.record
        if record["ID"] not in AGENTUREN:
            continue

        geo = shape_record.shape.__geo_interface__
        feature = {
            "type": "Feature",
            "properties": {
                "id": record["ID"],
                "name": record["region"],
                "ba_schluessel": record["ID_DWH"],
                "valid_from": str(record["valid_from"]),
                "valid_to": str(record["valid_to"]),
            },
            "geometry": {
                "type": geo["type"],
                "coordinates": reproject_coords(geo["coordinates"], transformer),
            },
        }
        collection = {
            "type": "FeatureCollection",
            "name": record["region"],
            "features": [feature],
        }

        slug = AGENTUREN[record["ID"]]
        outfile = HERE / f"agenturbezirk_{slug}.geojson"
        outfile.write_text(json.dumps(collection, ensure_ascii=False), encoding="utf-8")
        print(f"geschrieben: {outfile.name}")


if __name__ == "__main__":
    main()
