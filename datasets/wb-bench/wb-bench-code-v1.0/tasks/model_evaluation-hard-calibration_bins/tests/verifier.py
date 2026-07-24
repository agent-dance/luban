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

proc = run_cmd(["python", "app/calibration.py", "--data", "data/predictions.csv", "--out", "calibration_report.json", "--bins", "5"])
safe_record("cli exits", lambda: proc.returncode == 0)
report = load_json("calibration_report.json")
bins = report["bins"]
safe_record("five bins emitted", lambda: len(bins) == 5 and [b["index"] for b in bins] == [0,1,2,3,4])
safe_record("invalid rows skipped", lambda: report["summary"]["rows_skipped"] == 2)
safe_record("used rows and total weight", lambda: report["summary"]["rows_used"] == 8 and is_close(report["summary"]["total_weight"], 12.0))
safe_record("0.20 goes into second bin", lambda: bins[1]["count"] == 2 and is_close(bins[1]["avg_score"], 0.295))
safe_record("0.40 goes into third bin", lambda: bins[2]["count"] == 1 and is_close(bins[2]["observed_rate"], 1.0))
safe_record("0.80 and 1.00 in last bin", lambda: bins[4]["count"] == 2 and is_close(bins[4]["weight"], 4.0))
safe_record("last bin observed weighted", lambda: is_close(bins[4]["observed_rate"], 0.75))
safe_record("first bin avg score weighted", lambda: is_close(bins[0]["avg_score"], 0.126667))
safe_record("empty bin stays zero", lambda: bins[3]["count"] == 1 and is_close(bins[3]["avg_score"], 0.61))
safe_record("ece weighted", lambda: is_close(report["summary"]["ece"], 0.265))
safe_record("bin boundaries stable", lambda: bins[0]["lower"] == 0 and bins[-1]["upper"] == 1.0)
safe_record("gap field present", lambda: is_close(bins[2]["gap"], 0.6))

write_reward()
