# Task 06 - 实施成本、顺序与建设性红队估算

> 估算基线：2026-07-15 当前工作树。本文只估算把本项目带到本次报告定义的“语义展示策略”，不估算 Claude Code/Codex 的像素级复刻，也不把正在进行的未提交改动当成已交付能力。

## 0. 结论先行

“底层能力已经有七八成，所以 UI 再补几条摘要就行”这个推论站不住。现有 `ObservationStore`、`DetailStore`、`ActivityStore` 和 Agent typed result 确实省掉了重造底座的成本，但用户体验的最后一段恰好是最分散的部分：42 个静态工具注册点、动态 MCP 工具、多个 renderer、subagent 乱序生命周期、窄终端、screen reader 和旧 session 都要共同服从一个策略合同。

建议采用下面的**累计工程人日**作为预算，而不是单点承诺：

| 交付边界 | Low | Base | High | Base 的含义 |
| --- | ---: | ---: | ---: | --- |
| MVP | 10 | 18 | 28 | 可信基线、纯 policy、6 个核心命令族/约 12 个高频工具、Agent 终态摘要、关键回归 |
| 完整行为对齐 | 24 | 42 | 67 | MVP + 全部注册命令族、D0 聚合、完整 subagent work view、多 renderer、旧 session 兼容 |
| 生产加固 | 38 | 63 | 100 | 完整对齐 + PTY/窄屏/跨平台/a11y、100k 规模、race/故障注入、灰度和稳定化 |

对应的**增量**是：MVP `10/18/28`，MVP -> 完整对齐再加 `14/24/39`，完整对齐 -> 生产加固再加 `14/21/33`。表内没有重复计费。

置信度：MVP **medium-high**，完整对齐 **medium**，生产加固 **low-medium**。最大不确定性不是写 policy 的代码量，而是动态工具结果是否有稳定结构、旧 session 的兼容范围、真实终端/辅助技术矩阵，以及当前 dirty tree 何时稳定。

## 1. 估算口径与当前代码证据

### 1.1 人日和范围口径

- 1 人日 = 6 小时有效工程时间，包含编码、单元测试、局部 review 修订；不包含组织排队和等待外部用户反馈。
- Low 假设接口一次冻结、现有结果结构可直接复用、只有 macOS/Linux 主路径需要人工验证。
- Base 假设团队熟悉 Go 和本仓库，需要一轮 schema/UX 返工，且当前 dirty tree 可以在 Phase 0 后稳定。
- High 假设 formatter 遇到非结构化/动态 MCP 边缘状态，旧 session 要迁移，至少两轮 UX/a11y 修订，并出现一次共享文件集成返工。
- 新成员对上述数字加 `25%-40%`；兼职 reviewer 或跨时区协作再增加 `10%-20%` 日历时间，但不应伪装成工程人日。
- 不新增依赖、不重写 go-tui、不修改工具的业务语义、不承诺与 Claude/Codex 快捷键或像素一致。
- “全命令”按**命令族合同**验收，而不是给每个动态 MCP tool 手写 renderer；动态工具必须走 server/capability-aware fallback。

### 1.2 仓库规模与现有基线

| 证据 | 当前观察 | 对估算的影响 |
| --- | --- | --- |
| Go 规模 | 1,395 个 Go 文件，约 375,630 行 tracked Go；其中约 176,308 行在 `*_test.go` | 共享事件合同的回归半径大，不能用单文件 UI 改动的经验报价 |
| 测试面 | 643 个 `*_test.go`；本任务相关抽样范围内约 769 个 `Test*`，其中 `tui` 283、`ui` 69、Agent 相关 342、REPL/screen-reader 75 | 可复用测试很多，但改共享 struct/signature 会触发高集成成本 |
| 工具注册面 | `registry_setup.go:623-822` 有 42 个静态 `reg.Register(...)` 调用点，且 `RefreshDynamicMCPTools` 可增加动态工具 | formatter 必须按 family + safe fallback 设计；逐工具 switch 会失控 |
| 当前 dirty tree | 30 个 tracked 文件，约 `+554/-139`，另有 untracked 文件；直接涉及 `tui/root.go`、`repl_tui.go`、`repl_screen_reader.go`、`tools/agent*.go`、`ui/cost_tracker.go` | Phase 0 和 merge buffer 不能删；估算不能把 dirty 代码视为免费完成 |
| 当前测试 | `go test ./tools` 通过（约 74 秒）；`go test ./tui ./ui` 中 `ui` 通过（约 3 秒），`tui` 因 `root_status_bar_test.go:55` 的 `formatSessionUsageSummary` 参数漂移无法编译 | Agent contract 有可信局部基线；TUI 集成基线当前不可信 |

