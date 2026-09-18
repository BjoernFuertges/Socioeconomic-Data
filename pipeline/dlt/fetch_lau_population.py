"""
Downloads Eurostat's yearly LAU-NUTS correspondence tables and extracts the
German ("DE") sheet as lau_population_<year>.csv, in the shape
load_geodata_to_postgis.py's load_population_data() primary path expects
under <GEODATA_BASE_PATH>/statistics/lokale_verwaltungseinheiten_eu/output/.

Source: https://ec.europa.eu/eurostat/web/nuts/local-administrative-units
("Correspondence tables at LAU level", EU-wide workbook, one sheet per
country). Eurostat publishes one snapshot per year, not a historical time
series, so "year" here means the LAU/NUTS-3/population vintage the
correspondence table was published for, not necessarily the same thing as
a dataset's observation year - it's the closest available proxy and is
only used to resolve original_id_1 (LAU code) -> geodata_id_nuts / weight
population, not as an authoritative historical population count.

Usage:
    python fetch_lau_population.py [--base-path DIR] [--years 2018 2019 ...] [--force]

Without --base-path, uses $GEODATA_BASE_PATH (same env var
load_geodata_to_postgis.py reads) or, if unset, a local
./eurostat_lau_population/ directory next to this script.
"""

import argparse
import csv
import logging
import os
from pathlib import Path

import openpyxl
import requests

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# One EU-wide correspondence-table workbook per year, as linked from the
# Eurostat LAU page. Eurostat renames/reorders columns and the workbook
# filename most years - update/extend this list when a new vintage is
# published (check the page for the current .xlsx links).
SOURCES = {
    2018: "https://ec.europa.eu/eurostat/documents/345175/501971/EU-28-LAU-2018-NUTS-2016.xlsx",
    2019: "https://ec.europa.eu/eurostat/documents/345175/501971/EU-28-LAU-2019-NUTS-2016.xlsx",
    2020: "https://ec.europa.eu/eurostat/documents/345175/501971/EU-27-LAU-2020-NUTS-2021-NUTS-2016.xlsx",
    2021: "https://ec.europa.eu/eurostat/documents/345175/501971/EU-27-LAU-2021-NUTS-2021.xlsx",
    2022: "https://ec.europa.eu/eurostat/documents/345175/501971/EU-27-LAU-2022-NUTS-2021.xlsx",
    2023: "https://ec.europa.eu/eurostat/documents/345175/501971/EU-27-LAU-2023-NUTS-2021.xlsx",
    2024: "https://ec.europa.eu/eurostat/documents/345175/501971/EU-27-LAU-2024-NUTS-2024.xlsx",
    2025: "https://ec.europa.eu/eurostat/documents/345175/501971/EU-27-LAU-2025-NUTS-2024.xlsx",
}

# Header aliases seen across vintages -> normalized field name. "gisco_id"
# (EU LAU CODE) only exists from 2022 onward; older years fall back to the
# same "DE_<lau code>" pattern Eurostat itself uses once it introduces the
# column (verified against the 2022+ files).
HEADER_ALIASES = {
    "nuts3_code": {"nuts 3 code", "nuts3"},
    "lau_code": {"lau code"},
    "gisco_id": {"eu lau code"},
    "lau_name_national": {"lau name national"},
    "lau_name_latin": {"lau name latin"},
    "population": {"population"},
}
REQUIRED_FIELDS = {"nuts3_code", "lau_code", "lau_name_national", "lau_name_latin", "population"}

OUT_SUBPATH = Path("statistics") / "lokale_verwaltungseinheiten_eu" / "output"


def _download(url: str, dest: Path, force: bool) -> None:
    if dest.exists() and not force:
        logger.info(f"Using cached download: {dest}")
        return
    logger.info(f"Downloading {url} -> {dest}")
    dest.parent.mkdir(parents=True, exist_ok=True)
    resp = requests.get(url, headers={"User-Agent": "Mozilla/5.0"}, timeout=180)
    resp.raise_for_status()
    dest.write_bytes(resp.content)


