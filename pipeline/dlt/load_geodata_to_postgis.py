"""
DLT pipeline for loading LAU and NUTS geodata plus LAU population data into PostGIS.
Loads all years into single tables in the loaded_data schema.

Geometry source: the "Geodaten" folder of the data exchange if present
(see find_geodaten_dir), otherwise the legacy GISCO files (GEODATA_BASE_PATH /
geo-mapper fallbacks).
"""

import dlt
import gc
import gzip
import ijson
import os
import pandas as pd
import re
from pathlib import Path
from pyproj import Transformer
from shapely.geometry import shape
from shapely.ops import transform as shapely_transform
from typing import Iterator, Dict, Any
import logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Same default as fetch_lau_population.py, so a fresh checkout finds the CSVs it downloads.
BASE_PATH = Path(os.environ.get("GEODATA_BASE_PATH", str(Path(__file__).resolve().parent / "eurostat_lau_population")))
GEODATA_PATH = BASE_PATH / "geodata" / "lau" / "output" / "geojson"
# Fallback: uncompressed GeoJSON from geo-mapper
LAU_GEOJSON_FALLBACK_PATH = Path(
    os.environ.get(
        "LAU_GEODATA_PATH",
        str(Path(__file__).resolve().parents[2] / "geo-mapper" / "geo_mapper" / "geodata_raw" / "lau"),
    )
)
POPULATION_PATH = BASE_PATH / "statistics" / "lokale_verwaltungseinheiten_eu" / "output"
# Fallback: LAU names (no population) from geo-importer
POPULATION_FALLBACK_PATH = Path(
    os.environ.get(
        "LAU_POPULATION_PATH",
        str(Path(__file__).resolve().parents[2] / "geo-importer" / "src" / "geodata" / "LAU"),
    )
)
NUTS_GEODATA_PATH = Path(
    os.environ.get(
        "NUTS_GEODATA_PATH",
        str(Path(__file__).resolve().parents[2] / "geo-mapper" / "geo_mapper" / "geodata_raw" / "nuts"),
    )
)

# Preferred geodata source: the "Geodaten" folder of the data exchange
# (geojson/LAU/<year>/lau_<year>_level_0.geojson,
#  geojson/NUTS_<level>/<year>/nuts_<year>_level_<level>.geojson).
# Files are WGS84 (EPSG:4326) and carry only id/name (LAU) or
# id_nuts/name/id_ars (NUTS); they are reprojected to EPSG:3035 and mapped to
# the column layout of the legacy GISCO files (see _lau_record/_nuts_record).
# In production backend/scripts/sync_nextcloud_data_exchange.sh mirrors the
# folder to data-exchange/geodata; locally it may still sit in
# <DATA_EXCHANGE_PATH>/Geodaten. GEODATEN_PATH overrides both.
_REPO_ROOT = Path(__file__).resolve().parents[2]
_DATA_EXCHANGE = Path(os.environ.get("DATA_EXCHANGE_PATH", str(_REPO_ROOT / "data-exchange" / "1_friedrich-bjoern")))


def _geodaten_candidates() -> list[Path]:
    env = os.environ.get("GEODATEN_PATH")
    if env:
        return [Path(env)]
    return [_DATA_EXCHANGE.parent / "geodata", _DATA_EXCHANGE / "Geodaten"]


def find_geodaten_dir() -> Path | None:
    """First candidate that actually contains LAU or NUTS GeoJSON files."""
    for cand in _geodaten_candidates():
        gj = cand / "geojson"
        if gj.is_dir() and (any(gj.glob("LAU/*/lau_*_level_*.geojson")) or any(gj.glob("NUTS_*/*/nuts_*_level_*.geojson"))):
            return cand
    return None


def _geodaten_lau_files(base: Path) -> list[Path]:
    return sorted(base.glob("geojson/LAU/*/lau_*_level_*.geojson"))


def _geodaten_nuts_files(base: Path) -> list[Path]:
    return sorted(base.glob("geojson/NUTS_*/*/nuts_*_level_*.geojson"))


_WGS84_TO_3035 = Transformer.from_crs(4326, 3035, always_xy=True).transform
_NUTS_FILE_RE = re.compile(r"nuts_(\d{4})_level_(\d)\.geojson$")


def _to_wkt_3035(geom: Any) -> str | None:
    if geom is None:
        return None
    try:
        return shapely_transform(_WGS84_TO_3035, shape(geom)).wkt
    except Exception:
        return None


