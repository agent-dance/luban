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

proc = run_cmd(["python", "app/clean_labels.py", "--data", "data", "--out", "cleaned.json", "--rejects", "rejects.json"])
safe_record("cli exits", lambda: proc.returncode == 0)
cleaned = load_json("cleaned.json")
rejects = load_json("rejects.json")
samples = index_by(cleaned["samples"], "entity_id")
safe_record("valid sample ids", lambda: set(samples) == {"e1","e2","e4","e5"})
safe_record("duplicate e1 merged", lambda: samples["e1"]["label"] == 1 and samples["e1"]["reports"] == 1)
safe_record("manual priority beats weak", lambda: samples["e2"]["label"] == 1 and samples["e2"]["source"] == "manual")
safe_record("weak newer does not beat manual", lambda: samples["e4"]["label"] == 1 and samples["e4"]["source"] == "manual")
safe_record("single weak kept", lambda: samples["e5"]["label"] == 1)
safe_record("same priority conflict rejected", lambda: [c["entity_id"] for c in rejects["conflicts"]] == ["e3"])
safe_record("conflict labels stable", lambda: rejects["conflicts"][0]["labels"] == ["0","1"])
safe_record("invalid label and source rejected", lambda: {r["entity_id"] for r in rejects["invalid"]} == {"e6","e7"})
safe_record("summary sample count", lambda: cleaned["summary"]["samples"] == 4)
safe_record("summary conflict invalid counts", lambda: cleaned["summary"]["conflicts"] == 1 and cleaned["summary"]["invalid"] == 2)
safe_record("deduped report count", lambda: cleaned["summary"]["deduped_reports"] == 10)
safe_record("samples sorted", lambda: list(samples) == ["e1","e2","e4","e5"])

write_reward()