最终集成更新：上述 `tui` 失败是本任务取证时的瞬时工作树状态。主代理稍后复跑时，并发用户改动已补齐调用，`go test ./tui` 通过；成本模型仍保留 Phase 0，因为同一 dirty tree 在调查期间发生过基线漂移。

### 1.3 可复用能力不是“剩余工作为零”

- `tui/observation_store.go:124-337` 已有 session/turn/work-unit/actor、稳定 call/result 关联、结构化 outcome 和无损 result envelope；`522-532` 的 `defaultResultDisclosure` 目前只依据 outcome/isError，缺少 risk、actionability、intent、side effect、volume、family 和 reason code。
- `tui/detail_store.go` 已有 digest、私有文件/内存 store 和 evidence journal；策略实现应继续只做投影，不能为了 D0 聚合删除 evidence。
- `tui/activity_store.go` 已有 kind/phase/state/actionability、actor、progress、control、stable ordering 和 actionability 排序；它适合扩展为 work view reducer，不需要另建第二套运行态 store。
- `tools/agent_output_union.go:18-303` 已有 completed/error/aborted/partial typed union、duration、token、transcript、artifact/output 字段；`tools/agent_contract.go:139-258` 能把确定性 result 映射回 `ToolResultBlock`。
- `tui/root.go:1392-1600` 已有 Agent 领域摘要，`1638-1792` 已有 Activity 总计和列表；但 formatter 仍直接落在大文件中，work view 还没有完整父子树、异常后代提升和 full-thread 路径。
- `ui/screen_reader_renderer.go:450-479`、`ui/term_renderer.go:93-140`、`tui/renderer.go:250-278` 是必须共享同一 policy 结果的 renderer 接缝。各自重新猜 outcome 会产生分叉。
- `tui/performance_acceptance_test.go:14-82` 已有 100,000 observation/viewport 测试，`tui/information_accessibility_test.go` 和 `ui/screen_reader_renderer_test.go` 已有窄屏与辅助技术测试。这些降低测试脚手架成本，但不能替代新 policy/aggregation 的性质测试。

## 2. 按阶段和工作流估算

以下数字为该阶段的**增量人日**。每个格为 `Low / Base / High`。

| 工作流 | MVP | 完整对齐增量 | 生产加固增量 | 具体代码/测试义务 |
| --- | ---: | ---: | ---: | --- |
| 可信基线与回归锁定 | 1 / 2 / 3 | 0 / 1 / 2 | 1 / 1 / 2 | 修复/归档当前 TUI 基线；锁住 disclosure、Agent result、Activity、screen reader；记录非相关失败 |
| Policy、facts、schema/兼容 | 2 / 3 / 5 | 1 / 2 / 4 | 1 / 1 / 2 | `PresentationFacts -> Decision{level,surface,reasons}`；optional/versioned session metadata；P0-P10 冲突表 |
| 命令族 formatter | 3 / 5 / 8 | 3 / 5 / 8 | 1 / 2 / 3 | MVP 覆盖 Shell、File、Search、Web/MCP、Agent/Task、Decision；完整阶段覆盖其余 registry family 和 dynamic fallback |
| 聚合与 subagent/work view | 1 / 2 / 3 | 4 / 6 / 10 | 2 / 3 / 5 | D0 member index、frozen group、late event、parent/child、needs-input 上浮、row/peek/full-thread、取消传播 |
| TUI/terminal/screen-reader UX | 1 / 2 / 3 | 2 / 3 / 5 | 2 / 4 / 6 | `root/state/renderer` 集成、焦点/scroll/draft 恢复、40/80/120 列、CJK、append-only SR；JSON/brief 合同不回退 |
| 测试、review 与稳定化 | 2 / 4 / 6 | 3 / 6 / 8 | 5 / 7 / 10 | table/property/golden、call/result 乱序、race、100k、PTY、故障注入、真实六 Agent 混合场景、修订 buffer |
| 发布文档、迁移与回滚演练 | 0 / 0 / 0 | 1 / 1 / 2 | 2 / 3 / 5 | feature flag/shadow telemetry、旧 session fixtures、用户可见变更、rollback drill |
| **阶段增量** | **10 / 18 / 28** | **14 / 24 / 39** | **14 / 21 / 33** | 逐行求和 |
| **累计** | **10 / 18 / 28** | **24 / 42 / 67** | **38 / 63 / 100** | 不重复计费 |