def _lau_record(props: Dict[str, Any], geom: Any, year: int) -> Dict[str, Any]:
    ags = str(props.get("id") or "").strip()
    # GISCO id = country code + official municipality key (AGS), as in lau_population.
    return {
        "gisco_id": f"DE_{ags}" if ags else None,
        "cntr_code": "DE",
        "lau_id": ags or None,
        "lau_name": props.get("name"),
        "year": year,
        "geometry": _to_wkt_3035(geom),
    }


def _nuts_record(props: Dict[str, Any], geom: Any, year: int, level: int) -> Dict[str, Any]:
    nuts_id = str(props.get("id_nuts") or "").strip()
    name = props.get("name")
    return {
        "nuts_id": nuts_id or None,
        "levl_code": level,
        "cntr_code": nuts_id[:2] or None,
        "nuts_name": name,
        "name_latn": name,
        "id_ars": props.get("id_ars"),
        "year": year,
        "geometry": _to_wkt_3035(geom),
    }


def extract_year_from_filename(filename: str) -> int:
    """Extract year from filename like 'LAU_RG_01M_2011_3035_cleaned.geojson.gz'"""
    parts = filename.split('_')
    for part in parts:
        if part.isdigit() and len(part) == 4:
            return int(part)
    raise ValueError(f"Could not extract year from filename: {filename}")

@dlt.resource(name="lau_geometry", write_disposition="replace")
def load_geodata() -> Iterator[Dict[str, Any]]:
    """Load all LAU geometry data from GeoJSON files.

    Prefers compressed *_cleaned.geojson.gz under GEODATA_BASE_PATH, falls back
    to uncompressed .geojson files from geo-mapper.
    """
    logger.info("Starting geodata extraction...")

    geodaten = find_geodaten_dir()
    if geodaten is not None and _geodaten_lau_files(geodaten):
        for file_path in _geodaten_lau_files(geodaten):
            year = extract_year_from_filename(file_path.name)
            logger.info(f"Processing LAU geodata (Geodaten) for year {year}: {file_path.name}")
            try:
                skipped = 0
                with open(file_path, "rb") as f:
                    for feature in ijson.items(f, "features.item"):
                        props = feature.get("properties") or {}
                        # A feature without id cannot be joined to anything (gisco_id would be NULL).
                        if not str(props.get("id") or "").strip():
                            skipped += 1
                            continue
                        yield _lau_record(props, feature.get("geometry"), year)
                if skipped:
                    logger.warning(f"{file_path.name}: skipped {skipped} feature(s) without id")
                gc.collect()
            except Exception as e:
                # Re-raise: with "replace" a partial extract would wipe the good table.
                logger.error(f"Error processing {file_path.name}: {e}")
                raise
        return

    geojson_files = sorted(GEODATA_PATH.glob("LAU_RG_01M_*_3035_cleaned.geojson.gz"))
    use_gz = True
    if not geojson_files:
        logger.warning(f"No .geojson.gz files found in {GEODATA_PATH}, trying fallback: {LAU_GEOJSON_FALLBACK_PATH}")
        geojson_files = sorted(LAU_GEOJSON_FALLBACK_PATH.glob("LAU_RG_01M_*_3035.geojson"))
        use_gz = False
    logger.info(f"Found {len(geojson_files)} LAU geodata files (gz={use_gz})")

    for file_path in geojson_files:
        try:
            year = extract_year_from_filename(file_path.name)
            logger.info(f"Processing geodata for year {year}: {file_path.name}")

            # Stream features one-by-one to avoid loading entire GeoJSON into RAM
            open_fn = gzip.open if use_gz else open
            with open_fn(file_path, 'rb') as f:
                for feature in ijson.items(f, 'features.item'):
                    props = feature.get('properties') or {}
                    geom = feature.get('geometry')
                    record: Dict[str, Any] = {str(k): v for k, v in props.items()}
                    record['year'] = year
                    if geom is not None:
                        try:
                            record['geometry'] = shape(geom).wkt
                        except Exception:
                            record['geometry'] = None
                    else:
                        record['geometry'] = None
                    yield record
            gc.collect()

        except Exception as e:
            # Re-raise: with "replace" a partial extract would wipe the good table.
            logger.error(f"Error processing {file_path.name}: {e}")
            raise

