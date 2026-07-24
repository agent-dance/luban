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

def is_close(a, b, tol=1e-6):
    return abs(float(a) - float(b)) <= tol

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

proc = run_cmd(["python", "app/evaluate.py", "--data", "data/predictions.csv", "--out", "threshold_report.json"])
safe_record("cli exits", lambda: proc.returncode == 0)
report = load_json("threshold_report.json")
by_t = index_by(report["thresholds"], "threshold", float)
safe_record("three thresholds sorted", lambda: list(by_t) == [0.3, 0.5, 0.7])
safe_record("threshold uses >= boundary at 0.3", lambda: is_close(by_t[0.3]["overall"]["tp"], 6.0) and is_close(by_t[0.3]["overall"]["fp"], 3.5))
safe_record("overall weighted fn at 0.7", lambda: is_close(by_t[0.7]["overall"]["fn"], 3.5))
safe_record("overall weighted precision at 0.5", lambda: is_close(by_t[0.5]["overall"]["precision"], 0.6))
safe_record("overall weighted recall at 0.5", lambda: is_close(by_t[0.5]["overall"]["recall"], 0.5))
safe_record("overall f1 at 0.5", lambda: is_close(by_t[0.5]["overall"]["f1"], 0.545455))
safe_record("empty segment becomes unknown", lambda: "unknown" in by_t[0.5]["segments"])
safe_record("free segment fp keeps weight 2", lambda: is_close(by_t[0.7]["segments"]["free"]["fp"], 2.0))
safe_record("pro segment threshold 0.3 catches positive boundary", lambda: is_close(by_t[0.3]["segments"]["pro"]["tp"], 3.0))
safe_record("unknown segment has tp and fp at 0.3", lambda: is_close(by_t[0.3]["segments"]["unknown"]["tp"], 1.5) and is_close(by_t[0.3]["segments"]["unknown"]["fp"], 0.5))
safe_record("segment keys stable", lambda: sorted(by_t[0.5]["segments"]) == ["free", "pro", "unknown"])
safe_record("zero denominator metrics do not crash", lambda: by_t[0.7]["segments"]["pro"]["precision"] == 0.0)
safe_record("report only thresholds top level", lambda: sorted(report.keys()) == ["thresholds"])

write_reward()
