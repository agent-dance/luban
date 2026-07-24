# Terminal-Native Transcript and Transitional Basic Mouse Reporting

## Status

Revised decision recorded; implementation planning and real-terminal
validation are pending.

The long-term default is a terminal-native transcript architecture. The normal
conversation runs on the terminal's primary screen without mouse capture.
Completed content is committed to terminal scrollback, while only the live
tail, transient status, input composer, and modal surfaces remain dynamically
rendered.

The earlier Basic mouse-reporting design remains documented as a bounded
transition option. It is not the target architecture and will not be exposed as
a peer user experience. Normal use must not require a mouse-mode setting,
terminal preference, or selection modifier.

## Problem and Product Requirements

The fullscreen TUI currently enables xterm normal tracking, button-motion
tracking, and SGR coordinates (`?1000h`, `?1002h`, and `?1006h`).
Button-motion tracking sends left-button drags to the application, so the
terminal cannot use the same gesture for native text selection.

This creates the observed failures:

- application scrolling and clickable transcript controls work, but native
  selection requires a terminal-specific bypass modifier;
- iTerm2 can warn that mouse reporting prevented a selection when the user
  drags and then invokes the terminal copy action;
- disabling mouse reporting in the current fullscreen design restores
  selection but removes the application's wheel events;
- the removed application-level transcript selection path could paint reverse
  styling over blank code-block cells and produce a black rectangle;
- returning `false` from `RootComponent.HandleMouse` cannot return a mouse
  event to the terminal after the terminal has already reported it.

The product requires one normal interaction model that provides:

- unmodified terminal-native selection;
- native wheel and trackpad scrolling through finalized conversation history;
- no iTerm2 mouse-reporting warning;
- no application-owned transcript selection overlay or black code-block
  artifact;
- native double-click word selection, triple-click line selection, drag
  auto-scroll, terminal search, and selection across scrollback where the
  terminal supports them;
- complete keyboard access to every application action;
- no required user configuration or terminal-specific mode selection.

The requirement does not include preserving pointer activation in the main
transcript. Mouse reporting is terminal-global; a portable protocol cannot
capture only a tool header click while guaranteeing that the same unmodified
left-button gesture remains entirely terminal-owned everywhere.

## Research Evidence

### Current Project

The current application explicitly starts fullscreen with `tui.WithMouse()` in
`tui/app.go`. The bundled terminal emits `1000 + 1002 + 1006` from
`pkg/go-tui/escape.go`, and exact lifecycle behavior is covered in
`pkg/go-tui/terminal_hygiene_unix_test.go`.

The application currently uses mouse input only for transcript wheel scrolling
and unmodified tool-segment header activation in `tui/root.go`. It does not
need transcript drag motion. The transcript, live stream, status, dialogs, and
composer are nevertheless rendered together in one fullscreen framebuffer.

The bundled framework already contains most terminal primitives needed for a
different architecture:

- `WithInlineHeight` and primary-screen inline startup in
  `pkg/go-tui/app_options.go` and `pkg/go-tui/app_inline_startup.go`;
- `PrintAboveElement` and `QueuePrintAboveElement` for committing rendered
  content to scrollback in `pkg/go-tui/app_lifecycle.go`;
- `StreamAbove` for coordinated partial-line streaming into inline history;
- dynamic inline-to-alternate-screen transitions in
  `pkg/go-tui/app_screen.go`;
- inline-aware suspend, resume, external-process handoff, and terminal cleanup;
- `AppState.Messages` as the retained semantic transcript.

These primitives reduce framework work, but they do not make the application
migration a configuration change. `AppState.Messages` is mutable:

- tool results update earlier transcript anchors;
- streaming messages are re-rendered when finalized;
- brief output can replace earlier assistant content;
- session restore, switch, fork, and clear can replace the entire message set;
- tool grouping, disclosure, and detail projections can change after initial
  insertion.

The design therefore needs a semantic settlement boundary and an idempotent
commit ledger. It must not simply print each new slice index with
`PrintAboveElement`.

### Terminal Protocol Boundary

iTerm2 documents `1000` as press/release tracking and `1002` as the level that
also reports motion while a button is held:
<https://iterm2.com/feature-reporting/#MOUSE>.

