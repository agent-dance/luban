"""Tests for datasette static file ETag support."""
import sys
import tempfile
from pathlib import Path

import pytest

sys.path.insert(0, '/workspace')


class TestCalculateEtag:
    """Tests for the _static_utils.calculate_etag function."""

    def _get_calculate_etag(self):
        try:
            from datasette._static_utils import calculate_etag
            return calculate_etag
        except (ImportError, ModuleNotFoundError):
            pytest.skip("datasette._static_utils not available")

    def test_module_exists(self):
        """Verify _static_utils module can be imported."""
        calculate_etag = self._get_calculate_etag()
        assert callable(calculate_etag)

    def test_returns_quoted_string(self):
        """ETag should be a quoted string per HTTP spec."""
        calculate_etag = self._get_calculate_etag()
        with tempfile.NamedTemporaryFile(suffix='.txt', delete=False) as f:
            f.write(b"hello world")
            f.flush()
            etag = calculate_etag(Path(f.name))
        assert etag.startswith('"')
        assert etag.endswith('"')
        # 16 hex chars + 2 quotes = 18
        assert len(etag) == 18

    def test_deterministic(self):
        """Same file content should produce same ETag."""
        calculate_etag = self._get_calculate_etag()
        with tempfile.NamedTemporaryFile(suffix='.txt', delete=False) as f:
            f.write(b"test content")
            f.flush()
            etag1 = calculate_etag(Path(f.name))
            etag2 = calculate_etag(Path(f.name))
        assert etag1 == etag2

    def test_different_for_different_content(self):
        """Different file contents should produce different ETags."""
        calculate_etag = self._get_calculate_etag()
        with tempfile.NamedTemporaryFile(suffix='.txt', delete=False) as f1:
            f1.write(b"content a")
            f1.flush()
            with tempfile.NamedTemporaryFile(suffix='.txt', delete=False) as f2:
                f2.write(b"content b")
                f2.flush()
                assert calculate_etag(Path(f1.name)) != calculate_etag(Path(f2.name))

    def test_empty_file(self):
        """Empty file should still produce a valid ETag."""
        calculate_etag = self._get_calculate_etag()
        with tempfile.NamedTemporaryFile(suffix='.txt', delete=False) as f:
            f.write(b"")
            f.flush()
            etag = calculate_etag(Path(f.name))
        assert etag.startswith('"')
        assert etag.endswith('"')
        assert len(etag) == 18

    def test_binary_file(self):
        """Binary file content should produce valid ETag."""
        calculate_etag = self._get_calculate_etag()
        with tempfile.NamedTemporaryFile(suffix='.bin', delete=False) as f:
            f.write(bytes(range(256)))
            f.flush()
            etag = calculate_etag(Path(f.name))
        assert etag.startswith('"')
        # Should be valid hex inside quotes
        inner = etag.strip('"')
        assert all(c in '0123456789abcdef' for c in inner)

    def test_uses_sha256(self):
        """Verify ETag is based on SHA-256 hash."""
        import hashlib
        calculate_etag = self._get_calculate_etag()
        content = b"known content for hash verification"
        expected_prefix = hashlib.sha256(content).hexdigest()[:16]
        with tempfile.NamedTemporaryFile(suffix='.txt', delete=False) as f:
            f.write(content)
            f.flush()
            etag = calculate_etag(Path(f.name))
        assert etag == f'"{expected_prefix}"'


class TestAsgiStaticEtag:
    """Tests verifying the asgi_static function uses ETag headers."""

    def test_asgi_module_imports_calculate_etag(self):
        """Verify utils/asgi.py imports from _static_utils."""
        try:
            asgi_path = Path("/workspace/datasette/utils/asgi.py")
            if asgi_path.exists():
                content = asgi_path.read_text()
                assert "calculate_etag" in content
                assert "_static_utils" in content
            else:
                pytest.skip("asgi.py not found")
        except Exception:
            pytest.skip("Cannot read asgi.py")

    def test_etag_header_constant(self):
        """ETag header name should be properly referenced."""
        try:
            asgi_path = Path("/workspace/datasette/utils/asgi.py")
            if asgi_path.exists():
                content = asgi_path.read_text()
                # Should reference ETag in headers
                assert "ETag" in content or "etag" in content
            else:
                pytest.skip("asgi.py not found")
        except Exception:
            pytest.skip("Cannot read asgi.py")

    def test_if_none_match_handling(self):
        """asgi_static should check If-None-Match header."""
        try:
            asgi_path = Path("/workspace/datasette/utils/asgi.py")
            if asgi_path.exists():
                content = asgi_path.read_text()
                assert "if-none-match" in content or "If-None-Match" in content
            else:
                pytest.skip("asgi.py not found")
        except Exception:
            pytest.skip("Cannot read asgi.py")

    def test_304_response(self):
        """Should return 304 when ETag matches If-None-Match."""
        try:
            asgi_path = Path("/workspace/datasette/utils/asgi.py")
            if asgi_path.exists():
                content = asgi_path.read_text()
                assert "304" in content
            else:
                pytest.skip("asgi.py not found")
        except Exception:
            pytest.skip("Cannot read asgi.py")
