# Workspace Instructions

## Internationalization

- All new user-visible copy must use a semantic `i18n.Key` with `i18n.Text` or
  `i18n.Format`; do not pass English sentences directly to rendering or logging
  APIs, and do not use English sentences as translation keys.
- Declare every new key as a typed `i18n.Key` in `i18n/keys.go` or a focused
  semantic catalog file under `i18n/`, and provide idiomatic translations for
  every language returned by `i18n.AllLanguages()`.
- New code must not use the legacy `i18n.T` or `i18n.TString` helpers. They
  remain only for incremental migration of existing call sites.
- UI, terminal, and screen-reader surfaces must read the active runtime
  language; never force `i18n.LangEN` for user-visible copy.
- Do not translate ASCII logos, product/brand names, model or provider IDs,
  slash commands, protocol names, environment variables, paths, or raw tool
  output.
- Add or update a focused test for each new semantic key. `i18n` catalog
  completeness must remain covered by `ValidateSemanticCatalog`.
- Treat i18n compliance as a completion gate for every change that can affect
  user-visible output: run `go test ./i18n` plus the focused package tests, and
  do not declare completion while a new direct copy literal, legacy helper
  call, forced display language, or incomplete catalog entry remains.
- When editing an existing user-visible surface, preserve raw external values
  as parameters and migrate any hard-coded product copy in the touched path to
  semantic keys; do not add a second compatibility path that bypasses i18n.
- Use `i18n.WrapError` when a raw external cause must remain visible. Use
  `i18n.WrapInternalError` when an internal cause must remain available through
  `errors.Is/As` but its diagnostic text must not leak into localized copy.
- `go test ./i18n` is the executable policy: do not weaken, baseline, or bypass
  its source guard to admit a literal. Any narrow exception must be adjacent,
  rule-scoped, category-scoped, and explain why the content is a protocol,
  identifier, brand, ASCII logo, compatibility value, or raw external output.
