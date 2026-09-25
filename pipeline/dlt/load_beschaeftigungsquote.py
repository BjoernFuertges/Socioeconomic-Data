"""
DLT pipeline for loading employment rate data (Beschäftigungsquote) at LAU/Gemeinden-Ebene.

Similar to load_soziodata_generic.py but with:
- Uses meta.yaml if provided (fallback to sensible defaults)
- LAU (Gemeinden) level support
- Percentage value handling (0-100%)

Usage:
    python load_beschaeftigungsquote.py <dataset_subdir>

Where <dataset_subdir> is relative to DATA_EXCHANGE_PATH, e.g.:
    beschaeftigungsquote/results_beschaeftigungsquote_gemeinden

Expected structure:
    <dataset_subdir>/
    mapped_pairs.csv  -- columns: original_id_1, original_name, geodata_name,
                 geodata_id_lau, mapper, parameter, <year1>, <year2>, ...
    meta.yaml         -- optional but recommended; provides geodata_level/year and metadata
"""

import sys
import dlt
import pandas as pd
import os
import logging
import yaml
from pathlib import Path
from typing import Iterator, Dict, Any

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

_META_COLS = {
    "original_id_1",
    "original_name",
    "geodata_name",
    "geodata_id_lau",
    "mapper",
    "parameter",
}


def _parse_german_number(raw: Any) -> float | None:
    """Parse German-formatted number (thousands separator = '.', decimal = ',')."""
    if pd.isna(raw) or str(raw).strip() == "":
        return None
    s = str(raw).strip()
    s = s.replace(".", "").replace(",", ".")
    try:
        return float(s)
    except ValueError:
        logger.warning(f"Could not parse value: {raw!r}")
        return None


def _year_columns(df: pd.DataFrame) -> list[str]:
    """Return column names that are 4-digit years."""
    year_cols = []
    for col in df.columns:
        col_str = str(col).strip()
        if col_str.isdigit() and len(col_str) == 4:
            year_cols.append(col)
    return sorted(year_cols)


def _load_meta(dataset_dir: Path) -> Dict[str, Any]:
    """Load optional meta.yaml; return defaults if missing."""
    meta_path = dataset_dir / "meta.yaml"
    defaults: Dict[str, Any] = {
        "geodata_level": "LAU",
        "geodata_year": 2024,
        "name": "Beschaeftigungsquote",
        "number_type": "relative",
        "unit": "prozent",
    }
    if not meta_path.exists():
        return defaults

    with open(meta_path, "r", encoding="utf-8") as f:
        loaded = yaml.safe_load(f) or {}
    defaults.update({k: v for k, v in loaded.items() if v is not None})
    return defaults


def make_resource(dataset_dir: Path, dataset_name: str):
    """
    Factory that returns a configured @dlt.resource function.
    Requires only the CSV; meta.yaml is optional and generated with defaults if missing.
    """

    @dlt.resource(name=dataset_name, write_disposition="replace")
    def _resource() -> Iterator[Dict[str, Any]]:
        csv_path = dataset_dir / "mapped_pairs.csv"
        if not csv_path.exists():
            raise FileNotFoundError(f"mapped_pairs.csv not found at {csv_path}")

        logger.info(f"Loading CSV: {csv_path}")
        df = pd.read_csv(csv_path)
        logger.info(f"Shape: {df.shape}")
        logger.info(f"Columns: {df.columns.tolist()}")

        year_cols = _year_columns(df)
        logger.info(f"Found {len(year_cols)} year columns: {year_cols}")

        metadata = _load_meta(dataset_dir)

        record_count = 0
        for _, row in df.iterrows():
            original_id = row.get("original_id_1")
            original_name = row.get("original_name")
            geodata_name = row.get("geodata_name")
            mapper = row.get("mapper")
            parameter = row.get("parameter")

            # IDs may vary by mapping output version
            geodata_id_nuts = row.get("geodata_id_nuts")
            geodata_id_lau = row.get("geodata_id_lau")
            geodata_id_ars = row.get("geodata_id_ars")

            for year_col in year_cols:
                try:
                    year = int(year_col)
                except ValueError:
                    logger.warning(f"Could not parse year from column: {year_col}")
                    continue

                raw_value = row.get(year_col)
                value = _parse_german_number(raw_value)

                record = {
                    "original_id_1": original_id,
                    "original_name": original_name,
                    "geodata_name": geodata_name,
                    "geodata_id_nuts": geodata_id_nuts,
                    "geodata_id_lau": geodata_id_lau,
                    "geodata_id_ars": geodata_id_ars,
                    "mapper": mapper,
                    "parameter": parameter,
                    "geodata_level": metadata["geodata_level"],
                    "geodata_year": metadata["geodata_year"],
                    "year": year,
                    "value": value,
                    "dataset_name": dataset_name,
                }
                yield record
                record_count += 1

        logger.info(f"Total records yielded: {record_count}")

    return _resource()


def main():
    if len(sys.argv) < 2:
        print("Usage: python load_beschaeftigungsquote.py <dataset_subdir>")
        print("Example: python load_beschaeftigungsquote.py beschaeftigungsquote/results_beschaeftigungsquote_gemeinden")
        sys.exit(1)

    dataset_subdir = sys.argv[1]
    exchange_path = Path(os.environ.get("DATA_EXCHANGE_PATH", str(Path(__file__).resolve().parents[2])))
    dataset_dir = exchange_path / dataset_subdir
    dataset_name = dataset_dir.name

    if not dataset_dir.exists():
        raise FileNotFoundError(f"Dataset directory not found: {dataset_dir}")

    logger.info(f"Starting DLT pipeline for {dataset_name}")
    logger.info(f"Dataset directory: {dataset_dir}")

    # Initialize DLT pipeline
    pipeline = dlt.pipeline(
        pipeline_name="socioeconomic_data",
        destination="postgres",
        dataset_name="loaded_data",
    )

    # Create and run resource
    resource = make_resource(dataset_dir, dataset_name)
    load_info = pipeline.run(resource)

    logger.info(f"Load completed: {load_info}")


if __name__ == "__main__":
    main()
