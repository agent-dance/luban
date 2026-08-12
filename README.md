# LUBAN Code

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [Deutsch](README.de.md)

LUBAN Code is a Go-native coding agent for long repository jobs. It keeps the session record intact while shrinking only the model-facing view, and it does not let a proxy URL silently change a provider's native protocol.

> Source preview `v0.1.0`. Build it from source; release binaries and package-manager installs are not published yet.

![LUBAN Code TUI running with the OpenAI gpt-5-6 model](docs/assets/screenshots/luban-tui.png)

_The actual TUI, captured from the current Windows source build on 2026-08-12. No API key or endpoint address is shown._

## What is different here

### Compact the provider view, not the evidence

Long sessions usually force a bad choice: keep every old tool result in the prompt, or replace history with a summary that is hard to audit. LUBAN keeps the original transcript. Under a narrow, reviewed production policy, it can replace older `Inspect` results only in the provider view with a deterministic projection that retains paths, line ranges, pagination, digests and proof.

The projection is admitted only when a whole-request estimate says the token saving still pays after cold-cache and recovery costs. Unknown price, incomplete usage data, failed evidence or insufficient savings means no projection. A bad projection rolls back; three consecutive anomalies trip a session circuit breaker.

The current production scope is deliberately small: `openai/gpt-5.6-sol*` and `deepseek/deepseek-v4-flash*`, for `Inspect` only. The [design note](docs/design/progressive-context-compaction.md) and [80k paired-run evidence](benchmark-results/progressive-context-compaction-v7-80k-2026-08-10/README.md) spell out both the mechanism and its limits.

### A proxy changes the route, not the provider

`BaseURL` is transport configuration. It does not turn native OpenAI, DeepSeek, Anthropic, Vertex or Bedrock traffic into a generic OpenAI-compatible dialect. Authentication, cache controls, Responses versus Chat semantics and provider-specific request fields remain tied to the selected provider.

Automatic Responses-to-Chat negotiation exists only for an explicitly compatible provider. The current implementation treats `404`, `405` and `501` as endpoint-unavailable signals; authentication, rate-limit and schema failures return as errors and do not trigger a protocol fallback.

### A small coding kernel with operational depth

In the default production configuration, the model-facing coding kernel is `Inspect`, `ApplyPatch` and `Run`; enabling the `ContextUpdate` shadow path adds that internal tool. Around the kernel, the runtime adds resumable sessions, parallel subagents, optional Git worktrees, permission challenges, lifecycle hooks, MCP connections and an NDJSON/Go SDK boundary. Subagents receive an immutable permission snapshot at launch, so a later parent-session permission change cannot widen a running child's authority.

The terminal UI reports context, cache, cost, compaction and subagent activity. An append-only `--screen-reader` mode avoids cursor control, mouse capture and animation. Runtime copy is catalogued in English, Simplified Chinese, German, Japanese, Korean and Russian; switch it with `Ctrl+L` or `/language`.

## Measured, with the receipts attached

In the frozen 15-task local comparison, the selected LUBAN runs used less elapsed time, fewer tokens and fewer model calls than the selected Codex runs:

| Observed total | LUBAN | Codex | Difference |
| --- | ---: | ---: | ---: |
| Elapsed time | 4,020.6 s | 5,644.5 s | -28.8% |
| Tokens | 6,857,490 | 17,889,019 | -61.7% |
| LLM calls | 245 | 354 | -30.8% |
| Produced a patch | 15/15 | 15/15 | equal |

This is a frozen local sample, not a general win claim. Only the original five tasks had official grader results, and both agents resolved 3/5; the other ten tasks were not graded. Run selection happened after optimization and the model runs had no fixed seed. Read the [full HTML report](benchmark-results/agentic-2026-07-27/representative15-report.html), [selected machine data](benchmark-results/agentic-2026-07-27/raw/candidates/selected-15task-20260731.json) and [protocol](benchmark/agentic/README.md) before drawing a broader conclusion.

