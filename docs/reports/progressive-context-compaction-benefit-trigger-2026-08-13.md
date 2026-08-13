# 渐进式上下文压缩 6K 提前收益触发报告

日期：2026-08-13
结论：`openai/gpt-5.6-sol*` 的 `Inspect` 丰富转写采用 6,000 token 提前收益门槛；只处理已经被后续 assistant 完整消费的结果，只选择最新安全后缀，禁止破坏上一请求已命中的缓存前缀，并且每个 session 最多提前触发一次。该策略进入生产默认；DeepSeek 与实际 context pressure 路径保持原样。

## 1. 为什么要改变原来的批量等待

原方案通常等待 mutation boundary 或 context pressure 再批量处理。等待不会提高确定性转写的内容质量，却会让目标结果在历史中的位置越来越早；一旦 provider 已缓存到该结果之后，后续再改写就会让更长的 cached suffix 失效。

这次优化的目标不是“每次工具调用都压缩”，而是尽早抓住一个同时满足三项条件的窗口：结果已安全消费、累计可省 token 足够、第一处改写仍在上一请求缓存命中前缀之后。

最终状态机为：

```text
新结果 ──尚未消费──> KEEP
  │
  └─已消费──> rich REWRITE 可省 ≥ 6K？
                    │ 否：继续累计
                    ▼ 是
           first_changed > cache_frontier？
                    │ 否：KEEP，等待新的可行后缀
                    ▼ 是
              本 session 已提前触发？
                    │ 是：KEEP 到真实 pressure
                    ▼ 否
               原子提交一次
```

## 2. 固定评测口径

正式 A/B 固定如下变量：

- case：`danielmiessler__Fabric-2098`；
- model：`gpt-5.6-sol`，reasoning effort `xhigh`；
- provider：OpenAI Responses，隐式 default service tier；
- 有效 context window：100,000 token；
- 工具策略：仅 `Inspect`，确定性 `progressive-inspect-rewrite/v1`；
- 两组使用相同 compaction 配置，实际 semantic compaction 均为 0，provider failures 均为 0；
- 价格直接使用 Luban 现有价格表：input `$5/M`、cached input `$0.5/M`、cache write `$6.25/M`、output `$30/M`；
- 费用来自实际 Responses usage ledger，公式为 `uncached×5 + cached×0.5 + cache_write×6.25 + output×30` 美元/M token；
- 模型无可固定 seed，所以用三条真实独立轨迹的中位数描述总体效果；gate 决策、实际 replacement 和缓存失效量可以逐请求直接归因。

Control 使用相同 progressive 基础能力，但关闭提前收益触发；候选组打开最终 6K 策略。v23/v24 的实际 admitted batch 已满足最终版新增的两个硬约束：每条只触发一次、`invalidated_cached_tokens=0`，因此与最终策略在触发行为上等价；v25 精确包含最终缓存硬 gate。发布前补充的 post-mutation frontier 修正不在三条候选的执行路径上：三次实际触发都发生在 mutation 之前，并有独立回归测试覆盖该修正。

## 3. 正式 A/B

### 3.1 每条真实轨迹

| 组别 / run | Agent turns | input | cached | cached/input | output | 费用 | provider time | 实际触发 | 节省 token | 缓存失效 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Control v23 r1 | 9 | 152,329 | 120,320 | 78.99% | 5,868 | `$0.396245` | 137.382s | 0 | 0 | 0 |
| Control v23 r2 | 12 | 276,556 | 228,864 | 82.76% | 6,528 | `$0.548732` | 150.725s | 0 | 0 | 0 |
| Control v25 r1 | 14 | 427,178 | 374,784 | 87.74% | 10,828 | `$0.774202` | 233.571s | 0 | 0 | 0 |
| 6K v23 r1 | 11 | 191,352 | 143,872 | 75.19% | 3,807 | `$0.423546` | 105.848s | turn 4 / 1 次 | 6,627 | 0 |
| 6K v24 r1 | 9 | 115,098 | 65,024 | 56.49% | 3,409 | `$0.385152` | 84.739s | turn 4 / 1 次 | 7,964 | 0 |
| 6K v25 r1 | 10 | 138,816 | 86,016 | 61.96% | 4,789 | `$0.450678` | 488.370s | turn 7 / 1 次 | 8,788 | 0 |

