# ExitPlanMode Parity Report

- Original: `src/tools/ExitPlanModeTool/ExitPlanModeV2Tool.ts`
- Go: `tools/planmode.go`

## Verdict

- Summary: Exact surface; Go now persists and restores local plan-mode state and surfaces allowed prompt categories, but the original approval-aware exit workflow is still richer.

## Name And Description

- Name parity: exact.
- Description parity: Both versions describe the tool around the same core capability: Exits plan mode and hands control back to execution mode. Wording differences are minor unless called out under key gaps.

## Parameters And Types

- Type signature: allowedPrompts?: Array<{tool: "Bash", prompt: string}>.
- Parameter parity: top-level parameter names match the original audit; ordering differences are non-semantic.

## Implementation Overview

- Core capability: Exits plan mode and hands control back to execution mode.
- Typical scenarios: Use it when planning is complete and the session should transition from planning to execution.
- Core pain point addressed: It addresses the handoff boundary: planning and execution should not blur together without an explicit transition.
- Main challenges: The hard parts are approvals, request IDs, leader/teammate handoff, and permission orchestration around the exit step.
- Strategy consistency: Partially consistent. Both versions expose the same exit surface, and Go now preserves local plan-state and allowed-prompt metadata more faithfully, but the original still carries much richer approval orchestration.

## Visual Flow And Decision Map

### How To Read The Diagram

- Read the blue path first to understand the shared happy path.
- Read the yellow diamonds next; they are the branch conditions that decide which path the tool takes.
- Read the red boxes last; they mark exact places where Go diverges from `../src`, or where a path is intentionally out of scope.

```mermaid
flowchart TD
    A(["Start"])
    B{"Currently in plan mode?"}
    C(["Return not-in-plan-mode result"])
    D["Load recorded plan file / content"]
    E{"Leader approval required?"}
    F["Request approval and wait"]
    G{"Approved?"}
    H(["Return rejection or stay in plan mode"])
    I["Restore pre-plan state"]
    J(["Return approved plan output"])
    X1["Go has no permission dialog or teammate approval branch"]
    X2["Go simply exits PlanState and echoes the stored plan"]
    A --> B
    B -- "No" --> C
    B -- "Yes" --> D
    D --> E
    E -- "Yes" --> F
    E -- "No" --> I
    F --> G
    G -- "No" --> H
    G -- "Yes" --> I
    I --> J
    E -.-> X1
    I -.-> X2
    classDef start fill:#e8f4fd,stroke:#1d4ed8,color:#0f172a;
    classDef step fill:#eff6ff,stroke:#60a5fa,color:#0f172a;
    classDef decision fill:#fef3c7,stroke:#d97706,color:#7c2d12;
    classDef gap fill:#fee2e2,stroke:#dc2626,color:#7f1d1d;
    classDef result fill:#dcfce7,stroke:#16a34a,color:#14532d;
    class A start;
    class D,F,I step;
    class B,E,G decision;
    class X1,X2 gap;
    class C,H,J result;
```

### Decision Points

- `Currently in plan mode?` decides whether this is a valid state transition or a no-op / error.
- `Leader approval required?` is where the original branches into teammate mailbox approval flow instead of a purely local exit.
- `Approved?` controls whether the runtime actually restores state and exits plan mode.

### Flow-Divergence Hotspots

- The original exit flow is approval-aware and can involve team lead messaging.
- Go currently skips that orchestration and behaves like a local state flip plus plan-file readback.


## Output And Format

- Output comparison: The original returns a structured result integrated into its approval workflow; Go returns a plain-text plan summary that now also includes allowed-prompt guidance.

## Key Gaps

- Leader approval, teammate handoff, and full permission orchestration remain the main gaps.
