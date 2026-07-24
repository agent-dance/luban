# ExitWorktree Parity Report

- Original: `src/tools/ExitWorktreeTool/ExitWorktreeTool.ts`
- Go: `tools/worktree.go`

## Verdict

- Summary: Exact surface; Go now mirrors more of the original keep-or-remove worktree flow with canonical repo cleanup and persisted state recovery.

## Name And Description

- Name parity: exact.
- Description parity: Both versions describe the tool around the same core capability: Keeps or removes the current isolated worktree and cleans up related state. Wording differences are minor unless called out under key gaps.

## Parameters And Types

- Type signature: action: string, discard_changes?: boolean.
- Parameter parity: top-level parameter names match the original audit; ordering differences are non-semantic.

## Implementation Overview

- Core capability: Keeps or removes the current isolated worktree and cleans up related state.
- Typical scenarios: Use it when work in the isolated checkout is done and the session must either keep the branch or tear the environment down cleanly.
- Core pain point addressed: It solves isolation: risky or branch-heavy changes should happen in a separate git worktree instead of polluting the main session state.
- Main challenges: The hard parts are git worktree lifecycle, branch cleanup, and keeping session state aligned with filesystem state.
- Strategy consistency: Largely consistent. Both versions center on explicit worktree state transitions backed by git operations.

## Visual Flow And Decision Map

### How To Read The Diagram

- Read the blue path first to understand the shared happy path.
- Read the yellow diamonds next; they are the branch conditions that decide which path the tool takes.
- Read the red boxes last; they mark exact places where Go diverges from `../src`, or where a path is intentionally out of scope.

```mermaid
flowchart TD
    A(["Start"])
    B{"Active worktree session exists?"}
    C(["Return rejection"])
    D{"Keep or remove?"}
    E["Restore original session state"]
    F(["Return keep summary"])
    G{"Discard changes allowed?"}
    H["Count changed files and commits"]
    I{"Safe to remove?"}
    J["Remove worktree and branch"]
    K(["Return remove summary"])
    X1["Go checks file changes only; original also counts extra commits"]
    X2["Go never fully restores persisted worktree session state"]
    A --> B
    B -- "No" --> C
    B -- "Yes" --> D
    D -- "Keep" --> E
    D -- "Remove" --> G
    E --> F
    G -- "Yes" --> J
    G -- "No" --> H
    H --> I
    I -- "No" --> C
    I -- "Yes" --> J
    J --> K
    H -.-> X1
    E -.-> X2
    classDef start fill:#e8f4fd,stroke:#1d4ed8,color:#0f172a;
    classDef step fill:#eff6ff,stroke:#60a5fa,color:#0f172a;
    classDef decision fill:#fef3c7,stroke:#d97706,color:#7c2d12;
    classDef gap fill:#fee2e2,stroke:#dc2626,color:#7f1d1d;
    classDef result fill:#dcfce7,stroke:#16a34a,color:#14532d;
    class A start;
    class E,H,J step;
    class B,D,G,I decision;
    class X1,X2 gap;
    class C,F,K result;
```

### Decision Points

- `Active worktree session exists?` prevents invalid exits when the session never entered worktree mode in the first place.
- `Keep or remove?` selects between restoring the main session state and actually destroying the isolated git state.
- `Discard changes allowed?` and `Safe to remove?` are the protective gates before destructive cleanup.

### Flow-Divergence Hotspots

- The original remove guard is stricter: it fails closed on both changed files and extra commits, and treats unknown state as unsafe.
- Go never fully adopted the original persisted session restore path, so keep / remove act on lighter in-memory worktree state.


## Output And Format

- Output comparison: The original returns a richer structured result tied to session state; Go returns a plain-text keep/remove summary, but the backing state is now persisted and repo-root aware.

## Key Gaps

- The main remaining differences are advanced hook/session integrations rather than the visible action choices.
