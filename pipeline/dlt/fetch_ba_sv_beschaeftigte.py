"""
Downloads the number of employees subject to social insurance contributions
("sozialversicherungspflichtig Beschäftigte", at place of work) per Kreis
from the official, auth-free time-series API of the Statistik der
Bundesagentur für Arbeit and writes it as a generic Kreis dataset
(mapped_pairs.csv + meta.yaml) for load_soziodata_generic.py:

    <out-dir>/sv_beschaeftigte/results_sv_beschaeftigte_kreise/
        mapped_pairs.csv   original_id_1 (5-digit Kreisschlüssel), original_name,
                           geodata_name, geodata_id_nuts, geodata_id_ars,
                           mapper, parameter, 2007 ... 2025 (integers)
        meta.yaml          provenance/license/description, see META below
        unmapped.csv       BA Kreise without a NUTS-3 (2024) counterpart

Source: EckwerteZeitreiheBST?Kreis AO=<official Kreis name>, see
https://statistik.arbeitsagentur.de/DE/Statischer-Content/Service/API/API-BST-Zeitreihe.html
(monthly series from January 2007). The Kreis list (name + 8-digit BA_SCHL,
first 5 digits = Kreisschlüssel) comes from the same JS data file
load_arbeitslosenzahlen_afa.py uses for the Agenturbezirke. Each year's value
is the June figure (reference date 30 June); only complete years are used.

NUTS-3 assignment is not guessed: geodata_id_ars -> geodata_id_nuts is looked
up in the mapped_pairs.csv of an existing Kreis dataset mapped onto NUTS 2024
(--nuts-lookup). The BA publishes every series on its current territorial
status (retroactively), which can be newer than NUTS 2024; BA Kreise that
were part of another NUTS-2024 Kreis are summed into it via
TERRITORIAL_MERGES (own calculation, stated in meta.yaml). Anything else
without a match goes to unmapped.csv and is logged, never dropped silently.

Idempotent: skips if the output already exists (--force to redo), writes
deterministically sorted output via temp files. With --cache-dir, raw
responses are stored/reused there (<cache-dir>/kreise.json,
<cache-dir>/kreis_z/<BA_SCHL>.csv), so a rerun does not hit the API again.

Usage:
    python fetch_ba_sv_beschaeftigte.py [--out-dir DIR] [--nuts-lookup CSV]
                                        [--cache-dir DIR] [--last-year 2025] [--force]

Without --out-dir, uses $DATA_EXCHANGE_PATH (same env var
load_soziodata_generic.py reads) or, if unset, data-exchange/1_friedrich-bjoern/.
"""

import argparse
import csv
import json
import logging
import os
import time
import urllib.parse
from pathlib import Path
from typing import Any, Dict, List, Optional

import requests
import yaml

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

REGION_LIST_URL = (
    "https://statistik.arbeitsagentur.de/Auswahl/Layout/Statistiktabellen/JS/"
    "DR-LZR-persistListenfelderJSON.js"
)
ZEITREIHE_URL = (
    "https://statistik-dr.arbeitsagentur.de/bifrontend/bids-api/ct/v1/"
    "tableFetch/csv/EckwerteZeitreiheBST"
)
USER_AGENT = "Socioeconomic-Data pipeline (fetch_ba_sv_beschaeftigte.py)"
REQUEST_DELAY_SECONDS = 0.4
REQUEST_TIMEOUT_SECONDS = 60
VALUE_COLUMN = "Sozialversicherungspflichtig Beschäftigte"
REFERENCE_MONTH = "Juni"
FIRST_YEAR = 2007  # first month of the BA series is January 2007

DATASET_SUBDIR = Path("sv_beschaeftigte") / "results_sv_beschaeftigte_kreise"
REPO_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUT_DIR = REPO_ROOT / "data-exchange" / "1_friedrich-bjoern"
DEFAULT_NUTS_LOOKUP = DEFAULT_OUT_DIR / "auszubildende" / "results_auszubildende_kreise" / "mapped_pairs.csv"
LOOKUP_GEODATA_YEAR = 2024

