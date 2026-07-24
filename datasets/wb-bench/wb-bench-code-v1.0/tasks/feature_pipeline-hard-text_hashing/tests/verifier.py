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

def run_cmd_env(args, env_updates):
    env = os.environ.copy()
    env.update(env_updates)
    return subprocess.run(args, cwd=ROOT, text=True, capture_output=True, timeout=20, env=env)

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

proc = run_cmd(["python", "app/text_features.py", "--data", "data/texts.csv", "--out", "text_features.jsonl", "--dim", "8"])
proc_seed1 = run_cmd_env(["python", "app/text_features.py", "--data", "data/texts.csv", "--out", "text_features_seed1.jsonl", "--dim", "8"], {"PYTHONHASHSEED": "1"})
proc_seed2 = run_cmd_env(["python", "app/text_features.py", "--data", "data/texts.csv", "--out", "text_features_seed2.jsonl", "--dim", "8"], {"PYTHONHASHSEED": "2"})
safe_record("cli exits", lambda: proc.returncode == 0)
rows_out = jsonl("text_features.jsonl")
seed1 = jsonl("text_features_seed1.jsonl")
seed2 = jsonl("text_features_seed2.jsonl")
by_id = index_by(rows_out, "id")
safe_record("all rows emitted", lambda: set(by_id) == {"a","b","c","d"})
safe_record("stopword removed and lowercase punctuation normalized", lambda: by_id["a"]["token_count"] == 3 and by_id["a"]["unique_tokens"] == 2)
safe_record("repeated quick counted", lambda: sum(by_id["a"]["features"].values()) == 3)
safe_record("hyphen splits serving", lambda: by_id["b"]["token_count"] == 4)
safe_record("model repeated in b", lambda: sum(by_id["b"]["features"].values()) == 4)
safe_record("empty text handled", lambda: by_id["c"]["token_count"] == 0 and by_id["c"]["features"] == {})
safe_record("slash splits a b and removes of", lambda: by_id["d"]["token_count"] == 4)
safe_record("bucket keys are strings in range", lambda: len(rows_out) == 4 and all(k.isdigit() and 0 <= int(k) < 8 for r in rows_out for k in r["features"]))
safe_record("features sorted by numeric bucket", lambda: len(rows_out) == 4 and all(list(r["features"]) == sorted(r["features"], key=int) for r in rows_out))
safe_record("repeated tokens aggregate into buckets", lambda: max(by_id["b"]["features"].values()) >= 2)
safe_record("stable sparse shape without empty buckets", lambda: len(by_id["b"]["features"]) <= by_id["b"]["unique_tokens"])
safe_record("hash stable across python hash seeds", lambda: proc_seed1.returncode == 0 and proc_seed2.returncode == 0 and seed1 == seed2 == rows_out)
safe_record("no stopword token leakage", lambda: len(rows_out) == 4 and all(sum(r["features"].values()) == r["token_count"] for r in rows_out))

write_reward()
