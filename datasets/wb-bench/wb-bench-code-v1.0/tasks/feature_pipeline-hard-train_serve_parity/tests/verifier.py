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

p1 = run_cmd(["python", "app/build_features.py", "--data", "data", "--mode", "train", "--out", "train_features.jsonl"])
p2 = run_cmd(["python", "app/build_features.py", "--data", "data", "--mode", "serve", "--out", "serve_features.jsonl"])
safe_record("both cli modes exit", lambda: p1.returncode == 0 and p2.returncode == 0)
train = jsonl("train_features.jsonl")
serve = jsonl("serve_features.jsonl")
safe_record("row counts", lambda: len(train) == 4 and len(serve) == 3)
safe_record("output order stable", lambda: list(train[0].keys()) == ["age_z", "country_id", "device_id", "is_new_user", "spend_30d_z", "user_id"] or set(train[0]) == {"user_id","country_id","device_id","age_z","spend_30d_z","is_new_user"})
safe_record("known categories encoded", lambda: train[0]["country_id"] == 1 and train[0]["device_id"] == 1)
safe_record("oov category encoded zero", lambda: train[1]["country_id"] == 0 and serve[1]["country_id"] == 0 and serve[1]["device_id"] == 0)
safe_record("missing device encoded zero", lambda: train[2]["device_id"] == 0)
safe_record("age z score", lambda: is_close(train[0]["age_z"], 1.0) and is_close(train[3]["age_z"], -1.0))
safe_record("missing age uses mean", lambda: is_close(train[1]["age_z"], 0.0) and is_close(serve[2]["age_z"], 0.0))
safe_record("missing spend uses configured zero", lambda: is_close(train[2]["spend_30d_z"], -2.0) and is_close(serve[1]["spend_30d_z"], -2.0))
safe_record("spend normalization", lambda: is_close(serve[2]["spend_30d_z"], 2.0))
safe_record("new user threshold inclusive", lambda: train[2]["is_new_user"] == 1 and train[3]["is_new_user"] == 0 and serve[0]["is_new_user"] == 1)
safe_record("serve keeps same feature names as train", lambda: set(train[0]) == set(serve[0]))

write_reward()
