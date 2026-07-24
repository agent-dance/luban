"""Rule command construction."""

from __future__ import annotations

from workbuddy_bench.judge import JudgeSpec

from .manifest import VerifierFamily, VerifierManifest


JUDGE_TYPE = "rule_script"


def build_rule_judge(
    *,
    item_id: str,
    manifest: VerifierManifest,
    timeout_sec: float | None,
) -> JudgeSpec:
    config: dict[str, object] = {
        "command": command_for_manifest(manifest),
        "cwd": manifest.cwd,
        "shell": True,
        "score_json": "/logs/verifier/score.json",
        "reward_json": "/logs/verifier/reward.json",
    }
    if timeout_sec is not None:
        config["timeout_sec"] = timeout_sec
    return JudgeSpec(
        name="rule",
        type=JUDGE_TYPE,
        family="rule",
        item_ids=[item_id],
        config=config,
        metadata={"family": manifest.family.value},
    )


def command_for_manifest(manifest: VerifierManifest) -> str:
    if manifest.family == VerifierFamily.SCRIPT_VERIFIER:
        return _script_verifier_command()
    if manifest.family == VerifierFamily.PYTEST_INJECTED:
        return _pytest_injected_command(manifest.command)
    if manifest.family == VerifierFamily.REPO_UNDERSTANDING:
        return _repo_understanding_command()
    raise ValueError(f"unsupported verifier family: {manifest.family}")


def prepare_command() -> str:
    return r"""
set -euo pipefail
mkdir -p /logs/verifier
cd /workspace
find /workspace -type d -name __pycache__ -prune -exec rm -rf {} + 2>/dev/null || true
git add -A
git diff --staged > /logs/verifier/agent.patch 2>/dev/null || true
git reset >/dev/null 2>&1 || true
""".strip()


def _script_verifier_command() -> str:
    return r"""
set -euo pipefail
cd /workspace
START_TIME=$(date +%s)
mkdir -p /logs/verifier
echo "=== Running verifier.py ==="
WORKSPACE=/workspace LOG_DIR=/logs/verifier PYTHONDONTWRITEBYTECODE=1 \
    python3 /tests/verifier.py > /logs/verifier/test_output.txt 2>&1 || true
cat /logs/verifier/test_output.txt 2>/dev/null || true
END_TIME=$(date +%s)
WALL_TIME=$((END_TIME - START_TIME))
printf "%s" "$WALL_TIME" > /tmp/verifier_wall_time
python3 - <<'PY'
import json
from pathlib import Path

path = Path("/logs/verifier/reward.json")
if path.exists():
    try:
        data = json.loads(path.read_text())
    except Exception as exc:
        data = {
            "overall": 0.0,
            "test_pass_rate": 0.0,
            "tests_passed": 0,
            "tests_total": 1,
            "test_status": "build_error",
            "error": f"verifier wrote malformed reward.json: {type(exc).__name__}: {exc}",
            "tests": [],
        }
else:
    data = {
        "overall": 0.0,
        "test_pass_rate": 0.0,
        "tests_passed": 0,
        "tests_total": 1,
        "test_status": "build_error",
        "error": "verifier did not write reward.json",
        "tests": [],
    }
data["wall_time_sec"] = int(Path("/tmp/verifier_wall_time").read_text())
path.write_text(json.dumps(data, indent=2, ensure_ascii=False))
Path("/logs/verifier/reward.txt").write_text(str(data.get("overall", 0)))
PY
cat /logs/verifier/reward.json
""".strip()