Basic reporting without `1002` can improve native dragging on some terminals,
including iTerm2:
<https://github.com/gnachman/iTerm2/blob/2138f28e2ee40fcc035316f9da9549539bb8cfbc/sources/TerminalView/PTYMouseHandler.m#L729-L741>.

However, the xterm mouse protocol specifies which events are reported to the
application. It does not guarantee that every terminal, IDE host, or
multiplexer will turn unreported drag motion into a native selection. `1000`
also still reports presses, releases, and wheel events, so double-click,
triple-click, right-click, and terminal scrolling remain terminal-dependent.

There is no standard region-scoped mode that reports clicks only over
application-declared cells. This is the hard limit behind the decision.

### OpenCode and OpenTUI

OpenCode's fullscreen TUI uses application-managed selection and has received
the same iTerm2 warning report:
<https://github.com/anomalyco/opencode/issues/24046>.

OpenTUI has proposed a Basic tracking level that omits `1002`:
<https://github.com/anomalyco/opentui/pull/1039>. That proposal supports the
short-term Basic option, but it is not a portability guarantee and does not
solve the full click/scroll/selection conflict.

OpenCode's split-footer approach disables mouse reporting and writes immutable
output into terminal scrollback. It demonstrates the correct ownership
boundary, but its implementation is not a drop-in dependency for this project.

### Codex Reference Architecture

The inspected Codex source is commit
`315195492c80fdade38e917c18f9584efd599304`, retrieved on 2026-07-17.

Codex does not solve the conflict with a more elaborate mouse event router. It
does not enable mouse capture in its normal terminal modes:
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/tui.rs#L175-L190>.

Its event layer explicitly ignores mouse events:
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/tui/event_stream.rs#L236-L268>.

The main UI initializes as an inline viewport on the primary screen:
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/tui.rs#L380-L464>.

Finalized history is inserted above that viewport into terminal scrollback:
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/insert_history.rs#L193-L255>.

Temporary fullscreen views enter the alternate screen and enable `DECSET 1007`
so supporting terminals translate wheel gestures into cursor keys. They do not
enable mouse capture, and `1007` is symmetrically disabled on exit:
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/tui.rs#L193-L220>,
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/tui.rs#L734-L769>.

Codex retains semantic history and can clear and replay scrollback after
resize. This is a source-backed hybrid, not a write-only log:
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/app/resize_reflow.rs#L1-L15>,
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/app/resize_reflow.rs#L300-L433>.

It separates visible terminal selection from semantic copy. Whole-response
copy reads retained raw Markdown rather than selected screen cells:
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/chatwidget/interaction.rs#L253-L280>.

It also deliberately ignores syntax-theme background colors:
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/render/highlight.rs#L494-L516>.

Ordinary Codex fenced code blocks are emitted as source lines without a
header, border, line-number gutter, or application wrap:
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/markdown_render.rs#L820-L865>,
<https://github.com/openai/codex/blob/315195492c80fdade38e917c18f9584efd599304/codex-rs/tui/src/markdown_render.rs#L1808-L1835>.

### Codex Decision History

Codex's history is stronger evidence than its current escape sequences alone:

1. Early Codex enabled global mouse capture for scrolling, then documented the
   same iTerm drag-selection conflict and offered a modifier and mouse toggle
   as workarounds:
   <https://github.com/openai/codex/commit/dc7b83666ac32b4c0153924039a099e0a783e93f>,
   <https://github.com/openai/codex/commit/7ca84087e69163c187e2161a1d6ef2007149e8af>.
2. The project then moved to an append-only log and explicitly let the terminal
   handle scrollback and text selection:
   <https://github.com/openai/codex/issues/1247#issuecomment-3146072996>,
   <https://github.com/openai/codex/commit/480e82b00daaf038afdd2e3304ee3b801f3661cf>.
3. Codex later implemented an experimental fullscreen, transcript-owned TUI
   with application scrolling, mouse selection, and semantic copy:
   <https://github.com/openai/codex/commit/b093565bfb5b2c016cb157127edb1ad62bfc7a27>.
