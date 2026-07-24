
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


from itsdangerous_like.tokens import BadSignature, SignatureExpired, TokenSigner

now = [1000]
signer = TokenSigner("secret", time_fn=lambda: now[0])
try:
    token = signer.dumps("payload")
    safe_record("roundtrip works and keeps max_age boundary", lambda: signer.loads(token, max_age=10) == "payload")
    now[0] = 1011
    def expired():
        try: signer.loads(token, max_age=10)
        except SignatureExpired as exc: return exc.payload == "payload" and exc.reason == "expired" and exc.date_signed == 1000
        return False
    safe_record("expired token has stable exception details", expired)
    now[0] = 1000
    bad = token[:-1] + ("0" if token[-1] != "0" else "1")
    def bad_sig():
        try: signer.loads(bad)
        except BadSignature as exc: return exc.payload == "payload" and exc.reason == "bad_signature" and "Signature" in str(exc)
        return False
    safe_record("bad signature reports payload and reason", bad_sig)
    other = TokenSigner("secret", salt="other", time_fn=lambda: 1000)
    def wrong_salt():
        try: other.loads(token)
        except BadSignature as exc: return getattr(exc, "reason", None) == "bad_signature" and getattr(exc, "payload", None) == "payload"
        return False
    safe_record("wrong salt reports bad signature details", wrong_salt)
    def malformed():
        try: signer.loads("abc")
        except BadSignature as exc: return exc.reason == "malformed"
        return False
    safe_record("malformed token reason is stable", malformed)
    parts = token.split("."); parts[1] = "not-int"; bad_ts = ".".join(parts[:-1]); bad_ts = bad_ts + "." + signer._sign(".".join(parts[:-1]))
    def bad_timestamp():
        try: signer.loads(bad_ts)
        except BadSignature as exc: return exc.reason == "bad_timestamp" and exc.payload == "payload"
        return False
    safe_record("bad timestamp reason is stable", bad_timestamp)
    future = TokenSigner("secret", time_fn=lambda: 2000).dumps("future")
    now_signer = TokenSigner("secret", time_fn=lambda: 1990)
    safe_record("future timestamp is rejected", lambda: raises(BadSignature, lambda: now_signer.loads(future), "future"))
    now[0] = 1000
    def expired_message_is_stable():
        now[0] = 1012
        try: signer.loads(token, max_age=10)
        except SignatureExpired as exc: return "max_age" in str(exc) and exc.payload == "payload"
        return False
    safe_record("expired message is stable", expired_message_is_stable)
    now[0] = 2000
    def max_age_none_keeps_payload():
        return signer.loads(token, max_age=None) == "payload"
    safe_record("max_age none disables age check", max_age_none_keeps_payload)
    bad_payload = "__8.1000." + signer._sign("__8.1000")
    def bad_payload_case():
        try: signer.loads(bad_payload)
        except BadSignature as exc: return getattr(exc, "reason", None) in {"bad_payload", "bad_signature", "malformed"}
        return False
    safe_record("bad payload base64 is BadSignature", bad_payload_case)
    safe_record("exception subclasses are compatible and carry reason", lambda: issubclass(SignatureExpired, BadSignature) and hasattr(BadSignature("x"), "reason"))
    def different_secret():
        try: TokenSigner("other", time_fn=lambda:1000).loads(token)
        except BadSignature as exc: return getattr(exc, "reason", None) == "bad_signature" and getattr(exc, "payload", None) == "payload"
        return False
    safe_record("different secret reports stable details", different_secret)
finally:
    write_reward()
