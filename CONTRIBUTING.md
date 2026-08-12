# Contributing to LUBAN Code

Thanks for taking the time to improve LUBAN Code. The useful contributions here tend to be narrow, evidenced and explicit about platform limits.

## Set up

You need Git, Go `1.26.1` and Bash. Shell-form `Run` commands invoke Bash on every platform; Windows contributors can use Git Bash or WSL Bash.

```sh
git clone https://github.com/agent-dance/luban.git
cd luban
go test ./cli ./brand ./auth ./provider ./commands
go build -o luban-code ./cmd/luban-code
```

The broader suite includes platform-specific tests. If a test cannot run on your operating system, report the exact package, test name and failure instead of describing the suite as green.

## Keep changes reviewable

- Explain the user-visible problem and the boundary your change touches.
- Add a focused test for the changed behavior.
- Preserve provider-native contracts. `BaseURL` is a transport setting, not permission to change protocol identity.
- Do not broaden sandbox, permission, hook or credential behavior without a threat-model note.
- Include benchmark inputs, raw results and caveats with performance claims.
- Do not commit API keys, OAuth tokens, transcripts, private tool output or generated runtime state.

## User-visible copy and i18n

New UI, terminal, log and screen-reader copy must use a semantic `i18n.Key` with `i18n.Text` or `i18n.Format`. Add idiomatic translations for every language in `i18n.AllLanguages()` and a focused test. Do not introduce `i18n.T`, `i18n.TString`, a forced display language or an English sentence as a translation key.

Before submitting a change that affects user-visible output, run:

```sh
go test ./i18n
go test ./path/to/the/changed/package
```

`go test ./i18n` is a policy gate. Fix the call site or catalog; do not weaken the source guard to admit new hard-coded copy.

## Pull requests

Keep each pull request focused. State what you tested, on which operating system, and what you did not test. Link an issue when one exists. Screenshots help with TUI changes, but include a text description for reviewers using a screen reader.

The repository does not yet publish a root license. A contribution process cannot grant reuse rights that the owner has not published; confirm the licensing terms with the owner before contributing substantial code.
