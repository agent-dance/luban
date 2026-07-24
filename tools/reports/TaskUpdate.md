# TaskUpdate Parity Report

- Original: `src/tools/TaskUpdateTool/TaskUpdateTool.ts`
- Go: `tools/tasks.go`

## Verdict

- Summary: Exact surface; the update contract is aligned, and Go now updates tasks inside a persistent, scope-aware, locked backend.

## Name And Description

- Name parity: exact.
- Description parity: Both versions describe the tool around the same core capability: Mutates task fields, relationships, and status inside the shared task system. Wording differences are minor unless called out under key gaps.

## Parameters And Types

- Type signature: taskId: string, subject?: string, description?: string, activeForm?: string, status?: string, addBlocks?: string[], addBlockedBy?: string[], owner?: string, metadata?: object.
- Parameter parity: top-level parameter names match the original audit; ordering differences are non-semantic.

## Implementation Overview

- Core capability: Mutates task fields, relationships, and status inside the shared task system.
- Typical scenarios: Use it when tracked work changes state, ownership, dependencies, or descriptive metadata.
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
    B["Parse id and optional fields"]
    C["Resolve task-list scope"]
    D{"Task found?"}
    E(["Return not-found result"])
    F["Apply field updates"]
    G{"Task completed by this update?"}
    H["Persist updated task"]
    I(["Return updated task"])
    X1["Original only exposes this tool when task-v2 is enabled"]
    X2["Original also runs completion hooks, mailbox side effects, and richer UI behavior"]
    X3["Go treats empty strings as not provided for several fields"]
    A --> B
    B --> C
    C --> D
    D -- "No" --> E
    D -- "Yes" --> F
    F --> G
    G -- "Yes / No" --> H
    H --> I
    A -.-> X1
    G -.-> X2
    F -.-> X3
    classDef start fill:#e8f4fd,stroke:#1d4ed8,color:#0f172a;
    classDef step fill:#eff6ff,stroke:#60a5fa,color:#0f172a;
    classDef decision fill:#fef3c7,stroke:#d97706,color:#7c2d12;
    classDef gap fill:#fee2e2,stroke:#dc2626,color:#7f1d1d;
    classDef result fill:#dcfce7,stroke:#16a34a,color:#14532d;
    class A start;
    class B,C,F,H step;
    class D,G decision;
    class X1,X2,X3 gap;
    class E,I result;
```

### Decision Points

- `Task found?` decides whether updates can even be applied.
- `Task completed by this update?` is where the original branches into task-completion hooks and side effects.

### Flow-Divergence Hotspots

- The shared path is apply-update-and-persist.
- The original wraps completion transitions in hooks, mailbox side effects, and richer UI changes; Go currently does not.
- Go also simplifies several field semantics by treating empty strings as missing input.


## Output And Format

- Output comparison: The original returns a structured update result; Go returns a plain-text summary on top of the newer persistent task substrate.

## Key Gaps

- The remaining gaps are mainly hook/app-state integration, richer typed results, and the broader original teammate/runtime lifecycle.