### 2.1 MVP 的明确边界

MVP 不是“全产品缩小版”，而是验证架构和注意力下限：

1. 先让所有失败/partial/denied/timeout/decision 不会被 quiet/volume 规则压低。
2. 覆盖约 12 个高频/高风险工具，但按 6 个 formatter family 实现，避免 12 份复制代码。
3. Agent 只要求 spawn/running/terminal 的确定性摘要、duration/usage/artifact 入口；完整并发树和 late-event 收敛放到完整对齐。
4. 保持现有 Summary/Detail/Evidence 与 show-all 下钻，不在 MVP 引入持久化 D0 group schema。
5. 验收必须同时覆盖 visual TUI、screen reader 和通用 terminal 的最低信息合同。

若 MVP 被要求同时支持“全部命令、完整 subagent tree、旧 session 迁移、Windows PTY”，那它已经不是 MVP，应直接按完整或生产区间报价。

### 2.2 完整行为对齐的明确边界

- 所有静态注册工具都有 family formatter 或显式 safe fallback；dynamic MCP 由 server/tool/schema annotation 驱动。
- D0 只存在于 presentation projection；group 保存 member ID/evidence refs，turn 冻结后不可静默改写。
- subagent 完成 row -> peek -> full transcript 的三级路径，权限/失败从折叠组上浮。
- classic/fullscreen/terminal/screen reader 共用同一 `PresentationDecision`，仅布局和输出协议不同。
- 旧 session 没有新 metadata 时可恢复到保守默认；新增字段 optional/versioned。
- 不承诺每一种 terminal/辅助技术组合的生产认证，也不承诺真实 100k + 六 Agent 的长期 soak。

### 2.3 生产加固为何不是“再跑一次测试”

- 真实 PTY、tmux/IDE terminal、macOS/Linux，Windows 至少 compile/link + 单测；若 Windows 原生交互也是发布门，Base 再加 `3-6` 人日。
- 40x12、80x24、120x40，CJK/emoji/combining/长路径、ANSI/control chars、二进制和超长 stdout。
- 100,000 observations + 用户 pin + live tail；race、session switch、late result、disconnect/retry、parent cancel、inactive child approval。
- screen reader 人工走查、键盘-only、焦点/scroll/draft 恢复；自动快照只能证明文本存在，不能证明可操作。
- feature flag、shadow decision diff、灰度指标和 rollback drill。没有这些，所谓“可回滚”只是口头愿望。

## 3. 角色、人力与最大有效并发

### 3.1 Base 人日按角色

| 角色/责任 | MVP | 完整对齐累计 | 生产加固累计 |
| --- | ---: | ---: | ---: |
| Tech lead / policy 与 schema owner | 3 | 7 | 9 |
| Go formatter / tool contract engineer | 5 | 9 | 12 |
| TUI / interaction engineer | 3 | 8 | 12 |
| Agent/runtime/activity engineer | 2 | 7 | 10 |
| Test + accessibility + performance engineer | 4 | 8 | 15 |
| Reviewer / migration / release owner | 1 | 3 | 5 |
| **合计** | **18** | **42** | **63** |

角色是责任帽，不要求六个全职人。小团队可以一人兼多帽；但同一人兼 policy owner 和最终 verifier 时，应安排独立 review，不能让实现者自己宣布注意力规则“肯定没问题”。

### 3.2 六路并发的正确用法

