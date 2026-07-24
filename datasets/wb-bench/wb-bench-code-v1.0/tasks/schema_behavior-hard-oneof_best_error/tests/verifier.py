import json
import os
import sys
from pathlib import Path
sys.dont_write_bytecode = True
ROOT = Path(os.environ.get("WORKSPACE", "/workspace"))
LOG_DIR = Path(os.environ.get("LOG_DIR", "/logs/verifier"))
LOG_DIR.mkdir(parents=True, exist_ok=True)
sys.path.insert(0, str(ROOT))
RESULTS = []
def record(name, passed, detail=""):
    RESULTS.append({"name": name, "passed": bool(passed), "detail": str(detail)})
def safe_record(name, check):
    try:
        record(name, check())
    except Exception as exc:
        record(name, False, f"{type(exc).__name__}: {exc}")
def write_reward():
    passed = sum(1 for item in RESULTS if item["passed"])
    total = len(RESULTS) or 1
    reward = {"overall": passed / total, "test_pass_rate": passed / total, "tests_passed": passed, "tests_total": total, "test_status": "pass" if passed == total else "no_pass", "tests": RESULTS}
    (LOG_DIR / "reward.json").write_text(json.dumps(reward, indent=2, ensure_ascii=False))
    (LOG_DIR / "reward.txt").write_text(str(reward["overall"]))
    print(json.dumps(reward, indent=2, ensure_ascii=False))

from jsonschema_like.best import ValidationError, best_match, validate

animal_schema = {"oneOf": [
    {"type": "object", "required": ["kind", "meows"], "properties": {"kind": {"enum": ["cat"]}, "meows": {"type": "number"}}},
    {"type": "object", "required": ["kind", "barks"], "properties": {"kind": {"enum": ["dog"]}, "barks": {"type": "number"}}},
]}


def best(data):
    return best_match(validate(data, animal_schema))


def loc(data):
    return best(data).path


def msg(data):
    return best(data).message


def error_fingerprint(error):
    return (error.message, tuple(error.path), tuple(error.schema_path))


def validate_error_set(data):
    return {error_fingerprint(error) for error in validate(data, animal_schema)}

try:
    safe_record("cat branch type error is best", lambda: loc({"kind": "cat", "meows": "yes"}) == ["meows"])
    safe_record("dog branch type error is best", lambda: loc({"kind": "dog", "barks": "loud"}) == ["barks"])
    safe_record("wrong tag points at kind", lambda: loc({"kind": "bird", "meows": 1}) == ["kind"])
    safe_record("missing cat value points at meows", lambda: loc({"kind": "cat"}) == ["meows"])
    safe_record("generic oneOf is not selected", lambda: msg({"kind": "cat", "meows": "yes"}) != "is not valid under any schema")
    safe_record("selected cat branch beats dog enum mismatch", lambda: loc({"kind": "cat", "barks": "bad"}) == ["meows"])
    safe_record("cat type error beats other branch missing", lambda: best({"kind": "cat", "meows": "bad"}).message == "expected number")
    safe_record("manual deeper error wins over generic", lambda: best_match([ValidationError("is not valid under any schema", []), ValidationError("expected number", ["a", 0], ["type"])]).path == ["a", 0])
    safe_record("manual all enum still points at discriminator", lambda: best_match([ValidationError("is not valid under any schema", []), ValidationError("not in enum", ["kind"], ["oneOf", 0, "properties", "kind", "enum"]), ValidationError("not in enum", ["kind"], ["oneOf", 1, "properties", "kind", "enum"])]).path == ["kind"])
    safe_record("as_dict exposes best error", lambda: best({"kind": "dog", "barks": "bad"}).as_dict()["path"] == ["barks"])
    safe_record("validate keeps full oneof error set for cat type error", lambda: validate_error_set({"kind": "cat", "meows": "bad"}) == {
        ("is not valid under any schema", (), ("oneOf",)),
        ("expected number", ("meows",), ("oneOf", 0, "properties", "meows", "type")),
        ("missing barks", ("barks",), ("oneOf", 1, "required")),
        ("not in enum", ("kind",), ("oneOf", 1, "properties", "kind", "enum")),
    })
    safe_record("validate keeps full oneof error set for dog type error", lambda: validate_error_set({"kind": "dog", "barks": "bad"}) == {
        ("is not valid under any schema", (), ("oneOf",)),
        ("missing meows", ("meows",), ("oneOf", 0, "required")),
        ("not in enum", ("kind",), ("oneOf", 0, "properties", "kind", "enum")),
        ("expected number", ("barks",), ("oneOf", 1, "properties", "barks", "type")),
    })
finally:
    write_reward()
