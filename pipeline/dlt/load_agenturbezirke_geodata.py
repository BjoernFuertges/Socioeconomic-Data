"""
DLT pipeline for loading the official BA-Gebietsstruktur (Agenturbezirke) into PostGIS.

Downloads the same shapefile as arbeitslosenzahlen-berlin/fetch_geodata.sh
(https://statistik.arbeitsagentur.de .../BA-Gebietsstrukturen-shp.zip), but
keeps all ~146 Agenturbezirke nationwide instead of filtering to the 3
Berlin districts. Agenturbezirke are a distinct geo-level from NUTS/LAU/ARS
(Bundesagentur-internal district partition), so this feeds its own
loaded_data table rather than nuts_geometry/lau_geometry.
"""

import dlt
import logging
import os
import zipfile
from pathlib import Path
from typing import Any, Dict, Iterator

import pyogrio.raw
import requests
import shapely
from pyproj import Transformer
from shapely.ops import transform

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

SHAPEFILE_URL = (
    "https://statistik.arbeitsagentur.de/DE/Statischer-Content/Grundlagen/"
    "Klassifikationen/Regionale-Gliederungen/BA-Gebietsstruktur/"
    "Generische-Publikationen/BA-Gebietsstrukturen-shp.zip"
)
SHP_NAME = "Deutschland_Agenturbezirke_gen.shp"
RAW_DIR = Path(
    os.environ.get(
        "AGENTURBEZIRKE_GEODATA_PATH",
        str(Path(__file__).resolve().parent / "geodata_raw" / "agenturbezirke"),
    )
)

SOURCE_CRS = "EPSG:25832"
TARGET_CRS = "EPSG:4326"


def _ensure_shapefile() -> Path:
    shp_path = RAW_DIR / SHP_NAME
    if shp_path.exists():
        logger.info(f"Using cached shapefile: {shp_path}")
        return shp_path

    RAW_DIR.mkdir(parents=True, exist_ok=True)
    zip_path = RAW_DIR / "BA-Gebietsstrukturen-shp.zip"
    logger.info(f"Downloading shapefile from {SHAPEFILE_URL}")
    resp = requests.get(SHAPEFILE_URL, timeout=60)
    resp.raise_for_status()
    zip_path.write_bytes(resp.content)
    with zipfile.ZipFile(zip_path) as zf:
        zf.extractall(RAW_DIR)

    if not shp_path.exists():
        raise FileNotFoundError(f"Expected shapefile not found after extraction: {shp_path}")
    return shp_path


@dlt.resource(name="agenturbezirk_geometry", write_disposition="replace")
def load_agenturbezirk_geometry() -> Iterator[Dict[str, Any]]:
    shp_path = _ensure_shapefile()
    logger.info(f"Reading shapefile: {shp_path}")

    meta, _fids, geometry, field_data = pyogrio.raw.read(str(shp_path))
    field_names = list(meta["fields"])
    fields_by_name = dict(zip(field_names, field_data))
    transformer = Transformer.from_crs(SOURCE_CRS, TARGET_CRS, always_xy=True)

    count = len(geometry)
    logger.info(f"Found {count} Agenturbezirke")
    for i in range(count):
        geometry_wkt = None
        geom_wkb = geometry[i]
        if geom_wkb is not None:
            try:
                geom = shapely.from_wkb(geom_wkb)
                geom_wgs84 = transform(transformer.transform, geom)
                geometry_wkt = geom_wgs84.wkt
            except Exception as e:
                logger.warning(f"Could not reproject geometry for feature {i}: {e}")

        valid_from = fields_by_name["valid_from"][i]
        valid_to = fields_by_name["valid_to"][i]
        yield {
            "ba_schl": str(fields_by_name["ID"][i]),
            "name": fields_by_name["region"][i],
            "id_dwh": str(fields_by_name["ID_DWH"][i]),
            "valid_from": str(valid_from) if valid_from is not None else None,
            "valid_to": str(valid_to) if valid_to is not None else None,
            "geometry": geometry_wkt,
        }


def main() -> None:
    pipeline = dlt.pipeline(
        pipeline_name="agenturbezirk_geodata_pipeline",
        destination="postgres",
        dataset_name="loaded_data",
    )
    load_info = pipeline.run(load_agenturbezirk_geometry())
    logger.info(f"Load completed: {load_info}")

    print("\n" + "=" * 70)
    print("AGENTURBEZIRK GEODATA LOAD SUMMARY")
    print("=" * 70)
    print(f"Result: {load_info}")
    print("=" * 70)


if __name__ == "__main__":
    main()
