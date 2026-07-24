
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


from urllib3_like.proxy import prepare_proxy_request, redirect_request
from urllib3_like.request import Request

try:
    req=prepare_proxy_request("https://api.example/a", {"Authorization":"Bearer app"}, {"Authorization":"Basic proxy"})
    safe_record("proxy authorization is renamed", lambda: req.headers["Proxy-Authorization"] == "Basic proxy")
    safe_record("origin authorization is preserved for origin request", lambda: req.headers["Authorization"] == "Bearer app" and req.headers["Proxy-Authorization"] != req.headers["Authorization"])
    safe_record("raw proxy Authorization is not left behind", lambda: list(k for k in req.headers if k.lower()=="authorization") == ["Authorization"] and "Proxy-Authorization" in req.headers)
    req=Request("https://api.example/a", {"Authorization":"Bearer app","Cookie":"sid=1","X":"1"})
    redir=redirect_request(req, "https://other.example/b")
    safe_record("cross origin redirect strips authorization", lambda: "Authorization" not in redir.headers)
    safe_record("cross origin redirect strips cookie", lambda: "Cookie" not in redir.headers and redir.headers["X"] == "1")
    same=redirect_request(req, "https://api.example/b")
    safe_record("same origin redirect preserves auth", lambda: same.headers["Authorization"] == "Bearer app" and same.headers["Cookie"] == "sid=1" and same.headers["X"] == "1" and "Proxy-Authorization" not in same.headers)
    safe_record("scheme change strips sensitive headers", lambda: "Authorization" not in redirect_request(req, "http://api.example/b").headers)
    safe_record("header value newline is rejected", lambda: raises(ValueError, lambda: prepare_proxy_request("https://x", {"X":"ok" + chr(13) + chr(10) + "Bad: 1"})))
    safe_record("proxy auth header survives with existing name", lambda: prepare_proxy_request("https://x", {}, {"Proxy-Authorization":"Basic p"}).headers == {"Proxy-Authorization":"Basic p"})
    safe_record("non sensitive custom header preserved across cross redirect", lambda: redirect_request(req, "https://other.example/b").headers == {"X":"1"})
    safe_record("origin tuple includes scheme and host", lambda: Request("https://api.example/a").origin == ("https", "api.example") and Request("http://api.example/a").origin != Request("https://api.example/a").origin)
    safe_record("empty headers work", lambda: prepare_proxy_request("https://x").headers == {} and redirect_request(Request("https://x", {"Cookie":"c=1"}), "https://y").headers == {})
finally:
    write_reward()