# BA Kreisschlüssel -> Kreisschlüssel of the NUTS-2024 Kreis it belonged to.
# Hanau became kreisfrei on 2026-01-01 (split off Main-Kinzig-Kreis, NUTS
# DE719). The BA back-calculates the whole series on its current territorial
# status, so Main-Kinzig-Kreis excludes Hanau in every year; summing both
# restores the NUTS-2024 territory.
# Valid only for NUTS 2024 (LOOKUP_GEODATA_YEAR): NUTS 2027 lists Hanau as its
# own NUTS-3 region (draft act EUR-Lex Ares(2025)6622463: DE71G Hanau, Main-
# Kinzig-Kreis renumbered DE71F; confirm against Regulation (EU) 2026/195).
# For a 2027 lookup the BA series fit the NUTS Kreise as they are and this
# map must be emptied - otherwise Hanau would be counted twice. The check
# below refuses that combination.
TERRITORIAL_MERGES = {"06415": "06435"}
MERGES_VALID_UNTIL_NUTS_YEAR = 2026  # merges only apply to NUTS vintages <= this

MAPPER = "ba_kreis_schluessel"
PARAMETER = "id_ars"
META_COLUMNS = [
    "original_id_1",
    "original_name",
    "geodata_name",
    "geodata_id_nuts",
    "geodata_id_ars",
    "mapper",
    "parameter",
]

META = {
    "geodata_level": "NUTS_3",
    "geodata_year": LOOKUP_GEODATA_YEAR,
    # Same spelling as the documented contract ("absolute or relative", see
    # stg_dataset_metadata_schema.yml) and load_arbeitslosenzahlen_afa.py.
    "number_type": "absolute",
    "name": "Sozialversicherungspflichtig Beschäftigte",
    "description": "Anzahl der sozialversicherungspflichtig Beschäftigten am Arbeitsort je Kreis (Stichtag 30. Juni)",
    "description_long": (
        "Sozialversicherungspflichtig Beschäftigte am Arbeitsort (Kreis, in dem der Betrieb liegt), "
        "Stichtag 30. Juni des jeweiligen Jahres (Monatswert Juni der BA-Zeitreihe). "
        "Gebietsstand der Bundesagentur für Arbeit: laut BA werden die Zeitreihen rückwirkend auf den "
        "aktuellen Gebietsstand umgerechnet (nur bei Göttingen stichprobenhaft geprüft). "
        "Eigene Berechnung: Hanau, Brüder-Grimm-Stadt (seit 2026 kreisfrei, BA-Kreis 06415) ist für "
        "den NUTS-Gebietsstand 2024 dem Main-Kinzig-Kreis (06435) zugeschlagen (Summe beider BA-Werte). "
        "Aus Gründen der statistischen Geheimhaltung sind die Zahlen i. d. R. auf Vielfache von 10 gerundet."
    ),
    "unit": "Personen",
    "source": "Statistik der Bundesagentur für Arbeit, Eckwerte Beschäftigung (Zeitreihe)",
    "algorithm": "Sozialversicherungspflichtig Beschäftigte am Arbeitsort, Wert für Juni des Jahres",
    # Explicit license: load_soziodata_generic._resolve_license() uses it as-is
    # (no BBSR lookup, the source is not BBSR).
    "license": "© Statistik der Bundesagentur für Arbeit – Nutzung mit Quellenangabe, Inhalte nicht verändert",
    "license_url": "https://statistik.arbeitsagentur.de/",
}

GERMAN_MONTHS = {
    "Januar": 1, "Februar": 2, "März": 3, "April": 4, "Mai": 5, "Juni": 6,
    "Juli": 7, "August": 8, "September": 9, "Oktober": 10, "November": 11, "Dezember": 12,
}


def _get(url: str) -> bytes:
    """GET with one retry; raises on failure (a partial extract must not be written)."""
    for attempt in (1, 2):
        try:
            resp = requests.get(url, headers={"User-Agent": USER_AGENT}, timeout=REQUEST_TIMEOUT_SECONDS)
            resp.raise_for_status()
            return resp.content
        except requests.exceptions.RequestException as e:
            if attempt == 2:
                raise
            logger.warning(f"{url}: {e}, retrying once...")
            time.sleep(2)
    raise AssertionError("unreachable")