4. After about a month of real-world experimentation, maintainers reported
   better deterministic resize and code-copy behavior but too many regressions
   across terminals, operating systems, tmux, input methods, mouse and
   trackpad behavior, keyboard layouts, and alternate-screen handling. They
   removed the experiment and returned to terminal-native scrolling,
   selection, and copy:
   <https://github.com/openai/codex/issues/8344#issuecomment-3782449267>,
   <https://github.com/openai/codex/commit/a489b64cb59b2d603a0b1c918b9716f41e0a741d>.
5. The current hybrid adds source-backed resize replay and an optional
   copy-friendly raw representation without reintroducing application mouse
   selection:
   <https://github.com/openai/codex/commit/5591912f0bf176257f71b3efbd37ee4479dfdfaf>,
   <https://github.com/openai/codex/commit/5e0a4adbe564cff56edc6d3a844181ce1df7794b>.

This does not prove that a fullscreen application-managed TUI is impossible.
It shows that terminal-native ownership has the strongest evidence for the
required default experience and the smallest compatibility surface.

## Considered Architectures

| Architecture | Native selection | Wheel behavior | Application click | Portability | Cost |
|---|---|---|---|---|---|
| Current `1000 + 1002 + 1006` fullscreen | Modifier-dependent | Application-owned | Preserved | Known iTerm warning | Already implemented |
| Basic `1000 + 1006` fullscreen | Terminal-dependent unreported drag | Application-owned | Partly preserved | Requires terminal matrix | About 4.5-7 days |
| Application-managed semantic selection | Reimplemented by application | Application-owned | Preserved | Large Unicode, theme, and terminal surface | About 3-4 weeks minimum |
| Terminal-native transcript | Terminal-owned | Terminal scrollback; `1007` in overlays | Main transcript click removed | Smallest application mouse surface | About 3-6 weeks |

The terminal-native transcript is the only architecture that removes the
mouse-reporting conflict rather than trying to route around it.

## Decision

Use one user-facing default: **terminal-native transcript with a keyboard-first
dynamic footer**.

The normal conversation surface will:

- run on the primary terminal screen;
- not enable `1000`, `1002`, `1003`, `1006`, or another mouse-reporting mode;
- commit settled transcript content into native terminal scrollback;
- retain only unsettled content, transient status, the composer, and active
  decision UI in a bounded dynamic viewport;
- leave wheel, trackpad, drag, double-click, triple-click, right-click, terminal
  search, and terminal copy gestures to the terminal;
- expose every application action through the keyboard.

Temporary fullscreen transcript, detail, diff, settings, picker, and similar
views may use the alternate screen. They may enable only `1007` for
wheel-to-arrow translation and must always provide authoritative keyboard
navigation.

The application will not expose terminal-native and Basic as peer modes. Basic
may be implemented only as a short-lived delivery bridge if product scheduling
requires it. Work that exists solely for Basic must not constrain the
terminal-native architecture.

The existing `/mouse off` command may remain during migration as a diagnostic
or terminal-recovery path. Once the terminal-native architecture is the
default, mouse reporting is already off and the command is no longer part of
the primary interaction contract.

## Terminal-Native Architecture

### Semantic Source of Truth

`AppState.Messages` and the observation/detail stores remain the source of
truth. Terminal scrollback is a rendered projection, not authoritative state.
Session restore, export, semantic copy, detail views, and exceptional replay
read retained semantic state.

The application introduces a settlement state machine and commit ledger keyed
by stable session, epoch, turn, message, observation, or group identity. A
slice index is not a stable commit key.

The ledger guarantees:

- a settled unit is inserted exactly once;
- units are inserted in semantic reading order;
- a late event cannot silently rewrite an already committed row;
- session replacement and replay cannot duplicate prior output;
- failures before a completed terminal write do not advance the commit cursor.

### Settled and Live Content

Content remains live while its visible representation may still change.
Examples include:

- streaming assistant Markdown before finalization;
- an active tool call before its stable summary and outcome are known;
- progress, spinner, elapsed-time, permission, and retry state;
- a tool group whose final membership or aggregate outcome is unsettled.

Once settled, a user message, assistant response, tool summary, or stable status
unit is rendered once and inserted into scrollback. The same unit is removed
from the live viewport in the same synchronized update so it never appears
twice.

If a genuinely late state transition arrives after settlement, it is appended
as a new correction or follow-up unit. Ordinary operation does not mutate
scrollback in place.

