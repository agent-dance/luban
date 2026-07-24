import csv
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

def rows(rel):
    try:
        with (ROOT / rel).open(newline="", encoding="utf-8") as f:
            return list(csv.DictReader(f))
    except Exception:
        return []

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

proc = run_cmd(["python", "app/clean_numeric.py", "--data", "data", "--out", "cleaned.csv", "--summary", "summary.json"])
safe_record("cli exits", lambda: proc.returncode == 0)
out = rows("cleaned.csv")
summary = load_json("summary.json")
by = index_by(out, "id")
safe_record("all rows retained", lambda: set(by) == {"r1","r2","r3","r4","r5"})
safe_record("header stable", lambda: list(out[0]) == ["id","age","income","score"])
safe_record("missing age filled", lambda: by["r2"]["age"] == "35")
safe_record("bad age filled", lambda: by["r4"]["age"] == "35")
safe_record("age low/high clipped", lambda: by["r3"]["age"] == "18" and by["r5"]["age"] == "80")
safe_record("income high low and missing", lambda: by["r2"]["income"] == "200000" and by["r3"]["income"] == "0" and by["r5"]["income"] == "50000")
safe_record("score clipped and filled", lambda: by["r2"]["score"] == "1" and by["r3"]["score"] == "0" and by["r4"]["score"] == "0.5")
safe_record("summary rows", lambda: summary["rows"] == 5)
safe_record("age summary", lambda: summary["age"] == {"filled": 2, "clipped_low": 1, "clipped_high": 1})
safe_record("income summary", lambda: summary["income"] == {"filled": 1, "clipped_low": 1, "clipped_high": 1})
safe_record("score summary", lambda: summary["score"] == {"filled": 1, "clipped_low": 1, "clipped_high": 1})
safe_record("normal row unchanged", lambda: by["r1"] == {"id":"r1","age":"25","income":"60000","score":"0.8"})

write_reward()
