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

from jsonschema_like.unevaluated import ValidationError, validate

schema_allof = {"type":"object", "allOf":[{"properties":{"a":{"type":"string"}}}, {"properties":{"b":{"type":"number"}}}], "unevaluatedProperties": False}
schema_anyof = {"type":"object", "anyOf":[{"properties":{"kind":{"type":"string"}, "name":{"type":"string"}}}, {"properties":{"kind":{"type":"string"}, "score":{"type":"number"}}}], "unevaluatedProperties": False}
schema_nested = {"type":"object", "properties":{"outer":{"type":"object", "allOf":[{"properties":{"x":{"type":"string"}}}], "unevaluatedProperties": False}}, "unevaluatedProperties": False}

def paths(errors):
    return [e.path for e in errors]

def schema_paths(errors):
    return [e.schema_path for e in errors]

def messages(errors):
    return [e.message for e in errors]

try:
    safe_record("allof extra is unevaluated", lambda: paths(validate({"a":"x","b":1,"c":2}, schema_allof)) == [["c"]])
    safe_record("allof invalid prop is still evaluated", lambda: ["a"] not in [p for p,m in zip(paths(validate({"a":1,"b":2}, schema_allof)), messages(validate({"a":1,"b":2}, schema_allof))) if m == "unevaluated property not allowed"])
    safe_record("anyof failed branch field stays unevaluated", lambda: ["name"] in paths(validate({"kind":"x","name":1,"extra":2}, schema_anyof)))
    safe_record("anyof extra after success is caught", lambda: paths(validate({"kind":"user","name":"n","extra":1}, schema_anyof)) == [["extra"]])
    safe_record("anyof common property evaluated once", lambda: ["kind"] not in paths(validate({"kind":"user","name":"n"}, schema_anyof)))
    safe_record("nested unevaluated path is concrete", lambda: paths(validate({"outer":{"x":"ok","y":1}}, schema_nested)) == [["outer","y"]])
    safe_record("top unevaluated still checked", lambda: paths(validate({"outer":{"x":"ok"},"z":1}, schema_nested)) == [["z"]])
    safe_record("property validation recurses", lambda: paths(validate({"outer":{"x":1}}, schema_nested)) == [["outer","x"]])
    safe_record("allof two extras are both reported", lambda: paths(validate({"a":"x","b":1,"c":2,"d":3}, schema_allof)) == [["c"], ["d"]])
    safe_record("nested allof extra keeps schema path", lambda: schema_paths(validate({"outer":{"x":"ok","y":1}}, schema_nested))[0] == ["properties","outer","unevaluatedProperties"])
    safe_record("anyof failed branch field and extra both reported", lambda: paths(validate({"kind":"x","name":1,"extra":2}, schema_anyof)) == [["name"], ["extra"]])
    safe_record("allof evaluated numeric field not reported", lambda: ["b"] not in paths(validate({"a":"x","b":1,"c":2}, schema_allof)))
    safe_record("top and nested unevaluated can both report", lambda: paths(validate({"outer":{"x":"ok","y":1},"z":1}, schema_nested)) == [["outer","y"], ["z"]])
    safe_record("allof invalid plus extra separates errors", lambda: paths(validate({"a":1,"b":2,"c":3}, schema_allof)) == [["a"], ["c"]])
    safe_record("anyof first branch extra score is caught when only first matches", lambda: paths(validate({"kind":"user","name":"n","score":"bad"}, schema_anyof)) == [["score"]])
finally:
    write_reward()
