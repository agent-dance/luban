# Luban TUI 3D 赛车缺陷修复与端到端复验报告

- 复验日期：2026-08-03（Asia/Shanghai）
- 角色：高级 QA / 修复验收负责人
- 仓库：`/Users/blurooo/project/luban`
- 源码基线：`main@c79ad672`，在用户已有脏工作区上修复；未清理、覆盖或回退无关变更
- 模型：`openai/gpt-5.6-sol`
- 推理强度：`high`
- TUI：150 × 44
- 修复验收会话：`2465e186-68dc-4b5c-b6c2-20878c9b673a`
- 赛车工作区：`/tmp/luban-3d-car-e2e.VpjBki`
- TUI 证据目录：`/tmp/luban-repair-qa.B1BFEe`
- 原始问题报告：`qa/luban-tui-3d-racing-e2e-2026-08-02.md`

> 本文件独立登记修复过程中新发现的问题、根因、修复、真实证据和最终状态。临时目录中的日志、PTY 原始流和截图可能被系统清理；代码与测试是长期回归门禁。

## 1. 验收结论

本轮不是只跑单元测试。实际重新构建 Luban，多次启动 150 × 44 TUI，使用内置 `openai` provider、`gpt-5.6-sol` 和 `high` 推理强度恢复同一会话，完成真实工具调用、长时 `/compact`、Esc 取消、压缩期间输入排队、压缩后事实召回、协议记忆、`-print --session-id` 恢复和真实 Chrome 赛车验收。

按用户指定口径，`/compact` 就是本次“基于 LLM 的自动压缩机制”入口：人工只提交命令，LLM 摘要、边界安装、model view 替换、用量更新、审计 transcript 和 manifest 持久化均由系统自动完成；本轮未把会话人为推进到 token 阈值。

最终判断：

1. LLM 压缩核心链路通过。动态进度、阶段、耗时、Esc、输入排队、单一终态回执、两代摘要 provenance、父链和 transcript 都有真实证据。
2. 内置 OpenAI 兼容路径通过。Responses 不兼容时自动回退 Chat；压缩先确认无工具 Responses 后，恢复完整工具目录仍可重新协商并记住 Chat。
3. TUI 的长请求阶段、ApplyPatch 指标、Inspect 部分失败、退出文案、CJK/emoji 流式渲染和估算校准提示通过。
4. 赛车应用达到精致的风格化程序化 Le Mans 概念车目标，具备真实 WebGL、桌面/移动交互、本地依赖、10 秒 watchdog、故障说明和重试。它不是商业 DCC 或照片级汽车资产，报告不作此类夸大。
5. 修复过程中发现的 Esc 竞态、压缩请求协议漂移、compactor 返回普通对话、print 恢复遗漏和 print 缓存回执粘连均已登记并关闭。

## 2. 实际运行矩阵

| 运行 | 目的 | 结果 |
| --- | --- | --- |
| 初始修复 TUI | 内置 OpenAI、真实工具阶段、ApplyPatch、Inspect、CJK、退出 | 通过；自动 Responses 400 → Chat |
| v2 TUI | 长时 `/compact`、输入排队、成功回执 | 通过；47,649 → 21,223 token，40 → 25 条消息，约 01:31 |
| v2/v3 Esc | 压缩取消竞态 | 首次发现误报；修复后只显示取消，无摘要安装 |
| v3 TUI | 第二次 compact 复验 | 发现 compactor 返回普通对话而非 JSON；未安装错误边界 |
| v4 TUI | 修复后的第二代 compact、压缩后协议和召回 | 通过；21,635 → 13,618 token，29 → 24 条消息，00:50 |
| v5 print | `-print --session-id` 恢复 | 发现并修复遗漏 Resume；恢复请求从错误的 3 条变为完整 29 条 |
| v6 print | 无尾换行正文与缓存回执边界 | 通过；`PRINT_LAYOUT_OK` 与缓存回执分两行 |
| v7 print | 最终源码封板后的恢复与排版复验 | 通过；原样输出 `FINAL_E2E_OK`，缓存回执独立成行 |
| 赛车 `npm test` | 静态契约 + 真实 Chrome/SwiftShader | 5/5 + 4/4 通过 |
| Luban 全仓 | 主 module 全量 Go | `go test ./... -count=1` 通过 |
| go-tui 子模块 | terminal buffer/diff/宽字符 | `go test ./... -count=1` 通过 |
| i18n | semantic catalog 和 source guard | `go test ./i18n -count=1` 通过 |