### Primary-Screen Inline Viewport

The inline viewport owns only the bottom rows needed for:

- the active streaming tail;
- current tool or progress state;
- the active permission or decision prompt;
- the status line and input composer.

Its height may change as the composer or live tail grows. It must not repaint
rows already committed above it. Finalized history has no application scroll
offset, sticky-bottom flag, wheel step size, or application scrollbar.

The framework's existing `WithInlineHeight`, `PrintAboveElement`, and inline
session implementation are reused. `StreamAbove` is reused only for content
whose incremental bytes are already safe to make permanent; live Markdown
that can be restructured at finalization remains in the dynamic footer. The
application root must be split into a static history publisher and that footer.

### Alternate-Screen Views

Semantic interactions that require retained state rather than terminal
scrollback are shown in temporary keyboard-driven views:

- full transcript navigation and application search results;
- tool groups, observation details, exact evidence, and large output;
- diffs, settings, model or session pickers, and other modal surfaces.

Entering such a view:

1. preserves the primary-screen inline layout and composer draft;
2. enters the alternate screen;
3. enables `1007` when supported;
4. renders a full-size keyboard-navigable view.

Leaving the view:

1. disables `1007`;
2. exits the alternate screen;
3. restores inline geometry, draft, focus, and primary scrollback;
4. schedules a full repaint of only the owned dynamic viewport.

No alternate-screen view enables mouse capture. Lack of `1007` support removes
wheel convenience, not functionality; cursor keys, paging keys, Home/End, and
view-specific bindings remain authoritative.

### Rendering and Code Blocks

The main transcript uses a copy-friendly rich representation rather than a
second user-selectable Raw mode.

The rendering contract is:

- no transcript selection `AttrReverse`, selection background, blank-cell
  overlay, or copy-on-release path; composer input selection is a separate
  editable-control concern and may retain its model-owned highlight;
- code syntax highlighting changes foreground and necessary text attributes
  only, and discards every token background rather than only the theme's
  default background;
- code content does not install a terminal-default or opaque panel background;
- the main transcript avoids decorative code gutters and line numbers that
  pollute partial native copy;
- an optional language label is separate from the selectable code body;
- ordinary code lines are not application-wrapped, truncated, or replaced with
  ellipses; source newlines are preserved and physical wrapping belongs to the
  terminal;
- every rendered row finishes with balanced SGR state so foreground,
  background, bold, and reverse attributes cannot leak;
- links use sanitized OSC 8 metadata when available and retain a visible,
  copyable destination where a label would otherwise hide it;
- raw external tool output is sanitized at the rendering boundary without
  translating protocol identifiers, paths, or output content.

This policy removes both the application selection mechanism that produced the
isolated black rectangle and the code-theme background mechanism that can
produce larger black panels on a non-default application theme.

Unified diffs are a separate semantic surface. They may use gutters or
addition/deletion backgrounds only under their own theme, capability, and copy
contract; that exception does not weaken the ordinary fenced-code rules.

### Copy Contracts

The single mode provides two operations, not two mouse modes:

- **visible copy** is the terminal's normal copy of terminal-owned selected
  cells. The application does not intercept the shortcut, replace the selected
  text, copy on release, or emit OSC52 for a terminal selection.
- **semantic copy** is the existing whole-assistant-message, input-selection,
  and export behavior. It reads clean logical content from retained application
  state and remains independent of terminal selection.

Visible copy may include rendered prefixes, wrapping, truncation markers, or
tool summaries. It must never include ANSI control sequences, NUL bytes, hidden
buffer cells, or an application-triggered clipboard replacement.

Semantic whole-message copy must not depend on what is currently visible or
selected. Exact semantic extraction for an arbitrary mouse-selected subset is
not part of this design.

### Lifecycle and Terminal Ownership

The framework owns terminal modes as state; components must not write one-off
escape sequences.

Normal startup explicitly disables any legacy mouse modes that this process may
have enabled during migration, then enters inline operation. Shutdown, panic
cleanup, failed startup, suspension, and external-process handoff leave no
`1000`, `1002`, `1003`, `1006`, `1007`, alternate screen, hidden cursor, or
raw-mode residue.

Before an external interactive process or suspend:

1. pause the input reader;
2. close `1007` and leave any alternate screen;
3. restore keyboard, paste, focus, cursor, and raw-mode state;
4. transfer terminal ownership.

After return:

1. re-establish and verify raw mode;
2. restore keyboard, paste, and focus modes;
3. clear stale buffered input while the reader remains paused;
4. restore inline or alternate geometry in a synchronized full redraw;
5. restore `1007` only if an alternate view was active;
6. resume the input reader last.

The existing go-tui lifecycle is reusable and already has stronger panic and
input-generation protection than a simple sequential restore. `1007` must be
added to that same ownership model.

### Resize and Reflow

Resize is the largest architectural risk.

`PrintAboveElement` bakes rows at the current width. Retained semantic state
allows deterministic replay, but terminal protocols cannot selectively delete
only application-owned primary-screen scrollback. A full `ED 3` purge can also
remove shell history that predates application startup and invalidate an active
terminal selection.

The implementation must choose and document one release policy:

- prefer terminal soft wrapping and a copy-friendly logical-line
  representation so ordinary resize needs no application replay; or
- rebuild retained application history only when the supported terminal path
  can preserve the required history boundary; or
- accept a documented full-history purge during exceptional reflow, only after
  product approval and real-terminal validation.

The production core must not copy Codex's full scrollback purge implicitly.
The shell-history effect is a P0 product decision.

Streaming and resize are coordinated so provisional renderings never become
duplicate permanent rows. Rapid resize is debounced, and any replay is
source-backed, order-preserving, idempotent, Unicode-correct, and followed by
restoration of the composer draft and live tail.

## Terminal-Native Experience Contract

### Main Conversation

Finalized conversation history behaves like normal terminal output:

- unmodified drag creates a terminal-native selection;
- double-click, triple-click, right-click, drag auto-scroll, and terminal search
  remain available according to terminal behavior;
- wheel and trackpad gestures use the terminal's speed, inertia, scrollbar, and
  user preferences;
- the terminal copy action does not trigger the iTerm2 mouse-reporting warning;
- selection can extend through native scrollback rather than only the currently
  rendered application viewport;
- finalized rows do not animate, expand, collapse, or repaint during ordinary
  streaming;
- no application preference or selection modifier is required.

If the user scrolls into history while new output arrives, the application does
not force an application scroll offset back to the bottom because no such
offset exists. Whether a terminal itself scrolls on output remains controlled
by that terminal's preferences.

### Live Tail and Composer

The live tail remains selectable because mouse reporting is disabled, but it is
not immutable. Streaming, tool progress, permission changes, composer edits,
resize, or settlement can disturb a selection that overlaps dynamically
redrawn rows.

Selection of finalized scrollback is the stable target. The application must
keep the live region bounded and avoid unnecessary animation or timer-driven
repaint, but it cannot promise durability for a selection that includes live
cells.

### Tool Summaries and Details

Finalized tool rows in the main transcript are static text, not pointer targets.
Clicking does not expand them because the application never receives the click.

Every former pointer action must have a keyboard path before release. The
existing focused-or-latest tool action and detail commands are the basis for:

- opening the focused or most recent tool group in a detail view;
- moving to previous and next tools;
- expanding disclosure levels;
- opening exact evidence or large output;
- returning to the unchanged main transcript.

The design does not require a new hard-coded shortcut or English hint. Any
future user-visible label, status, help text, or compatibility warning must use
a semantic i18n key with complete translations and focused tests.

### Links

Opening a web link remains terminal-owned through OSC 8 or terminal URL
detection. The link gesture may include a terminal-defined modifier and is not
an application mouse event.

Local paths and line references remain visible and copyable. Semantic file
opening keeps a keyboard or external-editor path; pointer opening of local files
is not a portability requirement.

### Application Guarantees

The application guarantees:

- no xterm mouse-reporting mode in the main surface or alternate views;
- alternate views enable at most `1007` with symmetric lifecycle cleanup;
- no transcript click or drag consumption;
- no application-owned transcript selection painting or clipboard replacement;
- settled history is append-only during ordinary streaming;
- every application action has a keyboard-accessible path;
- visible terminal copy and semantic application copy remain separate;
- code-block rendering cannot introduce a selection or theme background
  rectangle.

