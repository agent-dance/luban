
import ast
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

sys.dont_write_bytecode = True
ROOT = Path(os.environ.get("WORKSPACE", "/workspace"))
LOG_DIR = Path(os.environ.get("LOG_DIR", "/logs/verifier"))
LOG_DIR.mkdir(parents=True, exist_ok=True)
MODULE_FILE = ROOT / 'markupsafe_like/formatting.py'
TEST_DIR = ROOT / "tests"
MUTATIONS = [{'name': 'mapping values not escaped', 'replace': [('escaped={key: escape(item) for key, item in value.items()}', 'escaped=dict(value.items())')]}, {'name': 'mapping subclasses not supported', 'replace': [('if isinstance(value, Mapping):', 'if isinstance(value, dict):')]}, {'name': 'mapping percent literal mishandled', 'replace': [('if isinstance(value, Mapping): escaped={key: escape(item) for key, item in value.items()}', 'if isinstance(value, Mapping): escaped={key: escape(item) for key, item in value.items()}; return Markup(str.__mod__(self.replace("%%", "%"), escaped))')]}, {'name': 'tuple values not escaped', 'replace': [('escaped=tuple(escape(item) for item in value)', 'escaped=tuple(value)')]}, {'name': 'single value not escaped', 'replace': [('else: escaped=escape(value)', 'else: escaped=value')]}, {'name': 'key error leaks', 'replace': [('raise ValueError(f"missing format key: {exc.args[0]}") from None', 'raise')]}, {'name': 'wrong missing key message', 'replace': [('missing format key: {exc.args[0]}', 'missing format key')]}, {'name': 'result loses markup type', 'replace': [('return Markup(str.__mod__(self, escaped))', 'return str.__mod__(self, escaped)')]}, {'name': 'ampersand escape order wrong', 'replace': [('return text.replace("&", "&amp;").replace("<", "&lt;")', 'return text.replace("<", "&lt;").replace("&", "&amp;")')]}, {'name': 'double quotes not escaped', 'replace': [('.replace(\'"\', "&#34;")', '')]}, {'name': 'apostrophes not escaped', 'replace': [('.replace("\'", "&#39;")', '')]}]
RESULTS = []

def record(name, passed, detail=""):
    RESULTS.append({"name": name, "passed": bool(passed), "detail": str(detail)})

def count_tests():
    total = 0
    for path in TEST_DIR.glob("test*.py"):
        try:
            tree = ast.parse(path.read_text())
        except SyntaxError:
            continue
        for node in ast.walk(tree):
            if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name.startswith("test_"):
                total += 1
    return total

def is_generated_artifact(path):
    clean = path.rstrip("/")
    parts = set(clean.split("/"))
    return (
        "__pycache__" in parts
        or ".pytest_cache" in parts
        or clean.endswith(".pyc")
        or clean == ".coverage"
        or clean.startswith("htmlcov/")
    )

def clear_generated_artifacts():
    for name in ("__pycache__", ".pytest_cache"):
        for path in ROOT.rglob(name):
            if path.is_dir():
                shutil.rmtree(path, ignore_errors=True)
            else:
                try:
                    path.unlink()
                except FileNotFoundError:
                    pass
    for path in ROOT.rglob("*.pyc"):
        try:
            path.unlink()
        except FileNotFoundError:
            pass

def changed_files():
    clear_generated_artifacts()
    result = subprocess.run(["git", "status", "--porcelain"], cwd=ROOT, text=True, capture_output=True, timeout=10)
    if result.returncode != 0:
        return [], result.stderr or result.stdout
    files = []
    for line in result.stdout.splitlines():
        if not line.strip():
            continue
        path = line[3:].strip()
        if is_generated_artifact(path):
            continue
        files.append(path)
    return files, ""

def run_tests():
    clear_generated_artifacts()
    env = dict(os.environ)
    env["PYTHONPATH"] = str(ROOT)
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    result = subprocess.run(
        [sys.executable, "-m", "unittest", "discover", "-s", str(TEST_DIR), "-p", "test*.py"],
        cwd=ROOT,
        text=True,
        capture_output=True,
        timeout=20,
        env=env,
    )
    return result.returncode == 0, (result.stdout + "\n" + result.stderr).strip()[-1600:]

def apply_mutation(source, mutation):
    mutated = source
    for old, new in mutation["replace"]:
        if old not in mutated:
            return None, f"missing replacement for {mutation['name']}: {old[:80]!r}"
        mutated = mutated.replace(old, new, 1)
    return mutated, ""

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

try:
    tests_total = count_tests()
    record("at least 15 tests are present", tests_total >= 15, f"found {tests_total}")
    changed, err = changed_files()
    record("only tests changed", bool(changed) and all(p.startswith("tests/") for p in changed), changed or err)
    ok, detail = run_tests()
    record("tests pass on correct implementation", ok, detail)
    original = MODULE_FILE.read_text()
    if tests_total >= 15 and ok:
        for mutation in MUTATIONS:
            mutated, mut_err = apply_mutation(original, mutation)
            if mutated is None:
                record(f"tests catch mutation: {mutation['name']}", False, mut_err)
                continue
            try:
                MODULE_FILE.write_text(mutated)
                mut_ok, output = run_tests()
                record(f"tests catch mutation: {mutation['name']}", not mut_ok, output)
            finally:
                MODULE_FILE.write_text(original)
    else:
        for mutation in MUTATIONS:
            record(f"tests catch mutation: {mutation['name']}", False, "skipped because the submitted tests did not run cleanly")
finally:
    write_reward()
