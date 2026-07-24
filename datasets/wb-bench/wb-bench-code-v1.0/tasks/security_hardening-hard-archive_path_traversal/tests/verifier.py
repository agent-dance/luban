
import json
import os
import sys
import tempfile
from pathlib import Path

sys.dont_write_bytecode = True
ROOT = Path(os.environ.get("WORKSPACE", "/workspace"))
LOG_DIR = Path(os.environ.get("LOG_DIR", "/logs/verifier"))
LOG_DIR.mkdir(parents=True, exist_ok=True)
sys.path.insert(0, str(ROOT))
RESULTS = []


def record(name, passed, detail=""):
    RESULTS.append({"name": name, "passed": bool(passed), "detail": str(detail)})


def safe_record(name, check):
    try:
        record(name, check())
    except Exception as exc:
        record(name, False, f"{type(exc).__name__}: {exc}")


def raises(exc_type, fn, contains=None):
    try:
        fn()
    except exc_type as exc:
        return contains is None or contains in str(exc)
    except Exception:
        return False
    return False


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


import os
from pathlib import Path
from archive_like.errors import UnsafeArchiveError
from archive_like.extract import extract_members
from archive_like.path import target_path

try:
    with tempfile.TemporaryDirectory() as td:
        base=Path(td)/"out"
        written=extract_members(base, [{"name":"docs/a.txt","data":b"a"},{"name":"docs","type":"dir"}])
        safe_record("normal file is extracted", lambda: (base/"docs"/"a.txt").read_bytes() == b"a")
        safe_record("returned paths stay under base", lambda: all(os.path.commonpath([base, p]) == str(base) for p in written))
        safe_record("parent traversal is blocked", lambda: raises(UnsafeArchiveError, lambda: extract_members(base, [{"name":"../x","data":b"x"}])))
        safe_record("absolute path is blocked", lambda: raises(UnsafeArchiveError, lambda: extract_members(base, [{"name":"/tmp/x","data":b"x"}])))
        safe_record("encoded traversal is blocked", lambda: raises(UnsafeArchiveError, lambda: extract_members(base, [{"name":"%2e%2e/x","data":b"x"}])))
        safe_record("double encoded traversal is blocked", lambda: raises(UnsafeArchiveError, lambda: extract_members(base, [{"name":"%252e%252e/x","data":b"x"}])))
        safe_record("windows separator traversal is blocked", lambda: raises(UnsafeArchiveError, lambda: extract_members(base, [{"name":"..\\\\x","data":b"x"}])))
        safe_record("safe encoded slash is allowed", lambda: target_path(base, "docs%2fa.txt").endswith("docs/a.txt"))
        safe_record("symlink member is rejected", lambda: raises(UnsafeArchiveError, lambda: extract_members(base, [{"name":"link","type":"symlink","target":"docs/a.txt"}])))
        safe_record("unknown member type is rejected", lambda: raises(UnsafeArchiveError, lambda: extract_members(base, [{"name":"dev","type":"device"}])))
        extract_members(base, [{"name":"nested/dir/file.bin","data":b"b"}])
        safe_record("nested directories are created", lambda: (base/"nested"/"dir"/"file.bin").read_bytes() == b"b")
        safe_record("blocked write does not create outside file", lambda: not (Path(td)/"x").exists())
finally:
    write_reward()