任务层允许最高 6 路并发，但实现层的**初始有效并发只有 1-2**。如果六个人第一天同时改 `tui/root.go`，这不叫加速，叫把 merge conflict 包装成团队协作。

接口冻结后可拆为六条 lane：

| Lane | Owner | 独占写入面 | 可并行起点 |
| --- | --- | --- | --- |
| A | policy/schema owner | 新 presentation policy/types + observation adapter | Phase 0 绿后立即；先冻结接口 |
| B | formatter owner 1 | Shell/File/Search formatter + fixtures | A 发布 facts/decision interface 后 |
| C | formatter owner 2 | Web/MCP/Task/Decision formatter + registry coverage | A 发布 interface 后 |
| D | Agent owner | `tools/agent*` facts adapter、Activity/subagent reducer | A 定义 actor/work-unit contract 后 |
| E | renderer/a11y owner | TUI/terminal/screen-reader adapters、narrow layout | A 决策输出稳定后 |
| F | verifier | property/golden/PTY/perf、shadow comparison、独立 review | Phase 0 即可建 fixture，接口后补断言 |

共享文件 `tui/root.go`、`tui/state.go`、`tui/renderer.go`、`repl_tui.go` 必须由一个 integration owner 串行合入。建议先新增包/adapter，再做一次薄集成；不要让 lane B/C/D/E 各自往 `root.go` 塞 switch。

经验性并行效率假设：2 人约 `1.6-1.8x`，3 人约 `2.1-2.4x`，6 路在接口冻结后约 `3.0-3.6x`，不是 `6x`。生产加固的 PTY/a11y/soak 仍有大量串行等待。

### 3.3 Base 日历时间（不是承诺）

| Staffing | MVP | 完整对齐累计 | 生产加固累计 | 条件 |
| --- | --- | --- | --- | --- |
| 1 名熟悉仓库的 senior | 4-5 周 | 9-11 周 | 14-17 周 | 含 review/稳定化间隔 |
| 2 名 senior | 2-3 周 | 5-6 周 | 8-10 周 | policy 与 renderer/formatter 分工 |
| 3 名工程师 | 2 周 | 4-5 周 | 6-8 周 | 至少 1 名能独立做 a11y/测试 |
| 6 路责任 lane | 1.5-2 周 | 3-4 周 | 5-6 周 | 只在接口冻结、文件所有权明确时成立 |

这些日历数还假设产品/UX 可在 1 个工作日内回应关键展示取舍。审批或用户研究排队应单列，不应偷偷压缩工程验证。

## 4. 依赖图、关键路径与阶段门

```text
P0 当前基线可重复
  -> P1 facts/decision contract 冻结
      -> P2a 核心 formatter ---------+
      -> P2b Agent/activity adapter --+-> P3 薄 renderer 集成
      -> P2c test fixtures -----------+      -> P4 D0 聚合 + work view
                                              -> P5 session 兼容 + 多 renderer
                                                  -> P6 PTY/a11y/perf/fault
                                                      -> P7 灰度/稳定化
```

关键路径的 Base 约为：P0 `2pd` -> P1 `3pd` -> 首批 formatter/Agent adapter `5pd` -> renderer integration `4pd` -> aggregation/work view `6pd` -> multi-renderer/session `5pd` -> production gate/stabilization `9pd`。这是约 `34` 个串行关键路径人日；其余 formatter、fixtures 和 review 可并行，因此不能用总人日直接除以 6。

阶段门：

| Gate | 必须证明什么 | 失败时怎么办 |
| --- | --- | --- |
| G0 Baseline | `go test ./tui ./ui ./tools` 至少能稳定复现；已知非相关失败有清单 | 暂停 schema 改动，先隔离 dirty tree/签名漂移 |
| G1 Policy | 所有非成功下限、decision D3、P0 redaction、user pin 冲突均有 table test | 不接 renderer；继续在纯函数层修 |
| G2 Formatter | 核心 family 六态 + generic safe fallback；registry coverage 无遗漏 | 未覆盖工具走保守 D2，不做静默摘要 |
| G3 Aggregation | 每个 D0 member 可由 visible group 找回；乱序/同名并发不串组 | 关闭 D0，仅保留 D1 observation |
| G4 Agent | 六 Agent 混合场景能回答谁、做什么、失败/阻塞、如何进入证据 | work view 不上线，沿用现有 Activity 摘要 |
| G5 Multi-renderer | visual/terminal/screen reader 共享同一 reason code 和披露下限 | 单独禁用有问题的 adapter，不改变 policy |
| G6 Production | race/perf/PTY/a11y/restore/rollback 全部有证据 | feature flag 保持旧投影默认 |