## 3. 原始 Luban 缺陷关闭矩阵

| ID | 原严重度 | 最终状态 | 修复与验收证据 |
| --- | --- | --- | --- |
| LUBAN-TUI-001 | P1 | 关闭 | 内置 `openai` 首次 Responses 400 后自动切到 Chat 并完成实际 ApplyPatch；后续请求记忆协议。 |
| LUBAN-TUI-002 | P2 | 关闭，保留低风险观察 | provider 错误加入脱敏 endpoint、format、request ID 和格式建议；同一根因主错误去重。真实兼容协商仍可见一次“请求重试”，随后成功，这是可解释的协议协商而非重复终态错误。 |
| LUBAN-TUI-003 | P1 | 关闭 | 真实显示准备、等待首 token、模型思考/生成工具输入、执行工具、工具后等待、最终答复和长阶段提示，阶段耗时持续更新。 |
| LUBAN-TUI-004 | P2 | 关闭 | 实际卡片为 `参数 9.1 KiB · 变更 1 个文件 / +44 -70 · 回执 71 B`；生成阶段按实际接收字节显示 `已接收工具输入 10.0 KiB`，三种指标不再混淆。 |
| LUBAN-TUI-005 | P2 | 关闭 | Inspect 部分完成真实显示 `read_is_directory`；成功/失败请求分别计数，同一请求多个错误不会被误算为多个失败请求。 |
| LUBAN-TUI-006 | P2 | 关闭（防护与恢复） | ApplyPatch schema 和本地预检明确禁止重复 target，并要求无上下文删除前完整读取；确定性错误为结构化、不可原样重试。后续真实轮次没有重复旧错误。 |
| LUBAN-TUI-007 | P1 | 关闭 | `/compact` 后立即显示动态不定进度条、阶段、耗时、Esc 和队列；真实运行持续 50 秒/91 秒不消失，不伪造百分比。 |
| LUBAN-TUI-008 | P2 | 关闭 | running 文案不再在 completed 后重放；成功、失败或取消均原位收敛为一份终态回执。 |
| LUBAN-TUI-009 | P1 | 关闭 | runtime/developer/meta 以带 provenance 的 provider 投影输入，不冒充 user；真实摘要的 “All user messages” 只含实际用户消息。 |
| LUBAN-TUI-010 | P2 | 关闭 | transcript path 由已校验 manifest 解析；第一代指向 g5 的 135,217 B/40 消息文件，第二代指向 g9 的 136,891 B/45 消息文件，SHA 与文件名一致。 |
| LUBAN-TUI-011 | P2 | 关闭，性能门禁继续加固 | 动画限制为 125 ms/帧（8 FPS）并使用 cell diff；v4 约 50 秒压缩的完整 PTY 原始流为 31,574 B，较原报告巨量输出显著下降。 |
| LUBAN-TUI-012 | P2 | 关闭 | 实际 `/exit` 为低风险回执，只出现一次“已请求退出”，随后一条“再见！”。 |
| LUBAN-TUI-013 | P2 | 关闭 | 压缩后禁止工具召回严格四行、187 个字符、仅 text，遵守最新简洁要求。 |
| LUBAN-TUI-014 | P3 | 关闭 | buffer、diff、wrap、truncate 全链路按 grapheme cluster 处理；实际流式 `中文 /context ✏️ 👩🏽‍💻` 最终与逐帧均正确。 |
| LUBAN-TUI-015 | P3 | 关闭 | 压缩完成显示 `≈ 本地估算；将在下一次 provider 响应后校准`；后续真实响应完成校准。 |

### 3.1 关于 LUBAN-TUI-004 与 006 的验收边界

- 大型工具输入无法得到 provider 侧可信“总量百分比”，因此 UI 不伪造进度百分比。它会立即显示“正在生成 ApplyPatch 的工具输入”，再按实际收到的字节显示累计值；首批与最终精确值必达，中间按 1 KiB 里程碑节流，字节更新不重置阶段耗时，也不暴露补丁内容。
- schema/预检能阻断或显著降低 `duplicate_target`、未完整读取的无上下文删除；模型生成行为本身是概率性的，不能合理承诺永不产生违规候选。完成标准是“发送前防护、错误准确、不可原样重试、可恢复”，而不是对 LLM 作零概率保证。

## 4. 修复期间新增缺陷

### LUBAN-TUI-016：Esc 取消与 provider EOF 竞态被误报为压缩失败

