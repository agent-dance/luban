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

def jsonl(rel):
    try:
        return [safeify(json.loads(line)) for line in (ROOT / rel).read_text().splitlines() if line.strip()]
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

proc = run_cmd(["python", "app/window_features.py", "--data", "data", "--out", "window_features.jsonl"])
safe_record("cli exits", lambda: proc.returncode == 0)
rows_out = jsonl("window_features.jsonl")
by_user = index_by(rows_out, "user_id")
safe_record("three anchors emitted", lambda: set(by_user) == {"u1","u2","u3"})
safe_record("event exactly at anchor excluded", lambda: by_user["u1"]["amount_1d"] == 30)
safe_record("1d lower boundary included", lambda: by_user["u1"]["events_1d"] == 2 and by_user["u1"]["amount_1d"] == 30)
safe_record("7d lower boundary included", lambda: by_user["u1"]["events_7d"] == 3 and by_user["u1"]["amount_7d"] == 60)
safe_record("outside 7d excluded", lambda: by_user["u1"]["amount_7d"] != 100)
safe_record("u2 1d count", lambda: by_user["u2"]["events_1d"] == 1 and by_user["u2"]["amount_1d"] == 5)
safe_record("u2 7d includes exact lower boundary", lambda: by_user["u2"]["events_7d"] == 2 and by_user["u2"]["amount_7d"] == 12)
safe_record("future other user ignored", lambda: by_user["u2"]["amount_7d"] != 112)
safe_record("last event hours u1", lambda: is_close(by_user["u1"]["last_event_hours"], 0.000278, 1e-6))
safe_record("last event hours u2", lambda: is_close(by_user["u2"]["last_event_hours"], 0.000278, 1e-6))
safe_record("no event user has zeros and null recency", lambda: by_user["u3"]["events_7d"] == 0 and by_user["u3"]["amount_7d"] == 0 and by_user["u3"]["last_event_hours"] is None)
safe_record("anchor time preserved", lambda: by_user["u1"]["anchor_time"] == "2026-06-10T12:00:00")

write_reward()