def _find_kreise(node: Any, found: Dict[str, str]) -> None:
    """Recursively collect formValues entries with Filtername == "Kreis"."""
    if isinstance(node, dict):
        fv = node.get("formValues")
        if isinstance(fv, dict) and fv.get("Filtername") == "Kreis":
            found[fv["BA_SCHL"]] = fv["DESC"]
        for value in node.values():
            _find_kreise(value, found)
    elif isinstance(node, list):
        for value in node:
            _find_kreise(value, found)


def fetch_kreise(cache_dir: Optional[Path]) -> Dict[str, str]:
    """Returns {BA_SCHL (8 digits): official Kreis name}, deduplicated."""
    cache_file = cache_dir / "kreise.json" if cache_dir else None
    if cache_file and cache_file.exists():
        logger.info(f"Using cached Kreis list: {cache_file}")
        entries = json.loads(cache_file.read_text(encoding="utf-8"))
        kreise = {e["BA_SCHL"]: e["DESC"] for e in entries}
    else:
        text = _get(REGION_LIST_URL).decode("utf-8-sig")
        obj = json.loads(text[text.index("{"): text.rindex("}") + 1])
        kreise: Dict[str, str] = {}
        _find_kreise(obj, kreise)
        if cache_file:
            cache_file.parent.mkdir(parents=True, exist_ok=True)
            cache_file.write_text(
                json.dumps([{"BA_SCHL": k, "DESC": v} for k, v in sorted(kreise.items())], ensure_ascii=False),
                encoding="utf-8",
            )
    if not kreise:
        raise RuntimeError("No 'Kreis' entries found in persistListenfelderJSON")
    return kreise


def fetch_kreis_csv(ba_schl: str, name: str, cache_dir: Optional[Path]) -> str:
    cache_file = cache_dir / "kreis_z" / f"{ba_schl}.csv" if cache_dir else None
    if cache_file and cache_file.exists():
        return cache_file.read_bytes().decode("utf-8-sig")
    url = f"{ZEITREIHE_URL}?{urllib.parse.quote('Kreis AO')}={urllib.parse.quote(name, safe='')}"
    raw = _get(url)
    time.sleep(REQUEST_DELAY_SECONDS)
    text = raw.decode("utf-8-sig")
    if text.lstrip().startswith("{"):
        # error responses come as JSON instead of CSV
        raise RuntimeError(f"Kreis {name!r}: error response instead of CSV: {text[:200]}")
    if cache_file:
        cache_file.parent.mkdir(parents=True, exist_ok=True)
        cache_file.write_bytes(raw)
    return text


def parse_june_values(text: str, name: str, years: List[int]) -> Dict[int, int]:
    """Returns {year: June value of VALUE_COLUMN}; raises on an unexpected layout."""
    lines = text.splitlines()
    kreis_line = next((line for line in lines if line.startswith("Kreis:;")), None)
    if kreis_line is None or kreis_line.split(";", 1)[1].strip() != name:
        raise ValueError(f"Kreis {name!r}: response is for {kreis_line!r}")
    try:
        header_idx = next(i for i, line in enumerate(lines) if line.startswith("Berichtsmonat;"))
    except StopIteration:
        raise ValueError(f"Kreis {name!r}: no 'Berichtsmonat;' header line") from None

    reader = csv.reader(lines[header_idx:], delimiter=";")
    header = [h.strip() for h in next(reader)]
    col = header.index(VALUE_COLUMN)
    values: Dict[int, int] = {}
    for row in reader:
        if not row or not row[0].strip() or row[0].startswith("©"):
            break  # copyright/license footer
        parts = row[0].strip().split(" ")
        if len(parts) != 2 or parts[0] not in GERMAN_MONTHS or parts[0] != REFERENCE_MONTH:
            continue
        year = int(parts[1])
        raw = row[col].strip()
        if year in years and raw.isdigit():
            values[year] = int(raw)
        elif year in years:
            logger.warning(f"Kreis {name!r}: non-numeric value {raw!r} for {REFERENCE_MONTH} {year}, left empty")
    return values