- 严重度：P1
- 最终状态：关闭
- 复现：LLM 摘要流被 Esc 取消时，provider 可能同时以空 EOF 收尾。
- 原结果：取消会被解析为 `response did not contain valid text`，UI 有机会显示失败原因。
- 根因：compactor 解析空 EOF 后没有再次以 `ctx.Err()` 作为优先终态；loop 可能处理候选摘要。
- 修复：provider EOF、QueryLoop、CoreEngine、command presentation 和 TUI 均以用户取消优先；取消后的候选摘要绝不安装或持久化。
- 真实证据：只显示 `上下文压缩已取消（手动） · 已取消` 和 `已取消上下文压缩 · 00:01`，没有 valid-text 泄漏、boundary 或 save。

### LUBAN-TUI-017：严格摘要解析器不能稳定处理普通说明文本 + JSON envelope

- 严重度：P1
- 最终状态：关闭
- 根因：部分兼容 provider 会在 `compact-summary/v2` envelope 前生成简短说明；旧解析器把整个响应当纯 JSON。
- 修复：解析器只接受唯一、完整、schema 正确的 envelope；允许边界外普通说明但不会重复吸收、污染摘要或降低 schema 校验。
- 验证：单 envelope、说明前缀、多个 envelope、截断 JSON、错误 schema 和取消竞态均有测试。

### LUBAN-TUI-018：无工具 compact 成功会错误永久确认 Responses，后续全工具请求 400

- 严重度：P1
- 最终状态：关闭
- 复现：compaction 请求无工具并在 Responses 成功；恢复正常会话后携带 Inspect/ApplyPatch/Run 完整目录。
- 根因：协议状态机把 tool-less 成功当成对 full-tools 兼容性的永久确认；后续协议类 400 不再降级。
- 修复：已确认 Responses 仍可在“尚未产生输出”的协议拒绝上安全回退 Chat，并记忆 Chat；不在输出后切协议，避免重复副作用。
- 真实证据：v4 压缩后 full-tools 请求出现一次 Responses 400，透明切 Chat 并成功；下一条 `PROTOCOL_MEMORY_OK` 没有第二次探测。

### LUBAN-TUI-019：compaction provider history 以 tool result 结尾时返回普通对话而非摘要

- 严重度：P1
- 最终状态：关闭
- 复现：压缩投影只把指令放 system，消息尾部恰好是 tool result。
- 原结果：模型自然延续工具任务，返回普通完成说明；严格解析正确拒绝，因此没有损坏边界，但压缩失败。
- 根因：请求末尾缺少明确的运行时 summarization turn boundary。
- 修复：在 provider 投影末尾添加带
  `<compaction-source role="runtime" kind="summarization_request">` 的 meta user 请求；system 明确它不是普通用户消息，摘要也不得把它列入用户消息。
- 真实证据：v4 compaction 返回唯一 `compact-summary/v2` envelope，成功 21,635 → 13,618；摘要不含 summarization_request、system 或内部 marker。

### LUBAN-CLI-020：`-print --session-id` 路径在 Query 前没有 Resume

- 严重度：P1
- 最终状态：关闭
- 原结果：provider 只收到 3 条新消息；请求虽成功，session 保存失败且上下文不完整。
- 根因：`main.go` 的 print 分支早于通用 `eng.Resume`。
- 修复：`PrintModeConfig.Resume` 在 Query 前执行 `eng.Resume(ctx, sessionID)`；测试断言调用顺序和 workspace identity。
- 真实证据：修复后二进制恢复同一 TUI session，provider request 携带 29 条压缩后消息，输出 `PRINT_RESUME_OK`，保存成功。

### LUBAN-CLI-021：print 模式缓存用量回执粘在无尾换行正文后

- 严重度：P3
- 最终状态：关闭
- 原结果：`PRINT_RESUME_OK[缓存：读取 13K / 创建 0K / 未缓存 0K]`。
- 根因：`Text` 按流不加换行，`Usage` 在 turn end 立即渲染，`RunPrintMode` 的最终 newline 来得太晚。
- 修复：TermRenderer 跟踪实际输出行边界；只有缓存回执确实可见且正文没有尾换行时补一个换行。正文已有换行不增加空行，silent usage 不改变正文。
- 真实证据：v6 实际输出为：

```text
PRINT_LAYOUT_OK
[缓存：读取 13K / 创建 0K / 未缓存 0K]
```

## 5. LLM 压缩完整性复验

### 5.1 第一代成功压缩

