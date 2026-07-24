"""Minimal helpers stub for test conftest compatibility."""
from unittest.mock import MagicMock
from contextlib import contextmanager

MOCK_DEFAULT_GIT_REVISION = "abcdef1234567890"


class TestLocker:
    pass


class TestRepository:
    pass


def get_package(*args, **kwargs):
    return MagicMock()


def http_setup_redirect(*args, **kwargs):
    pass


@contextmanager
def isolated_environment(*args, **kwargs):
    yield


def mock_clone(*args, **kwargs):
    pass


def set_keyring_backend(*args, **kwargs):
    pass


@contextmanager
def switch_working_directory(*args, **kwargs):
    yield


@contextmanager
def with_working_directory(*args, **kwargs):
    yield


def mock_metadata_entry_points(*args, **kwargs):
    return []
