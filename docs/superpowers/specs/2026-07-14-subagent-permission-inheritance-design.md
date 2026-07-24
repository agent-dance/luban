# Subagent Permission Inheritance

## Status

Approved design, pending implementation plan and code changes.

## Problem

When the foreground agent runs in Auto mode, an ordinary subagent can still
prompt for permission. The parent runtime already exposes Auto as
`bypassPermissions`, but the ordinary Agent launch path does not use that value
as the child's default. Instead, it derives the child mode from model-controlled
Agent input or agent-profile configuration and normalizes an empty value to
`default`. The child permission wrapper then places that explicit `default`
override on every tool request, downgrading the parent's Auto policy to a
prompting policy.

This makes permission behavior depend on what the model puts in an Agent tool
call rather than on the permission policy selected by the user.

## Decision

Every subagent receives an immutable snapshot of its parent's final effective
permission policy when it is spawned. Permission policy is not an Agent tool
argument and is not an agent-profile setting.

The snapshot remains fixed for the lifetime of the child, including retained
and background children. A later foreground mode change affects only children
spawned after the change.

## Authority Boundary

Only trusted runtime state may establish a subagent's permission policy. The
following model- or configuration-facing inputs will be removed:

- the `mode` property in the Agent tool schema;
- `AgentInput.Mode` and its compatibility normalization and validation;
- `permissionMode` in Markdown/YAML and inline JSON agent profiles;
- `PermissionMode` in public agent definitions;
- profile defaults that select `dontAsk`, `bubble`, `plan`, or
  `bypassPermissions`.

Legacy custom profiles that declare `permissionMode` must fail validation with
a clear migration error. Silently ignoring the field would create a false
belief that the configured policy still applies.

Internal state may record the inherited mode for execution, persistence,
resumption, and audit. That state must be named as inherited runtime state, not
accepted through an Agent invocation or profile.

## Effective Policy

At spawn time the runtime clones the parent's `ToolRuntimeContext`, including:

- permission mode;
- allowed, denied, and ask rules;
- tool allowlists and denylists;
- project root and allowed directories;
- runtime feature and sandbox-related constraints;
- session and actor attribution needed for approval presentation and audit.

The child runtime overlays only child identity, child working-directory scope,
and inherited model metadata. It must not replace the copied permission mode
with an Agent input or profile value.

If the parent snapshot has no permission mode, the child uses the runtime's
canonical safe default. No model-provided fallback is consulted.

The effective child capability is:

```text
parent permission snapshot
  intersect child filesystem/worktree scope
  intersect profile tool visibility and deny rules
  intersect bypass-immune safety policy
```

An agent profile may continue to restrict the tool registry and add deny rules.
It cannot turn an allowed-tool declaration into a permission grant. In
particular, a matching profile allowed rule must not rewrite a permission
request to `acceptEdits`.

## Prompt Routing

`bubble` and `dontAsk` currently combine permission policy with unattended
prompt behavior. They will no longer be permission modes for subagents.

Prompt presentation is an internal launch concern:

- synchronous children may present an inherited Ask-policy decision through
  the parent session's approval UI;
- unattended background children fail closed when approval cannot be
  presented;
- fork or team runtimes that support parent-session approval forwarding may use
  an internal approval-routing option.

This routing option is not model-visible, does not change allow/deny/ask policy,
and cannot grant authority.

## Plan Mode

Subagents inherit Plan mode exactly like Auto or Ask mode. They cannot select,
enter, or exit a permission mode themselves. Model-driven plan transitions
remain owned by the foreground runtime. Child registries therefore do not
expose permission-mode transition tools merely because a profile or Agent input
requested a mode.

## Spawn Paths

The invariant applies to ordinary, forked, background, retained, resumed,
worktree, and team subagents.

Each path must either receive the parent's spawn-time permission snapshot or
fail closed. A remote runtime that cannot transport and enforce the snapshot
must reject the launch instead of falling back to `default` or an agent profile.

Retained-session records persist the inherited permission snapshot separately
from `AgentInput`. Resume reconstructs the child from that persisted snapshot,
not from the foreground agent's current mode.

## Data Flow

```text
user selects Auto / Ask / Plan
  -> RuntimeScope publishes final ToolRuntimeContext
  -> Agent launch captures and clones that context
  -> child registry receives the cloned policy and narrower child scope
  -> child permission wrapper attaches the inherited mode to each request
  -> parent permission dispatcher evaluates the request and presents approvals
     when the inherited policy requires them
```

No edge in this flow reads permission policy from model output or an agent
profile.

## Safety Properties

- Auto inheritance does not bypass mandatory safety checks, explicit denies,
  tool denylists, protected paths, or sandbox constraints.
- Ask inheritance cannot be upgraded to Auto by selecting a custom agent.
- Plan inheritance cannot be exited by a child.
- A profile may remove tools or deny operations but cannot grant permissions.
- A background child cannot acquire a later foreground Auto mode.
- A child spawned in Auto remains in its captured Auto policy if the foreground
  later switches to Ask or Plan.

## Compatibility

This intentionally removes unsupported permission fields rather than retaining
no-op compatibility aliases. Existing custom profiles containing
`permissionMode` receive a validation error explaining that subagents now
inherit their parent's spawn-time policy and that the field must be removed.

Persisted background tasks from the previous format do not contain a trusted
parent snapshot, and their recorded child mode may have originated from the
legacy model/profile-controlled field. They therefore fail closed with a
migration error rather than sampling the current foreground or reviving an
untrusted recorded mode.

## Test Strategy

Implementation follows red-green-refactor. Regression tests will first prove
the current failure and then cover both non-downgrade and non-escalation.

1. Auto parent plus an ordinary child performs a safe tool action without a
   normal permission prompt.
2. The Agent schema omits `mode`, and an Agent invocation containing that legacy
   property is rejected before a child starts.
3. Direct internal launch helpers have no permission-mode input through which
   an Ask parent can be escalated to Auto.
4. A custom profile without `permissionMode` inherits Auto, Ask, and Plan in
   table-driven cases.
5. A custom profile declaring `permissionMode` fails validation with the
   migration message.
6. Agent schemas and public agent definitions contain no permission-mode input.
7. Profile deny rules still deny; profile allowed-tool declarations do not
   bypass the parent dispatcher.
8. A background child remains on its spawn-time snapshot after the foreground
   changes mode.
9. A resumed child restores its persisted inherited snapshot.
10. Team and fork paths obey the same snapshot rule and preserve internal
    approval routing without changing policy.
11. Existing mandatory safety and allowlist/denylist tests continue to pass in
    Auto mode.

Verification includes focused permission and Agent tests, the complete Go test
suite, static analysis, formatting, and race-sensitive tests for the affected
packages.

## Non-Goals

- Redesigning the foreground permission checker.
- Weakening mandatory safety or protected-path checks.
- Making child permissions follow live foreground changes.
- Adding new permission modes or dependencies.
- Refactoring unrelated Agent lifecycle or TUI behavior.