| 字段 | 实际值 |
| --- | --- |
| generation | 6 |
| trigger | `manual` |
| token | 47,649 → 21,223（减少 55.46%） |
| 消息 | 40 → 25 |
| compactor usage | 31,816 input / 3,761 output |
| 压缩前 transcript | 135,217 B / 40 消息 / SHA `369488…dd5d` |
| boundary digest | `49ff3efb…24de` |
| summary digest | `5c7d7edf…5508` |

真实摘要的 “All user messages” 只列实际用户任务；skill catalog、flight verification、developer/runtime 和 compactor 指令均未被写成用户消息。

### 5.2 第二代成功压缩

| 字段 | 实际值 |
| --- | --- |
| generation | 10 |
| trigger | `manual` |
| token | 21,635 → 13,618（减少 8,017） |
| 消息 | 29 → 24 |
| compactor usage | 11,680 input / 2,397 output |
| 压缩前 transcript | 136,891 B / 45 消息 / SHA `6ccfbf…4fa4` |
| boundary digest | `e3364a…0350` |
| summary digest | `9b9755…9b63` |
| parent summary | `5c7d7e…5508` |

第二代压缩真实持续约 50 秒，界面经历准备、生成 LLM 摘要和持久化，并显示动态 bar、耗时和 Esc。最终回执为：

```text
上下文已压缩：约 21635 → 13618 tokens · 29 → 24 条消息 · 00:50
≈ 本地估算；将在下一次 provider 响应后校准
```

### 5.3 压缩后恢复与当前持久化

- 压缩后召回答复恰好四行、187 字符、仅 text、无工具调用。
- `PROTOCOL_MEMORY_OK` 原样返回，证明 Chat 协议被记忆。
- v7 print 最终恢复后，当前 manifest 为 generation 15：
  - manifest digest：`sha256:d7e4b06bc4c19ab9f273983607458c2dff8310a9bd6d06122f359e5c000945f8`
  - audit：56 条 / 138,765 B / `sha256:faa1b5…920a`
  - model view：34 条 / 37,860 B / `sha256:0e411e…e903`
  - parent manifest：`sha256:97dc82…b2be`
- generation 1→15 父链、artifact bytes、消息数、digest 和 retained refs 均通过内容地址校验；后续普通回合只追加，不改写 compact boundary。

## 6. 工具与 UI 实际证据

### 6.1 工具

- ApplyPatch：真实参数 9.1 KiB；1 个文件；`+44 -70`；回执 71 B。
- ApplyPatch 参数流：覆盖 1 B、10 KiB 和多文件补丁；TUI 显示实际累计接收字节而非虚假百分比。
- Inspect：目录读取失败明确为 `read_is_directory`，而不是误导性的“来源停止”。
- 工具阶段：模型打开 tool block 时立刻显示生成工具输入；随后显示执行、结果和工具后等待。
- 工具错误：`duplicate_target`、`read_required`、permission diagnostics 都保留结构化 code、path、retryable 和恢复建议。

### 6.2 TUI 与中文

- 头部实际为 `openai/gpt-5.6-sol [🧠 高] [354K] [$5/$30]`。
- 压缩期间输入会显示排队内容；压缩完成后自动继续，而不是静默丢失。
- Esc 取消只产生取消终态；成功只产生成功终态。
- `/exit` 是低风险回执，无重复句。
- `中文 /context ✏️ 👩🏽‍💻` 按 grapheme 正确保存、diff 和渲染。
- 约 50 秒 v4 会话原始 PTY 共 31,574 B；动画为 8 FPS 上限，未再出现原报告数万 token/5 秒的高频输出。

## 7. 3D 赛车应用复验

### 7.1 缺陷状态

| ID | 最终状态 | 证据 |
| --- | --- | --- |
| CAR-E2E-001 | 关闭 | WebGL 成功时错误层 hidden/display none；失败时可见。 |
| CAR-E2E-002 | 按“精致风格化程序化概念车”口径关闭 | v6 连续前肩、中央鼻锥、侧箱、融合座舱顶/鲨鱼鳍、克制灯带和压低 Bloom；仍明确不是照片级资产。 |
| CAR-E2E-003 | 关闭 | Three.js 0.169.0 本地锁定；10 秒 watchdog；依赖/WebGL/首帧失败退出 loader，并可全新导航重试。 |
| CAR-E2E-004 | 关闭 | favicon 本地存在，真实 Chrome 无 404。 |
| CAR-E2E-005 | 关闭 | 不再只依赖源码正则；真实 Chrome/SwiftShader 行为测试覆盖离线、依赖失败→重试、WebGL 失败和移动布局。 |
| CAR-E2E-006 | 关闭 | 全仓 Go 与 SwiftShader 并行时首帧用例曾触发 30 秒总预算；失败帧实际已 ready。首帧测试改为 60 秒，所有行为和像素非空断言保持不变。 |