The progressive-compaction experiment is similarly scoped. One 80k paired run kept the frozen evaluator equal at `2/2 + 455/455` while total tokens fell from `1,362,070` to `444,419` and estimated cost from `$5.207999` to `$1.004185`. The two traces diverged before the first projection because sampling was not fixed. These are measurements from two real traces and fixed-rate cost estimates, not a causal average of the projection's effect.

## Build from source

You need Git and the Go version declared by [`go.mod`](go.mod), currently `1.26.1`. Shell-form `Run` steps invoke Bash; on Windows, put Git Bash, WSL Bash or another `bash` executable on `PATH`.

macOS or Linux:

```sh
git clone https://github.com/agent-dance/luban.git
cd luban
go build -o luban-code ./cmd/luban-code
./luban-code --version
```

Windows PowerShell:

```powershell
git clone https://github.com/agent-dance/luban.git
Set-Location luban
go build -o .\luban-code.exe .\cmd\luban-code
.\luban-code.exe --version
```

The module currently contains a local `replace`, so `go install github.com/agent-dance/luban/cmd/luban-code@latest` is not a supported installation path.

## Connect a provider and run

You can configure credentials with environment variables. Select the provider explicitly when several credentials are present:

```sh
export PROVIDER=openai
export OPENAI_API_KEY="..."
./luban-code
```

```powershell
$env:PROVIDER = "openai"
$env:OPENAI_API_KEY = "..."
.\luban-code.exe
```

DeepSeek works with `PROVIDER=deepseek` and `DEEPSEEK_API_KEY`; it is also the default provider. Ollama defaults to `http://localhost:11434/v1` and model `llama3.1`. You can instead start the TUI and press `Alt+P` to choose a provider, model and supported authentication method.

Run one prompt without opening the TUI:

```sh
./luban-code -p "Review this repository and report the highest-risk issue"
```

![A verified LUBAN Code v0.1.0 one-shot run returning LUBAN READY](docs/assets/screenshots/luban-live-run.png)

_The second command made a real request through the locally configured OpenAI endpoint and exited with code 0. The local prompt path is redacted. This is a runtime check, not a provider-compatibility or performance benchmark._

Inside the TUI, `/init` can add `LUBAN.md` and project settings without overwriting existing files. It does not configure credentials.

## Know the boundaries before using it

- Linux OS sandboxing requires Bubblewrap. macOS uses `sandbox-exec`. Windows has no OS sandbox backend today; `--force-sandbox-tools` fails closed when a verified backend is unavailable.
- Agent Teams are experimental and opt-in. Parallel subagents and worktree isolation should not be read as a remote distributed swarm.
- Provider registration and protocol tests are not certification for every model or third-party gateway.
- Local credentials are plaintext JSON. Unix-like systems write them with mode `0600`; Windows currently has no equivalent ACL guarantee. They are not an encrypted vault or operating-system keychain.
- Node.js is needed only for Node-based MCP servers, not for the core CLI.
- There is no root license yet. Standard copyright applies until the owner publishes one.

## Repository evidence

- [Progressive context design](docs/design/progressive-context-compaction.md)
- [Progressive rollout report](docs/reports/progressive-context-compaction-rollout-2026-08-11.md)
- [15-task benchmark report](benchmark-results/agentic-2026-07-27/representative15-report.html)
- [Agentic benchmark protocol](benchmark/agentic/README.md)

Contributions should follow [CONTRIBUTING.md](CONTRIBUTING.md). Security reports should follow [SECURITY.md](SECURITY.md).
The five-language editorial and runtime checks are recorded in the [README release review](docs/release/readme-review-2026-08-12.md).

Send security-sensitive findings through GitHub's [private vulnerability report](https://github.com/agent-dance/luban/security/advisories/new), not a public issue. See [SECURITY.md](SECURITY.md) for the requested details.