def _column_map(header_row: tuple) -> dict[str, int]:
    result: dict[str, int] = {}
    for idx, cell in enumerate(header_row):
        if cell is None:
            continue
        key = str(cell).strip().lower()
        for field, aliases in HEADER_ALIASES.items():
            if key in aliases:
                result[field] = idx
    missing = REQUIRED_FIELDS - result.keys()
    if missing:
        raise ValueError(f"Missing expected column(s) {missing} in DE sheet header {header_row}")
    return result


def extract_de_sheet(xlsx_path: Path, out_csv: Path) -> int:
    wb = openpyxl.load_workbook(xlsx_path, read_only=True)
    try:
        if "DE" not in wb.sheetnames:
            raise ValueError(f"No 'DE' sheet in {xlsx_path} (sheets: {wb.sheetnames})")
        rows = wb["DE"].iter_rows(values_only=True)
        cols = _column_map(next(rows))

        out_csv.parent.mkdir(parents=True, exist_ok=True)
        count = 0
        with open(out_csv, "w", newline="", encoding="utf-8") as f:
            writer = csv.writer(f, delimiter=";")
            writer.writerow(
                [
                    "gisco_id",
                    "nuts_3_code",
                    "lau_nat_code",
                    "lau_name_national",
                    "lau_name_latin",
                    "population",
                    "country",
                ]
            )
            for row in rows:
                lau_code, nuts3 = row[cols["lau_code"]], row[cols["nuts3_code"]]
                if lau_code is None or nuts3 is None:
                    continue
                lau_code = str(lau_code).strip()
                gisco_cell = row[cols["gisco_id"]] if "gisco_id" in cols else None
                gisco_id = str(gisco_cell).strip() if gisco_cell is not None else f"DE_{lau_code}"
                # stg_beschaeftigungsquote_gemeinden.sql's original_id_1 lost its
                # leading zero in an earlier float round-trip (pandas inferred the
                # AGS column as numeric; the model only strips a trailing ".0", see
                # its cleaned CTE) - lau_nat_code must match that same lossy
                # convention, or the LAU->NUTS-3 join there never hits for any
                # AGS starting with 0 (state codes 01-09, i.e. most of Germany).
                lau_nat_code = str(int(lau_code))
                writer.writerow(
                    [
                        gisco_id,
                        str(nuts3).strip(),
                        lau_nat_code,
                        row[cols["lau_name_national"]],
                        row[cols["lau_name_latin"]],
                        row[cols["population"]],
                        "DE",
                    ]
                )
                count += 1
        return count
    finally:
        wb.close()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument(
        "--base-path",
        default=os.environ.get("GEODATA_BASE_PATH") or str(Path(__file__).resolve().parent / "eurostat_lau_population"),
        help="GEODATA_BASE_PATH-equivalent root; CSVs land under <base-path>/statistics/lokale_verwaltungseinheiten_eu/output/",
    )
    parser.add_argument("--force", action="store_true", help="Re-download and re-extract even if the output CSV already exists")
    parser.add_argument("--years", type=int, nargs="*", default=sorted(SOURCES), help="Subset of years to fetch (default: all known years)")
    args = parser.parse_args()

    base_path = Path(args.base_path)
    download_dir = base_path / "_downloads"
    out_dir = base_path / OUT_SUBPATH

    for year in args.years:
        if year not in SOURCES:
            logger.warning(f"No known source URL for year {year}, skipping")
            continue
        out_csv = out_dir / f"lau_population_{year}.csv"
        if out_csv.exists() and not args.force:
            logger.info(f"{out_csv} already exists, skipping (use --force to redo)")
            continue
        xlsx_path = download_dir / f"lau_{year}.xlsx"
        _download(SOURCES[year], xlsx_path, args.force)
        count = extract_de_sheet(xlsx_path, out_csv)
        logger.info(f"Wrote {count} German LAU rows for {year} -> {out_csv}")


if __name__ == "__main__":
    main()
