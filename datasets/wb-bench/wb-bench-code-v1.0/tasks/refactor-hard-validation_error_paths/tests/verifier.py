
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
try:
    import jsonschema_like.validator as validator_module
    validate = getattr(validator_module, "validate")
    best_match = getattr(validator_module, "best_match")
except Exception as import_error:
    def validate(*args, _import_error=import_error, **kwargs):
        raise _import_error
    def best_match(*args, _import_error=import_error, **kwargs):
        raise _import_error
schema={"type":"object","required":["name","items"],"properties":{"name":{"type":"string"},"status":{"enum":["new","done"]},"items":{"type":"array","items":{"type":"object","required":["qty"],"properties":{"qty":{"type":"number"}}}}}}
try:
    safe_record("valid object has no errors", lambda: validate({"name":"a","items":[{"qty":1}],"status":"new"}, schema) == [])
    def nested_errors():
        return validate({"items":[{"qty":"x"},{}],"status":"bad"}, schema)
    safe_record("reports four errors", lambda: len(nested_errors())==4)
    safe_record("required path root", lambda: nested_errors()[0].path == [] and "name" in nested_errors()[0].message)
    safe_record("required schema path", lambda: nested_errors()[0].schema_path == ["required"])
    safe_record("enum error path", lambda: any(e.path==["status"] and e.schema_path==["properties","status","enum"] for e in nested_errors()))
    safe_record("array item type path", lambda: any(e.path==["items",0,"qty"] for e in nested_errors()))
    safe_record("array item schema path", lambda: any(e.schema_path==["properties","items","items","properties","qty","type"] for e in nested_errors()))
    safe_record("nested required path", lambda: any(e.path==["items",1] and "qty" in e.message for e in nested_errors()))
    safe_record("object type error", lambda: validate([], schema)[0].message == "is not of type object")
    safe_record("array type error", lambda: validate({"name":"a","items":"no"}, schema)[0].path == ["items"])
    safe_record("best match prefers shallow", lambda: best_match(nested_errors()).path == [])
    safe_record("best match empty none", lambda: best_match([]) is None)
finally: write_reward()
