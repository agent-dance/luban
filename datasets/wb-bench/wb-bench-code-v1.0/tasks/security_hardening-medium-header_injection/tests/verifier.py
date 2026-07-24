
import json
import os
import sys
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


from tornado_like.headers import HTTPHeaders

try:
    def cookies_case():
        h = HTTPHeaders(); h.add("Set-Cookie", "a=1"); h.add("Set-Cookie", "b=2")
        return h.get_list("set-cookie") == ["a=1", "b=2"]
    safe_record("multi value headers are preserved", cookies_case)
    def replace_case():
        h = HTTPHeaders(); h.set("X-Test", "ok"); h.set("X-Test", "new")
        return h.get_list("x-test") == ["new"]
    safe_record("set replaces values after validation", replace_case)
    def wsgi_case():
        h = HTTPHeaders(); h.set("X-Test", "new")
        return ("X-Test", "new") in h.to_wsgi_list()
    safe_record("wsgi list preserves original case after validation", wsgi_case)
    safe_record("crlf in value is rejected", lambda: raises(ValueError, lambda: HTTPHeaders().set("X-Bad", "ok\r\nInjected: yes"), "X-Bad"))
    safe_record("newline in add value is rejected", lambda: raises(ValueError, lambda: HTTPHeaders().add("X-Bad", "ok\nno")))
    safe_record("nul in value is rejected", lambda: raises(ValueError, lambda: HTTPHeaders().set("X-Bad", "ok\x00no")))
    safe_record("colon in header name is rejected", lambda: raises(ValueError, lambda: HTTPHeaders().set("Bad:Name", "x"), "header name"))
    safe_record("space in header name is rejected", lambda: raises(ValueError, lambda: HTTPHeaders().set("Bad Name", "x")))
    safe_record("empty header name is rejected", lambda: raises(ValueError, lambda: HTTPHeaders().set("", "x")))
    safe_record("horizontal tab in value is allowed after validation", lambda: (lambda h: (h.set("X-Tab", "a\\tb") or h.get_list("x-tab") == ["a\\tb"]))(HTTPHeaders()))
    safe_record("numeric values are stringified after validation", lambda: (lambda h: (h.add("X-Num", 3) or h.get_list("x-num") == ["3"]))(HTTPHeaders()))
    safe_record("case insensitive lookup works after validation", lambda: (lambda h: (h.add("X-Num", 3) or h.get_list("X-NUM") == ["3"]))(HTTPHeaders()))
finally:
    write_reward()