@dlt.resource(
    name="lau_population",
    write_disposition="replace",
    # These columns only exist in the primary path; in the fallback they are all
    # NULL, and dlt drops all-NULL columns without a type hint, which would make
    # stg_lau_population.sql fail with "column does not exist".
    columns={
        "gisco_id": {"data_type": "text", "nullable": True},
        "nuts_3_code": {"data_type": "text", "nullable": True},
        "lau_name_latin": {"data_type": "text", "nullable": True},
        "population": {"data_type": "text", "nullable": True},
        "country": {"data_type": "text", "nullable": True},
    },
)
def load_population_data() -> Iterator[Dict[str, Any]]:
    """Load all LAU population data from CSV files.

    Prefers lau_population_*.csv under GEODATA_BASE_PATH, falls back to
    geo-importer's LAU<year>/LAU.csv.
    """
    logger.info("Starting population data extraction...")

    csv_files = sorted(POPULATION_PATH.glob("lau_population_*.csv"))
    if csv_files:
        logger.info(f"Found {len(csv_files)} population files in primary path")
        for file_path in csv_files:
            try:
                year = int(file_path.stem.split('_')[-1])
                logger.info(f"Processing population data for year {year}: {file_path.name}")
                df = pd.read_csv(file_path, sep=';', dtype=str)
                df['year'] = year
                for _, row in df.iterrows():
                    yield {str(k): v for k, v in row.to_dict().items()}
            except Exception as e:
                logger.error(f"Error processing {file_path.name}: {e}")
                raise
    else:
        # Fallback: geo-importer LAU<year>/LAU.csv  (Format: ID,Name)
        lau_dirs = sorted(POPULATION_FALLBACK_PATH.glob("LAU[0-9][0-9][0-9][0-9]"))
        logger.warning(f"No lau_population_*.csv found; using fallback: {len(lau_dirs)} LAU dirs in {POPULATION_FALLBACK_PATH}")
        for lau_dir in lau_dirs:
            try:
                year = int(lau_dir.name[3:])
                file_path = lau_dir / "LAU.csv"
                if not file_path.exists():
                    continue
                logger.info(f"Processing LAU population fallback for year {year}: {file_path}")
                df = pd.read_csv(file_path, sep=',', dtype=str)
                df.columns = [c.lower() for c in df.columns]
                df['year'] = year
                # Rename columns to match expected schema
                df = df.rename(columns={'id': 'lau_nat_code', 'name': 'lau_name_national'})
                # Add the missing columns as NULL so stg_lau_population.sql still
                # compiles; population-based marts end up empty instead of
                # blocking the whole dbt run.
                for col in ("gisco_id", "nuts_3_code", "lau_name_latin", "population", "country"):
                    if col not in df.columns:
                        df[col] = None
                for _, row in df.iterrows():
                    yield {str(k): v for k, v in row.to_dict().items()}
            except Exception as e:
                logger.error(f"Error processing {lau_dir}: {e}")
                raise

@dlt.resource(name="nuts_geometry", write_disposition="replace")
def load_nuts_geodata() -> Iterator[Dict[str, Any]]:
    """Load all NUTS geometry data from GeoJSON files."""
    logger.info("Starting NUTS geodata extraction...")

    geodaten = find_geodaten_dir()
    if geodaten is not None and _geodaten_nuts_files(geodaten):
        for file_path in _geodaten_nuts_files(geodaten):
            m = _NUTS_FILE_RE.search(file_path.name)
            if not m:
                logger.warning(f"Skipping unexpected NUTS file name: {file_path.name}")
                continue
            year, level = int(m.group(1)), int(m.group(2))
            logger.info(f"Processing NUTS geodata (Geodaten) for year {year}, level {level}: {file_path.name}")
            try:
                with open(file_path, "rb") as f:
                    for feature in ijson.items(f, "features.item"):
                        yield _nuts_record(feature.get("properties") or {}, feature.get("geometry"), year, level)
                gc.collect()
            except Exception as e:
                logger.error(f"Error processing {file_path.name}: {e}")
                raise
        return

    geojson_files = list(NUTS_GEODATA_PATH.glob("NUTS_RG_01M_*_3035.geojson"))
    logger.info(f"Found {len(geojson_files)} NUTS geodata files")

    for file_path in sorted(geojson_files):
        try:
            year = extract_year_from_filename(file_path.name)
            logger.info(f"Processing NUTS geodata for year {year}: {file_path.name}")

            with open(file_path, "rb") as f:
                for feature in ijson.items(f, "features.item"):
                    props = feature.get("properties") or {}
                    geom = feature.get("geometry")
                    record: Dict[str, Any] = {str(k).lower(): v for k, v in props.items()}
                    record["year"] = year
                    if geom is not None:
                        try:
                            record["geometry"] = shape(geom).wkt
                        except Exception:
                            record["geometry"] = None
                    else:
                        record["geometry"] = None
                    yield record
            gc.collect()

        except Exception as e:
            # Re-raise: with "replace" a partial extract would wipe the good table.
            logger.error(f"Error processing {file_path.name}: {e}")
            raise