v25 的 provider 488.370s（进程 wall time 490.135s）是单条长尾，不伴随失败、重试或额外压缩；因此正式时间结论使用三次中位数，同时保留该异常值而不裁剪。

### 3.2 中位数

| 指标 | Control 中位数 | 6K 中位数 | 变化 |
|---|---:|---:|---:|
| Agent turns | 12 | 10 | **-16.67%** |
| input token | 276,556 | 138,816 | **-49.81%** |
| cached input token | 228,864 | 86,016 | -62.42% |
| cached/input | 82.76% | 61.96% | -20.79 pp |
| output token | 6,528 | 3,807 | **-41.68%** |
| 实际费用 | `$0.548732` | `$0.423546` | **-22.81%** |
| provider time | 150.725s | 105.848s | **-29.77%** |
| semantic compact | 0 | 0 | 持平 |
| provider failures | 0 | 0 | 持平 |

缓存比例下降不能解释成渐进压缩破坏了缓存：三条候选实际触发的 `invalidated_cached_tokens` 都严格为 0。绝对 cached token 下降是总输入大幅下降的结果；本策略的缓存安全指标是“第一处变化之前的稳定前缀是否覆盖全部上一轮 cache read”，而不是要求发送更少内容后仍维持同一 cached/input 比例。

### 3.3 质量

相同冻结 evaluator 的代表性 Control 与候选都通过：

| 轨迹 | patch 可应用 | FAIL_TO_PASS | PASS_TO_PASS |
|---|---:|---:|---:|
| Control v22 | 是 | 1/1 | 841/841 |
| 6K v24 | 是 | 1/1 | 841/841 |

候选正式三条 patch 都修改同一 2 个文件并产生有效实现；v23/v24 为 `+48/-3`，v25 为 `+50/-3`。质量评测没有观察到回退。

## 4. 阈值与失败方案

| 版本 / 方案 | 真实结果 | 判断 | 导致的优化 |
|---|---|---|---|
| 等到 mutation 后再压缩 | v21 候选可省约 8.3K，但稳定前缀仅约 4.2K，已命中缓存后缀约 18.8K；估计净值 `-$0.074151` | 等待确实错过缓存窗口 | 允许已消费结果在 mutation 前进入普通收益候选 |
| 2K–3K、逐结果 top-up | v22 3K 实际触发 6 次；相对 Control，turn 14 vs 13、费用 `$0.786554` vs `$0.597339`、时间 195.188s vs 144.576s | **负收益**；单批账面正值掩盖了多次 continuation reset | 提高门槛；普通收益每 session 最多一次 |
| 8K | v22 r2 触发 1 次并省 8,091 token；相对 Control input -35.65%、费用 -4.12%，但时间 +5.80%、output +1.33%，且触发时有 4,176 cached token 落在改写位置之后 | 有价值但不够全面正向 | 继续寻找更早且不破坏缓存的位置 |
| 8K + 位置感知 | v23 轨迹累计可省 9,205 token，但会失效 11,669 cached token，正确拒绝；0 次触发 | 总账很好但不能归因于策略 | 不把“未触发”的随机好轨迹当收益 |
| 6K + 允许重复 top-up | v23 r3 两次触发后轨迹放大到 44 turns 并在 900s 超时 | 尾部风险不可接受 | 增加每 session 一次提前 reset 的硬预算 |
| 6K + 最新安全后缀 + 一次触发 | v23/v24 两条均 turn 4 触发，省 6,627/7,964 token，缓存失效 0；总体指标正向 | 最小可用门槛 | 候选生产方案 |
| 再加缓存硬 gate | v25 turns 4–6 分别因 2,860/7,084/10,701 cached token 会失效而拒绝；turn 7 找到 `invalidated=0` 的 8,788-token batch 后才提交 | gate 与 provider 真实 cache frontier 一致 | 最终生产方案 |

