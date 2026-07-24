
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


from aiohttp_like.pool import Pool
from aiohttp_like.response import ClientResponse
from aiohttp_like.stream import Stream, StreamError

try:
    def read_full_body():
        p=Pool(); r=ClientResponse(p, Stream([b"a", b"b"]))
        return r.read() == b"ab" and p.active == 0 and p.released == 1 and r.closed
    safe_record("read returns full body", read_full_body)
    def read_release_once():
        p=Pool(); r=ClientResponse(p, Stream([b"a", b"b"]))
        r.read()
        return p.active == 0 and p.released == 1 and r.closed and (r.close() or True) and p.released == 1
    safe_record("read releases connection once", read_release_once)
    def read_releases_after_error():
        p=Pool(); r=ClientResponse(p, Stream([b"a"], fail_at=1))
        return raises(StreamError, lambda: r.read()) and p.active == 0 and p.released == 1
    safe_record("read releases after stream error", read_releases_after_error)
    def early_iterator_release():
        p=Pool(); r=ClientResponse(p, Stream([b"a", b"b"])); chunks=[]
        for chunk in r.iter_chunks():
            chunks.append(chunk)
            break
        r.close()
        return chunks == [b"a"] and p.released == 1
    safe_record("early iterator exit releases via generator close", early_iterator_release)
    def double_close_once():
        p=Pool(); r=ClientResponse(p, Stream([b"a"])); r.close(); r.close()
        return p.active == 0 and p.released == 1
    safe_record("double close only releases once", double_close_once)
    def context_exit():
        p=Pool()
        with ClientResponse(p, Stream([b"x"])) as r:
            pass
        return p.active == 0 and p.released == 1
    safe_record("context manager releases on exit", context_exit)
    def context_exception():
        p = Pool()
        try:
            with ClientResponse(p, Stream([b"x"])) as r:
                raise RuntimeError("boom")
        except RuntimeError:
            return p.active == 0 and p.released == 1
        return False
    safe_record("context manager releases on exception", context_exception)
    def empty_body_release():
        p=Pool(); r=ClientResponse(p, Stream([]))
        return r.read() == b"" and p.released == 1
    safe_record("empty body read releases", empty_body_release)
    def full_iterator_release():
        p=Pool(); r=ClientResponse(p, Stream([b"a", b"b"]))
        return list(r.iter_chunks()) == [b"a", b"b"] and p.released == 1
    safe_record("full iterator releases at exhaustion", full_iterator_release)
    def iterator_error_release():
        p=Pool(); r=ClientResponse(p, Stream([b"a"], fail_at=0))
        return raises(StreamError, lambda: list(r.iter_chunks())) and p.released == 1
    safe_record("iterator releases on first chunk error", iterator_error_release)
    def many_responses_release():
        p=Pool(); responses=[ClientResponse(p, Stream([bytes([i])])) for i in range(5)]
        for resp in responses: resp.read()
        return p.active == 0 and p.released == 5
    safe_record("many responses all release", many_responses_release)
    def close_after_read_idempotent():
        p=Pool(); r=ClientResponse(p, Stream([b"x"])); r.read(); r.close()
        return p.released == 1
    safe_record("close after read is idempotent", close_after_read_idempotent)
finally:
    write_reward()
