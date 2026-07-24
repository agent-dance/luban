
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


import rich_like.markup as markup
MarkupError = markup.MarkupError
render_markup = markup.render_markup
parse_markup = getattr(markup, "parse_markup", None)
escape = getattr(markup, "escape", None)
Segment = getattr(markup, "Segment", None)

try:
    safe_record("plain render is unchanged", lambda: render_markup("hello") == "hello" and callable(parse_markup) and callable(escape))
    safe_record("simple tag render strips markup", lambda: render_markup("[bold]Hi[/bold]") == "Hi" and callable(parse_markup) and Segment is not None)
    segs=parse_markup("[bold]Hi[/bold] there") if parse_markup else []
    safe_record("parse returns styled segments", lambda: segs[0].text == "Hi" and segs[0].style == "bold")
    safe_record("plain trailing segment has no style", lambda: segs[1].text == " there" and segs[1].style is None)
    def unclosed():
        try: parse_markup("[bold]Hi")
        except MarkupError as exc: return exc.code == "unclosed_tag" and exc.position == len("[bold]Hi")
        return False
    safe_record("unclosed tag has stable code and position", unclosed)
    def mismatch():
        try: parse_markup("[bold]Hi[/red]")
        except MarkupError as exc: return exc.code == "unmatched_close" and exc.position == 8
        return False
    safe_record("mismatched close has stable code and position", mismatch)
    safe_record("escape escapes brackets", lambda: escape is not None and escape("[bold]") == r"\[bold\]")
    safe_record("nested style applies innermost segment", lambda: [s.style for s in parse_markup("[red]a[bold]b[/bold]c[/red]")] == ["red","bold","red"])
    safe_record("render nested markup keeps text order", lambda: render_markup("[red]a[bold]b[/bold]c[/red]") == "abc" and [s.style for s in parse_markup("[red]a[bold]b[/bold]c[/red]")] == ["red","bold","red"])
    safe_record("Segment dataclass is public", lambda: Segment is not None and Segment("x", "bold").text == "x")
    safe_record("closing without open raises", lambda: raises(MarkupError, lambda: parse_markup("[/bold]")))
    safe_record("multiple tags render all text", lambda: render_markup("[red]R[/red][blue]B[/blue]") == "RB" and len(parse_markup("[red]R[/red][blue]B[/blue]")) == 2)
finally:
    write_reward()