## 5. 替代方案与 do-nothing

| 方案 | 工程人日 Low/Base/High | 得到什么 | 明确放弃什么 | 何时合理 |
| --- | ---: | --- | --- | --- |
| Do nothing | 0 / 0 / 0 | 保留现有三档 disclosure、evidence 和 Activity 基础 | 通用成功摘要、并发判断成本、screen-reader/visual 漂移继续存在；当前 TUI 基线失败仍需另案处理 | 没有用户投诉/长会话/并发 Agent 使用，且团队愿意承受判断成本 |
| 只修基线 + 现有文案 | 2 / 4 / 7 | 当前 TUI 重新可测，修最刺眼的 Agent/error 文案 | 没有统一优先级；新工具继续漂移；不能安全 D0 聚合 | 只需要短期止血或准备 demo |
| Formatter-only | 5 / 8 / 13 | 高频成功输出明显更可读 | quiet/full/风险下限仍散落在 renderer；长期维护成本高 | 明确不做自动折叠/聚合，仅改善措辞 |
| Policy + 6 核心 family | 8 / 13 / 20 | 大部分高频价值和可测的错误/decision 下限 | 完整 subagent tree、边缘工具、旧 session metadata、生产矩阵延后 | 预算受限且需要可演进基础 |
| 本文 MVP | 10 / 18 / 28 | 核心策略、Agent 终态、多 renderer 最低合同 | 完整命令/聚合/work view/生产验证延后 | 默认推荐的第一笔投资 |
| 完整/生产 | 24-100 累计范围 | 一致、可下钻、可演进并有发布证据 | 成本和跨模块协调显著 | 并发 Agent 和长会话已是核心产品能力 |

Do-nothing 不是“免费”：它只是把成本留在每次用户检查详情、误判失败、维护 renderer 分支和排查并发错配中。当前没有真实用户行为数据，本文拒绝编造节省百分比。先采集以下基线再做 ROI：show-all/expand 比率、失败后重复命令率、用户寻找 Agent 结果的时间、collapsed failure 反馈数、screen-reader 重复/遗漏事件数。

最小变更建议：若预算不到 10 人日，优先做“可信基线 + 纯 policy + Shell/File/Search/Agent formatter”，并让未覆盖工具保守显示 D2。不要先做 D0 聚合；隐藏是最便宜的代码、最昂贵的错误。

## 6. 红队：估算成立所依赖的假设

| 隐含假设 | 为什么可能翻车 | 估算影响 | 现在如何验证 |
| --- | --- | ---: | --- |
| result 大多结构化 | 许多工具只有 prose/Content，动态 MCP schema 不受本项目控制 | formatter +5-12pd | 采样各 family 的 success/error/partial envelope，统计字段完整率 |
| 42 个注册点可归为少数 family | 条件工具、remote、cron、goal、MCP annotation 可能需要独立语义 | 完整阶段 +3-8pd | 建 registry coverage 清单；每个工具显式声明 family/fallback |
| 当前 dirty tree 很快稳定 | 当前 TUI 已因签名漂移无法编译，且 Agent/cost/i18n 同时在变 | +2-7pd 或延期 | G0 冻结 commit/patch 基线；记录 owner，禁止估算任务覆盖用户改动 |
| 一个 policy 可供所有 renderer 直接消费 | 现有 visual TUI、term、screen reader、JSON/brief 的协议不同 | +3-6pd | 先做 renderer-neutral decision golden，再分别测 adapter |
| Activity 足以表达 Agent 生命周期 | duration/cost/artifact/parent/late state 并未全部统一 | +4-10pd | 用六 Agent 混合 trace 做 schema spike，不先写 UI |
| 旧 session 只需缺省值 | 用户 pin、group freeze、reason code 若要持久化会引入版本兼容 | +2-6pd | 列出至少 3 个历史 session fixture 做 resume round-trip |
| 100k 测试等于真实性能安全 | 单测不包含 PTY flush、宽度计算、screen reader 和 live tail 组合 | +3-8pd | production 阶段做真实 PTY/soak，而非只看 element node count |

