# Tool parity fixture format

Fixtures in this directory are consumed by `tools/parity_harness_test.go`.
Each fixture is a small golden contract derived from a TypeScript source
reference and is executed through the production Go registry dispatcher. The
harness creates an isolated workspace and state stores per fixture.

Minimum fields for new fixtures:

- `name`: stable subtest name.
- `tool`: model-visible Go tool name registered in the harness.
- `category`: one of `read-only`, `mutating`, `stateful`, `web`, or a more
  specific label used by the task adding the fixture.
- `ts_reference`: TypeScript source file with an explicit line citation.
- `ts_behavior`: concise description of the cited behavior being locked.
- `input`: tool input. `${workspace}` and `${server_url}` placeholders are
  expanded by the harness.

Optional assertion sections:

- `runtime.interactive`: defaults to `true`. Set it explicitly to `false` for
  TS behaviors that only exist when task-v2 is disabled, such as TodoWrite.
- `expected_enabled`: asserts the production registry visibility gate.
- `permission`: expected pre-dispatch permission behavior and message snippets.
- `permission.suggestions_json_contains`: snippets required in proposed
  permission updates, such as a `localSettings` domain allow rule.
- `skip_execution`: checks visibility/permission/state without dispatching the
  tool. Use this for a permission-only golden, not to hide an output mismatch.
- `expected_validation_error`: registry-level input validation snippet.
- `expected_model_text`: exact provider-visible tool result text.
- `expected_model_text_contains`: substrings expected in model-visible text.
- `expected_typed_data_json`: exact JSON form of `ToolResultBlock.Data`.
- `expected_typed_data_json_contains`: substrings expected after marshaling
  `ToolResultBlock.Data`.
- `expected_content_json`: top-level JSON fields expected in model content.
- `expected_state`: filesystem, read-state, todo-store, and web-cache effects.

Setup fields:

- `setup.files`: files created before dispatch.
- `setup.todos`: TodoWrite state seeded before dispatch.
- `setup.http_response`: an isolated HTTP response; use `${server_url}` in the
  tool input.

Every fixture must contain at least one assertion. Unknown fixture fields fail
loading, which prevents misspelled assertions from silently producing a green
test. Add a JSON file only; dispatcher construction and state isolation remain
centralized in the harness.
