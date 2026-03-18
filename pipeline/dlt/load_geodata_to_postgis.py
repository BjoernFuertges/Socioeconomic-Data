"""
DLT Pipeline for loading LAU geodata and population data into PostGIS database.
Loads all years into single tables with year columns in the 'store' schema.
"""

import dlt
import geopandas as gpd
import pandas as pd
import gzip
import json
import os
from pathlib import Path
from typing import Iterator, Dict, Any
import logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Base paths — GEODATA_BASE_PATH zeigt auf den Output des socioeconomic-data-parser
BASE_PATH = Path(os.environ.get("GEODATA_BASE_PATH", "/home/bjoern/Software/HTW/soziooekonomische-daten/socioeconomic-data-parser"))
GEODATA_PATH = BASE_PATH / "geodata" / "lau" / "output" / "geojson"
POPULATION_PATH = BASE_PATH / "statistics" / "lokale_verwaltungseinheiten_eu" / "output"

def extract_year_from_filename(filename: str) -> int:
    """Extract year from filename like 'LAU_RG_01M_2011_3035_cleaned.geojson.gz'"""
    parts = filename.split('_')
    for part in parts:
        if part.isdigit() and len(part) == 4:
            return int(part)
    raise ValueError(f"Could not extract year from filename: {filename}")

@dlt.resource(name="lau_geometry", write_disposition="replace")
def load_geodata() -> Iterator[Dict[str, Any]]:
    """Load all LAU geometry data from GeoJSON files."""
    logger.info("Starting geodata extraction...")
    
    geojson_files = list(GEODATA_PATH.glob("LAU_RG_01M_*_3035_cleaned.geojson.gz"))
    logger.info(f"Found {len(geojson_files)} geodata files")
    
    for file_path in sorted(geojson_files):
        try:
            year = extract_year_from_filename(file_path.name)
            logger.info(f"Processing geodata for year {year}: {file_path.name}")
            
            # Read compressed GeoJSON
            with gzip.open(file_path, 'rt', encoding='utf-8') as f:
                geojson_data = json.load(f)
            
            # Convert to GeoDataFrame
            gdf = gpd.GeoDataFrame.from_features(geojson_data['features'])
            
            # Add year column
            gdf['year'] = year
            
            # Convert to records for DLT
            for _, row in gdf.iterrows():
                record: Dict[str, Any] = {str(k): v for k, v in row.to_dict().items()}
                # Convert geometry to WKT for PostGIS
                if 'geometry' in record and record['geometry'] is not None:
                    record['geometry'] = str(record['geometry'])
                yield record
                
        except Exception as e:
            logger.error(f"Error processing {file_path.name}: {e}")
            continue

@dlt.resource(name="lau_population", write_disposition="replace")
def load_population_data() -> Iterator[Dict[str, Any]]:
    """Load all LAU population data from CSV files."""
    logger.info("Starting population data extraction...")
    
    csv_files = list(POPULATION_PATH.glob("lau_population_*.csv"))
    logger.info(f"Found {len(csv_files)} population files")
    
    for file_path in sorted(csv_files):
        try:
            # Extract year from filename like 'lau_population_2011.csv'
            year = int(file_path.stem.split('_')[-1])
            logger.info(f"Processing population data for year {year}: {file_path.name}")
            
            # Read CSV
            df = pd.read_csv(file_path, sep=';', dtype=str)
            
            # Add year column
            df['year'] = year
            
            # Convert to records for DLT
            for _, row in df.iterrows():
                yield {str(k): v for k, v in row.to_dict().items()}
                
        except Exception as e:
            logger.error(f"Error processing {file_path.name}: {e}")
            continue

def create_pipeline():
    """Create and configure the DLT pipeline."""
    
    # Configure PostGIS destination
    pipeline = dlt.pipeline(
        pipeline_name="lau_geodata_pipeline",
        destination="postgres",
        dataset_name="loaded_data"
    )
    
    return pipeline

def main():
    """Main execution function."""
    logger.info("Starting LAU geodata and population data pipeline...")
    
    # Create pipeline
    pipeline = create_pipeline()
    
    try:
        # Load geodata
        logger.info("Loading geodata...")
        geodata_info = pipeline.run(
            load_geodata(),
            table_name="lau_geometry"
        )
        logger.info(f"Geodata load completed: {geodata_info}")
        
        # Load population data
        logger.info("Loading population data...")
        population_info = pipeline.run(
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
        print(f"Population: {population_info}")
        print("="*50)
        
    except Exception as e:
        logger.error(f"Pipeline failed: {e}")
        raise

if __name__ == "__main__":
    main()
