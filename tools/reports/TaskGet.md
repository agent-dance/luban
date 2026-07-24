# TaskGet Parity Report

- Original: `src/tools/TaskGetTool/TaskGetTool.ts`
- Go: `tools/tasks.go`

## Verdict

- Summary: Exact surface; the lookup contract is aligned, and the task object now comes from a persistent, scope-aware Go backend, though the original runtime still carries richer hooks and typing.

## Name And Description

- Name parity: exact.
- Description parity: Both versions describe the tool around the same core capability: Fetches one task by identifier from the shared task system. Wording differences are minor unless called out under key gaps.

## Parameters And Types

- Type signature: taskId: string.
- Parameter parity: top-level parameter names match the original audit; ordering differences are non-semantic.

## Implementation Overview

- Core capability: Fetches one task by identifier from the shared task system.
- Typical scenarios: Use it when the model needs the authoritative state of a specific task before taking the next action.
- Core pain point addressed: It addresses visibility and coordination for multi-step work so the model does not have to track the whole plan in free text.
- Main challenges: The hard parts are persistence, dependency edges, blocking semantics, and keeping task state consistent across tools and sessions.
- Strategy consistency: Partially consistent. Both versions revolve around a shared persistent task system, and Go now mirrors more of the original scope, locking, and runtime-task substrate, but the original still has a richer hook-aware app-state runtime.

## Visual Flow And Decision Map

### How To Read The Diagram

- Read the blue path first to understand the shared happy path.
- Read the yellow diamonds next; they are the branch conditions that decide which path the tool takes.
- Read the red boxes last; they mark exact places where Go diverges from `../src`, or where a path is intentionally out of scope.

```mermaid
flowchart TD
    A(["Start"])
    B["Parse task id"]
    C["Resolve task-list scope"]
    D{"Task found?"}
    E(["Return not-found result"])
    F(["Return task object"])
    X1["Original only exposes this tool when task-v2 is enabled"]
    X2["Original task lookup can resolve richer team / teammate scopes"]
    A --> B
    B --> C
    C --> D
    D -- "No" --> E
    D -- "Yes" --> F
    B -.-> X1
    C -.-> X2
    classDef start fill:#e8f4fd,stroke:#1d4ed8,color:#0f172a;
    classDef step fill:#eff6ff,stroke:#60a5fa,color:#0f172a;
    classDef decision fill:#fef3c7,stroke:#d97706,color:#7c2d12;
    classDef gap fill:#fee2e2,stroke:#dc2626,color:#7f1d1d;
    classDef result fill:#dcfce7,stroke:#16a34a,color:#14532d;
    class A start;
    class B,C step;
    class D decision;
    class X1,X2 gap;
    class E,F result;
```

### Decision Points

- `Task found?` is the visible branch, but the more important hidden branch is scope resolution before lookup starts.

### Flow-Divergence Hotspots

- Lookup itself is straightforward once the correct task list is resolved.
- The main differences are exposure gating and the richer original scope-resolution logic for teams and teammates.


## Output And Format

- Output comparison: The original returns a structured task object; Go returns a text rendering backed by the newer persistent task substrate.

## Key Gaps

- The remaining gaps are mainly hook/app-state integration, richer typed results, and the broader original teammate/runtime lifecycle.
