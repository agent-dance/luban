
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


from fastapi_like.openapi import build_parameters

route = {"parameters":[
    {"name":"q","in":"query","type":"string","required":False,"nullable":True,"description":"search"},
    {"name":"user_agent","alias":"User-Agent","in":"header","type":"string","examples":{"chrome":{"value":"Chrome"}}},
    {"name":"session","in":"cookie","type":"string","deprecated":True},
    {"name":"item_id","in":"path","type":"integer","format":"int64","required":False},
    {"name":"tags","in":"query","type":"array","style":"form","explode":True,"default":[]},
    {"name":"kind","in":"query","type":"string","enum":["a","b"]},
]}
try:
    params = build_parameters(route)
    by_name = {p["name"]: p for p in params}
    safe_record("all parameters are emitted", lambda: len(params) == 6)
    safe_record("query nullable is preserved", lambda: by_name["q"]["schema"]["nullable"] is True)
    safe_record("description is preserved", lambda: by_name["q"]["description"] == "search")
    safe_record("header alias is used", lambda: "User-Agent" in by_name and by_name["User-Agent"]["in"] == "header")
    safe_record("examples are preserved", lambda: by_name["User-Agent"]["examples"]["chrome"]["value"] == "Chrome")
    safe_record("cookie deprecated flag is preserved", lambda: by_name["session"]["deprecated"] is True)
    safe_record("path params are always required", lambda: by_name["item_id"]["required"] is True)
    safe_record("format is preserved in schema", lambda: by_name["item_id"]["schema"]["format"] == "int64")
    safe_record("default is preserved", lambda: by_name["tags"]["schema"]["default"] == [])
    safe_record("style and explode are preserved", lambda: by_name["tags"]["style"] == "form" and by_name["tags"]["explode"] is True)
    safe_record("enum is preserved as list", lambda: by_name["kind"]["schema"]["enum"] == ["a","b"])
    safe_record("input order is stable", lambda: [p["name"] for p in params] == ["q","User-Agent","session","item_id","tags","kind"])
finally:
    write_reward()