范围变化的重估触发器：

- 要求 Windows 原生交互验收：`+3-6pd`。
- 要求像素/快捷键复制竞品：先单独 discovery，不能塞进现有区间。
- 要求模型生成摘要决定状态：应拒绝；这破坏确定性，不是加几个人日能补救。
- 要求每个动态 MCP tool 定制 formatter：按实际 server/tool 数重新估算，当前区间不包含。
- 要求 telemetry/远程配置/在线 A/B 基础设施从零建设：`+5-12pd`。
- 新依赖或替换 TUI 框架：属于新项目级决策，本文估算失效。

## 7. 风险、前置信号与缓解

| 优先级 | 风险 | 概率/影响 | 最早可见信号 | 缓解/owner |
| --- | --- | --- | --- | --- |
| P0 | policy 把关键失败折得太深 | M / Critical | `quiet + failed` 测试失败；用户频繁 show-all；失败后重复执行 | 非成功 D2 下限、decision D3、reason-code property test / policy owner |
| P0 | D0 聚合错配同名并发/late result | M / Critical | group member 跨 actor/work-unit；orphan/conflict 增加；计数回退 | stable composite key、frozen group、late event 独立升级；可单独关闭 D0 / activity owner |
| P0 | dirty tree 把新回归和既有回归混在一起 | H / High | G0 反复变化；同一测试在无关 patch 前后漂移 | 冻结基线 SHA/patch、shared-file owner、每阶段重放测试 / integration owner |
| P1 | formatter 退化成 `root.go` 巨型 switch | H / High | 每加工具都改 root；同一字段在 3 个 renderer 重写 | family registry + facts adapter + coverage test；root 只消费 presentation / formatter owner |
| P1 | visual 与 screen reader 分叉 | M / High | outcome/reason code 不同；spinner/token 重复朗读 | 共享 decision；append-only transition test + 人工走查 / a11y owner |
| P1 | Agent typed result 和 Activity 状态不一致 | M / High | result completed 但 row running；父取消后 child 无归属 | 确定性 reducer、sequence/epoch、混合 trace + cancellation fixtures / Agent owner |
| P1 | 旧 session 无法恢复或 pin 丢失 | M / High | resume golden diff；unknown metadata 导致 D0 | optional/versioned metadata、保守 fallback、migration fixture / schema owner |
| P2 | 100k + pins + live tail 性能退化 | L-M / High | render time/alloc 随总历史线性增长；输入卡顿 | viewport projection、benchmark threshold、soak / perf owner |
| P2 | 产品效果不可证明 | M / Medium | 摘要更短但 show-all/重复命令不降 | 先定义 baseline metrics，MVP 后做对照观察 / product owner |

## 8. Pre-mortem：六个月后项目失败了

| 验尸结论 | 当时被忽略的前兆 | So what | 现在的预防措施 |
| --- | --- | --- | --- |
| UI 很干净，但隐藏了真正失败 | warning 被绿色 success 尾注吞掉；failed observation 也能进 D0 | 用户在错误状态继续操作，展示策略反而降低可信度 | P0-P10 property tests；failure/partial/denied/timeout 永不低于 D2；上线前 shadow diff |
| 六个开发 lane 互相覆盖，最终没人知道基线 | `root.go/state.go/repl_tui.go` 每天冲突，测试失败归属不明 | 42 人日变成 60+，而且回归证据不可审计 | 一个 integration owner；新增包优先；G0 冻结；每 lane 独占写入面 |
| formatter 很多，但新增工具仍显示垃圾文本 | 新工具不声明 family；dynamic MCP 直接走 raw truncation | 维护成本持续上升，所谓“完整对齐”在下一次注册后立即失效 | registry coverage test；generic safe fallback；tool/family declaration 是注册门 |
| subagent 面板变成另一个日志瀑布 | 每个 tool tick 追加行；needs-input 只藏在 group count | 用户既看不清关键路径，也找不到阻塞 | row 原地更新；只记录 semantic transition；异常独立节点；full transcript 下钻 |
| screen reader 用户拿到的是另一个产品 | visual 改 policy，SR 仍从 raw tool content 推断；焦点恢复无人测 | 合规和可用性同时失败，后补成本远高于同构设计 | decision-neutral golden；SR append-only 去重；人工 keyboard/SR gate |
| 上线后无法回滚旧 session | D0/group metadata 写入旧 schema，旧版本不认识；pin 丢失 | 一次展示改动演变为 session 数据迁移事故 | optional/versioned projection；dual-read；保留 observation/evidence；真实 rollback drill |

