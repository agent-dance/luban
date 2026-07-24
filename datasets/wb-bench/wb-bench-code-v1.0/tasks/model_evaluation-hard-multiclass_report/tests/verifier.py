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

proc = run_cmd(["python", "app/report.py", "--data", "data", "--out", "multiclass_report.json"])
safe_record("cli exits", lambda: proc.returncode == 0)
report = load_json("multiclass_report.json")
labels = report["labels"]
preds = index_by(report["predictions"], "id")
safe_record("tie follows label order for row2", lambda: preds["2"]["predicted_label"] == "cat")
safe_record("tie follows label order for row7", lambda: preds["7"]["predicted_label"] == "dog")
safe_record("unknown truth counted not crashed", lambda: report["summary"]["unknown_truth"] == 1 and preds["6"]["known_label"] is False)
safe_record("known row count", lambda: report["summary"]["known_rows"] == 6)
safe_record("accuracy", lambda: is_close(report["summary"]["accuracy"], 0.5))
safe_record("cat metrics include fp from tie and unknown", lambda: labels["cat"]["tp"] == 1 and labels["cat"]["fp"] == 1)
safe_record("dog metrics", lambda: labels["dog"]["tp"] == 1 and labels["dog"]["fp"] == 2 and labels["dog"]["fn"] == 1)
safe_record("bird metrics", lambda: labels["bird"]["tp"] == 1 and labels["bird"]["fn"] == 1)
safe_record("fish zero recall", lambda: labels["fish"]["support"] == 1 and labels["fish"]["recall"] == 0)
safe_record("macro f1 includes fish", lambda: is_close(report["summary"]["macro_f1"], 0.433333))
safe_record("weighted f1", lambda: is_close(report["summary"]["weighted_f1"], 0.466667))
safe_record("label keys stable", lambda: list(labels) == ["cat", "dog", "bird", "fish"])

write_reward()
