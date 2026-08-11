# 渐进式上下文压缩真实 A/B — 2026-08-10

这是 `ninja-build__ninja-2749` 上一次完整、真实 provider、冻结质量评测通过的配对 A/B。两组均自然结束，没有早停；所有 63 个 Responses 请求均成功并由 provider 返回 `gpt-5.6-sol`。

## 固定条件

- 模型：`gpt-5.6-sol`
- reasoning effort：`xhigh`
- API：Responses
- service tier：隐式 provider `default`
- 实验上下文上限：80,000 tokens
- output reserve：16,384 tokens
- 自动语义压缩阈值：50,616 tokens
- 渐进投影水位：42,616 tokens
- Luban binary SHA-256：`417bc37e970a922d2055b6354ac878bf0a145c87e7ae6f1b694eb8803c12c80b`
- 唯一功能开关变量：`LUBAN_PROGRESSIVE_CONTEXT_COMPACTION`

计价采用：uncached input `$5.00/M`、cached input `$0.50/M`、cache write `$6.25/M`、output `$30.00/M`。

## 结果

| 指标 | Control | Progressive | 变化 |
| --- | ---: | ---: | ---: |
| 冻结评测 | 2/2 + 455/455 | 2/2 + 455/455 | 质量等价，均 `resolved=true` |
| agent turns | 39 | 17 | -22 / -56.410% |
| 首次语义压缩 turn | 11 | 完成前未触发 | 11 → 完成以后 |
| 语义压缩次数 | 7 | 0 | -7 / -100% |
| provider 请求数 | 46 | 17 | -29 / -63.043% |
| input tokens | 1,290,781 | 434,153 | -856,628 / -66.365% |
| cached input tokens | 752,128 | 327,680 | -424,448 / -56.433% |
| uncached input tokens | 538,653 | 106,473 | -432,180 / -80.233% |
| cache ratio | 58.269% | 75.476% | +17.206 percentage points |
| output tokens | 71,289 | 10,266 | -61,023 / -85.599% |
| total tokens | 1,362,070 | 444,419 | -917,651 / -67.372% |
| 估算费用 | $5.207999 | $1.004185 | -$4.203814 / -80.718% |
| wall time | 1,583.041 s | 267.642 s | -1,315.399 s / -83.093% |
| provider request time | 1,580.663275 s | 266.779761 s | -1,313.883514 s / -83.122% |
| provider 失败 | 0 | 0 | 0 |

实验组在 turns 9、17 做了两个冻结批次：7 个旧 `Inspect` 结果从 80,753 bytes 转写为 40,279 bytes，减少 40,474 bytes；最近 3 个源码读取、所有失败状态、所有 `Run` 输出和原始审计历史均完整保留。

第一次投影的缓存断点可从逐请求账本直接观察：投影前请求为 31,663 input / 26,112 cached；投影请求为 26,835 / 1,536；紧接着恢复为 30,810 / 26,112，再下一请求为 31,040 / 30,208。缓存损失只持续一次请求，稳定 lineage 使前缀随后重新命中。

费用按 provider usage 重算：

```text
control = 538,653 * $5/M + 752,128 * $0.5/M + 71,289 * $30/M
        = $5.207999

variant = 106,473 * $5/M + 327,680 * $0.5/M + 10,266 * $30/M
        = $1.004185
```

两组 cache write tokens 均为 0。输入、缓存、输出、served model、请求耗时均来自实际 provider response/代理账本，不是字符数或 token 模拟。

## 证据

- `comparison.json`：可机器重算的汇总与差值。
- `<arm>/summary.json`：完整运行配置、provider usage、上下文事件和 binary identity。
- `<arm>/evaluation.json`：冻结 `FAIL_TO_PASS` / `PASS_TO_PASS` 结果。
- `<arm>/raw/runs/.../provider-requests.jsonl`：逐请求 status、usage、served model 和耗时。
- `<arm>/raw/runs/.../events.jsonl`：内容最小化的运行事件、压缩 turn 和投影批次。
- `<arm>/raw/runs/.../model.patch`：任务 patch。
- `<arm>/raw/evaluation/.../`：重建与测试证据。

## 边界

这些数字是两条已经发生的完整轨迹的精确账单，不是总体因果均值。Responses API 没有为本实验提供可固定的采样 seed，而且两组在第一次投影前已经因模型随机性产生不同输出，因此不能把全部 80.718% 费用下降都归因于投影。可直接归因并由事件证明的是：投影在压力水位生效、一次缓存断点后前缀恢复、实验轨迹避免了语义压缩，并在冻结任务质量不下降的情况下自然完成。默认开启仍需多 case、多次反向排序配对实验。
