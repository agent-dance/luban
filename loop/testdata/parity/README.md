# Loop parity fixtures

Schema version `1` is intentionally small so future parity tasks can add cases
without writing new fake-provider boilerplate.

Required top-level fields:

- `id`, `title`, `status`, `updated_at`
- `source_tests`: existing Go tests or TS references that justify the expected behavior
- `coverage_tags`: stable feature/recovery tags consumed by `loop_metrics.py`
- `parity_tasks`: task ids covered by this fixture
- `input`: `user_message`, `max_turns`, `max_tokens`, optional `disable_max_turns`
- `tools`: fake registry entries with `kind` of `echo`, `static`, `attachment`, or `tool_search`
- `turns`: provider stream turns, each with `events`, optional `delay_ms`, optional typed API `error`
- `expected`: event subsequence, final text, message count, provider calls, tool visibility, or expected error

`status: active` fixtures run in `TestParityFixtures`. `status: pending` and
`status: expected_failure` fixtures are coverage metadata only; they keep metrics
honest for features that are not implemented yet without breaking the fast suite.
