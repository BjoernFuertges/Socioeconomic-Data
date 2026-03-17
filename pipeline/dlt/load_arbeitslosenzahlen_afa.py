"""
DLT Pipeline for loading unemployment data from Agentur für Arbeit (transposed) into PostgreSQL.
Transposes the CSV (swaps rows/columns) and creates a normalized long-format table.

Data source: Agentur für Arbeit - Arbeitslosenzahlen nach Regionen
"""

import dlt
import pandas as pd
import json
import os
from pathlib import Path
from typing import Iterator, Dict, Any
import logging
from datetime import datetime

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Paths — DATA_EXCHANGE_PATH wird per Env-Variable gesetzt (z.B. /app/data-exchange/1_friedrich-bjoern)
_EXCHANGE = Path(os.environ.get("DATA_EXCHANGE_PATH", "/home/bjoern/Software/HTW/soziooekonomische-daten"))
CSV_PATH = _EXCHANGE / "arbeitslosenzahlen" / "mapped_pairs.csv"
META_PATH = _EXCHANGE / "arbeitslosenzahlen" / "meta.json"

def generate_dataset_id(metadata: Dict[str, Any]) -> str:
    """
    Generate a unique dataset_id from metadata.
    Uses series and table_name to create a stable identifier.
    """
    series = metadata['metadata']['series'].lower().replace(' ', '_').replace('-', '_')
    table_name = metadata['metadata']['table_name'].lower().replace(' ', '_').replace('-', '_')
    # Use series as primary identifier, table_name as secondary
    dataset_id = f"afa_{series}_{table_name}"
    # Remove any special characters
    dataset_id = ''.join(c if c.isalnum() or c == '_' else '_' for c in dataset_id)
    return dataset_id

@dlt.resource(
    name="arbeitslosenzahlen_metadata", 
    write_disposition="merge",
    primary_key="dataset_id"
)
def load_metadata() -> Iterator[Dict[str, Any]]:
    """
    Load metadata from meta.json file.
    Uses merge mode to update existing metadata or insert new ones.
    """
    logger.info(f"Loading metadata from: {META_PATH}")
    
    with open(META_PATH, 'r', encoding='utf-8') as f:
        metadata = json.load(f)
    
    # Generate dynamic dataset_id from metadata
    dataset_id = generate_dataset_id(metadata)
    logger.info(f"Generated dataset_id: {dataset_id}")
    
    # Extract relevant metadata
    meta_record = {
        'dataset_id': dataset_id,  # Unique identifier for linking (primary key)
        'title': metadata['metadata']['title'],
        'series': metadata['metadata']['series'],
        'publisher_name': metadata['metadata']['publisher_name'],
        'source_name': metadata['metadata']['source_name'],
        'source_url': metadata['metadata']['source_url'],
        'description': metadata['metadata']['description'],
        'table_name': metadata['metadata']['table_name'],
        'page_number': metadata['metadata']['page_number'],
        'load_date': metadata['metadata']['load_date'],
        'collection_date': metadata['metadata']['collection_date'],
        'start_date': metadata['metadata']['time_resolution']['start_date'],
        'end_date': metadata['metadata']['time_resolution']['end_date'],
        'categories': ', '.join(metadata['metadata']['categories']),
        'geodata_level': metadata['geodata_level'],
        'geodata_year': metadata['geodata_year'],
        'methodology': metadata['metadata']['methodology'],
        'license': metadata['metadata']['license'],
        'updated_at': datetime.now().isoformat()  # Track when metadata was last updated
    }
    
    yield meta_record
    logger.info(f"Metadata loaded successfully with dataset_id: {dataset_id}")
    
    return dataset_id  # Return for use in data loading

