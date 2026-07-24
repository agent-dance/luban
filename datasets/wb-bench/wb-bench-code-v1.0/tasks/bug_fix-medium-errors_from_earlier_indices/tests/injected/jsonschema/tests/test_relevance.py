"""Behavioral tests for best_match relevance."""
from jsonschema import exceptions
from jsonschema.validators import _LATEST_VERSION


class TestBasicValidate:
    """Test basic jsonschema validation passes for valid data."""
    pytestmark = []  # Override module-level skipif

    def test_basic_string_validation(self):
        from jsonschema import validate
        validate(instance='hello', schema={'type': 'string'})


class TestRelevanceKey:
    def test_import(self):
        schema = {"anyOf": [{"type": "object"}, {"items": {"const": 37}}]}
        errors = list(_LATEST_VERSION(schema).iter_errors([12, 12]))
        best = exceptions.best_match(errors)
        assert best.validator == "const"

    def test_returns_tuple(self):
        schema = {"anyOf": [{"items": {"const": 37}}]}
        errors = list(_LATEST_VERSION(schema).iter_errors([12, 12]))
        best = exceptions.best_match(errors)
        reversed_best = exceptions.best_match(reversed(errors))
        assert best._contents() == reversed_best._contents()
