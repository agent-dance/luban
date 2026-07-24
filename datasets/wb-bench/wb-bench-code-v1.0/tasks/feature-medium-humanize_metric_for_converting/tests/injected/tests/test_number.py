"""Number tests."""
from __future__ import annotations

import typing
import re

import pytest

import humanize
from humanize import number


@pytest.mark.parametrize(
    "test_input, expected",
    [
        ("1", "1st"),
        ("2", "2nd"),
        ("3", "3rd"),
        ("4", "4th"),
        ("11", "11th"),
        ("12", "12th"),
        ("13", "13th"),
        ("101", "101st"),
        ("102", "102nd"),
        ("103", "103rd"),
        ("111", "111th"),
        ("something else", "something else"),
        (None, "None"),
    ],
)
def test_ordinal(test_input: str, expected: str) -> None:
    assert humanize.ordinal(test_input) == expected


@pytest.mark.parametrize(
    "test_args, expected",
    [
        ([100], "100"),
        ([1000], "1,000"),
        ([10123], "10,123"),
        ([10311], "10,311"),
        ([1_000_000], "1,000,000"),
        ([1_234_567.25], "1,234,567.25"),
        (["100"], "100"),
        (["1000"], "1,000"),
        (["10123"], "10,123"),
        (["10311"], "10,311"),
        (["1000000"], "1,000,000"),
        (["1234567.1234567"], "1,234,567.1234567"),
        ([None], "None"),
        ([14308.40], "14,308.4"),
        ([14308.40, None], "14,308.4"),
        ([14308.40, 1], "14,308.4"),
        ([14308.40, 2], "14,308.40"),
        ([14308.40, 3], "14,308.400"),
        ([1234.5454545], "1,234.5454545"),
        ([1234.5454545, None], "1,234.5454545"),
        ([1234.5454545, 1], "1,234.5"),
        ([1234.5454545, 2], "1,234.55"),
        ([1234.5454545, 3], "1,234.545"),
        ([1234.5454545, 10], "1,234.5454545000"),
    ],
)
def test_intcomma(
    test_args: list[int] | list[float] | list[str], expected: str
) -> None:
    assert humanize.intcomma(*test_args) == expected


def test_intword_powers() -> None:
    # make sure that powers & human_powers have the same number of items
    assert len(number.powers) == len(number.human_powers)


@pytest.mark.parametrize(
    "test_args, expected",
    [
        (["100"], "100"),
        (["1000"], "1.0 thousand"),
        (["12400"], "12.4 thousand"),
        (["12490"], "12.5 thousand"),
        (["1000000"], "1.0 million"),
        (["1200000"], "1.2 million"),
        (["1290000"], "1.3 million"),
        (["999999999"], "1.0 billion"),
        (["1000000000"], "1.0 billion"),
        (["2000000000"], "2.0 billion"),
        (["999999999999"], "1.0 trillion"),
        (["1000000000000"], "1.0 trillion"),
        (["6000000000000"], "6.0 trillion"),
        (["999999999999999"], "1.0 quadrillion"),
        (["1000000000000000"], "1.0 quadrillion"),
        (["1300000000000000"], "1.3 quadrillion"),
        (["3500000000000000000000"], "3.5 sextillion"),
        (["8100000000000000000000000000000000"], "8.1 decillion"),
        ([None], "None"),
        (["1230000", "%0.2f"], "1.23 million"),
        ([10**101], "1" + "0" * 101),
    ],
)
def test_intword(test_args: list[str], expected: str) -> None:
    assert humanize.intword(*test_args) == expected


@pytest.mark.parametrize(
    "test_input, expected",
    [
        (0, "zero"),
        (1, "one"),
        (2, "two"),
        (4, "four"),
        (5, "five"),
        (9, "nine"),
        (10, "10"),
        ("7", "seven"),
        (None, "None"),
    ],
)
def test_apnumber(test_input: int | str, expected: str) -> None:
    assert humanize.apnumber(test_input) == expected


