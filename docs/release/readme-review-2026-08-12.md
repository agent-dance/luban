# README release review — 2026-08-12

Scope: `README.md`, `README.zh-CN.md`, `README.ja.md`, `README.ko.md`, `README.de.md` and `docs/assets/screenshots/luban-live-run.png`.

This is an editorial audit trail, not a product certification.

## Language tooling used

The review used user-level skills installed for this release pass:

- Chinese: `op7418/humanizer-zh@humanizer-zh`
- English: `alirezarezvani/claude-skills@content-humanizer`
- Japanese: `coji/natural-japanese@natural-japanese`
- Korean: `daleseo/korean-skills@humanizer`
- German: `its-a-unixsystem/skills@humanizer-german`

The English draft was written before the content-humanizer pass, as that skill requires. Japanese full mode included its lint, outline and terminology tools. Korean review used all six bundled pattern references. Chinese and German reviews followed their language-specific checklists.

## Pass 1 — language and voice

Goal: remove templated AI cadence without losing technical meaning.

- English mechanical humanity score: `92/100`; no flagged AI vocabulary or hedging.
- Japanese lint initially found two abstract-subject constructions, both rewritten. Outline structure had no template-heading hits.
- Korean modal and translation-pattern scan found two avoidable `수 있습니다` forms; both became direct statements.
- Chinese and German marketing-cliché scans found no hype terms. German heading language was made more concrete.

Every change retained names, numbers, negation, scope and causal qualifiers.

## Pass 2 — fact parity and claim boundaries

Goal: make the five documents agree on facts while keeping idiomatic prose.

Checked in all five files:

- version `v0.1.0` and source-only installation;
- Go `1.26.1` and Windows Bash dependency for shell-form `Run`;
- production compaction scope: two model families, `Inspect` only;
- provider-native contract behavior and compatible-only negotiation;
- 15-task totals, `3/5` official grader tie and ten ungraded tasks;
- the single-pair compaction numbers and no-fixed-seed caveat;
- six runtime UI languages versus five README languages;
- Windows sandbox, credential-store, Agent Teams and licensing limits.

Claims such as “beats Codex,” “all providers,” “cross-platform sandbox,” “encrypted vault” and “production-ready” were rejected.

An independent Chinese review then tightened four cross-language claims: the default tool kernel now records the optional `ContextUpdate` shadow path, paired-run costs are described as fixed-rate estimates rather than bills, and environment-variable setup no longer carries an unsupported auditability comparison. The review also found that security guidance had no actionable private route; GitHub Private Vulnerability Reporting was enabled for `agent-dance/luban`, and every language now links to the live advisory form.

## Pass 3 — rendered release QA

Goal: verify what a visitor can open and run.

- Built `luban-code v0.1.0` from the current source.
- The first TUI launch exposed a Windows checkpoint failure: Go's directory `fsync` path returned `Access is denied`, and synthetic `os.FileMode` bits were being treated as ACLs. The implementation now uses platform-specific directory-sync and private-file checks, with Windows regression tests. A rebuilt binary opened the TUI successfully.
- Made a real model request through the locally configured OpenAI endpoint; it returned `LUBAN READY` and exit code 0.
- Captured both the live TUI and the successful one-shot request without API keys or endpoint addresses.
- Opened the existing benchmark HTML report with Playwright, inspected its semantic snapshot and captured a full-page QA image under ignored `output/playwright/`.
- Checked local Markdown links and image targets.
- Re-ran language tooling, `go test ./i18n`, focused provider/CLI tests, focused checkpoint tests, builds, and Markdown diff checks. The full Windows `internal/ui/tui` package still has unrelated platform and presentation failures, so it is not recorded as green.

## Release blockers kept visible

- No root license has been selected.
- No tag, release binary, package-manager formula or release workflow exists.
- The version string is not yet bound to Git build metadata.
- Windows has no OS sandbox backend.
- The complete Windows `internal/app` suite is not currently a green release gate.

The README labels the project a source preview until those points are resolved.
