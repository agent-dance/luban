
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


from click_like.formatter import HelpFormatter, get_stats, reset_stats

text = "Run the command with a project path and reuse the same explanation for many subcommands."
try:
    f = HelpFormatter(width=42, indent=4)
    expected = f.format_commands([("build", text), ("deploy", text)])
    reset_stats()
    out = f.format_commands([(f"cmd{i}", text) for i in range(40)])
    safe_record("repeated text only wraps once per formatter", lambda: get_stats()["wrap_calls"] <= 2)
    safe_record("batch output keeps names while reusing wraps", lambda: "cmd0:" in out and "cmd39:" in out and get_stats()["wrap_calls"] <= 2)
    safe_record("indent is preserved with cached text", lambda: all(line.startswith("    ") for line in out.splitlines() if line and not line.endswith(":")) and get_stats()["wrap_calls"] <= 2)
    safe_record("single output stays stable with no extra wraps on second pass", lambda: HelpFormatter(width=42, indent=4).format_commands([("build", text), ("deploy", text)]) == expected)
    reset_stats(); fresh = HelpFormatter(width=42, indent=4); fresh.format_paragraph(text); fresh.format_paragraph(text)
    safe_record("format_paragraph uses cache", lambda: get_stats()["wrap_calls"] == 1)
    reset_stats(); HelpFormatter(width=32, indent=2).format_paragraph(text); HelpFormatter(width=50, indent=2).format_paragraph(text)
    safe_record("different formatter widths do not share incorrectly", lambda: get_stats()["wrap_calls"] == 2)
    narrow = HelpFormatter(width=24, indent=2).format_paragraph("alpha beta gamma delta epsilon")
    wide = HelpFormatter(width=60, indent=2).format_paragraph("alpha beta gamma delta epsilon")
    safe_record("different widths produce different wrapping after cache lookup", lambda: narrow != wide)
    def cache_eviction():
        reset_stats()
        small = HelpFormatter(width=35, indent=2, cache_size=2)
        for item in ["a b c d", "e f g h", "a b c d", "i j k l", "a b c d", "e f g h"]:
            small.format_paragraph(item)
        return get_stats()["wrap_calls"] == 4
    safe_record("cache size evicts old entries", cache_eviction)
    reset_stats(); big = HelpFormatter(width=44, indent=3)
    big.format_commands([(f"same{i}", text if i % 2 else text.upper()) for i in range(80)])
    safe_record("two repeated summaries stay low call count", lambda: get_stats()["wrap_calls"] <= 3)
    reset_stats(); blank = HelpFormatter(width=30, indent=5); blank.format_paragraph(""); blank.format_paragraph("")
    safe_record("blank text is cached too", lambda: get_stats()["wrap_calls"] == 1 and blank.format_paragraph("") == "     ")
    reset_stats(); tiny = HelpFormatter(width=10, indent=20); tiny.format_paragraph("tiny width still works"); tiny.format_paragraph("tiny width still works")
    safe_record("small effective width is cached and does not crash", lambda: get_stats()["wrap_calls"] == 1)
    reset_stats(); start = time.perf_counter(); HelpFormatter(width=50, indent=2).format_commands([(str(i), text) for i in range(1200)]); elapsed = time.perf_counter() - start
    safe_record("large repeated batch is quick and low wrap count", lambda: elapsed < 0.25 and get_stats()["wrap_calls"] <= 2)
finally:
    write_reward()