def _normalize_ars(raw: str) -> str:
    s = str(raw).strip()
    if s.endswith(".0"):
        s = s[:-2]
    return s.zfill(5) if s.isdigit() else s


def load_nuts_lookup(path: Path) -> Dict[str, Dict[str, str]]:
    """{5-digit ARS: {geodata_id_nuts, geodata_name}} from an existing NUTS-2024 mapped_pairs.csv."""
    meta_path = path.parent / "meta.yaml"
    if meta_path.exists():
        year = (yaml.safe_load(meta_path.read_text(encoding="utf-8")) or {}).get("geodata_year")
        if year != LOOKUP_GEODATA_YEAR:
            raise ValueError(f"{path}: geodata_year {year}, expected {LOOKUP_GEODATA_YEAR}")
    lookup: Dict[str, Dict[str, str]] = {}
    with open(path, newline="", encoding="utf-8") as f:
        for row in csv.DictReader(f):
            ars = _normalize_ars(row["geodata_id_ars"])
            nuts = row["geodata_id_nuts"].strip()
            if not ars or not nuts:
                continue
            if ars in lookup and lookup[ars]["geodata_id_nuts"] != nuts:
                raise ValueError(f"{path}: ARS {ars} maps to {lookup[ars]['geodata_id_nuts']} and {nuts}")
            lookup[ars] = {"geodata_id_nuts": nuts, "geodata_name": row["geodata_name"].strip()}
    return lookup


def _write_csv_atomic(path: Path, header: List[str], rows: List[List[Any]]) -> None:
    tmp = path.with_suffix(path.suffix + ".tmp")
    with open(tmp, "w", newline="", encoding="utf-8") as f:
        writer = csv.writer(f)
        writer.writerow(header)
        writer.writerows(rows)
    tmp.replace(path)


