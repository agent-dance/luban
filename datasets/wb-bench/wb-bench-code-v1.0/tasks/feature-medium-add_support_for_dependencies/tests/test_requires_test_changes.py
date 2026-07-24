from pathlib import Path


def test_agent_patch_includes_test_changes():
    patch_path = Path("/logs/verifier/pre_injected_agent.patch")
    assert patch_path.exists(), "agent patch snapshot was not captured before injected tests"
    patch = patch_path.read_text(errors="replace")
    changed = []
    for line in patch.splitlines():
        if not line.startswith("diff --git "):
            continue
        parts = line.split()
        if len(parts) < 4:
            continue
        path = parts[3][2:] if parts[3].startswith("b/") else parts[3]
        if path.endswith(".py") and (
            path.startswith("tests/")
            or path.startswith("testing/")
            or "/tests/" in path
            or path.endswith("_test.py")
            or path.startswith("test/")
        ):
            changed.append(path)
    assert changed, "expected the solution patch to include at least one Python test file change"
