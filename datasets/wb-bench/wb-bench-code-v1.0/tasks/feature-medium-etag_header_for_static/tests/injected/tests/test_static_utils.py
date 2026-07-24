"""Tests for the _static_utils module."""
import pytest
import tempfile
from pathlib import Path



try:
    import datasette._static_utils
    _UTIL_AVAILABLE = True
except (ImportError, ModuleNotFoundError):
    _UTIL_AVAILABLE = False

pytestmark = pytest.mark.skipif(not _UTIL_AVAILABLE, reason="utility module not created by agent")

class TestCalculateEtag:
    def test_import(self):
        assert callable(calculate_etag)

    def test_returns_quoted_string(self):
        with tempfile.NamedTemporaryFile(suffix=".txt") as f:
            f.write(b"hello world")
            f.flush()
            etag = calculate_etag(Path(f.name))
            assert etag.startswith('"')
            assert etag.endswith('"')

    def test_same_content_same_etag(self):
        with tempfile.NamedTemporaryFile(suffix=".txt") as f:
            f.write(b"test content")
            f.flush()
            etag1 = calculate_etag(Path(f.name))
            etag2 = calculate_etag(Path(f.name))
            assert etag1 == etag2
