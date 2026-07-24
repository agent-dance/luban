import json
import os
import subprocess
from pathlib import Path

ROOT = Path(os.environ.get("WORKSPACE", "/workspace"))
LOG_DIR = Path(os.environ.get("LOG_DIR", "/logs/verifier"))
LOG_DIR.mkdir(parents=True, exist_ok=True)
RESULTS = []

def record(name, passed, detail=""):
    RESULTS.append({"name": name, "passed": bool(passed), "detail": str(detail)})

def safe_record(name, check):
    try:
        record(name, check())
    except Exception as exc:
        record(name, False, f"{type(exc).__name__}: {exc}")

def run_cmd(args):
    return subprocess.run(args, cwd=ROOT, text=True, capture_output=True, timeout=20)

class Missing:
    def __getitem__(self, key): return self
    def __getattr__(self, key): return self
    def get(self, key, default=None): return default
    def __iter__(self): return iter(())
    def __len__(self): return 0
    def __bool__(self): return False
    def __contains__(self, item): return False
    def __lt__(self, other): return False
    def __le__(self, other): return False
    def __gt__(self, other): return False
    def __ge__(self, other): return False
    def keys(self): return []
    def items(self): return []
    def values(self): return []
    def __eq__(self, other): return False
    def __repr__(self): return "<missing>"

MISSING = Missing()

class SafeDict(dict):
    def __missing__(self, key):
        return MISSING

def safeify(value):
    if isinstance(value, dict):
        return SafeDict({k: safeify(v) for k, v in value.items()})
    if isinstance(value, list):
        return [safeify(v) for v in value]
    return value

def load_json(rel):
    try:
        return safeify(json.loads((ROOT / rel).read_text(encoding="utf-8")))
    except Exception:
        return SafeDict()

def listify(value, key=None):
    if isinstance(value, list):
        return value
    if isinstance(value, dict):
        out = []
        for item_key, item_value in value.items():
            if isinstance(item_value, dict):
                row = SafeDict(item_value)
                if key and key not in row:
                    row[key] = item_key
                out.append(row)
        return out
    return []

def index_by(value, key, coerce=None):
    out = SafeDict()
    for row in listify(value, key):
        if isinstance(row, dict) and key in row:
            idx = row[key]
            if coerce:
                try:
                    idx = coerce(idx)
                except Exception:
                    pass
            out[idx] = row
    return out

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

proc = run_cmd(["python", "app/schema_audit.py", "--data", "data", "--out", "drift_report.json"])
safe_record("cli exits", lambda: proc.returncode == 0)
report = load_json("drift_report.json")
rows_by = index_by(report["rows"], "row", int)
safe_record("row counts", lambda: report["summary"]["rows"] == 5 and report["summary"]["valid_rows"] == 2 and report["summary"]["invalid_rows"] == 3)
safe_record("normal row has no issues", lambda: rows_by[1]["issues"] == [])
safe_record("string age mismatch", lambda: rows_by[2]["issues"][0]["field"] == "age" and rows_by[2]["issues"][0]["type"] == "type_mismatch")
safe_record("enum violation country", lambda: any(i["type"] == "enum_violation" and i["field"] == "country" for i in rows_by[2]["issues"]))
safe_record("missing required age", lambda: rows_by[3]["issues"] == [{"field": "age", "type": "missing_required"}])
safe_record("score type mismatch", lambda: any(i["field"] == "score" and i["type"] == "type_mismatch" for i in rows_by[4]["issues"]))
safe_record("unexpected debug field", lambda: any(i["field"] == "debug" and i["type"] == "unexpected_field" for i in rows_by[4]["issues"]))
safe_record("optional null country allowed", lambda: rows_by[5]["issues"] == [])
safe_record("field counts", lambda: report["summary"]["field_issue_counts"] == {"age": 2, "country": 1, "debug": 1, "score": 1})
safe_record("user ids preserved", lambda: rows_by[4]["user_id"] == "u4")
safe_record("float accepts int score", lambda: rows_by[5]["issues"] == [])
safe_record("issue rows stable order", lambda: [r["row"] for r in report["rows"]] == [1,2,3,4,5])

write_reward()
