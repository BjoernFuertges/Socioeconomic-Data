"""
Generic DLT pipeline for loading socioeconomic data (mapped_pairs.csv + meta.yaml).

Usage:
    python load_soziodata_generic.py <dataset_subdir>

Where <dataset_subdir> is a path relative to DATA_EXCHANGE_PATH, e.g.:
    bevoelkerung/results_bevoelkerung_kreise
    schuldnerquote/results_schuldnerquote_kreise

Expected directory layout:
    <DATA_EXCHANGE_PATH>/<dataset_subdir>/
        mapped_pairs.csv    -- columns: original_id_1, original_name, geodata_name,
                               geodata_id_nuts, geodata_id_ars, mapper, parameter,
                               <year1>, <year2>, ...
        meta.yaml           -- geodata_level, geodata_year

The dataset_name (and resulting DB table name) is derived from the last path segment of
<dataset_subdir>, e.g. "results_bevoelkerung_kreise" → table "results_bevoelkerung_kreise"
in the "loaded_data" dataset.
"""

import sys
import dlt
import pandas as pd
import yaml
import os
import logging
from datetime import datetime, timezone
from pathlib import Path
from typing import Iterator, Dict, Any

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Fixed metadata columns present in every mapped_pairs.csv of this format
_META_COLS = {
    "original_id_1",
    "original_name",
    "geodata_name",
    "geodata_id_nuts",
    "geodata_id_ars",
    "mapper",
    "parameter",
}


def _parse_german_number(raw: Any) -> float | None:
    """
    Parse a German-formatted number string to float.

    Examples:
        "357.948,00"  →  357948.0   (thousands separator = '.', decimal = ',')
        "13,15"       →  13.15
        "87.276,00"   →  87276.0
    """
    if pd.isna(raw) or str(raw).strip() == "":
        return None
    s = str(raw).strip()
    # Remove thousands separator (dot), replace decimal separator (comma) with dot
    s = s.replace(".", "").replace(",", ".")
    try:
        return float(s)
    except ValueError:
        logger.warning(f"Could not parse value: {raw!r}")
        return None


def _load_meta(dataset_dir: Path) -> Dict[str, Any]:
    meta_path = dataset_dir / "meta.yaml"
    if not meta_path.exists():
        raise FileNotFoundError(f"meta.yaml not found at {meta_path}")
    with open(meta_path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)


def _year_columns(df: pd.DataFrame) -> list[str]:
    """Return column names that represent years (4-digit integer headers)."""
    year_cols = []
    for col in df.columns:
        col_str = str(col).strip()
        if col_str.isdigit() and len(col_str) == 4:
            year_cols.append(col)
    return year_cols


def make_resource(dataset_dir: Path, dataset_name: str, meta: Dict[str, Any]):
    """
    Factory that returns a configured @dlt.resource function for the given dataset.
    We use a factory instead of a plain decorated function so that dataset_name can be
    passed dynamically as the dlt resource name.
    """
    geodata_level: str = meta.get("geodata_level", "")
    geodata_year: int | None = meta.get("geodata_year")

    @dlt.resource(name=dataset_name, write_disposition="append")
    def _resource() -> Iterator[Dict[str, Any]]:
        csv_path = dataset_dir / "mapped_pairs.csv"
        if not csv_path.exists():
            raise FileNotFoundError(f"mapped_pairs.csv not found at {csv_path}")

        logger.info(f"Loading CSV: {csv_path}")
        df = pd.read_csv(csv_path)
        logger.info(f"Shape: {df.shape}")

        year_cols = _year_columns(df)
        logger.info(f"Found {len(year_cols)} year columns: {year_cols[:5]}...")

        record_count = 0
        for _, row in df.iterrows():
            # Build the region-level metadata dict
            region = {
                col: row.get(col)
                for col in _META_COLS
                if col in df.columns
            }
            region["geodata_level"] = geodata_level
            region["geodata_year"] = geodata_year
            region["dataset_name"] = dataset_name

            for year_col in year_cols:
                raw_value = row[year_col]
                value = _parse_german_number(raw_value)
                if value is None:
                    continue

                yield {
                    **region,
                    "year": int(year_col),
                    "value": value,
                    "loaded_at": datetime.now(timezone.utc).isoformat(),
                }
                record_count += 1

                if record_count % 5000 == 0:
                    logger.info(f"  {record_count} records processed...")

        logger.info(f"Total records yielded: {record_count}")

    return _resource


def main() -> None:
    if len(sys.argv) < 2:
        print(
            "Usage: python load_soziodata_generic.py <dataset_subdir>\n"
            "  e.g. python load_soziodata_generic.py bevoelkerung/results_bevoelkerung_kreise",
            file=sys.stderr,
        )
        sys.exit(1)

    dataset_subdir = sys.argv[1]
    exchange_base = Path(
        os.environ.get(
            "DATA_EXCHANGE_PATH",
            "/home/bjoern/Software/HTW/soziooekonomische-daten/data-exchange/1_friedrich-bjoern",
        )
    )
    dataset_dir = exchange_base / dataset_subdir
    dataset_name = Path(dataset_subdir).name  # last path segment

    logger.info(f"Dataset directory : {dataset_dir}")
    logger.info(f"Dataset name (table): {dataset_name}")

    meta_path = dataset_dir / "meta.yaml"
    csv_path = dataset_dir / "mapped_pairs.csv"

    missing_inputs = False
    if not meta_path.exists():
        logger.warning(f"Skipping dataset '{dataset_name}': required metadata file is missing: {meta_path}")
        missing_inputs = True
    if not csv_path.exists():
        logger.warning(f"Skipping dataset '{dataset_name}': required data file is missing: {csv_path}")
        missing_inputs = True
    if missing_inputs:
        return

    meta = _load_meta(dataset_dir)
    logger.info(f"Metadata: geodata_level={meta.get('geodata_level')}, geodata_year={meta.get('geodata_year')}")

    pipeline = dlt.pipeline(
        pipeline_name="soziodata_pipeline",
        destination="postgres",
        dataset_name="loaded_data",
    )

    resource = make_resource(dataset_dir, dataset_name, meta)

    logger.info(f"Running pipeline for table '{dataset_name}'...")
    load_info = pipeline.run(resource())
    logger.info(f"Load completed: {load_info}")

    print("\n" + "=" * 70)
    print("PIPELINE SUMMARY")
    print("=" * 70)
    print(f"Table  : {dataset_name}")
    print(f"Result : {load_info}")
    print("=" * 70)


if __name__ == "__main__":
    main()
