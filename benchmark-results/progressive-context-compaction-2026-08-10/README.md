# Progressive context compaction A/B — 2026-08-10

This artifact records one real-provider paired run for `ninja-build__ninja-2749`.

## Fixed configuration

- Model requested and served: `gpt-5.6-sol`
- Reasoning effort: `xhigh`
- Service tier: provider default (implicit `default`)
- API: Responses
- Experimental context budget: 64,000 tokens
- Auto-compaction threshold: 34,616 input tokens
- Luban binary SHA-256: `9f90ea3305f360e6b91678bb585fb48febc908d1c27ea2d03ae3e5a7d25eebc8`
- Control: progressive projection disabled
- Variant: `LUBAN_PROGRESSIVE_CONTEXT_COMPACTION=1`

Prices used: input `$5.00/M`, cached input `$0.50/M`, cache write `$6.25/M`, output `$30.00/M`.

## Result

| Metric | Control | Progressive | Delta |
| --- | ---: | ---: | ---: |
| First semantic compaction turn | 10 | none through turn 34 | after completion |
| Semantic compaction turns | 10, 11, 12, 13, 17 | none | 5 to 0 |
| Agent turns | 20 | 34 | +14 |
| Provider requests | 25 | 34 | +9 |
| Input tokens | 504,679 | 324,904 | -179,775 (-35.622%) |
| Cached input tokens | 216,576 | 195,584 | -20,992 (-9.693%) |
| Cache ratio | 42.914% | 60.197% | +17.284 pp |
| Output tokens | 40,857 | 47,695 | +6,838 (+16.736%) |
| Total tokens | 545,536 | 372,599 | -172,937 (-31.700%) |
| Estimated cost | $2.774513 | $2.175242 | -$0.599271 (-21.599%) |
| Wall time | 1,038.156 s | 1,231.888 s | +193.732 s (+18.661%) |
| Provider request time | 1,036.288547 s | 1,230.608589 s | +194.320042 s (+18.752%) |
| Provider failures | 0 | 0 | 0 |
| Frozen evaluator resolved | true | **false** | quality gate failed |

The variant projected 25 consumed tool-result batches: 174,542 original bytes became 10,335 proof bytes, saving 164,207 repeated bytes before tokenization.

The control is `resolved=true` under the frozen 2 FAIL_TO_PASS plus 455 PASS_TO_PASS criteria. Its own newly added, out-of-catalog `BuildTest.DyndepReadyDepfileDependency` still failed, so this pair is not a fully green quality-equivalent sample. The exact usage ledger remains valid, but it must not be generalized into a rollout estimate.

## Evidence layout

- `control/summary.json` and `progressive/summary.json`: run configuration, exact provider usage totals, timing and content-free context metrics.
- `control/evaluation.json` and `progressive/evaluation.json`: frozen hidden-test verdicts.
- `<arm>/raw/runs/ninja-build__ninja-2749/luban/provider-requests.jsonl`: per-request status, time, usage and served model; no prompts or model output.
- `<arm>/raw/runs/ninja-build__ninja-2749/luban/events.jsonl`: content-minimized Luban stream events.
- `<arm>/raw/runs/ninja-build__ninja-2749/luban/model.patch`: generated task patch.
- `<arm>/raw/evaluation/ninja-build__ninja-2749/luban/`: rebuild and test evidence.

## Verdict

The token and cost effect is real for this trajectory, but the MVP is not shippable. The progressive patch failed to compile because it called a non-existent four-argument `RecomputeDirty`; the control passed the frozen task evaluator. Keep the feature disabled and move to shadow `ContextUpdate` plus source-dependency retention before another paired run.
