
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


from jinja_like.compiler import get_stats as compile_stats, reset_stats as reset_compile
from jinja_like.environment import Environment
from jinja_like.lexer import get_stats as lex_stats, reset_stats as reset_lex

source = "Hello {{ name }} {% upper mood %}"
try:
    reset_lex(); reset_compile()
    env = Environment(cache_size=8)
    for i in range(30):
        assert env.render(source, {"name": "Ada", "mood": "ok"}) == "Hello Ada OK"
    safe_record("same template tokenizes once", lambda: lex_stats()["tokenize_calls"] == 1)
    safe_record("same template compiles once", lambda: compile_stats()["compile_calls"] == 1)
    def filters_dynamic_cached():
        reset_lex(); reset_compile()
        env = Environment()
        first = env.render(source, {"name":"Ada","mood":"ok"})
        second = env.render(source, {"name":"Ada","mood":"ok"}, filters={"upper": lambda v: f'<{v}>'})
        return first == "Hello Ada OK" and second == "Hello Ada <ok>" and compile_stats()["compile_calls"] == 1
    safe_record("filters stay dynamic with cached template", filters_dynamic_cached)
    def autoescape_cached():
        reset_lex(); reset_compile()
        env = Environment(autoescape=True)
        first = env.render("{{ x }}", {"x":"<b>&"})
        second = env.render("{{ x }}", {"x":"<i>"})
        return first == "&lt;b&gt;&amp;" and second == "&lt;i&gt;" and compile_stats()["compile_calls"] == 1
    safe_record("autoescape true escapes cached variable", autoescape_cached)
    reset_lex(); reset_compile(); a=Environment(autoescape=False); b=Environment(autoescape=True); a.render("{{ x }}", {"x":"<"}); b.render("{{ x }}", {"x":"<"})
    safe_record("autoescape is part of cache identity", lambda: lex_stats()["tokenize_calls"] == 2 and compile_stats()["compile_calls"] == 2)
    reset_lex(); reset_compile(); env=Environment(cache_size=2)
    for text in ["A {{ x }}", "B {{ x }}", "A {{ x }}", "C {{ x }}", "A {{ x }}", "B {{ x }}"]:
        env.render(text, {"x": "1"})
    safe_record("cache_size evicts least recently used template", lambda: compile_stats()["compile_calls"] == 4)
    reset_lex(); reset_compile(); env=Environment(cache_size=0); env.render(source, {"name":"a"}); env.render(source, {"name":"a"})
    safe_record("cache_size zero disables cache", lambda: lex_stats()["tokenize_calls"] == 2 and compile_stats()["compile_calls"] == 2)
    reset_lex(); reset_compile(); env=Environment()
    for i in range(200):
        env.render("{{ item }}", {"item": i})
    safe_record("cached template still renders changing context", lambda: env.render("{{ item }}", {"item": "last"}) == "last" and compile_stats()["compile_calls"] == 1)
    reset_lex(); reset_compile(); env=Environment()
    env.render("plain text", {}); env.render("plain text", {})
    safe_record("plain text template is cached", lambda: lex_stats()["tokenize_calls"] == 1 and compile_stats()["compile_calls"] == 1)
    reset_lex(); reset_compile(); env=Environment(); start=time.perf_counter()
    for _ in range(1000): env.render(source, {"name":"Ada","mood":"ok"})
    safe_record("large repeated render is quick", lambda: time.perf_counter()-start < 0.35 and compile_stats()["compile_calls"] == 1)
    safe_record("different template source compiles separately", lambda: (reset_lex() or reset_compile() or True) and (lambda e: (e.render("A {{ x }}", {"x":1}), e.render("B {{ x }}", {"x":1}), compile_stats()["compile_calls"] == 2))(Environment()))
    safe_record("missing variables stay empty with cached template", lambda: (reset_lex() or reset_compile() or True) and (lambda e: e.render("{{ missing }}!", {}) == "!" and e.render("{{ missing }}!", {}) == "!" and compile_stats()["compile_calls"] == 1)(Environment()))
finally:
    write_reward()
