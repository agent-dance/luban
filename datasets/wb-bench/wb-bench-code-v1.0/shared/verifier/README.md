# Shared Verifier

This plugin adapts task-local deterministic checks to `workbuddy_bench.judge:CompositeVerifier`.

Item source:

- Each task has `tests/verifier.toml`.
- The manifest declares the deterministic family: `script_verifier`, `pytest_injected`, or `repo_understanding`.
- The plugin builds one `rule` item per task: `<source_case>:rule`.

Runtime:

- `plugin.py` registers `HarborScriptRuleJudgeRunner` in Harbor runs and `ScriptRuleJudgeRunner` for local runner tests.
- `rule.py` builds the task-local command and reads `score.json` or `reward.json`.
- `tests/test.sh` is only a Harbor format stub and is not the scoring backend.

End-to-end check:

Root-level pytest contract tests and dedicated smoke job configs are no longer
part of this repository. Use a dataset job under `configs/jobs/`, or a temporary
local job config, for rollout/verify checks.
