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

def jsonl(rel):
    try:
        return [safeify(json.loads(line)) for line in (ROOT / rel).read_text().splitlines() if line.strip()]
    except Exception:
        return []

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

proc = run_cmd(["python", "app/audit_leakage.py", "--data", "data", "--clean", "clean.jsonl", "--rejects", "leakage.json"])
safe_record("cli exits", lambda: proc.returncode == 0)
clean = jsonl("clean.jsonl")
rejects = load_json("leakage.json")
clean_ids = [r["sample_id"] for r in clean]
reject_ids = [r["sample_id"] for r in rejects["rejected"]]
safe_record("s1 within tolerance is clean", lambda: "s1" in clean_ids)
safe_record("empty features clean", lambda: any(r["sample_id"] == "s3" and r["feature_count"] == 0 for r in clean))
safe_record("s2 rejected just past tolerance", lambda: "s2" in reject_ids)
safe_record("s4 rejected next day", lambda: "s4" in reject_ids)
safe_record("only leaked feature listed for s2", lambda: [f["name"] for f in rejects["rejected"][0]["leaked_features"]] == ["f_ticket"])
safe_record("non leaked feature omitted from reject", lambda: all(f["name"] != "f_login" for r in rejects["rejected"] for f in r["leaked_features"]))
safe_record("summary counts", lambda: rejects["summary"]["clean"] == 2 and rejects["summary"]["rejected"] == 2)
safe_record("tolerance exposed", lambda: rejects["summary"]["tolerance_minutes"] == 5)
safe_record("clean output sorted input order", lambda: clean_ids == ["s1","s3"])
safe_record("cutoff preserved in clean", lambda: clean[0]["cutoff_time"] == "2026-06-10T10:00:00")
safe_record("reject has cutoff time", lambda: rejects["rejected"][1]["cutoff_time"] == "2026-06-10T00:00:00")
safe_record("clean jsonl has exactly two lines", lambda: len(read_text("clean.jsonl").splitlines()) == 2)

write_reward()
