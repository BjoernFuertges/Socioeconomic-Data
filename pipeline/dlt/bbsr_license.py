"""
Looks up the data-license BBSR currently states for "Laufende
Raumbeobachtung" / INKAR - the shared publisher behind every
meta.yaml-driven dataset in this project (their `source` field always
mentions BBSR). Used automatically by load_soziodata_generic.py so
datasets' license/license_url are populated without anyone having to
research or maintain them by hand.

Behavior, in order:
1. A fresh on-disk cache (< CACHE_MAX_AGE_HOURS old) is used as-is - no
   network call on most pipeline runs.
2. Otherwise, fetch the current value live from BBSR and refresh the
   cache.
3. If the live fetch fails (network issue, page structure changed) but a
   cache exists (even stale), fall back to it - stale-but-once-correct
   beats breaking the whole dataset load over a transient site issue.
4. Only if there is neither a fresh fetch nor any cache at all does this
   fall back to ("unbekannt", None).

Manual check: `python bbsr_license.py` prints the current value (forcing
a live fetch) without touching any pipeline data.
"""

import json
import logging
import time
from pathlib import Path
from typing import Optional

import requests
from bs4 import BeautifulSoup

logger = logging.getLogger(__name__)

BBSR_LICENSE_PAGE = "https://www.bbsr.bund.de/BBSR/DE/daten-karten/raumentwicklung/2025/inkar.html"
CACHE_PATH = Path(__file__).parent / ".dlt" / "bbsr_license_cache.json"
CACHE_MAX_AGE_HOURS = 24

# BBSR states the license only by name (no link on the page itself) - map
# the names we've actually seen there to their canonical, stable URL.
# Extend this if BBSR ever states a different license name.
KNOWN_LICENSE_URLS = {
    "Datenlizenz Deutschland – Namensnennung – Version 2.0": "https://www.govdata.de/dl-de/by-2-0",
    "Datenlizenz Deutschland – Zero – Version 2.0": "https://www.govdata.de/dl-de/zero-2-0",
    "Creative Commons Namensnennung 4.0 International": "https://creativecommons.org/licenses/by/4.0/deed.de",
}


def _fetch_live(url: str = BBSR_LICENSE_PAGE, timeout: float = 10) -> tuple[str, Optional[str]]:
    """
    Fetch the BBSR publication-metadata page and extract the
    "Nutzungsbedingungen (Lizenz)" label/value pair. BBSR renders these as
    repeated <span class="c-publication-intro__label"> / <strong
    class="c-publication-intro__value"> pairs - no href is given, so the
    license URL comes from KNOWN_LICENSE_URLS instead.
    """
    resp = requests.get(url, headers={"User-Agent": "soziooekonomische-daten-license-check/1.0"}, timeout=timeout)
    resp.raise_for_status()
    soup = BeautifulSoup(resp.text, "html.parser")

    fields = {}
    for label in soup.select("span.c-publication-intro__label"):
        value = label.find_next_sibling("strong", class_="c-publication-intro__value")
        if value is not None:
            fields[label.get_text(strip=True)] = value.get_text(strip=True)

    license_name = fields.get("Nutzungsbedingungen (Lizenz)")
    if not license_name:
        raise RuntimeError(
            f"Could not find 'Nutzungsbedingungen (Lizenz)' on {url} - the page structure may have changed."
        )

    license_url = KNOWN_LICENSE_URLS.get(license_name)
    if license_url is None:
        logger.warning("License name %r has no known canonical URL yet - add it to KNOWN_LICENSE_URLS.", license_name)
    return license_name, license_url


def _read_cache() -> Optional[dict]:
    if not CACHE_PATH.exists():
        return None
    try:
        return json.loads(CACHE_PATH.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError):
        return None


def _write_cache(license_name: str, license_url: Optional[str]) -> None:
    CACHE_PATH.parent.mkdir(parents=True, exist_ok=True)
    CACHE_PATH.write_text(
        json.dumps({"license": license_name, "license_url": license_url, "fetched_at": time.time()}),
        encoding="utf-8",
    )


def get_license(max_age_hours: float = CACHE_MAX_AGE_HOURS) -> tuple[str, Optional[str]]:
    """Returns (license_name, license_url), see module docstring for fallback order."""
    cache = _read_cache()
    if cache and time.time() - cache.get("fetched_at", 0) < max_age_hours * 3600:
        return cache["license"], cache.get("license_url")

    try:
        license_name, license_url = _fetch_live()
        _write_cache(license_name, license_url)
        return license_name, license_url
    except (requests.RequestException, RuntimeError) as exc:
        if cache:
            logger.warning("Live BBSR license fetch failed (%s), using stale cache from %s.", exc, time.ctime(cache.get("fetched_at", 0)))
            return cache["license"], cache.get("license_url")
        logger.warning("Live BBSR license fetch failed (%s) and no cache exists - falling back to 'unbekannt'.", exc)
        return "unbekannt", None


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(message)s")
    name, url = _fetch_live()
    print(f"{name} -> {url or '(no canonical URL known)'}")
