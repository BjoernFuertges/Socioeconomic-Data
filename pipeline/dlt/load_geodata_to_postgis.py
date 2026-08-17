"""
DLT pipeline for loading LAU and NUTS geodata plus LAU population data into PostGIS.
Loads all years into single tables in the loaded_data schema.
"""

import dlt
import gc
import gzip
import ijson
import os
import pandas as pd
from pathlib import Path
from shapely.geometry import shape
from typing import Iterator, Dict, Any
import logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Base paths — GEODATA_BASE_PATH zeigt auf den Output des socioeconomic-data-parser
BASE_PATH = Path(os.environ.get("GEODATA_BASE_PATH", "/home/bjoern/Software/HTW/soziooekonomische-daten/socioeconomic-data-parser"))
GEODATA_PATH = BASE_PATH / "geodata" / "lau" / "output" / "geojson"
# Fallback: unkomprimierte GeoJSON-Dateien aus geo-mapper (lokal verfügbar)
LAU_GEOJSON_FALLBACK_PATH = Path(
    os.environ.get(
        "LAU_GEODATA_PATH",
        str(Path(__file__).resolve().parents[1] / "geo-mapper" / "geo_mapper" / "geodata_raw" / "lau"),
    )
)
POPULATION_PATH = BASE_PATH / "statistics" / "lokale_verwaltungseinheiten_eu" / "output"
# Fallback: LAU-Bevölkerungsdaten aus geo-importer
POPULATION_FALLBACK_PATH = Path(
    os.environ.get(
        "LAU_POPULATION_PATH",
        str(Path(__file__).resolve().parents[1] / "geo-importer" / "src" / "geodata" / "LAU"),
    )
)
NUTS_GEODATA_PATH = Path(
    os.environ.get(
        "NUTS_GEODATA_PATH",
        str(Path(__file__).resolve().parents[1] / "geo-mapper" / "geo_mapper" / "geodata_raw" / "nuts"),
    )
)

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

    Sucht zuerst nach komprimierten Dateien (*_cleaned.geojson.gz) im
    GEODATA_BASE_PATH-Output; fällt zurück auf unkomprimierte .geojson-Dateien
    im geo-mapper-Verzeichnis (lokal verfügbar).
    """
    logger.info("Starting geodata extraction...")

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
            logger.error(f"Error processing {file_path.name}: {e}")
            continue

@dlt.resource(
    name="lau_population",
    write_disposition="replace",
    # gisco_id/nuts_3_code/lau_name_latin/population/country kommen nur aus dem
    # primären GEODATA_BASE_PATH-Pfad; im Fallback (geo-importer LAU.csv) sind
    # sie durchgehend NULL. Ohne expliziten Typ-Hint lässt dlt eine Spalte, die
    # in allen Zeilen NULL ist, komplett weg (Typinferenz schlägt fehl), was
    # stg_lau_population.sql mit "column does not exist" abbrechen lässt.
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

    Sucht zuerst nach lau_population_*.csv im GEODATA_BASE_PATH-Output;
    fällt zurück auf die LAU-CSVs aus geo-importer (LAU<year>/LAU.csv).
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
                continue
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
                # Fallback hat keine Bevölkerungszahlen/gisco_id (nur im primären
                # GEODATA_BASE_PATH-Output vorhanden) — Spalten als NULL ergänzen,
                # damit stg_lau_population.sql (erwartet diese Spalten) nicht mit
                # "column does not exist" abbricht; nachgelagerte, populationsbasierte
                # Marts bleiben dadurch leer statt den ganzen dbt-Lauf zu blockieren.
                for col in ("gisco_id", "nuts_3_code", "lau_name_latin", "population", "country"):
                    if col not in df.columns:
                        df[col] = None
                for _, row in df.iterrows():
                    yield {str(k): v for k, v in row.to_dict().items()}
            except Exception as e:
                logger.error(f"Error processing {lau_dir}: {e}")
                continue

@dlt.resource(name="nuts_geometry", write_disposition="replace")
def load_nuts_geodata() -> Iterator[Dict[str, Any]]:
    """Load all NUTS geometry data from GeoJSON files."""
    logger.info("Starting NUTS geodata extraction...")

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
            logger.error(f"Error processing {file_path.name}: {e}")
            continue

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

    # Separate pipelines to avoid stale state cross-contamination
    pipeline = create_pipeline()
    pop_pipeline = create_population_pipeline()

    try:
        # Load geodata
        geojson_files = list(GEODATA_PATH.glob("LAU_RG_01M_*_3035_cleaned.geojson.gz")) or \
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

        nuts_geojson_files = list(NUTS_GEODATA_PATH.glob("NUTS_RG_01M_*_3035.geojson"))
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