### 7.2 最终视觉判断

v6 桌面和移动截图显示：

- 前轮仍可辨识，但由连续轮拱肩和侧箱纳入同一体量；不再像开放式方程式拼装。
- 深色座舱、顶棚与鲨鱼鳍形成连续结构；银色悬浮薄片感消失。
- 前灯收敛为贴合曲面的细灯带；地面中央青白过曝已移除，暗部和轮毂层次恢复。
- 车身冗余线条减少，主轮廓清楚；桌面和 390 × 844 移动端无横向溢出。

这满足本任务的“精美 3D 赛车”之风格化 WebGL 展示口径。若未来目标升级为商业 DCC、生产级汽车曲面或照片级渲染，应引入专业 GLTF、UV/法线/粗糙度资产，并建立人工美术基线；这属于新的资产等级目标，不应由当前基础几何代码被动承诺。

### 7.3 自动化结果

```text
npm test
静态/结构测试：5 passed
真实 Chrome/SwiftShader：4 passed
```

真实浏览器覆盖：

1. 阻断全部外网后仍从本地 Three.js 完成 WebGL 首帧并退出 loader。
2. 本地 Three.js 请求失败时显示明确错误；点击重试后以新导航成功恢复。
3. WebGL 初始化失败时显示 WebGL 专用恢复提示。
4. 390 × 844 下主文案、性能面板和页脚不碰撞且无横向溢出。

### 7.4 CAR-E2E-006：软件 WebGL 在并行高负载下的测试超时抖动

- 严重度：P3（测试可靠性）
- 最终状态：关闭
- 复现：`go test ./... -count=1` 与 Playwright/SwiftShader 并行运行；首帧用例在全局 30 秒达到 timeout。
- 证据：失败时的 Playwright 页面快照已经显示完整车型页面，`data-app-state=ready` 相关界面状态成立；其余三个浏览器用例通过。无 CPU 争用立即重跑为 4/4。
- 根因：SwiftShader 和 canvas `toDataURL` 是 CPU 密集操作，30 秒包含 Chrome 启动、首帧、多个 DOM 断言和像素读取，低于故障→重试用例已有的 60 秒预算。
- 修复：只把首帧用例总预算提升到 60 秒；没有放宽 ready、first-frame、WebGL renderer、loader、错误层、canvas 尺寸或非空 PNG 的任何断言。

## 8. 最终测试门禁

```text
go test ./... -count=1
=> 全部通过

cd pkg/go-tui && go test ./... -count=1
=> 全部通过

go test ./i18n -count=1
=> 通过

git diff --check
=> 通过

cd /tmp/luban-3d-car-e2e.VpjBki && npm test
=> 5 个静态/结构测试 + 4 个真实 Chrome 测试全部通过

# 软件 WebGL 负载复验：与 app/engine/agent/session/shell Go 测试并行
=> Chrome 4/4；五个高负载 Go 包全部通过
```

i18n completion gate 通过：新增用户可见文案均使用 typed semantic key、运行时活动语言和完整语言 catalog；没有新增 `i18n.T`/`TString`、强制英文展示或直接产品 copy 旁路。

## 9. 证据索引与剩余风险

- TUI raw：`/tmp/luban-repair-qa.B1BFEe/tui.raw`、`tui-v2.raw`、`tui-v3.raw`、`tui-v4.raw`
- TUI debug：`debug.jsonl`、`debug-v2.jsonl`、`debug-v3.jsonl`、`debug-v4.jsonl`
- print resume/layout/final：`debug-print-v5.jsonl`、`debug-print-v6.jsonl`、`debug-print-v7.jsonl`
- 桌面截图：`/tmp/luban-3d-car-e2e.VpjBki/desktop-v6-red.png`（432,264 B）
- 移动截图：`/tmp/luban-3d-car-e2e.VpjBki/mobile-v6-red.png`（131,660 B）

未作为阻断项的后续加固：

1. 为 ANSI 动画建立稳定的字节预算断言，而不只断言 8 FPS 上限和 diff 正确性。
2. 对两个协议都失败的真实网关场景补一轮付费 E2E；当前去重、脱敏和错误分类已有自动化覆盖。
3. 视觉资产若升级到照片级，另立专业 3D 资产需求和美术验收基线。

这些是覆盖深度或新资产等级的增强，不是本轮已复现主路径的开放缺陷。
