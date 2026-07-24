# Session view fidelity verification matrix

Status: behavioral exactness complete; production hardening partial  
Native host: Darwin/arm64, Go 1.26.1

| Gate | Current result | Evidence |
| --- | --- | --- |
| Schema convergence | Pass | `TestDurableSessionViewHasOneSchemaOwner` |
| Resume Cell parity | Pass (focused) | Six languages, 40x12/80x24/120x40, rune/style/width comparison |
| Fork Cell parity | Pass (focused) | Local rows, group state, cursor/chrome fields, evidence and activity presentation identity |
| Cursor and long composer viewport | Pass (focused) | Shared cursor State, v3 checkpoint, cursor-following max-height viewport |
| Alt+G/Ctrl+G and mouse group header | Pass (focused) | Preemptive dispatch, whole-row hit target, restored action rectangles |
| Checkpoint integrity/migration | Pass (focused) | Digest, checksum, manifest, fail-closed, v2→v3, dangling evidence |
| Evidence ownership | Pass (focused) | Fork remains readable after source artifact deletion |
| Settled event-loop capture | Pass (focused) | Queue drain, final stream/usage, atomic capture, out-of-order capture rejection |
| Actual repeated `/resume` | Pass (focused) | Target receives no navigation receipt |
| Full root tests | Pass | `go test ./... -count=1`; `tools` 104.445s, `tui` 6.907s |
| Full race | Pass | `go test -race ./... -count=1`; `tools` 117.807s, `tui` 11.653s |
| Vet/build/diff | Pass | `go vet ./...`, `go build ./...`, `git diff --check` |
| Nested `pkg/go-tui` full/race | Pass | All nested packages passed |
| Linux/Windows/Darwin amd64 cross-build | Pass | Compile/link only |
| macOS PTY | Pass | Focused checkpoint, fork, group-control, and contract tests under `/usr/bin/script` |
| tmux 80x24 | Pass | Detached pane reported `ok` and `__EXIT__:0` |
| visual-verdict | Pass, 100/100 | Live/resume/fork PNG hashes identical; pixel difference 0; threshold 90 |

## Not tested

- Native Linux PTY.
- Native Windows ConPTY.
- IDE terminal, VoiceOver, and NVDA manual certification.
- Two-process concurrent editing of the same session.
- Power-loss injection across transcript/checkpoint publication.
