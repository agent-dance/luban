import json
import os
import subprocess
import sys
import tempfile
import xml.etree.ElementTree as ET
from pathlib import Path

sys.dont_write_bytecode = True

ROOT = Path(os.environ.get("WORKSPACE", "/workspace"))
TARGET = ROOT / "target_python"
LOG_DIR = Path(os.environ.get("LOG_DIR", "/logs/verifier"))
LOG_DIR.mkdir(parents=True, exist_ok=True)
RESULTS = []


def record(name, passed, detail=""):
    RESULTS.append({"name": name, "passed": bool(passed), "detail": str(detail)})


def safe_record(name, check):
    try:
        record(name, check())
    except Exception as exc:
        record(name, False, f"{type(exc).__name__}: {exc}")


def run_cmd(args, cwd=TARGET, timeout=30):
    env = os.environ.copy()
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    return subprocess.run(args, cwd=str(cwd), text=True, capture_output=True, timeout=timeout, env=env)


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


def local(tag):
    return tag.rsplit("}", 1)[-1] if "}" in tag else tag


def descendants(root, name):
    return [el for el in root.iter() if local(el.tag) == name]


def parse_svg(text):
    return ET.fromstring(text)


SVG = (
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">'
    "<!-- made by app --><metadata>secret</metadata><title>Icon</title>"
    '<g fill="none" stroke="none"></g>'
    '<g fill="none"><path d="M0 0L10 10" stroke="red"/><text>Hello</text></g>'
    "</svg>"
)

SVG_TWO = (
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 5 5">'
    "<!-- remove me --><metadata>x</metadata><title>Two</title>"
    '<g stroke="none"></g><path d="M1 1L4 4" fill="blue"/><text>World</text>'
    "</svg>"
)

SVG_NESTED = (
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 6 6">'
    "<!-- generated\nby design tool -->"
    "<g><metadata>nested secret</metadata><title>Nested</title><g></g>"
    '<path d="M2 2L4 4" stroke="green"/></g>'
    "</svg>"
)


try:
    with tempfile.TemporaryDirectory() as td:
        tmp_dir = Path(td)
        src = tmp_dir / "icon.svg"
        out = tmp_dir / "icon.out.svg"
        src.write_text(SVG)
        cp = run_cmd([sys.executable, "main.py", str(src), "-o", str(out)])
        text = out.read_text() if out.exists() else ""
        record("single svg cleanup exits successfully", cp.returncode == 0, cp.stderr or cp.stdout)
        safe_record("single svg output file is written", lambda: out.exists())
        try:
            root = parse_svg(text)
        except Exception as exc:
            root = None
            record("output remains valid svg xml", False, f"{type(exc).__name__}: {exc}")
        else:
            record("output remains valid svg xml", local(root.tag) == "svg")

        safe_record("root svg viewBox is preserved", lambda: root is not None and root.attrib.get("viewBox") == "0 0 10 10")
        safe_record("xml comments are removed", lambda: "<!--" not in text)
        safe_record("metadata elements are removed", lambda: root is not None and not descendants(root, "metadata"))
        safe_record("title elements are removed", lambda: root is not None and not descendants(root, "title"))
        safe_record("empty group elements are removed", lambda: root is not None and all(list(g) or (g.text or "").strip() for g in descendants(root, "g")))
        safe_record("default fill none on groups is dropped", lambda: root is not None and all(g.attrib.get("fill") != "none" for g in descendants(root, "g")))
        safe_record("default stroke none is dropped", lambda: root is not None and all(el.attrib.get("stroke") != "none" for el in root.iter()))
        safe_record("visible path is preserved", lambda: root is not None and any(el.attrib.get("d") == "M0 0L10 10" for el in descendants(root, "path")))
        safe_record("visible text is preserved", lambda: "Hello" in text)

        nested = tmp_dir / "nested.svg"
        nested_out = tmp_dir / "nested.out.svg"
        nested.write_text(SVG_NESTED)
        cp_nested = run_cmd([sys.executable, "main.py", str(nested), "--output", str(nested_out)])
        nested_text = nested_out.read_text() if nested_out.exists() else ""
        record("nested svg cleanup exits successfully", cp_nested.returncode == 0, cp_nested.stderr or cp_nested.stdout)
        try:
            nested_root = parse_svg(nested_text)
        except Exception as exc:
            nested_root = None
            record("nested output remains valid svg xml", False, f"{type(exc).__name__}: {exc}")
        else:
            record("nested output remains valid svg xml", local(nested_root.tag) == "svg")
        safe_record("multi-line xml comments are removed", lambda: "<!--" not in nested_text and "design tool" not in nested_text)
        safe_record("nested metadata and title elements are removed", lambda: nested_root is not None and not descendants(nested_root, "metadata") and not descendants(nested_root, "title"))
        safe_record("nested visible path is preserved", lambda: nested_root is not None and any(el.attrib.get("d") == "M2 2L4 4" for el in descendants(nested_root, "path")))

        indir = tmp_dir / "icons"
        outdir = tmp_dir / "optimized"
        indir.mkdir()
        (indir / "a.svg").write_text(SVG)
        (indir / "b.svg").write_text(SVG_TWO)
        (indir / "skip.txt").write_text("ignore")
        cp2 = run_cmd([sys.executable, "main.py", str(indir), "--output", str(outdir)])
        record("directory cleanup exits successfully", cp2.returncode == 0, cp2.stderr or cp2.stdout)
        safe_record("directory mode writes optimized svg files", lambda: (outdir / "a.svg").exists() and (outdir / "b.svg").exists())
        safe_record("directory mode ignores non-svg files", lambda: not (outdir / "skip.txt").exists())
        safe_record("directory output removes comments and metadata", lambda: "<!--" not in (outdir / "a.svg").read_text() and "metadata" not in (outdir / "b.svg").read_text())
        safe_record("directory output preserves visible content", lambda: "Hello" in (outdir / "a.svg").read_text() and "World" in (outdir / "b.svg").read_text())
        safe_record("directory outputs remain parseable svg xml", lambda: all(local(parse_svg((outdir / name).read_text()).tag) == "svg" for name in ["a.svg", "b.svg"]))
finally:
    write_reward()