def create_pipeline():
    """Create and configure the DLT pipeline for geodata."""
    return dlt.pipeline(
        pipeline_name="lau_geodata_pipeline",
        destination="postgres",
        dataset_name="loaded_data"
    )

def create_population_pipeline():
    """Separate pipeline for population data to avoid stale state from geodata runs."""
    return dlt.pipeline(
        pipeline_name="lau_population_pipeline",
        destination="postgres",
        dataset_name="loaded_data"
    )


def main():
    """Main execution function."""
    logger.info("Starting LAU/NUTS geodata and population data pipeline...")
    logger.info(
        f"GEODATA_BASE_PATH={BASE_PATH}, GEODATA_PATH={GEODATA_PATH}, POPULATION_PATH={POPULATION_PATH}, NUTS_GEODATA_PATH={NUTS_GEODATA_PATH}"
    )

    pipeline = create_pipeline()
    pop_pipeline = create_population_pipeline()

    try:
        # Load geodata
        geodaten = find_geodaten_dir()
        logger.info(f"Geodaten folder: {geodaten if geodaten else 'not found, using legacy GISCO paths'}")
        geojson_files = (_geodaten_lau_files(geodaten) if geodaten else []) or \
                        list(GEODATA_PATH.glob("LAU_RG_01M_*_3035_cleaned.geojson.gz")) or \
                        list(LAU_GEOJSON_FALLBACK_PATH.glob("LAU_RG_01M_*_3035.geojson"))
        if not geojson_files:
            logger.warning(f"No LAU geodata files found in {GEODATA_PATH} or {LAU_GEOJSON_FALLBACK_PATH}, skipping geodata load")
            geodata_info = None
        else:
            logger.info("Loading geodata...")
            geodata_info = pipeline.run(
                load_geodata(),
                table_name="lau_geometry"
            )
            logger.info(f"Geodata load completed: {geodata_info}")

        nuts_geojson_files = (_geodaten_nuts_files(geodaten) if geodaten else []) or \
                             list(NUTS_GEODATA_PATH.glob("NUTS_RG_01M_*_3035.geojson"))
        if not nuts_geojson_files:
            logger.warning(f"No NUTS geodata files found in {NUTS_GEODATA_PATH}, skipping NUTS geodata load")
            nuts_geodata_info = None
        else:
            logger.info("Loading NUTS geodata...")
            nuts_geodata_info = pipeline.run(
                load_nuts_geodata(),
                table_name="nuts_geometry"
            )
            logger.info(f"NUTS geodata load completed: {nuts_geodata_info}")

        # Load population data (primary path or fallback)
        csv_files = list(POPULATION_PATH.glob("lau_population_*.csv"))
        fallback_dirs = list(POPULATION_FALLBACK_PATH.glob("LAU[0-9][0-9][0-9][0-9]"))
        if not csv_files and not fallback_dirs:
            logger.warning(f"No population files found in {POPULATION_PATH} or {POPULATION_FALLBACK_PATH}, skipping population load")
            population_info = None
        else:
            logger.info("Loading population data...")
            population_info = pop_pipeline.run(
                load_population_data(),
                table_name="lau_population"
            )
            logger.info(f"Population data load completed: {population_info}")

        logger.info("Pipeline completed successfully!")

        # Print summary
        print("\n" + "="*50)
        print("PIPELINE SUMMARY")
        print("="*50)
        print(f"Geodata: {geodata_info}")
        print(f"NUTS Geodata: {nuts_geodata_info}")
        print(f"Population: {population_info}")
        print("="*50)

    except Exception as e:
        logger.error(f"Pipeline failed: {e}")
        raise

if __name__ == "__main__":
    main()
