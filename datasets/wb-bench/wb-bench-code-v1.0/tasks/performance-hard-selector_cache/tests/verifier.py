
import json
import os
import sys
import time
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


from parsel_like.selectors import Selector, compile_css, get_stats, reset_stats

html = "<a class='nav' href='/a'>A</a><a class='nav' href='/b'>B</a><span class='item'>One</span><span class='item'>Two</span>"
try:
    reset_stats()
    for _ in range(30):
        Selector(html).css("a.nav::attr(href)").getall()
    safe_record("same css query compiles once", lambda: get_stats()["compile_calls"] == 1)
    safe_record("href extraction stays correct after cached compile", lambda: Selector(html).css("a.nav::attr(href)").getall() == ["/a", "/b"] and get_stats()["compile_calls"] == 1)
    reset_stats(); Selector(html).css("span.item::text").getall(); Selector(html).css("span.item::text").getall()
    safe_record("text extraction works with cached selector", lambda: Selector(html).css("span.item::text").getall() == ["One", "Two"] and get_stats()["compile_calls"] == 1)
    reset_stats(); Selector(html).css("a.nav::attr(href)"); Selector(html).css("span.item::text")
    safe_record("different queries compile separately", lambda: get_stats()["compile_calls"] == 2)
    reset_stats(); Selector("<a class='nav' href='/x'>X</a>").css("a.nav::attr(href)"); Selector("<a class='nav' href='/y'>Y</a>").css("a.nav::attr(href)")
    safe_record("compiled selector is reused across roots", lambda: get_stats()["compile_calls"] == 1)
    safe_record("cache does not cache extraction results", lambda: Selector("<a class='nav' href='/z'>Z</a>").css("a.nav::attr(href)").get() == "/z" and get_stats()["compile_calls"] == 1)
    reset_stats()
    for i in range(70):
        Selector(html).css(f"span.c{i}::text")
    Selector(html).css("span.c0::text")
    safe_record("cache has bounded eviction", lambda: get_stats()["compile_calls"] == 71)
    reset_stats(); Selector("<a class='nav'>A</a>").css("a.nav::attr(href)").getall(); Selector("<a class='nav'>B</a>").css("a.nav::attr(href)").getall()
    safe_record("missing attr is skipped without recompiling", lambda: get_stats()["compile_calls"] == 1)
    reset_stats(); Selector("<b>Bold</b>").css("*::text").get(); Selector("<i>Italic</i>").css("*::text").get()
    safe_record("wildcard text selector reuses compiled form", lambda: get_stats()["compile_calls"] == 1)
    reset_stats(); start=time.perf_counter()
    for _ in range(800):
        Selector(html).css("a.nav::attr(href)").getall()
    elapsed=time.perf_counter()-start
    safe_record("repeated selector batch is quick", lambda: elapsed < 0.35 and get_stats()["compile_calls"] == 1)
    reset_stats(); a = Selector("<p>none</p>").css("a.nav::attr(href)").get("fallback"); b = Selector("<p>none</p>").css("a.nav::attr(href)").get("fallback")
    safe_record("get default works with cached selector", lambda: a == b == "fallback" and get_stats()["compile_calls"] == 1)
    reset_stats(); Selector("<a href='/a'>A</a>").css("a").get(); Selector("<a href='/b'>B</a>").css("a").get()
    safe_record("raw element selector returns markup without recompiling", lambda: get_stats()["compile_calls"] == 1)
finally:
    write_reward()
