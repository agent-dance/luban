# Information-display verification matrix

Verification date: 2026-07-15  
Implementation base: `7c6eea019e6f31449149770e986face7edd4d9ec`; verified branch HEAD: `12a30d0`, branch `cl/tui`, dirty shared worktree
Native runtime: macOS/Darwin arm64, Go 1.26.1

This file records observed gates for the implemented behavioral alignment. A cross-build proves compile/link compatibility, not native interaction. A manual surface not available in this session is marked **Not tested**, never inferred from another row.

## Screenshot-driven remediation gates

| Gate | Result | Evidence |
| --- | --- | --- |
| Tool-segment contract and real event sequence | Pass | `tui/transcript_tool_segment_test.go`: heterogeneous calls, visible-text/session/actor/work-unit boundaries, invisible internal turns, reverse results, D0-hidden members, abnormal defaults, compact expanded rows, group toggles, and complete viewport segments. |
| 40/80/120-column grouped viewport | Pass | `TestTranscriptToolSegmentRealEventSequenceRendersOneMixedContainer`; one group header remains visible and settled member commands remain collapsed. |
| Read-only versus mutating network shell display | Pass | `tui/shell_display_regression_test.go`; literal GET/HEAD weather reads fold while POST/data/upload/local-output/dynamic/remote-exec shapes remain conservative. |
| ANSI/control and terminal-canvas containment | Pass | Controls are removed before visible-width bounding; weather-style box/braille canvases are omitted from default detail while retained evidence is asserted; plain colored prose remains as a sanitized preview. |
| Opaque root/background inheritance | Pass | `TestRootRenderFillsOpaqueBackground` and nested `pkg/go-tui/element_render_background_test.go`. |
| Focused TUI + i18n tests | Pass | `/opt/homebrew/bin/go test ./tui ./i18n -count=1`. |
| Focused TUI + i18n race | Pass | `/opt/homebrew/bin/go test -race ./tui ./i18n -count=1`. |
| Bundled go-tui full + race | Pass | Nested-module `/opt/homebrew/bin/go test ./... -count=1` and `-race ./... -count=1`. |
| macOS PTY | Pass | `/usr/bin/script -q /dev/null ... go test ./tui -run <remediation tests> -count=1`. |
| tmux 80x24 | Pass | Detached `TERM=xterm-256color` pane ran the remediation test set; pane output reported `ok`. |
| Full root repository after remediation | Not green (unrelated shared changes) | Repeated runs passed task-owned packages but failed concurrently edited exact-string, deep-clone, session-switcher, registry-publication, and tool-runtime tests outside task 07. No task-07 failure appeared. |

## Automated quality gates

| Gate | Result | Evidence |
| --- | --- | --- |
| `/opt/homebrew/bin/go test ./... -count=1` | Pass | Final stable-snapshot run passed all packages; `tools` completed in 71.186s. |
| `/opt/homebrew/bin/go test -race ./... -count=1` | Pass | Final full repository race run passed; `tools` completed in 92.835s, with `tui`, `ui`, `session`, and root also green. |
| `/opt/homebrew/bin/go vet ./...` | Pass | Exit 0, no diagnostics. |
| `/opt/homebrew/bin/go build ./...` | Pass | Exit 0. |
| `git diff --check` | Pass | Exit 0. |
| `gofmt` on changed implementation Go files | Pass | Formatter produced no subsequent differences in those files. |
| JSON task validation | Pass | All `implementation/*.json` parse with `jq`. |

## Platform builds

| Target | Gate | Result | Interpretation |
| --- | --- | --- | --- |
| Native Darwin/arm64 | `go build ./...` | Pass | Native compile/link. |
| Linux/amd64 | `GOOS=linux GOARCH=amd64 go build ./...` | Pass | Cross compile/link only. |
| Windows/amd64 | `GOOS=windows GOARCH=amd64 go build ./...` | Pass | Cross compile/link only. |
| Darwin/amd64 | `GOOS=darwin GOARCH=amd64 go build ./...` | Pass | Cross compile/link only. |

## Terminal and accessibility matrix

| Surface or condition | Result | Evidence and scope |
| --- | --- | --- |
| 40x12, 80x24, 120x40 | Pass | `TestDecisionLayoutMatrixAndLinearSemantics`, `TestFullRootViewportMatrixKeepsModeDecisionAndInputVisible`. |
| 40-column Activity view | Pass | Progress, state, action, and reason remain textual in `tui/information_accessibility_test.go`. |
| CJK and long paths | Pass | Decision, status, transcript, and viewport tests. |
| Emoji, ZWJ family/technologist sequences, combining marks, ambiguous-width glyphs | Pass | `TestDynamicResizePreservesEmojiCombiningAndAmbiguousWidthSemantics`. |
| Dynamic 120 -> 40 -> 80 -> 40 resize | Pass | Same test reuses one root and preserves decision semantics and input draft. |
| No-color/no-animation meaning | Pass | State and attention are asserted as text; screen reader omits spinner/tick noise. |
| Screen reader append-only and control-character safe | Pass | `ui/screen_reader_renderer_test.go`, `ui/tool_presentation_test.go`, `repl_screen_reader_lifecycle_test.go`. |
| Active-language adapter projection | Pass | Aggregate summaries, tool actions, and typed command receipts use centrally registered six-language semantic keys; Chinese integration and catalog completeness tests passed. |
| macOS PTY | Pass | `/usr/bin/script -q /dev/null /opt/homebrew/bin/go test ./tui -count=1`; final run passed in 1.836s. |
| tmux pane | Pass | 80x24 detached pane ran decision/root/Unicode resize matrix with `TERM=xterm-256color`; final run passed in 0.311s. |
| Classic semantic aggregation | Pass | `render_presentation_aggregate_test.go`; failures remain immediate. |
| JSON raw-event projection | Pass | Every routine call/result remains present; visual grouping is bypassed. |
| 100,000-member aggregation | Pass | Linear insertion threshold and stable member/evidence index test. |
| 100,000 activity updates | Pass | Stable insertion-order history, same-sequence advancement, unread state, and adaptive persistence cadence are covered. |
| Six-agent mixed-state deterministic rendering | Pass (simulated) | Activity tests cover needs-input, ready, failed/timed-out, running, completed, lineage, and latest-run selection. |
| Agent restart and incomplete outcomes | Pass | Runtime tests cover post-start owner-death reconciliation, precise partial/timeout/cancel/interruption, resume lineage, and redacted verification transcript anchors. |

## Not tested

- Native Linux PTY/tmux interaction.
- Native Windows Console/ConPTY interaction.
- IDE-integrated terminal behavior.
- Fullscreen application interaction as a manual end-to-end session; component and viewport behavior is automated.
- VoiceOver and NVDA speech output, rotor/navigation, and keyboard-only manual certification.
- A live six-agent model/provider soak; the six-state scenario is deterministic and simulated in tests.
- Slow-terminal/backpressure and multi-hour live-tail + pinned-evidence soak.
- Release rollback drill in a deployed binary.

These gaps limit the claim to **implemented and automatically verified behavioral alignment**. They do not invalidate the data model or automated semantics, but they remain prerequisites for a production accessibility/platform certification claim.
