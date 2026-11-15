"""Regression tests for load_soziodata_generic._parse_german_number.

Run from pipeline/dlt/:  python -m pytest tests/   (or: python tests/test_parse_german_number.py)
"""
import math
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import pytest  # noqa: E402

from load_soziodata_generic import _parse_german_number as parse  # noqa: E402


@pytest.mark.parametrize(
    "raw, expected",
    [
        ("357.948,00", 357948.0),
        ("1.234,5", 1234.5),
        ("13,15", 13.15),
        ("87.276,00", 87276.0),
        ("1.234", 1234.0),
        ("1.234.567", 1234567.0),
        ("46450", 46450.0),
        ("-3,5", -3.5),
        # Float stringified by pandas must not become 464500
        ("46450.0", 46450.0),
        ("13.5", 13.5),
        (46450.0, 46450.0),
        (46450, 46450.0),
        (" 7,25 ", 7.25),
    ],
)
def test_values(raw, expected):
    assert parse(raw) == pytest.approx(expected)


@pytest.mark.parametrize("raw", [None, "", "  ", float("nan"), "abc"])
def test_empty_or_invalid(raw):
    assert parse(raw) is None


if __name__ == "__main__":
    sys.exit(pytest.main([__file__, "-q"]))