def _repo_understanding_command() -> str:
    return r"""
set -euo pipefail
cd /workspace
START_TIME=$(date +%s)
mkdir -p /logs/verifier
echo "=== Scoring structured repo-understanding report ==="
python3 /tests/scorer.py --repo /workspace --report /workspace/analysis.json \
    --output /logs/verifier/reward.json > /logs/verifier/test_output.txt 2>&1 || true
cat /logs/verifier/test_output.txt 2>/dev/null || true
git add -A
git diff --staged -- . ':!analysis.json' > /logs/verifier/business.patch 2>/dev/null || true
END_TIME=$(date +%s)
WALL_TIME=$((END_TIME - START_TIME))
printf "%s" "$WALL_TIME" > /tmp/verifier_wall_time
python3 - <<'PY'
import json
from pathlib import Path

path = Path("/logs/verifier/reward.json")
if path.exists():
    try:
        data = json.loads(path.read_text())
    except Exception as exc:
        data = {
            "overall": 0.0,
            "test_pass_rate": 0.0,
            "test_status": "build_error",
            "error": f"scorer wrote malformed reward.json: {type(exc).__name__}: {exc}",
        }
else:
    data = {
        "overall": 0.0,
        "test_pass_rate": 0.0,
        "test_status": "build_error",
        "error": "scorer did not write reward.json",
    }
business_patch = Path("/logs/verifier/business.patch").read_text(errors="replace")
if business_patch.strip():
    data["overall"] = 0.0
    data["test_pass_rate"] = 0.0
    data["test_status"] = "no_pass"
    data["error"] = "business code was modified; only analysis.json may be changed"
    data["business_patch_lines"] = len(business_patch.splitlines())
data["wall_time_sec"] = int(Path("/tmp/verifier_wall_time").read_text())
path.write_text(json.dumps(data, indent=2, ensure_ascii=False))
Path("/logs/verifier/reward.txt").write_text(str(data.get("overall", 0)))
PY
cat /logs/verifier/reward.json
""".strip()


def _pytest_injected_command(pytest_command: str) -> str:
    template = r"""
set -euo pipefail
cd /workspace
START_TIME=$(date +%s)
mkdir -p /logs/verifier
if [ -d /tests/injected ]; then
python3 - <<'PY'
from pathlib import Path
import shutil

src_root = Path("/tests/injected")
dst_root = Path("/workspace")
for src_path in src_root.rglob("*"):
    if not src_path.is_file():
        continue
    dest_path = dst_root / src_path.relative_to(src_root)
    dest_path.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src_path, dest_path)
PY
fi
echo "=== Running injected tests ==="
set +e
__PYTEST_COMMAND__
PYTEST_EXIT=$?
set -e
__PARSE_PYTEST_RESULTS__
cat /logs/verifier/test_output.txt 2>/dev/null || true
END_TIME=$(date +%s)
WALL_TIME=$((END_TIME - START_TIME))
__SCORE_PYTEST_RESULTS__
cat /logs/verifier/score.json
python3 - <<'PY'
import json
score = json.load(open("/logs/verifier/score.json"))
numeric = {
    key: value
    for key, value in score.items()
    if isinstance(value, (int, float)) and not isinstance(value, bool)
}
overall = score.get("overall", numeric.get("overall", 0.0))
if not isinstance(overall, (int, float)) or isinstance(overall, bool):
    overall = 0.0
numeric["reward"] = overall
json.dump(numeric, open("/logs/verifier/reward.json", "w"), indent=2)
PY
cat /logs/verifier/reward.json
""".strip()
    return (
        template.replace("__PYTEST_COMMAND__", pytest_command)
        .replace("__PARSE_PYTEST_RESULTS__", _parse_pytest_results_snippet())
        .replace("__SCORE_PYTEST_RESULTS__", _score_pytest_results_snippet())
    )


