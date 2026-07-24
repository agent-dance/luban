
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


from httpx_like.pool import ConnectionPool, PoolTimeout

class CancelledError(Exception): pass
try:
    pool = ConnectionPool(max_connections=1)
    with pool.stream() as conn:
        ident = conn.ident
    safe_record("success releases connection exactly once", lambda: pool.active_count == 0 and pool.available_count == 1)
    reused_conn = pool.acquire()
    safe_record("released connection can be reused", lambda: reused_conn.ident == ident and pool.active_count == 1 and pool.available_count == 0)
    pool.release(reused_conn)

    def make_exception_pool():
        p = ConnectionPool(max_connections=1)
        try:
            with p.stream():
                raise RuntimeError("boom")
        except RuntimeError:
            pass
        return p
    pool = make_exception_pool()
    safe_record("exception releases connection", lambda: pool.active_count == 0 and pool.available_count == 1)
    def acquire_after_error():
        p = make_exception_pool()
        conn = p.acquire()
        ok = conn.ident == 0 and p.active_count == 1 and p.available_count == 0
        p.release(conn)
        return ok and p.available_count == 1 and p.active_count == 0
    safe_record("pool can acquire after exception release", acquire_after_error)
    safe_record("second acquire after exception is not duplicated", lambda: make_exception_pool().available_count == 1)

    def make_cancelled_pool():
        p = ConnectionPool(max_connections=1)
        try:
            with p.stream():
                raise CancelledError()
        except CancelledError:
            pass
        return p
    pool = make_cancelled_pool()
    safe_record("cancellation-like exception releases connection", lambda: pool.active_count == 0 and pool.available_count == 1)
    def stream_after_cancel():
        p = make_cancelled_pool()
        with p.stream() as conn:
            return conn.ident == 0
    safe_record("pool can stream after cancellation", stream_after_cancel)
    safe_record("cancellation recovery leaves one available connection", lambda: make_cancelled_pool().available_count == 1 and make_cancelled_pool().active_count == 0)

    pool = ConnectionPool(max_connections=1); conn = pool.acquire()
    pool.release(conn)
    safe_record("double release does not duplicate connection", lambda: pool.available_count == 1)

    pool = ConnectionPool(max_connections=1); c = pool.acquire(); c.close(); pool.release(c)
    safe_record("closed connection is not returned", lambda: pool.available_count == 0 and pool.active_count == 0)

    pool = ConnectionPool(max_connections=2); held = pool.acquire(); pool.close()
    safe_record("close clears active and available pool", lambda: pool.available_count == 0 and pool.active_count == 0)
    safe_record("close marks active connection closed", lambda: held.closed is True)

    pool = ConnectionPool(max_connections=2)
    try:
        with pool.stream():
            raise RuntimeError("first")
    except RuntimeError:
        pass
    try:
        with pool.stream():
            raise RuntimeError("second")
    except RuntimeError:
        pass
    safe_record("repeated exceptions do not exhaust pool", lambda: pool.active_count == 0 and pool.available_count == 2)
finally:
    write_reward()
