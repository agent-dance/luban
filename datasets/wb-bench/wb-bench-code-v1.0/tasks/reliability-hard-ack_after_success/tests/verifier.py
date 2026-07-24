
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


from celery_like.broker import Broker
from celery_like.errors import FatalTaskError, RetryableError
from celery_like.worker import Worker

try:
    broker=Broker([{"id":"ok","type":"ok"}]); worker=Worker(broker)
    safe_record("successful task returns handler result", lambda: worker.run_once({"ok": lambda task: "done"}) == "done" and broker.dead == [] and broker.requeued == [])
    safe_record("successful task is acked once", lambda: broker.acked == ["ok"] and broker.dead == [] and broker.requeued == [])
    broker=Broker([{"id":"r","type":"r"}]); worker=Worker(broker, max_retries=2)
    safe_record("retryable task is requeued without ack", lambda: worker.run_once({"r": lambda task: (_ for _ in ()).throw(RetryableError("try again"))}) is None and broker.acked == [] and broker.requeued == ["r"])
    safe_record("retry preserves task attempts", lambda: broker.acked == [] and len(broker.ready) == 1 and broker.ready[0]["attempts"] == 1)
    calls=[]
    def flaky(task):
        calls.append(task.get("attempts",0))
        if len(calls) == 1: raise RetryableError("again")
        return "ok"
    broker=Broker([{"id":"flaky","type":"f"}]); worker=Worker(broker, max_retries=2); worker.run_once({"f": flaky}); result=worker.run_once({"f": flaky})
    safe_record("requeued task can later ack after success", lambda: result == "ok" and broker.acked == ["flaky"] and broker.requeued == ["flaky"])
    broker=Broker([{"id":"fatal","type":"f"}]); worker=Worker(broker)
    safe_record("fatal task goes to dead letter without ack", lambda: worker.run_once({"f": lambda task: (_ for _ in ()).throw(FatalTaskError("bad"))}) is None and broker.acked == [] and broker.dead == [("fatal","FatalTaskError")])
    broker=Broker([{"id":"max","type":"r"}]); worker=Worker(broker, max_retries=0)
    safe_record("retry exhaustion dead letters task", lambda: worker.run_once({"r": lambda task: (_ for _ in ()).throw(RetryableError("x"))}) is None and broker.dead == [("max","max_retries")] and broker.requeued == [])
    broker=Broker([]); safe_record("empty broker returns None", lambda: Worker(broker).run_once({}) is None)
    broker=Broker([{"id":"boom","type":"r","attempts":1}]); worker=Worker(broker, max_retries=2)
    safe_record("retry keeps existing attempts and avoids ack", lambda: worker.run_once({"r": lambda task: (_ for _ in ()).throw(RetryableError("x"))}) is None and broker.acked == [] and broker.ready[0]["attempts"] == 2)
    broker=Broker([{"id":"x","type":"missing"}]); safe_record("missing handler is dead-lettered", lambda: Worker(broker).run_once({}) is None and broker.dead == [("x","KeyError")])
    broker=Broker([{"id":"f1","type":"f"},{"id":"f2","type":"f"}]); worker=Worker(broker)
    worker.run_once({"f": lambda task: (_ for _ in ()).throw(FatalTaskError("bad"))})
    worker.run_once({"f": lambda task: (_ for _ in ()).throw(FatalTaskError("bad"))})
    safe_record("multiple fatal failures never ack", lambda: broker.acked == [] and broker.dead == [("f1","FatalTaskError"),("f2","FatalTaskError")])
    broker=Broker([{"id":"n","type":"ok"}]); worker=Worker(broker); order=[]; worker.run_once({"ok": lambda task: order.append(list(broker.acked)) or None})
    safe_record("None success still acks after handler returns", lambda: order == [[]] and broker.acked == ["n"] and broker.dead == [] and broker.requeued == [])
    broker=Broker([{"id":"mix1","type":"r"},{"id":"mix2","type":"ok"}]); worker=Worker(broker, max_retries=0)
    worker.run_once({"r": lambda task: (_ for _ in ()).throw(RetryableError("x")), "ok": lambda task: "ok"})
    result = worker.run_once({"r": lambda task: (_ for _ in ()).throw(RetryableError("x")), "ok": lambda task: "ok"})
    safe_record("dead-lettered retry does not block next success", lambda: result == "ok" and broker.dead == [("mix1","max_retries")] and broker.acked == ["mix2"])
finally:
    write_reward()