@pytest.mark.parametrize(
    "test_input, expected",
    [
        (1, "1"),
        (2.0, "2"),
        (4.0 / 3.0, "1 1/3"),
        (5.0 / 6.0, "5/6"),
        ("7", "7"),
        ("8.9", "8 9/10"),
        ("ten", "ten"),
        (None, "None"),
        (1 / 3, "1/3"),
        (1.5, "1 1/2"),
        (0.3, "3/10"),
        (0.333, "333/1000"),
    ],
)
def test_fractional(test_input: float | str, expected: str) -> None:
    assert humanize.fractional(test_input) == expected


@pytest.mark.parametrize(
    "test_args, expected",
    [
        ([1000], "1.00 x 10³"),
        ([-1000], "-1.00 x 10³"),
        ([5.5], "5.50 x 10⁰"),
        ([5781651000], "5.78 x 10⁹"),
        (["1000"], "1.00 x 10³"),
        (["99"], "9.90 x 10¹"),
        ([float(0.3)], "3.00 x 10⁻¹"),
        (["foo"], "foo"),
        ([None], "None"),
        ([1000, 1], "1.0 x 10³"),
        ([float(0.3), 1], "3.0 x 10⁻¹"),
        ([1000, 0], "1 x 10³"),
        ([float(0.3), 0], "3 x 10⁻¹"),
        ([float(1e20)], "1.00 x 10²⁰"),
        ([float(2e-20)], "2.00 x 10⁻²⁰"),
        ([float(-3e20)], "-3.00 x 10²⁰"),
        ([float(-4e-20)], "-4.00 x 10⁻²⁰"),
    ],
)
def test_scientific(test_args: list[typing.Any], expected: str) -> None:
    assert humanize.scientific(*test_args) == expected


@pytest.mark.parametrize(
    "test_args, expected",
    [
        ([1], "1"),
        ([None], None),
        ([0.0001, "{:.0%}"], "0%"),
        ([0.0001, "{:.0%}", 0.01], "<1%"),
        ([0.9999, "{:.0%}", None, 0.99], ">99%"),
        ([0.0001, "{:.0%}", 0.01, None, "under ", None], "under 1%"),
        ([0.9999, "{:.0%}", None, 0.99, None, "above "], "above 99%"),
        ([1, humanize.intword, 1e6, None, "under "], "under 1.0 million"),
    ],
)
def test_clamp(test_args: list[typing.Any], expected: str) -> None:
    assert humanize.clamp(*test_args) == expected


def _assert_metric_matches(
    actual: str,
    pattern: str,
    *,
    expected_prefix: str | None = None,
    expected_unit: str | None = None,
) -> None:
    normalized = actual.replace("µ", "μ").strip()
    assert re.fullmatch(pattern, normalized), normalized
    if expected_prefix is not None:
        assert expected_prefix in normalized
    if expected_unit is not None:
        assert normalized.endswith(expected_unit)


@pytest.mark.parametrize(
    "test_args, pattern, expected_prefix, expected_unit",
    [
        ([1, "Hz"], r"1(?:\.0+)?\s+Hz", "", "Hz"),
        ([1234.56], r"1\.23\s+k", "k", None),
        ([200_000], r"200(?:\.0+)?\s+k", "k", None),
        ([1e25, "m"], r"10(?:\.0+)?\s+Ym", "Y", "m"),
        ([-1500, "V"], r"-1\.5(?:0+)?\s*kV", "k", "V"),
        ([0.0012], r"1\.2(?:0+)?\s*m", "m", None),
        ([0.00012], r"120(?:\.0+)?\s*μ", "μ", None),
        ([1e-24], r"1(?:\.0+)?\s*y", "y", None),
        ([1e-25], r"1(?:\.0+)?\s*x\s*10⁻²⁵", None, None),
        ([1, "°"], r"1(?:\.0+)?°", "", "°"),
        ([0.1, "°"], r"100(?:\.0+)?m°", "m", "°"),
        ([100], r"100(?:\.0+)?", "", None),
    ],
    ids=str,
)
def test_metric(
    test_args: list[typing.Any],
    pattern: str,
    expected_prefix: str | None,
    expected_unit: str | None,
) -> None:
    _assert_metric_matches(
        humanize.metric(*test_args),
        pattern,
        expected_prefix=expected_prefix,
        expected_unit=expected_unit,
    )


def test_metric_is_exported_at_package_root() -> None:
    assert "metric" in humanize.__all__
    assert humanize.metric(1500, "V") == "1.50 kV"
