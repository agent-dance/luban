
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
    reward = {"overall": passed / total, "test_pass_rate": passed / total, "tests_passed": passed, "tests_total": total, "test_status": "pass" if passed == total else "no_pass", "tests": RESULTS}
    (LOG_DIR / "reward.json").write_text(json.dumps(reward, indent=2, ensure_ascii=False))
    (LOG_DIR / "reward.txt").write_text(str(reward["overall"]))
    print(json.dumps(reward, indent=2, ensure_ascii=False))
import asyncio, contextvars
from asgiref_like.bridge import async_to_sync, sync_to_async
try:
    safe_record("sync_to_async returns result", lambda: asyncio.run(sync_to_async(lambda x:x+1)(2)) == 3)
    safe_record("sync_to_async passes kwargs", lambda: asyncio.run(sync_to_async(lambda *,name:name)(name="ada")) == "ada")
    marker=contextvars.ContextVar("marker", default="missing")
    async def cc(): marker.set("kept"); return await sync_to_async(lambda: marker.get())()
    safe_record("contextvars propagate", lambda: asyncio.run(cc()) == "kept")
    safe_record("thread_sensitive works", lambda: asyncio.run(sync_to_async(lambda:"same", thread_sensitive=True)()) == "same")
    def se():
        async def run():
            def boom(): raise ValueError("bad")
            await sync_to_async(boom)()
        try: asyncio.run(run())
        except ValueError: return True
        return False
    safe_record("sync exceptions propagate", se)
    async def plus(a,b): await asyncio.sleep(0); return a+b
    safe_record("async_to_sync returns result", lambda: async_to_sync(plus)(2,3) == 5)
    async def kw(name=None): return name
    safe_record("async_to_sync passes kwargs", lambda: async_to_sync(kw)(name="bea") == "bea")
    async def raises(): raise LookupError("x")
    def ae():
        try: async_to_sync(raises)()
        except LookupError: return True
        return False
    safe_record("async exceptions propagate", ae)
    async def nested():
        try: async_to_sync(plus)(1,1)
        except RuntimeError: return True
        return False
    safe_record("running loop is refused", lambda: asyncio.run(nested()))
    async def gather():
        w=sync_to_async(lambda x:x*2); return await asyncio.gather(w(2),w(4))
    safe_record("concurrent calls work", lambda: asyncio.run(gather()) == [4,8])
    safe_record("wrapper reusable", lambda: async_to_sync(plus)(1,2) == 3 and async_to_sync(plus)(3,4) == 7)
finally: write_reward()
