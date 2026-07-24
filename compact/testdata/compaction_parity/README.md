# Compaction parity fixtures

This directory documents the end-to-end compaction parity harness.

Fixtures are intentionally small and provider-free. They model message shapes
that are expensive to rediscover in each task:

- `long_tool_conversation`: many user/assistant turns with `tool_use` and
  matching `tool_result` blocks.
- `same_id_assistant_fragments`: adjacent assistant fragments sharing one API
  message id, used to preserve provider invariants at compact boundaries.
- `parallel_tool_results`: large sibling tool results in a single user message.
- `media_attachments`: image and document blocks that must be stripped before
  the summary API call.
- `compact_boundaries`: compact boundary marker metadata, including trigger,
  token counts, prior tail id, discovered tools, and preserved segment anchor.
- `post_compact_segments`: boundary, summary, preserved messages, attachments,
  and hook results in canonical order.
- `plans_skills_mcp_attachments`: post-compact reinjection state for active
  plans, plan-mode reminder, invoked skills, background tasks, deferred tools,
  agent listings, and MCP server snapshots.
- `session_metadata`: provider/model/cwd/branch/preview sidecar metadata used
  by resume and transcript parity cases.

Provider fakes cover the non-network scenarios used by the harness:

- `provider_fake_successful_summary`: summary stream returns text.
- `provider_fake_prompt_too_long_summary`: compact summary call fails with a
  prompt-too-long API error.
- `provider_fake_prompt_too_long_main_request`: main query call fails before
  compaction/recovery.
- `provider_fake_incomplete_summary`: summary stream ends with `max_tokens`.
- `provider_fake_empty_assistant_response`: stream completes without text.
- `provider_fake_media_size_error`: provider rejects oversized media so retry
  paths can strip or replace media without credentials.

When adding a parity task, add a direct regression slot to the manifest first.
If production behavior is not implemented yet, keep the slot with an explicit
`pending_task` value and skip it from Go with `TODO(task_xx)`. Completed P0
foundation tasks must assert concrete behavior.