def _parse_pytest_results_snippet() -> str:
    return r"""
eval "$(PYTEST_EXIT="$PYTEST_EXIT" python3 <<'PY'
import os
import re
import xml.etree.ElementTree as ET
from pathlib import Path

xml_path = "/logs/verifier/results.xml"
output_text = (
    Path("/logs/verifier/test_output.txt").read_text(errors="replace")
    if Path("/logs/verifier/test_output.txt").exists()
    else ""
)
exit_code = int(os.environ["PYTEST_EXIT"])

def parse_text_fallback(text: str):
    passed = failed = errors = skipped = total = 0
    ran_match = re.search(r"Ran (\d+) tests?", text)
    if ran_match:
        total = int(ran_match.group(1))
        failed_match = re.search(r"failures=(\d+)", text)
        error_match = re.search(r"errors=(\d+)", text)
        skipped_match = re.search(r"skipped=(\d+)", text)
        failed = int(failed_match.group(1)) if failed_match else 0
        errors = int(error_match.group(1)) if error_match else 0
        skipped = int(skipped_match.group(1)) if skipped_match else 0
        passed = max(0, total - failed - errors - skipped)
    rate = round(passed / total, 4) if total > 0 else 0
    return passed, failed, errors, skipped, total, rate

if Path(xml_path).exists():
    try:
        tree = ET.parse(xml_path)
        root = tree.getroot()
        suites = root.findall("testsuite") if root.tag == "testsuites" else [root]
        total = sum(int(s.get("tests", 0)) for s in suites)
        failed = sum(int(s.get("failures", 0)) for s in suites)
        errors = sum(int(s.get("errors", 0)) for s in suites)
        skipped = sum(int(s.get("skipped", 0)) for s in suites)
        passed = max(0, total - failed - errors - skipped)
        rate = round(passed / total, 4) if total > 0 else 0
    except Exception:
        passed, failed, errors, skipped, total, rate = parse_text_fallback(output_text)
else:
    passed, failed, errors, skipped, total, rate = parse_text_fallback(output_text)

if total <= 0:
    passed, failed, errors, skipped, total, rate = parse_text_fallback(output_text)

build_error_markers = (
    "ERROR: found no collectors",
    "ImportError while loading conftest",
    "fixture '",
)
arg_parse_error = "error: unrecognized arguments:" in output_text

if exit_code not in (0, 1):
    status = "build_error"
elif any(marker in output_text for marker in build_error_markers):
    status = "build_error"
elif arg_parse_error and total <= 0:
    status = "build_error"
elif total <= 0:
    status = "build_error"
elif rate >= 1.0:
    status = "full_pass"
elif rate > 0:
    status = "partial_pass"
else:
    status = "no_pass"

print(f"TEST_PASS_RATE={rate}")
print(f"TEST_PASSED={passed}")
print(f"TEST_FAILED={failed}")
print(f"TEST_ERRORS={errors}")
print(f"TEST_SKIPPED={skipped}")
print(f"TEST_TOTAL={total}")
print(f"TEST_STATUS={status}")
PY
)"
""".strip()


def _score_pytest_results_snippet() -> str:
    return r"""
SCORER="/environment/scorer.py"
[ ! -f "$SCORER" ] && SCORER="/scorer/scorer.py"
[ ! -f "$SCORER" ] && SCORER="/tests/scorer.py"
_RATE="${TEST_PASS_RATE:-0}"
_PASSED="${TEST_PASSED:-0}"
_TOTAL="${TEST_TOTAL:-0}"
_WALL="${WALL_TIME:-0}"
if [ -f /tests/gold.patch ] && [ -f "$SCORER" ]; then
    python3 "$SCORER" \
        --agent-patch /logs/verifier/agent.patch \
        --gold-patch /tests/gold.patch \
        --test-pass "$_RATE" \
        --tests-passed "$_PASSED" \
        --tests-total "$_TOTAL" \
        --wall-time "$_WALL" \
        ${TEST_STATUS:+--test-status "$TEST_STATUS"} \
        --output /logs/verifier/score.json
else
    _RATE="$_RATE" _PASSED="$_PASSED" _TOTAL="$_TOTAL" _WALL="$_WALL" \
        TEST_STATUS="${TEST_STATUS:-}" python3 - <<'PY'
import json
import os

tpr = float(os.environ.get("_RATE") or 0)
total = int(os.environ.get("_TOTAL") or 0)
status = os.environ.get("TEST_STATUS") or (
    "build_error"
    if total == 0
    else ("full_pass" if tpr >= 1 else ("partial_pass" if tpr > 0 else "no_pass"))
)
json.dump(
    {
        "test_pass_rate": tpr,
        "test_status": status,
        "overall": tpr,
        "tests_passed": int(os.environ.get("_PASSED") or 0),
        "tests_total": total,
        "wall_time_sec": float(os.environ.get("_WALL") or 0),
    },
    open("/logs/verifier/score.json", "w"),
    indent=2,
)
PY
fi
""".strip()
