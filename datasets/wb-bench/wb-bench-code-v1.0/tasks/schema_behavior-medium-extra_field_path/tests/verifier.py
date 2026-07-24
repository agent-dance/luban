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

from jsonschema_like.extra import validate

schema = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "profile": {"type": "object", "additionalProperties": False, "properties": {
            "name": {"type": "string"},
            "meta": {"type": "object", "additionalProperties": False, "properties": {"source": {"type": "string"}}},
        }},
        "items": {"type": "array", "items": {"type": "object", "additionalProperties": False, "properties": {"sku": {"type": "string"}, "qty": {"type": "number"}}}},
    },
}


def first(data):
    error = validate(data, schema)[0]
    return error.path, error.schema_path, error.message


def all_paths(data):
    return [error.path for error in validate(data, schema)]

try:
    safe_record("top extra keeps schema path", lambda: first({"debug": True}) == (["debug"], ["additionalProperties"], "additional property not allowed"))
    safe_record("nested extra keeps schema path", lambda: first({"profile": {"age": 3}})[1] == ["properties", "profile", "additionalProperties"] and first({"profile": {"age": 3}})[0] == ["profile", "age"])
    safe_record("deep nested schema path stable", lambda: first({"profile": {"meta": {"extra": "x"}}})[1] == ["properties", "profile", "properties", "meta", "additionalProperties"] and first({"profile": {"meta": {"extra": "x"}}})[0][-1] == "extra")
    safe_record("array item extra includes index", lambda: first({"items": [{"sku": "a", "extra": 1}]})[0] == ["items", 0, "extra"])
    safe_record("second array item extra keeps index", lambda: first({"items": [{"sku": "a"}, {"extra": 1}]})[0] == ["items", 1, "extra"])
    safe_record("array item schema path stable", lambda: first({"items": [{"extra": 1}]})[1] == ["properties", "items", "items", "additionalProperties"] and first({"items": [{"extra": 1}]})[0] == ["items", 0, "extra"])
    safe_record("multiple top extras keep concrete fields", lambda: all_paths({"z": 1, "a": 2}) == [["z"], ["a"]])
    safe_record("multiple nested extras keep concrete fields", lambda: all_paths({"profile": {"z": 1, "a": 2}}) == [["profile", "z"], ["profile", "a"]])
    safe_record("mixed type and extra includes concrete extra", lambda: ["profile", "unknown"] in all_paths({"profile": {"name": 1, "unknown": 2}}))
    safe_record("array mixed type and extra includes concrete extra", lambda: ["items", 0, "unknown"] in all_paths({"items": [{"sku": 3, "unknown": 2}]}))
    safe_record("numeric-like extra key is preserved", lambda: first({"items": [{"0": "bad"}]})[0] == ["items", 0, "0"])
    safe_record("empty string extra key is preserved in path", lambda: first({"profile": {"": 1}})[0] == ["profile", ""])
finally:
    write_reward()
