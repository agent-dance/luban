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

def read_text(rel):
    try:
        return (ROOT / rel).read_text()
    except Exception:
        return ""

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

proc = run_cmd(["python", "app/cross_features.py", "--data", "data", "--out", "cross_features.jsonl"])
safe_record("cli exits", lambda: proc.returncode == 0)
rows_out = jsonl("cross_features.jsonl")
by = index_by(rows_out, "user_id")
safe_record("all users emitted", lambda: set(by) == {"u1","u2","u3","u4"})
safe_record("trim and lowercase u1", lambda: by["u1"]["country_id"] == 1 and by["u1"]["device_id"] == 1)
safe_record("known cross u1", lambda: by["u1"]["country_device_id"] == 11 and by["u1"]["plan_channel_id"] == 31)
safe_record("known cross u2", lambda: by["u2"]["country_device_id"] == 21 and by["u2"]["plan_channel_id"] == 42)
safe_record("unknown country but known device", lambda: by["u3"]["country_id"] == 0 and by["u3"]["device_id"] == 2)
safe_record("unknown cross defaults even partial values known", lambda: by["u3"]["country_device_id"] == 0 and by["u3"]["plan_channel_id"] == 0)
safe_record("missing values default", lambda: by["u4"]["country_id"] == 0 and by["u4"]["plan_id"] == 0)
safe_record("unknown tablet channel default", lambda: by["u4"]["device_id"] == 0 and by["u4"]["channel_id"] == 0)
safe_record("output fields stable", lambda: set(rows_out[0]) == {"user_id","country_id","device_id","plan_id","channel_id","country_device_id","plan_channel_id"})
safe_record("jsonl row count exactly four", lambda: len(read_text("cross_features.jsonl").splitlines()) == 4)
safe_record("no raw categories leaked", lambda: all("country" not in r and "device" not in r for r in rows_out))
safe_record("encoded feature ids are ints", lambda: len(rows_out) == 4 and all(isinstance(v, int) for r in rows_out for k,v in r.items() if k != "user_id" and k.endswith("_id")))

write_reward()