一次缺失 provider usage 且单请求耗时 590.66s 的 v22 8K r1 被标为无效诊断轨迹，没有进入任何中位数或收益计算。

## 5. 核心优化点及各自价值

1. **从“阶段触发”改为“收益窗口触发”**：不再等待 mutation；只要结果已被后续 assistant 消费，就开始评估。这直接解决 v21 中“内容可省、缓存窗口已丢”的矛盾。
2. **选择最新安全后缀**：从最新候选向前扩展，而不是改写最老前缀，把第一处变化尽量右移。它最大化稳定缓存前缀，同时保持确定性丰富转写，不使用 `INDEX`。
3. **真实 cache frontier 硬 gate**：用上一请求 provider usage 的 cache read 和候选消息位置估算 `invalidated_cached_tokens`；普通收益路径只接受 0。v25 证明 gate 会等待到真正无缓存损失的窗口，而不是用未来收益补贴已知破坏。
4. **6K token 门槛**：3K 的多次小收益被 continuation reset 吞噬，8K 又可能错过更早窗口；6K 是本次扫描中最小且重复轨迹总体正向的点。
5. **每 session 一次提前 reset**：消除 v23 r3 的 top-up 放大风险；若后续真的出现 context pressure，原有压力路径仍可工作。
6. **压力路径隔离**：提前收益只对 OpenAI 开启，DeepSeek 的已验证 compact 策略不变；`INDEX` 和最老前缀选择只留给真实 context pressure。

## 6. 最终生产配置

```json
{
  "progressiveContext": {
    "enabled": true,
    "toolAllowlist": ["Inspect"],
    "benefitTrigger": true,
    "benefitTriggerProviderAllowlist": ["openai"],
    "benefitMinTokenSavings": 6000
  }
}
```

完整 production scope 仍由 `providerAllowlist`、`modelAllowlist`、`providerModelAllowlist`、stable rollout、kill switch、session token/tool budget 和 anomaly circuit 共同限制。未知价格、缺 provider usage、估算不完整、媒体、失败结果或缺 typed proof 均失败关闭。

## 7. 结论与局限

最终方案达到了本轮要求的方向：不是为了凑批次等待，而是在 6K 收益达标且缓存安全时尽早触发。三条真实候选的中位数相对三条真实 Control：input -49.81%、output -41.68%、费用 -22.81%、turns -16.67%、provider time -29.77%，冻结质量等价，且三次触发均未破坏已命中缓存。

这些总账中位数是实测效果，不是有 seed 的精确因果估计；模型路径差异仍会影响 turns 和输出。生产判断因此同时依赖更强的逐请求事实：真实提交次数为 1、实际 token replacement、缓存失效为 0、无 provider failure，以及质量 evaluator 全通过。后续若监控发现同一 scope 的费用、质量或尾部时延系统性回退，可直接用现有 kill switch 关闭新投影而不损失原始历史。

## 8. 可复核工件

- [正式与诊断运行目录](../../benchmark-results/progressive-benefit-trigger-gpt56-2026-08-13/)
- [最终缓存硬 gate v25 summary](../../benchmark-results/progressive-benefit-trigger-gpt56-2026-08-13/fabric-benefit-v25-6000-100k-r1/summary.json)
- [6K 质量 evaluator](../../benchmark-results/progressive-benefit-trigger-gpt56-2026-08-13/fabric-benefit-v24-6000-100k-r1/evaluation.json)
- [Control 质量 evaluator](../../benchmark-results/progressive-benefit-trigger-gpt56-2026-08-13/fabric-control-v22-100k-r1/evaluation.json)
