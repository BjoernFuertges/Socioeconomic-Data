"""
DLT pipeline for loading unemployment figures per Agenturbezirk from the
official, auth-free "Lange Zeitreihen" REST API of the Statistik der
Bundesagentur für Arbeit (EckwerteZeitreiheALO?Agentur für Arbeit=<name>,
see https://statistik.arbeitsagentur.de/.../API-ALO-Zeitreihe.html).
"""

import argparse
import csv
import json
import logging
import time
from datetime import datetime, timezone
from typing import Any, Dict, Iterator, List, Optional

import dlt
import requests

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

REGION_LIST_URL = (
    "https://statistik.arbeitsagentur.de/Auswahl/Layout/Statistiktabellen/JS/"
    "DR-LZR-persistListenfelderJSON.js"
)
ZEITREIHE_URL = (
    "https://statistik-dr.arbeitsagentur.de/bifrontend/bids-api/ct/v1/"
    "tableFetch/csv/EckwerteZeitreiheALO"
)
REQUEST_DELAY_SECONDS = 0.4
# "replace" would wipe the table, so abort rather than load if more districts than this fail.
MAX_SKIPPED_FRACTION = 0.1

# No named standard license (e.g. CC BY): the source states its own reuse terms
# in every response; license_url points to the docs page describing them.
LICENSE_NAME = "Freie Nutzung mit Quellenangabe (Statistik der Bundesagentur für Arbeit)"
LICENSE_URL = "https://statistik.arbeitsagentur.de/DE/Statischer-Content/Service/API/API-ALO-Zeitreihe.html"

GERMAN_MONTHS = {
    "Januar": 1, "Februar": 2, "März": 3, "April": 4, "Mai": 5, "Juni": 6,
    "Juli": 7, "August": 8, "September": 9, "Oktober": 10, "November": 11, "Dezember": 12,
}


def fetch_agenturbezirke() -> List[Dict[str, str]]:
    """
    Fetches the official Agenturbezirk list (name + BA key) from the same JS
    data field the interactive statistics page uses, instead of hardcoding it.
    """
    resp = requests.get(REGION_LIST_URL, timeout=30)
    resp.raise_for_status()
    text = resp.text
    obj = json.loads(text[text.index("{"): text.rindex("}") + 1])

    for entries in obj.values():
        if entries and entries[0].get("formValues", {}).get("Filtername") == "Agentur für Arbeit":
            return [
                {"name": e["formValues"]["DESC"], "ba_schl": e["formValues"]["BA_SCHL"]}
                for e in entries
            ]
    raise RuntimeError("Konnte 'Agentur für Arbeit'-Einträge in persistListenfelderJSON nicht finden")


def _parse_value(raw: str) -> Optional[float]:
    raw = raw.strip()
    if not raw or raw == "x":
        return None
    try:
        return float(raw)
    except ValueError:
        logger.warning(f"Konnte Wert nicht parsen: {raw!r}")
        return None


def fetch_agentur_timeseries(name: str) -> List[Dict[str, Any]]:
    """
    Fetches the monthly series for one Agenturbezirk. Returns an empty list
    instead of raising so a single bad district does not abort the run (the
    caller enforces MAX_SKIPPED_FRACTION).
    """
    try:
        resp = requests.get(ZEITREIHE_URL, params={"Agentur für Arbeit": name}, timeout=30)
    except requests.exceptions.RequestException as e:
        # One retry absorbs short DNS/connection blips without stalling the run on a real outage.
        logger.warning(f"Agenturbezirk {name!r}: Netzwerkfehler ({e}), ein Versuch erneut...")
        time.sleep(2)
        try:
            resp = requests.get(ZEITREIHE_URL, params={"Agentur für Arbeit": name}, timeout=30)
        except requests.exceptions.RequestException as e2:
            logger.warning(f"Agenturbezirk {name!r}: erneuter Netzwerkfehler ({e2}), überspringe")
            return []
    if resp.status_code != 200:
        logger.warning(f"Agenturbezirk {name!r}: HTTP {resp.status_code}, überspringe")
        return []

    text = resp.content.decode("utf-8-sig")
    if text.lstrip().startswith("{"):
        # error responses come as JSON instead of CSV (z.B. "501 Datensatz leer")
        logger.warning(f"Agenturbezirk {name!r}: Fehlerantwort statt CSV, überspringe: {text[:200]}")
        return []

    lines = text.splitlines()
    try:
        header_idx = next(i for i, line in enumerate(lines) if line.startswith("Berichtsmonat;"))
    except StopIteration:
        logger.warning(f"Agenturbezirk {name!r}: keine Kopfzeile 'Berichtsmonat;' gefunden, überspringe")
        return []

    rows: List[Dict[str, Any]] = []
    reader = csv.reader(lines[header_idx:], delimiter=";")
    next(reader)  # skip header
    for row in reader:
        if not row or not row[0].strip() or row[0].startswith("©"):
            break  # reached license/copyright footer
        berichtsmonat_parts = row[0].strip().split(" ")
        if len(berichtsmonat_parts) != 2 or berichtsmonat_parts[0] not in GERMAN_MONTHS:
            continue
        rows.append({
            "year": int(berichtsmonat_parts[1]),
            "month": GERMAN_MONTHS[berichtsmonat_parts[0]],
            "arbeitslose": _parse_value(row[1]) if len(row) > 1 else None,
            "unterbeschaeftigung": _parse_value(row[2]) if len(row) > 2 else None,
        })
    return rows


