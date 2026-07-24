
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


from redis_like.connection import Connection
from redis_like.errors import WatchError
from redis_like.pipeline import Pipeline

try:
    c=Connection(); p=Pipeline(c).set("a","1").get("a")
    safe_record("basic pipeline executes in order", lambda: p.execute() == [True, "1"] and p.commands == [] and p.watched == set())
    def success_then_reconnect_pipeline_still_clean():
        c=Connection(); p=Pipeline(c).set("a","1"); p.execute()
        c.fail_next=True
        return Pipeline(c).set("b","2").execute() == [True] and c.store == {"a":"1","b":"2"}
    safe_record("pipeline clears commands after success", success_then_reconnect_pipeline_still_clean)
    def safe_reconnect():
        c=Connection(); c.fail_next=True; p=Pipeline(c).set("a","1").get("a")
        return p.execute() == [True, "1"] and c.reconnects == 1 and p.commands == [] and p.watched == set()
    safe_record("safe pipeline reconnects and replays once", safe_reconnect)
    def safe_replay_clears_and_keeps_data():
        c=Connection(); c.fail_next=True; p=Pipeline(c).set("a","1").get("a")
        p.execute()
        return p.commands == [] and p.watched == set() and c.store == {"a":"1"}
    safe_record("safe replay does not leave stale commands", safe_replay_clears_and_keeps_data)
    def watched_not_replayed():
        c=Connection(); c.fail_next=True; p=Pipeline(c).watch("a").set("a","1")
        return raises(WatchError, lambda: p.execute(), "state lost") and p.commands == [] and p.watched == set() and c.store == {}
    safe_record("watched pipeline is not replayed after reconnect", watched_not_replayed)
    def watched_failure_allows_next_pipeline():
        c=Connection(); c.fail_next=True; p=Pipeline(c).watch("a").set("a","1")
        raises(WatchError, lambda: p.execute())
        return Pipeline(c).set("b","2").get("b").execute() == [True, "2"]
    safe_record("watched failure clears state", watched_failure_allows_next_pipeline)
    def incr_not_replayed():
        c=Connection(); c.fail_next=True; p=Pipeline(c).incr("n")
        return raises(WatchError, lambda: p.execute()) and c.store == {} and p.commands == []
    safe_record("non idempotent incr is not replayed", incr_not_replayed)
    def incr_then_failed_incr_is_not_duplicated():
        c=Connection(); Pipeline(c).set("n", "1").execute()
        ok = Pipeline(c).incr("n").get("n").execute() == [2, 2]
        c.fail_next = True
        failed = raises(WatchError, lambda: Pipeline(c).incr("n").execute())
        return ok and failed and c.store["n"] == 2
    safe_record("incr works without reconnect", incr_then_failed_incr_is_not_duplicated)
    def missing_get_replays_after_prior_success():
        c=Connection(); Pipeline(c).set("x","1").execute(); c.fail_next=True; p=Pipeline(c).get("missing")
        return p.execute() == [None] and p.commands == [] and p.watched == set() and c.reconnects == 1
    safe_record("missing get returns None", missing_get_replays_after_prior_success)
    def readonly_replay():
        c=Connection(); c.fail_next=True; p=Pipeline(c).get("missing")
        return p.execute() == [None] and c.reconnects == 1 and p.commands == []
    safe_record("read-only get can replay after reconnect", readonly_replay)
    def fail_flag_cleared():
        c=Connection(); c.fail_next=True; p=Pipeline(c).set("a","1")
        p.execute()
        return c.fail_next is False and Pipeline(c).get("a").execute() == ["1"]
    safe_record("connection fail flag is cleared after reconnect", fail_flag_cleared)
    c=Connection(); p=Pipeline(c); safe_record("empty pipeline returns empty list", lambda: p.execute() == [] and p.commands == [] and p.watched == set() and c.reconnects == 0)
    def watched_get_failure_clears_without_write():
        c=Connection(); c.fail_next=True; p=Pipeline(c).watch("a").get("a")
        return raises(WatchError, lambda: p.execute()) and p.commands == [] and c.store == {}
    safe_record("watched read failure also clears state", watched_get_failure_clears_without_write)
    def repeated_safe_reconnects_do_not_duplicate_set():
        c=Connection()
        for key in ("a", "b"):
            c.fail_next=True
            Pipeline(c).set(key, key).execute()
        return c.store == {"a":"a","b":"b"} and c.reconnects == 2
    safe_record("repeated safe reconnects do not duplicate writes", repeated_safe_reconnects_do_not_duplicate_set)
finally:
    write_reward()