@dlt.resource(name="arbeitslosenzahlen_agentur_arbeit", write_disposition="replace")
def load_unemployment_data_transposed(dataset_id: str, load_date: str) -> Iterator[Dict[str, Any]]:
    """
    Load and transpose unemployment data from Agentur für Arbeit.
    Creates normalized records with one row per region-timepoint combination.
    Each record contains unemployment numbers for a specific region and time period.
    
    Args:
        dataset_id: The unique identifier linking to metadata table
        load_date: The date when the data was originally loaded/collected
    """
    logger.info(f"Loading CSV from: {CSV_PATH}")
    logger.info(f"Using dataset_id: {dataset_id}")
    logger.info(f"Using load_date: {load_date}")
    
    # Read CSV
    df = pd.read_csv(CSV_PATH)
    logger.info(f"Original shape: {df.shape} (rows x columns)")
    
    # Get metadata columns (first 4 columns)
    metadata_cols = ['original_id', 'original_name', 'geodata_id', 'geodata_name']
    
    # Get last two columns (mapper, parameter)
    additional_cols = ['mapper', 'parameter']
    
    # Get time columns (all columns between metadata and additional columns)
    time_cols = [col for col in df.columns if col not in metadata_cols + additional_cols]
    
    logger.info(f"Found {len(time_cols)} time columns")
    logger.info(f"Time columns: {time_cols[:5]}... (showing first 5)")
    
    # Transpose: For each region, create records for each time point
    record_count = 0
    
    for idx, row in df.iterrows():
        # Get metadata for this region
        region_data = {
            'dataset_id': dataset_id,  # Link to metadata table
            'original_id': row['original_id'],
            'original_name': row['original_name'],
            'geodata_id': row['geodata_id'],
            'geodata_name': row['geodata_name'],
            'mapper': row['mapper'],
            'parameter': row['parameter']
        }
        
        # Create a record for each time column
        for time_col in time_cols:
            value = row[time_col]
            
            # Skip if value is NaN or empty
            if pd.isna(value) or value == '':
                continue
            
            # Determine if this is a year or a date
            is_date = '-' in str(time_col)
            
            record = {
                **region_data,
                'time_column': time_col,
                'is_date': is_date,
                'arbeitslose': value,  # Anzahl der Arbeitslosen
                'load_date': load_date  # When the data was loaded
            }
            
            # Parse time information
            if is_date:
                # It's a date like "1998-01-31 00:00:00"
                try:
                    date_obj = pd.to_datetime(time_col)
                    record['year'] = date_obj.year
                    record['month'] = date_obj.month
                    record['day'] = date_obj.day
                    record['time_type'] = 'monthly'
                except:
                    logger.warning(f"Could not parse date: {time_col}")
                    record['year'] = None
                    record['month'] = None
                    record['day'] = None
                    record['time_type'] = 'unknown'
            else:
                # It's a year like "1998"
                try:
                    record['year'] = int(time_col)
                    record['month'] = None
                    record['day'] = None
                    record['time_type'] = 'yearly'
                except:
                    logger.warning(f"Could not parse year: {time_col}")
                    record['year'] = None
                    record['month'] = None
                    record['day'] = None
                    record['time_type'] = 'unknown'
            
            yield record
            record_count += 1
            
            # Log progress
            if record_count % 10000 == 0:
                logger.info(f"Processed {record_count} records...")
    
    logger.info(f"Total records generated: {record_count}")

def create_pipeline():
    """Create and configure the DLT pipeline."""
    
    pipeline = dlt.pipeline(
        pipeline_name="arbeitslosenzahlen_pipeline",
        destination="postgres",
        dataset_name="loaded_data"
    )
    
    return pipeline

def main():
    """Main execution function."""
    logger.info("Starting unemployment data pipeline from Agentur für Arbeit (transposed)...")

    missing_inputs = False
    if not META_PATH.exists():
        logger.warning(f"Skipping pipeline: required metadata file is missing: {META_PATH}")
        missing_inputs = True
    if not CSV_PATH.exists():
        logger.warning(f"Skipping pipeline: required data file is missing: {CSV_PATH}")
        missing_inputs = True
    if missing_inputs:
        return
    
    # Generate dataset_id from metadata first
    logger.info(f"Reading metadata to generate dataset_id from: {META_PATH}")
    with open(META_PATH, 'r', encoding='utf-8') as f:
        metadata = json.load(f)
    dataset_id = generate_dataset_id(metadata)
    logger.info(f"Generated dataset_id: {dataset_id}")
    
    # Create pipeline
    pipeline = create_pipeline()
    
    try:
        # Load metadata first (will merge/update based on dataset_id)
        logger.info("Loading metadata...")
        metadata_info = pipeline.run(
            load_metadata(),
            table_name="arbeitslosenzahlen_metadata"
        )
        logger.info(f"Metadata load completed: {metadata_info}")
        
        # Load data with the same dataset_id and load_date from metadata
        logger.info("Loading and transposing unemployment data...")
        load_date = metadata['metadata']['load_date']
        load_info = pipeline.run(
            load_unemployment_data_transposed(dataset_id, load_date),
            table_name="arbeitslosenzahlen_agentur_arbeit"
        )
        logger.info(f"Data load completed: {load_info}")
        
        logger.info("Pipeline completed successfully!")
        
        # Print summary
        print("\n" + "="*70)
        print("PIPELINE SUMMARY")
        print("="*70)
        print(f"Metadata: {metadata_info}")
        print(f"Data: {load_info}")
        print("="*70)
        
    except Exception as e:
        logger.error(f"Pipeline failed: {e}")
        raise

if __name__ == "__main__":
    main()