### Terminal-Dependent Behavior

The application cannot guarantee:

- native selection when a terminal, tmux, Zellij, IDE host, or user preference
  independently captures the mouse;
- exact double-click, triple-click, right-click, auto-scroll, or link gestures;
- wheel-to-arrow translation where `1007` is unsupported;
- preservation of a selection across live redraw, overlay exit, resize, or
  scrollback reconstruction;
- that visible terminal copy equals original Markdown;
- that new output will not move a terminal configured to scroll on output;
- pointer activation of tool summaries, local paths, or alternate-view
  controls.

These are compatibility boundaries, not reasons for the application to
re-enable global mouse reporting.

## Transitional Basic Mouse Reporting

Basic remains a possible delivery bridge while the terminal-native
architecture is under construction. It is not the final product mode.

Conceptually, Basic performs:

~~~text
DECRST 1003  disable all-motion tracking
DECRST 1002  disable button-drag motion tracking
DECSET 1006  enable SGR coordinate encoding
DECSET 1000  enable normal press/release/wheel tracking
~~~

go-tui must own that level across startup, shutdown, suspend/resume, external
processes, and failed cleanup. A one-off `?1002l` write is not acceptable.

Interactive transcript actions fire only after a compatible press/release pair
on the same target. Press on one target and release elsewhere is treated as a
drag and does not activate a tool header.

Basic retains application wheel scrolling and some clicks, but it cannot
promise:

- uniform unmodified drag selection across terminals;
- double-click or triple-click native selection;
- native right-click behavior;
- selection auto-scroll beyond the application viewport;
- terminal-native wheel speed and trackpad inertia;
- stable selection through fullscreen redraw;
- clean semantic copy of visible cells.

Before any Basic implementation, a 0.5-to-1-day protocol spike must prove
unmodified drag, wheel, click-versus-drag, and warning behavior on primary
direct terminals. The full Basic implementation is estimated at 4 to 6 more
engineering days. A modifier-only result fails the spike.

If the terminal-native architecture is selected for immediate implementation,
Basic work is skipped rather than added to the terminal-native estimate.

## Known Experience Losses

The terminal-native decision deliberately exchanges main-transcript pointer
interaction for terminal-native history behavior.

### No Main-Transcript Click Activation

Tool summaries, disclosure headers, and completed message blocks cannot be
clicked to mutate the main transcript. Details move to keyboard-driven
alternate views.

### Immutable Finalized History

Committed rows cannot be expanded, collapsed, animated, or updated in place.
Settlement must wait until a stable summary exists. A later correction is a new
row or requires exceptional source-backed replay.

### Visible Copy Is Not Semantic Copy

Native selection copies displayed cells. Copy-friendly rendering reduces
noise, but arbitrary partial selection cannot automatically become original
Markdown. Whole-message semantic copy and export remain separate operations.

### Dynamic Boundary

Selection that overlaps the live tail, composer, prompt, or alternate view can
be disturbed by redraw. Only finalized primary-screen history is treated as
stable.

### Exceptional Reflow

Resize or session replacement may require replay. A full primary-screen purge
can invalidate the current selection and remove pre-application shell
scrollback. This is not an invisible implementation detail and must pass the P0
product gate described above.

### Multiplexer Ownership

tmux, Zellij, screen, and IDE terminals can independently capture or transform
mouse and scroll behavior. Mouse-on multiplexer configurations may require
their established bypass gesture even though the application enables no mouse
mode.

## Compatibility Risks

The main compatibility risks are no longer whether unreported `1002` drag
becomes a selection. They are:

- whether primary-screen inline history behaves correctly in each terminal;
- whether new output preserves a user's scrolled-back position;
- whether terminal soft wrapping and resize preserve long lines, CJK, emoji,
  combining marks, hyperlinks, and styled code;
- whether `1007` produces useful wheel-to-arrow navigation in alternate views;
- whether inline and alternate geometry survive suspend, resume, external
  editor, signals, failed startup, and clean exit;
- whether session switch, restore, clear, fork, rollback, and replay duplicate
  or omit settled content;
- whether tmux and Zellij mouse-on and mouse-off configurations preserve
  history and leave terminal modes clean;
- whether Windows Terminal/ConPTY, IDE terminals, and SSH paths retain primary
  scrollback;
