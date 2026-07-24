
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


from flask_like.redirects import safe_redirect_target

host = "https://app.example.com/base/"
try:
    safe_record("relative path returns relative target", lambda: safe_redirect_target("/dashboard?tab=1", host) == "/dashboard?tab=1")
    safe_record("relative no slash is resolved under host", lambda: safe_redirect_target("settings", host) == "/base/settings")
    safe_record("same host absolute becomes local path", lambda: safe_redirect_target("https://app.example.com/profile#me", host) == "/profile#me")
    safe_record("external absolute falls back", lambda: safe_redirect_target("https://evil.example/x", host) == "/")
    safe_record("scheme relative external falls back", lambda: safe_redirect_target("//evil.example/x", host) == "/")
    safe_record("triple slash falls back", lambda: safe_redirect_target("///evil.example/x", host) == "/")
    safe_record("backslash target falls back", lambda: safe_redirect_target("\\\\evil.example\\x", host) == "/")
    safe_record("encoded backslash target falls back", lambda: safe_redirect_target("%5c%5cevil.example/x", host) == "/")
    safe_record("crlf target falls back", lambda: safe_redirect_target("/ok%0d%0aLocation:%20//evil", host) == "/")
    safe_record("allowed external host is accepted", lambda: safe_redirect_target("https://docs.example.com/start", host, allowed_hosts={"docs.example.com"}) == "https://docs.example.com/start")
    safe_record("custom fallback is used", lambda: safe_redirect_target("https://evil.example", host, fallback="/login") == "/login")
    safe_record("empty target falls back", lambda: safe_redirect_target("", host, fallback="/home") == "/home")
finally:
    write_reward()
