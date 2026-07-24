
import json
import os
import sys
import time
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


def raises(exc_type, fn, contains=None):
    try:
        fn()
    except exc_type as exc:
        return contains is None or contains in str(exc)
    except Exception:
        return False
    return False


def write_reward():
    passed = sum(1 for item in RESULTS if item["passed"])
    total = len(RESULTS) or 1
    reward = {
        "overall": passed / total,
        "test_pass_rate": passed / total,
        "tests_passed": passed,
        "tests_total": total,
        "test_status": "pass" if passed == total else "no_pass",
        "tests": RESULTS,
    }
    (LOG_DIR / "reward.json").write_text(json.dumps(reward, indent=2, ensure_ascii=False))
    (LOG_DIR / "reward.txt").write_text(str(reward["overall"]))
    print(json.dumps(reward, indent=2, ensure_ascii=False))


from jsonschema_like.validator import ValidationError, get_stats, reset_stats, validate

schema = {"type":"object","required":["items"],"properties":{"items":{"type":"array","items":{"type":"object","required":["id"],"properties":{"id":{"type":"integer","minimum":1},"kind":{"type":"string","enum":["a","b"]}}}}}}
good = {"items":[{"id":1,"kind":"a"},{"id":2,"kind":"b"}]}
try:
    reset_stats()
    for _ in range(40):
        validate(good, schema)
    safe_record("same schema compiles once", lambda: get_stats()["compile_calls"] == 1)
    reset_stats(); validate(good, dict(schema)); validate(good, dict(schema))
    safe_record("equivalent schema dict reuses cache", lambda: get_stats()["compile_calls"] == 1)
    mutated = dict(schema); mutated["properties"] = dict(schema["properties"]); mutated["properties"]["items"] = dict(schema["properties"]["items"]); mutated["properties"]["items"]["type"] = "object"
    reset_stats(); validate(good, schema)
    safe_record("original schema still validates after external copy mutation", lambda: validate(good, schema) is True and get_stats()["compile_calls"] == 1)
    def bad_path():
        try: validate({"items":[{"id":0}]}, schema)
        except ValidationError as exc: return exc.path == ("items",0,"id") and exc.schema_path[-1] == "minimum"
        return False
    reset_stats(); bad_path(); bad_path()
    safe_record("error path remains precise and cached", lambda: bad_path() and get_stats()["compile_calls"] == 1)
    anyof = {"anyOf":[{"type":"integer","minimum":10},{"type":"string","enum":["ok"]}]}
    reset_stats(); validate(12, anyof); validate("ok", anyof)
    safe_record("anyOf branches share compiled schema", lambda: get_stats()["compile_calls"] == 1)
    def anyof_error():
        try: validate(3, anyof)
        except ValidationError as exc: return exc.path == () and "minimum" in exc.schema_path
        return False
    reset_stats(); anyof_error(); anyof_error()
    safe_record("anyOf failure keeps first useful error and reuses cache", lambda: anyof_error() and get_stats()["compile_calls"] == 1)
    reset_stats()
    for i in range(70):
        validate(i, {"type":"integer","minimum":i})
    validate(0, {"type":"integer","minimum":0})
    safe_record("schema cache is bounded", lambda: get_stats()["compile_calls"] == 71)
    schema2 = {"type":"object","required":["x"],"properties":{"x":{"type":"integer"}}}
    reset_stats(); validate({"x":1}, schema2); schema2["properties"]["x"]["type"] = "string"
    safe_record("schema content change invalidates cache", lambda: raises(ValidationError, lambda: validate({"x":1}, schema2), "expected string") and get_stats()["compile_calls"] == 2)
    reset_stats(); start=time.perf_counter()
    for _ in range(1000): validate(good, schema)
    safe_record("large repeated validation stays quick", lambda: time.perf_counter()-start < 0.35 and get_stats()["compile_calls"] == 1)
    reset_stats()
    for _ in range(5):
        raises(ValidationError, lambda: validate(True, {"type":"integer"}), "expected integer")
    safe_record("failing validations also reuse compiled schema", lambda: get_stats()["compile_calls"] == 1)
    reset_stats()
    for _ in range(3):
        raises(ValidationError, lambda: validate({}, {"type":"object","required":["x"]}), "missing required property x")
    safe_record("missing required path points to property and is cached", lambda: get_stats()["compile_calls"] == 1)
finally:
    write_reward()