- whether rich rendered output remains copy-friendly without adding a second
  normal product mode.

No unit test can prove native terminal selection. Real-terminal validation
remains a release gate.

## Delivery Plan and Cost

Estimates are cumulative single-engineer engineering time. Existing inline and
lifecycle primitives reduce framework cost; the settlement model and
application split remain the dominant work.

### Phase 0: Architecture Prototype

**6 to 9 engineering days cumulative.**

The prototype proves:

- primary-screen inline startup with no mouse reporting;
- a minimal settled/live boundary and idempotent commit cursor;
- one normal user/assistant turn;
- streaming finalization without duplicate rows;
- minimal successful and failed tool summaries;
- composer, permission prompt, normal exit, and one detail view;
- iTerm2 and at least one other direct terminal.

It is not a default release. Complex resume/fork, full reflow, Windows, tmux,
Zellij, and the complete message lifecycle remain out of scope.

### Phase 1: Production Core

**13 to 20 engineering days cumulative, approximately 3 to 4 weeks per
engineer.**

This phase adds:

- settlement for every message, tool, thinking, error, compaction, permission,
  and retry lifecycle;
- keyboard-first tool and transcript detail views;
- semantic copy, export, links, and copy-friendly code rendering;
- `1007` as lifecycle-owned terminal state;
- session switch, resume, fork, clear, rollback, and deduplicated replay;
- suspend/resume, external editor, panic, signal, and failed-startup recovery;
- resize policy implementation and the P0 shell-history decision;
- full focused tests, Go suite, vet, formatting, and direct-terminal matrix.

### Phase 2: Compatibility Hardening

**20 to 30 engineering days cumulative, approximately 4 to 6 working weeks.**

This phase covers:

- tmux, Zellij, SSH, IDE terminals, Windows Terminal/ConPTY, and remaining
  direct terminals;
- resize/reflow stress, very long sessions, rapid resize, and replay
  performance;
- CJK, emoji, grapheme clusters, wide characters, ANSI sanitation, and narrow
  windows;
- interrupted streams, late events, suspend races, input-method behavior, and
  crash cleanup;
- scrollbar stability, scroll-on-output behavior, selection, copy, and release
  observation.

If every environment must achieve identical behavior, plan at the top of the
range and retain contingency. Terminal-native reduces the compatibility
surface; it does not eliminate terminal diversity.

### Alternative Costs

- Transitional Basic: 0.5 to 1 day spike plus 4 to 6 implementation days.
- A new render-aware application semantic selection engine: at least 3 to 4
  engineering weeks before comparable real-terminal hardening.

These alternatives are mutually exclusive delivery paths, not increments added
to the terminal-native estimate.

## Verification

### Automated Gates

Terminal and lifecycle:

- primary-screen startup emits no `1000h`, `1002h`, `1003h`, or `1006h`;
- entering an alternate view emits `1007h` but no mouse-capture sequence;
- leaving emits `1007l` and restores inline geometry;
- suspend/resume, external-process handoff, normal exit, failed startup, panic,
  and signals leave no mouse, `1007`, alternate-screen, cursor, keyboard, paste,
  focus, or raw-mode residue;
- runtime migration from the legacy drag level explicitly disables every mode
  previously owned by this process.

Settlement and rendering:

- each settled unit is committed exactly once and in reading order;
- a live unit is removed in the same synchronized cycle in which its finalized
  form is committed;
- tool result, retry, error, compaction, late event, and session replay paths do
  not duplicate or rewrite settled history;
- ordinary streaming never repaints or rebuilds committed scrollback;
- code rendering contains no transcript `AttrReverse`, opaque/default
  background panel, leaked SGR state, ANSI injection, hidden cell, or NUL;
- visible-copy fixtures match rendered logical cells;
- semantic-copy fixtures match retained logical content independently of
  visible layout;
- every former pointer-only action has a keyboard path and focus test.

Resize and replay:

- idle and streaming resize preserve semantic ordering and do not duplicate
  provisional rows;
- the selected resize policy is deterministic for long lines, CJK, emoji,
  combining marks, and hyperlinks;
- any replay restores the composer draft, live tail, cursor, inline geometry,
  and commit ledger;