def build_meta(years: List[int]) -> Dict[str, Any]:
    # Same keys as geo-mapper's meta.yaml (see e.g. wohngeldhaushalte): they
    # describe the wide BA-derived table (0 = Kreisschlüssel, 1 = name,
    # 2.. = one column per year); load_soziodata_generic.py ignores them.
    value_columns = list(range(2, 2 + len(years)))
    return {
        "id_columns": [0],
        "name_column": 1,
        "value_columns": value_columns,
        "value_rename": [{idx: str(year)} for idx, year in zip(value_columns, years)],
        **META,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument(
        "--out-dir",
        default=os.environ.get("DATA_EXCHANGE_PATH") or str(DEFAULT_OUT_DIR),
        help=f"DATA_EXCHANGE_PATH-equivalent root; output lands under <out-dir>/{DATASET_SUBDIR}/",
    )
    parser.add_argument("--nuts-lookup", default=str(DEFAULT_NUTS_LOOKUP),
                        help="mapped_pairs.csv of an existing NUTS-2024 Kreis dataset (geodata_id_ars -> geodata_id_nuts)")
    parser.add_argument("--cache-dir", help="Store/reuse raw API responses here (kreise.json, kreis_z/<BA_SCHL>.csv)")
    parser.add_argument("--last-year", type=int, default=2025, help="Last complete year to include (default: 2025)")
    parser.add_argument("--force", action="store_true", help="Rebuild even if the output already exists")
    args = parser.parse_args()

    out_dir = Path(args.out_dir) / DATASET_SUBDIR
    csv_path, meta_path, unmapped_path = out_dir / "mapped_pairs.csv", out_dir / "meta.yaml", out_dir / "unmapped.csv"
    if csv_path.exists() and meta_path.exists() and not args.force:
        logger.info(f"{csv_path} already exists, skipping (use --force to redo)")
        return

    years = list(range(FIRST_YEAR, args.last_year + 1))
    cache_dir = Path(args.cache_dir) if args.cache_dir else None
    lookup = load_nuts_lookup(Path(args.nuts_lookup))
    kreise = fetch_kreise(cache_dir)
    logger.info(f"{len(kreise)} BA Kreise, {len(lookup)} NUTS-2024 Kreise in lookup")

    # Fetch everything first: a single failure aborts before any file is written.
    values_by_key: Dict[str, Dict[int, int]] = {}
    names_by_key: Dict[str, str] = {}
    for i, (ba_schl, name) in enumerate(sorted(kreise.items()), 1):
        key = ba_schl[:5]
        if key in values_by_key:
            raise ValueError(f"Duplicate Kreisschlüssel {key} ({names_by_key[key]!r}, {name!r})")
        values_by_key[key] = parse_june_values(fetch_kreis_csv(ba_schl, name, cache_dir), name, years)
        names_by_key[key] = name
        if i % 50 == 0:
            logger.info(f"  {i}/{len(kreise)} Kreise processed...")

    if TERRITORIAL_MERGES and LOOKUP_GEODATA_YEAR > MERGES_VALID_UNTIL_NUTS_YEAR:
        raise ValueError(
            f"TERRITORIAL_MERGES only apply to NUTS <= {MERGES_VALID_UNTIL_NUTS_YEAR}, but "
            f"LOOKUP_GEODATA_YEAR is {LOOKUP_GEODATA_YEAR}: empty TERRITORIAL_MERGES "
            "(the BA's current territorial status matches NUTS 2027) and check the NUTS lookup."
        )

    merged = 0
    for source_key, target_key in TERRITORIAL_MERGES.items():
        if source_key not in values_by_key:
            continue  # BA no longer/not yet lists it separately
        if target_key not in values_by_key:
            raise ValueError(f"Merge target {target_key} for {source_key} not in BA Kreis list")
        src, dst = values_by_key.pop(source_key), values_by_key[target_key]
        # Only years present on both sides, otherwise the sum would be partial.
        values_by_key[target_key] = {y: dst[y] + src[y] for y in years if y in dst and y in src}
        logger.info(
            f"Merged {names_by_key[source_key]!r} ({source_key}) into {names_by_key[target_key]!r} ({target_key})"
        )
        names_by_key[target_key] = f"{names_by_key[target_key]} (inkl. {names_by_key.pop(source_key)})"
        merged += 1

    rows, unmapped, missing_values = [], [], 0
    for key in sorted(values_by_key):
        match = lookup.get(key)
        if match is None:
            unmapped.append([key, names_by_key[key], f"no NUTS-{LOOKUP_GEODATA_YEAR} match for ARS {key}"])
            logger.warning(f"Unmapped BA Kreis {key} {names_by_key[key]!r}: not in {args.nuts_lookup}")
            continue
        vals = values_by_key[key]
        missing_values += sum(1 for y in years if y not in vals)
        rows.append(
            [key, names_by_key[key], match["geodata_name"], match["geodata_id_nuts"], key, MAPPER, PARAMETER]
            + [vals.get(y, "") for y in years]
        )
    unmatched_lookup = sorted(set(lookup) - set(values_by_key))
    for ars in unmatched_lookup:
        logger.warning(f"NUTS-2024 Kreis {ars} ({lookup[ars]['geodata_id_nuts']}) has no BA Kreis")

    out_dir.mkdir(parents=True, exist_ok=True)
    _write_csv_atomic(unmapped_path, ["original_id_1", "original_name", "reason"], unmapped)
    meta_tmp = meta_path.with_suffix(".yaml.tmp")
    meta_tmp.write_text(yaml.safe_dump(build_meta(years), sort_keys=False, allow_unicode=True), encoding="utf-8")
    meta_tmp.replace(meta_path)
    # mapped_pairs.csv last: its presence (with meta.yaml) marks the output as complete.
    _write_csv_atomic(csv_path, META_COLUMNS + [str(y) for y in years], rows)

    logger.info(
        f"Done: {len(kreise)} BA Kreise, {merged} merged, {len(rows)} mapped, {len(unmapped)} unmapped "
        f"(-> {unmapped_path}), {len(unmatched_lookup)} NUTS Kreise without BA data, "
        f"{missing_values} empty year cells -> {csv_path}"
    )


if __name__ == "__main__":
    main()
