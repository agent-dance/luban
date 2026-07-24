# Resume Session Namespace Isolation Design

## Problem

`DefaultRepository` currently treats every directory below `~/.claude/projects`
as a LUBAN Code session store. Claude Code owns that directory and writes a
different JSONL envelope format. When LUBAN Code lists or resumes one of those
files, `types.Message.UnmarshalJSON` receives a string-valued `content` field,
logs repeated corruption warnings, returns an empty partial history, and can
later overwrite the foreign transcript.

## Constraints

- Continue reading project-scoped and flat sessions created by DeepSeek Code.
- Continue reading project-scoped and flat sessions created by the original Go
  runtime in `~/.claude-go`.
- Continue reading the oldest flat sessions in `~/.claude/sessions`.
- Never enumerate Claude Code's `~/.claude/projects` as LUBAN Code sessions.
- Do not add dependencies or change the persisted LUBAN message format.

## Considered Approaches

1. Accept a string in `Message.UnmarshalJSON`. This only suppresses the first
   type error; it cannot interpret Claude Code's outer event envelope and would
   turn foreign records into misleading empty messages.
2. Import Claude Code transcripts. This requires a separate versioned importer
   for queue operations, sidechains, nested messages, and tool results. It is
   intentionally outside this bug fix.
3. Isolate repository namespaces. Full repository layouts are scanned only for
   LUBAN/DeepSeek/Go-owned config homes, while `~/.claude` contributes only its
   historical flat `sessions` directory. This is the selected approach.

## Design

`Repository` keeps two explicit fallback collections:

- `fallbackConfigHomes` for config homes whose `projects` and `sessions`
  layouts are owned by this Go application family.
- `fallbackLegacyRoots` for individual flat session directories that are safe
  to scan without traversing a foreign application's project namespace.

`DefaultRepository` registers `~/.deepseek-code` and `~/.claude-go` as complete
fallback config homes, then registers only `~/.claude/sessions` as a legacy
root. `allStoreDirs` combines these with the current `~/.prc-code` layout and
deduplicates the result.

## Error Handling

Foreign Claude Code project transcripts are not candidates in default
repository discovery, so unscoped listing and resolution never decode or
mutate them. Explicit project-directory arguments and constructed `Ref` values
remain trusted-path APIs; application call sites obtain them from
`ProjectDirForCWD`, `Search`, or `Resolve`. Existing behavior for a genuinely
corrupt LUBAN Code transcript remains unchanged and continues to return its
readable prefix.

## Testing

- Create a native-shaped transcript in a temporary `~/.claude/projects`
  directory and verify repository search and resolution ignore it.
- Create real LUBAN-format sessions in temporary `~/.claude-go/projects` and
  `~/.claude/sessions` directories and verify both remain resolvable.
- Run the `session` package tests, then the repository-wide Go test suite and
  static analysis.

## Self-review

The design has no placeholders. Namespace ownership is explicit, the selected
approach covers the reported failure without changing message semantics, and
the compatibility boundaries are independently testable.
