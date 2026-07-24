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

proc = run_cmd(["python", "app/evaluate.py", "--data", "data", "--out", "ranking_report.json", "--k", "3"])
safe_record("cli exits", lambda: proc.returncode == 0)
report = load_json("ranking_report.json")
users = index_by(report.get("users", []), "user_id")
safe_record("all label and prediction users included", lambda: set(users) == {"u1", "u2", "u3", "u4", "u5"})
safe_record("u1 tie uses rank_hint and duplicate item dedup", lambda: users["u1"]["ranked_items"] == ["i2", "i1", "i3"])
safe_record("u1 hits and recall", lambda: users["u1"]["hits"] == [1, 0, 1] and is_close(users["u1"]["recall"], 1.0))
safe_record("u2 stable tie and partial recall", lambda: users["u2"]["ranked_items"][:2] == ["i4", "i5"] and is_close(users["u2"]["recall"], 0.5))
safe_record("u3 false label not counted", lambda: users["u3"]["hits"] == [0, 1])
safe_record("u4 no relevant labels has zero metrics", lambda: users["u4"]["relevant"] == 0 and users["u4"]["recall"] == 0 and users["u4"]["map"] == 0)
safe_record("u5 label-only user included", lambda: users["u5"]["ranked_items"] == [] and users["u5"]["relevant"] == 1)
safe_record("summary precision", lambda: is_close(report["summary"]["mean_precision"], 0.266667))
safe_record("summary recall", lambda: is_close(report["summary"]["mean_recall"], 0.5))
safe_record("summary map", lambda: is_close(report["summary"]["mean_map"], 0.316667))
safe_record("summary ndcg", lambda: is_close(report["summary"]["mean_ndcg"], 0.387501))
safe_record("k field stable", lambda: report["k"] == 3)

write_reward()