## 9. 可逆性与回滚边界

总体不可逆性评分：**4/10**。纯 presentation 变更本应是双向门，但一旦把 group/policy metadata 写进 session、让用户依赖新的 pin/导航语义，回滚会升到 `6/10`。

| 决策/阶段 | 不可逆性 | 回滚边界 |
| --- | ---: | --- |
| 纯 policy shadow mode | 1/10 | 只记录 decision diff，不改变可见输出；删除/禁用即可 |
| 新 formatter + safe fallback | 2/10 | feature flag 切回旧 renderer；evidence 未改变 |
| D0 aggregation projection | 3/10 | 关闭 D0，所有成员恢复 D1；前提是从未删除 observation/evidence |
| Agent work view | 3/10 | 关闭 view，保留现有 Activity/status 和 Agent result summary |
| optional/versioned session metadata | 5/10 | dual-read、ignore unknown fields、旧 fixture round-trip |
| 破坏性 schema 重写/证据裁剪 | 8/10 | 不建议实施；一旦发布可能无法恢复丢失证据 |

建议发布顺序：`shadow-only -> opt-in -> default-on with legacy fallback -> remove legacy only after two release cycles and rollback drill`。任何阶段出现 failed/decision 被降级、D0 无法找回 member、resume 丢失 pin，应立即回退到上一投影；不需要回滚工具执行或删除 session 数据。

## 10. 验收和估算完成条件

### MVP

- 当前 TUI 测试基线可编译并有已知失败清单。
- policy 冲突矩阵通过：secret/full、quiet/failed、pinned/update、huge/decision。
- 六个核心 family 覆盖 queued/running/success/warning/error/expanded；未知工具有保守 fallback。
- Agent completed/error/aborted/partial 使用 typed facts，不从 headline 猜终态。
- visual、terminal、screen reader 的 outcome/has-more/actionability 不矛盾。

### 完整行为对齐

- registry coverage 无未声明工具族；dynamic MCP fallback 已验证。
- 任何 D0 member 都可通过 visible group 的 stable ID 找回并校验 evidence digest。
- 六 Agent 混合状态下，用户能回答谁在做什么、谁阻塞/失败、产物在哪里、如何进入完整 thread。
- 旧 session resume/pin/group 默认值可向后兼容；global show-all 不污染局部 disclosure。
- narrow/CJK/emoji/长路径保留 outcome、risk、object、next action 和 details 入口。

### 生产加固

- `go test ./...`、相关 `-race`、vet/static checks 和选定跨平台矩阵有可审计结果；非相关失败明确隔离。
- 100k/live-tail/large stdout/ANSI/control/binary/late event/parent cancel/permission 上浮均通过。
- PTY、tmux/IDE terminal、keyboard-only 和 screen reader 人工路径已走查。
- feature flag、shadow metrics、旧 session dual-read 和 rollback drill 成功。
- 上线观察指标已定义；不是仅凭“页面看起来短了”宣布完成。

## 11. 最终裁决

必须先做：恢复可信基线、冻结 renderer-neutral policy contract、让未知工具保守失败、用 stable ID 保证 D0 可逆。应当随后做：核心 formatter、Agent/activity adapter、六 Agent trace、screen-reader 同构测试。可以延后：全边缘命令定制、真实跨平台矩阵、长期 soak，但延后意味着只能叫 MVP/行为对齐，不能叫生产加固。

一句话：**这不是 3 天“把输出折叠一下”的活；Base 是 18/42/63 人日的分层投资，真正危险的不是多显示几行，而是少显示了那一行失败、权限或 partial effect。**