@dlt.resource(name="results_arbeitslosenzahlen_afa_agenturbezirke", write_disposition="replace")
def load_arbeitslosenzahlen(limit: Optional[int] = None) -> Iterator[Dict[str, Any]]:
    agenturbezirke = fetch_agenturbezirke()
    if limit:
        agenturbezirke = agenturbezirke[:limit]
    logger.info(f"Lade Zeitreihen für {len(agenturbezirke)} Agenturbezirke...")

    skipped = 0
    for i, agentur in enumerate(agenturbezirke, start=1):
        rows = fetch_agentur_timeseries(agentur["name"])
        if not rows:
            skipped += 1
        for row in rows:
            yield {
                "geodata_id_agentur": agentur["ba_schl"],
                "geodata_name": agentur["name"],
                **row,
                "loaded_at": datetime.now(timezone.utc).isoformat(),
            }
        if i % 20 == 0:
            logger.info(f"  {i}/{len(agenturbezirke)} Agenturbezirke verarbeitet ({skipped} übersprungen)...")
        time.sleep(REQUEST_DELAY_SECONDS)

    logger.info(
        f"Fertig: {len(agenturbezirke) - skipped}/{len(agenturbezirke)} Agenturbezirke geladen, "
        f"{skipped} übersprungen."
    )
    if not limit and skipped > len(agenturbezirke) * MAX_SKIPPED_FRACTION:
        raise RuntimeError(f"{skipped}/{len(agenturbezirke)} Agenturbezirke übersprungen, breche ab statt Tabelle zu ersetzen")


@dlt.resource(
    name="dataset_metadata",
    write_disposition="merge",
    primary_key="dataset_slug",
    # Type hints as in load_soziodata_generic.py: algorithm is always NULL here
    # and dlt would otherwise drop the column.
    columns={
        "license": {"data_type": "text"},
        "license_url": {"data_type": "text"},
        "algorithm": {"data_type": "text"},
    },
)
def arbeitslosenzahlen_afa_metadata() -> Iterator[Dict[str, Any]]:
    yield {
        "dataset_slug": "arbeitslosenzahlen-afa-agenturbezirke",
        "table_name": "results_arbeitslosenzahlen_afa_agenturbezirke",
        "name": "Arbeitslosenzahlen (Agenturbezirke)",
        "description": "Monatliche Arbeitslosen- und Unterbeschäftigungszahlen je Agenturbezirk der Bundesagentur für Arbeit.",
        "description_long": (
            "Direkt von der Statistik der Bundesagentur für Arbeit über die öffentliche "
            "'Lange Zeitreihen'-API bezogen (EckwerteZeitreiheALO, Agenturbezirk-Parametrisierung). "
            "Agenturbezirke sind eine eigenständige, von Kreisen/NUTS-3/LAU unabhängige "
            "Gebietsgliederung der Bundesagentur für Arbeit."
        ),
        "unit": "Personen",
        "source": "Statistik der Bundesagentur für Arbeit",
        "license": LICENSE_NAME,
        "license_url": LICENSE_URL,
        "algorithm": None,
        "number_type": "absolute",
        "geodata_level": "AGENTURBEZIRK",
        "geodata_year": datetime.now(timezone.utc).year,
        "updated_at": datetime.now(timezone.utc).isoformat(),
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--limit", type=int, default=None, help="Nur die ersten N Agenturbezirke laden (Dev/Test; ersetzt die Tabelle dabei ebenfalls komplett)")
    args = parser.parse_args()

    pipeline = dlt.pipeline(
        pipeline_name="arbeitslosenzahlen_afa_pipeline",
        destination="postgres",
        dataset_name="loaded_data",
    )
    load_info = pipeline.run([
        load_arbeitslosenzahlen(limit=args.limit),
        arbeitslosenzahlen_afa_metadata(),
    ])
    logger.info(f"Load completed: {load_info}")

    print("\n" + "=" * 70)
    print("ARBEITSLOSENZAHLEN AFA (AGENTURBEZIRKE) LOAD SUMMARY")
    print("=" * 70)
    print(f"Result: {load_info}")
    print("=" * 70)


if __name__ == "__main__":
    main()