- a full `ED 3` purge cannot be introduced without an explicit test and the P0
  shell-history product decision.

Repository gates:

- focused go-tui and application tests pass;
- `go test ./...`, formatting, and vet pass for implementation changes;
- `go test ./i18n` passes whenever implementation changes a user-visible
  surface;
- all new user-visible labels, help, status, and compatibility messages use
  semantic i18n keys with complete translations and focused tests.

This documentation-only revision does not add user-visible runtime copy and
does not require new i18n keys.

### Real-Terminal Matrix

All entries begin as `Not tested`. They may be recorded as `Pass`, `Fail`, or
`Limited` with terminal version, operating system, `TERM`, multiplexer version,
and multiplexer mouse setting. Unit tests cannot mark a real-terminal entry
`Pass`.

| Environment | Native wheel/trackpad | Drag and multi-click | Copy warning/artifact | Streaming stability | Alternate-view `1007` | Resize/history |
|---|---|---|---|---|---|---|
| iTerm2 direct | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |
| Terminal.app direct | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |
| Ghostty direct | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |
| Kitty direct | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |
| WezTerm direct | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |
| Alacritty direct | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |
| VS Code/IDE terminal | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |
| Windows Terminal/ConPTY | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |
| SSH without multiplexer | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |
| tmux mouse off | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |
| tmux mouse on | Not tested | Compatibility boundary | Not tested | Not tested | Not tested | Not tested |
| Zellij mouse off | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |
| Zellij mouse on | Not tested | Compatibility boundary | Not tested | Not tested | Not tested | Not tested |

For each environment record:

- primary-screen wheel and trackpad speed, inertia, scrollbar, and terminal
  search;
- unmodified drag, double-click word selection, triple-click line selection,
  right-click, drag auto-scroll, and cross-scrollback selection;
- terminal copy result, iTerm2 warning behavior, ANSI/NUL absence, and
  code-block appearance;
- selection over finalized history while the live footer streams;
- behavior when the terminal is scrolled back and new output arrives;
- alternate-view wheel navigation, keyboard paging, close, and primary-screen
  restoration;
- idle and streaming resize, composer draft preservation, and history
  completeness;
- suspend/resume, external editor, clean exit, crash recovery, and absence of
  leaked modes;
- multiplexer mouse-on and mouse-off behavior.

Direct supported terminals must pass unmodified drag, native wheel/trackpad,
terminal copy without the iTerm2 warning, no black code-block artifact, stable
finalized-history selection during live-footer repaint, and clean lifecycle
restoration.

A mouse-on multiplexer may be `Limited` for native drag because it intentionally
owns the gesture. It must still preserve history and leave terminal state clean.

## Non-Goals

- Reintroducing the removed transcript framebuffer selection overlay.
- Building a new application-managed semantic selection engine.
- Guaranteeing semantic Markdown extraction from arbitrary screen selection.
- Preserving pointer activation for finalized tool summaries.
- Adding draggable panes, scrollbars, pane resize, or drag-and-drop.
- Requiring terminal configuration or exposing multiple normal mouse modes.
- Copying Codex or OpenCode implementation details line for line.
- Claiming complete screen-reader support from native selection alone.

The architecture improves accessibility by making finalized output stable and
ensuring complete keyboard paths. A separate screen-reader design is still
required for animation, repeated announcements, focus semantics, and dynamic
status narration.

## Superseded Directions

This revision supersedes:

- the earlier decision in this file to make Basic mouse reporting the sole
  normal default;
- application-managed transcript drag selection;
- transcript `AttrReverse` selection painting and blank-cell overlays;
- automatic copy-on-release and OSC52 transport for terminal selections;
- unconditional `?1002h` button-motion tracking;
- the fullscreen, transcript-owned selection direction documented in the
  historical Phase 4 section of `prompt/go-tui-implementation-plan.md`.

Basic remains documented only as a transitional option. OpenCode-style
fullscreen application selection and a new semantic selection engine remain
considered alternatives, not hidden implementation plans.

`prompt/go-tui-implementation-plan.md` remains a historical plan. Implemented
reference documentation under `pkg/go-tui/docs/` must not describe the
terminal-native architecture as current behavior until the application split,
lifecycle state, automated gates, and real-terminal matrix are complete.
