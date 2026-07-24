"""Minimal conftest for poetry-tooling tests.

Overrides the original conftest.py (which has heavy fixture dependencies) but
still provides the ``fixture_dir`` helper that the scored factory tests need.
"""
from __future__ import annotations

from pathlib import Path

import pytest


@pytest.fixture
def fixture_dir():
    def _fixture_dir(name: str) -> Path:
        return Path(__file__).parent / "fixtures" / name

    return _fixture_dir
