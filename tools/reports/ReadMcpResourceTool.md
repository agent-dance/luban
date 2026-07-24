# ReadMcpResourceTool Parity Report

- Original: `src/tools/ReadMcpResourceTool/ReadMcpResourceTool.ts`
- Go: `tools/mcp_tools.go`

## Verdict

- Summary: Exact surface; the active MCP single-resource read path is close between the two implementations.

## Name And Description

- Name parity: exact.
- Description parity: Both versions describe the tool around the same core capability: Reads one MCP resource from a named server. Wording differences are minor unless called out under key gaps.

## Parameters And Types

- Type signature: server: string, uri: string.
- Parameter parity: top-level parameter names match the original audit; ordering differences are non-semantic.

## Implementation Overview

- Core capability: Reads one MCP resource from a named server.
- Typical scenarios: Use it when the model already knows the target resource and needs its content, not just discovery metadata.
- Core pain point addressed: It solves access to external MCP-managed resources without forcing the model to know each backend protocol directly.
- Main challenges: The hard parts are server discovery, connection management, and normalizing remote resource results.
- Strategy consistency: Largely consistent. Both versions use MCP as the abstraction boundary and then expose a model-facing tool on top.

## Visual Flow And Decision Map

### How To Read The Diagram

- Read the blue path first to understand the shared happy path.
- Read the yellow diamonds next; they are the branch conditions that decide which path the tool takes.
- Read the red boxes last; they mark exact places where Go diverges from `../src`, or where a path is intentionally out of scope.

```mermaid
flowchart TD
    A(["Start"])
    B["Parse server and uri"]
    C{"Required fields present?"}
    D(["Return validation error"])
    E["Connect target MCP server"]
    F{"HTTP fallback path?"}
    G["Read resource through HTTP bridge"]
    H["Read resource through live MCP client"]
    I(["Return resource content"])
    X1["Original wraps the same read in richer runtime result plumbing"]
    A --> B
    B --> C
    C -- "No" --> D
    C -- "Yes" --> E
    E --> F
    F -- "Yes" --> G
    F -- "No" --> H
    G --> I
    H --> I
    I -.-> X1
    classDef start fill:#e8f4fd,stroke:#1d4ed8,color:#0f172a;
    classDef step fill:#eff6ff,stroke:#60a5fa,color:#0f172a;
    classDef decision fill:#fef3c7,stroke:#d97706,color:#7c2d12;
    classDef gap fill:#fee2e2,stroke:#dc2626,color:#7f1d1d;
    classDef result fill:#dcfce7,stroke:#16a34a,color:#14532d;
    class A start;
    class B,E,G,H step;
    class C,F decision;
    class X1 gap;
    class D,I result;
```

### Decision Points

- `Required fields present?` rejects requests that do not identify both a server and a concrete resource URI.
- `HTTP fallback path?` decides whether the read goes through an HTTP bridge or a live MCP protocol client.

### Flow-Divergence Hotspots

- The active resource read path is already close between Go and the original.
- The remaining difference is mostly in surrounding runtime result plumbing and metadata, not in the core fetch itself.


## Output And Format

- Output comparison: The original returns a richer runtime result; Go returns plain-text resource content.

## Key Gaps

- The remaining gap is mostly surrounding runtime integration rather than the core read action.
