# Luban TUI 多轮 3D 赛车生成端到端 QA 报告

- 测试日期：2026-08-02（Asia/Shanghai）
- 测试角色：高级 QA / 项目验收负责人
- 仓库：`/Users/blurooo/project/luban`
- 仓库基线：`main@c79ad672`，在用户已有的脏工作区源码上构建；未清理或覆盖既有变更
- 被测二进制：`luban-code v0.1.0`
- 被测模型：`gpt-5.6-sol`
- 推理强度：`high`
- 会话 ID：`b2b4f0f8-5de0-465f-a059-0922ddb1af43`
- TUI 尺寸：150 × 44
- 生成物临时工作区：`/tmp/luban-3d-car-e2e.VpjBki`
- 主调试日志：`/tmp/luban-3d-car-e2e.VpjBki/debug-custom.jsonl`
- 第二代 LLM 压缩复验日志：`/tmp/luban-3d-car-e2e.VpjBki/debug-llm-compact-2.jsonl`
- 当前压缩 manifest：`/Users/blurooo/.luban-code/projects/private-tmp-luban-3d-car-e2e.VpjBki-81767878861d/b2b4f0f8-5de0-465f-a059-0922ddb1af43.context-v2.json`
- 辅助失败日志：`/tmp/luban-3d-car-e2e.VpjBki/debug.jsonl`

> 本文件是 QA 证据和后续修复清单，不包含产品代码修改。临时工作区、日志和截图可能在系统清理后消失；需要长期保留时应由后续任务归档。

## 1. 结论

本次实际启动 Luban TUI，使用 GPT-5.6 sol high 完成了四轮连续开发、一次排队输入、两次由 `/compact` 触发的 LLM 自动摘要压缩、两次压缩后无工具回忆测试，并在真实带 WebGL 的 Chrome 中对桌面端、390px 移动端、交互和 CDN 故障路径做了独立验收。本报告按用户指定口径把 `/compact` 作为自动压缩机制的入口：用户只负责触发，摘要生成、边界安装、模型上下文替换、用量更新和持久化均由系统自动完成；不要求把会话推进到 token 阈值触发点。

总体结论为“核心代理工作流有条件通过，但尚不满足无摩擦和发布级体验要求”：

1. TUI 能正确展示模型、推理强度、上下文、用量、成本和工具阶段；排队交互可靠，工具失败后也能恢复。
2. 两代 LLM 压缩均确实降低了占用，并在每次压缩后保留项目名、技术栈、四轮变更、确定性根因、精确调光参数和剩余风险；多代连续性、父 manifest 链和实际审计 transcript 通过。
3. 内置 `openai` 路由在当前凭据端点下直接 HTTP 400；只有改用自定义 provider + `chat-completions` 才能运行目标模型，主路径存在阻断。
4. 长轮次、补丁参数生成和 LLM 压缩期间的反馈不足；高频重绘产生巨量 ANSI 输出，离“顺畅无摩擦”仍有明显差距。
5. 生成物经过四轮后已是真实可交互 3D，而非图片伪装；第三轮修复了全屏错误层阻断，第四轮明显缓解了过曝和盒状轮廓。但最终车型仍是低面数程序化资产，未达到商业级“精美 3D 赛车”标准。
6. 断开 jsDelivr 后，生成页会永久停在“正在唤醒性能机器”，没有错误说明或重试动作；这是当前生成物仍未关闭的高优先级可靠性缺陷。

### 1.1 缺陷统计

| 范围 | P0 | P1 | P2 | P3 | 合计 |
| --- | ---: | ---: | ---: | ---: | ---: |
| Luban TUI / 上下文 / 工具 | 0 | 4 | 9 | 2 | 15 |
| 本次生成的 3D 赛车应用 | 1（已修复） | 2 | 1 | 1（已修复） | 5 |
| 总计 | 1 | 6 | 10 | 3 | 20 |

严重度口径：P0 为核心结果完全不可用；P1 为主路径阻断、数据/上下文可信度或高影响体验问题；P2 为明显摩擦、误导或局部能力退化；P3 为低影响呈现和一致性问题。

## 2. 实际测试配置与启动

### 2.1 构建

```text
go build -o /tmp/luban-3d-car-e2e.VpjBki/luban ./cmd/luban-code
/tmp/luban-3d-car-e2e.VpjBki/luban --version
=> luban-code v0.1.0
```

构建使用当前工作区源码，二进制和生成物均隔离在临时目录；没有改动或清理用户已有的工作区变更。

### 2.2 失败的内置 provider 路径

```text
./luban \
  --provider openai \
  --model gpt-5.6-sol \
  --reasoning-effort high \
  --pinned-model --allow-all --language zh
```

TUI 头部正确显示：

```text
LUBAN Code — openai/gpt-5.6-sol [🧠 高] [354K] [$5/$30]
```

但第一条消息约 2.4 秒后失败为 HTTP 400。

### 2.3 成功的自定义 provider 路径

```text
./luban \
  --provider custom-sub.blurooo.com \
  --model gpt-5.6-sol \
  --api chat-completions \
  --reasoning-effort high \
  --pinned-model --allow-all --language zh \
  --debug-file /tmp/luban-3d-car-e2e.VpjBki/debug-custom.jsonl
```

主日志完成了 26 次 conversation request/response 和 1 次 compaction request/response；恢复同一会话后的第二份日志又完成 1 次 conversation request/response 和 1 次 compaction request/response。目标模型的目录声明为 353,400 token 上下文、128,000 最大输出，并支持 high 推理强度。

### 2.4 独立浏览器验收

内置 Computer Use 被安全策略禁止控制 Terminal/iTerm，内置 Browser 初始化又返回“No browser is available”。这两项属于测试基础设施限制，不登记为 Luban 仓库缺陷。为继续完成真实验收，改用真实 PTY 启动 TUI，并使用本机 Google Chrome + Playwright headed 模式和 WebGL：

```text
Google Chrome headed
--ignore-gpu-blocklist --enable-webgl --disable-gpu-sandbox
```

浏览器验收覆盖 1440 × 900 和 390 × 844，另拦截全部 `cdn.jsdelivr.net` 请求验证依赖故障路径。

## 3. 多轮任务时间线

| 阶段 | 目标 | 结果 | 可见耗时 |
| --- | --- | --- | ---: |
| 第一轮 | 从空目录生成真实 3D 赛车展示 | 创建完整可运行项目；无真实浏览器验收 | 5m36s |
| 第二轮 | 打磨车身、灯、轮毂、展厅和移动端 | 成功修改；出现一次 `duplicate_target` 补丁错误后恢复 | 7m00s |
| 第三轮 | 修复真实 Chrome 中全屏错误层、404，并加回归 | 根因和修复正确；2 个测试通过 | 1m45s |
| 第一代 LLM 压缩 | `/compact` | 42,904 → 28,561 token；36 → 26 条消息 | 约 2m27s |
| 第一代压缩后回忆 | 禁止工具，仅依上下文复述事实 | 关键事实全部正确，无工具调用 | 40s |
| 第四轮 | 按真实 GPU 截图调光并改善盒状轮廓 | 画质明显改善；4 个测试通过 | 4m32s |
| 第二代 LLM 压缩 | 恢复同一会话后再次 `/compact` | 50,098 → 30,131 token；45 → 25 条消息 | 约 2m |
| 第二代压缩后回忆 | 禁止工具，复述四轮、根因、精确参数和资产上限 | 全部正确，无工具调用；但不够简洁 | 约 25s |

长轮次中还观察到两次约 1m06s、1m38s 的后续首 token 等待。延迟可能同时受 provider 和模型影响，但 TUI 对长等待和大型工具参数生成缺乏足够阶段反馈，仍属于产品可改善范围。

## 4. Luban 产品缺陷

### LUBAN-TUI-001：内置 OpenAI 路径展示目标模型可用，但实际 API 格式不兼容

- 严重度：P1
- 状态：未修复；有手动绕过路径
- 范围：provider 路由、模型能力预检

复现：按 2.2 的命令启动并提交任意任务。

实际结果：TUI 正常展示 `openai/gpt-5.6-sol [高] [354K]`，随后返回：

```text
错误：运行操作失败。请重试；如果问题持续存在，可打开诊断信息。
错误：查询回执：失败：API 调用失败：HTTP 400: Upstream request failed
```

当前凭据的 `base_url` 指向兼容端点，而内置 OpenAI 路由使用 Responses API；相同端点和模型在自定义 provider 的 `chat-completions` 模式下成功。

期望：启动前根据 endpoint/provider/model 能力验证 API format；如果目录已知只支持 chat-completions，应自动选择或明确提示可执行的修复命令，而不是先展示为可用再在首轮失败。

影响：用户明确选择 GPT-5.6 sol high 的主流程被阻断；错误发生在首条消息之后，浪费启动和输入成本。

建议与回归：为 provider + endpoint + model + API format 增加组合预检；增加“自定义 OpenAI-compatible base URL + Responses 不支持 + chat-completions 支持”的集成测试。

### LUBAN-TUI-002：同一 API 失败被重复呈现，且诊断不可操作

- 严重度：P2
- 状态：未修复
- 范围：错误聚合、用户文案

实际结果：同一次 HTTP 400 连续显示通用错误和嵌套查询错误，第二行仍只有 `Upstream request failed`，没有 endpoint、API format 或建议切换方式。

期望：合并为一条主错误，保留简洁用户文案；诊断详情中展示 provider、API format、请求 ID和经过脱敏的 endpoint，并给出兼容格式建议。

影响：重复信息占用主对话区，却没有帮助用户从 Responses/Chat Completions 不兼容中恢复。

回归建议：构造 provider 400、401、404、429 和 format mismatch，验证主界面只出现一次用户错误，详情仍保留原始外部原因。

### LUBAN-TUI-003：长模型轮次缺乏可区分的阶段和进度反馈

- 严重度：P1
- 状态：未修复
- 范围：请求状态、长任务体验

实际结果：第一、二、四轮分别耗时 5m36s、7m00s、4m32s；首 token 或工具后的继续请求可等待 1 分钟以上。绝大部分时间只显示统一的“工作中”动画、尝试数、连接和首 token，没有区分模型思考、工具参数生成、工具执行、验证或最终组织答案。

期望：至少区分“等待首 token”“模型生成工具输入”“工具执行”“等待工具后继续”“生成最终答复”，并持续显示阶段耗时；对超过阈值的阶段给出不打断工作的提示。

影响：用户无法判断系统仍健康、provider 卡住、模型在生成大补丁，还是工具已经挂起；长轮次体感接近无响应。

回归建议：用可控慢流分别延迟首 token、工具参数、工具结果和工具后续请求，断言阶段、计时、取消提示准确变化。

### LUBAN-TUI-004：大型 ApplyPatch 期间没有参数生成进度，完成后的 `70/71 B` 又容易被误读

- 严重度：P2
- 状态：未修复
- 范围：工具卡片、指标语义

实际结果：首轮生成约 28 KB、多文件补丁时只显示 spinner，文件直到工具调用完成才出现。工具行最终显示：

```text
ApplyPatch · 输入键：patch · 71 B
```

第三、四轮多文件修改也显示约 70/71 B。该数字实际更像工具回执大小，但紧邻“输入键：patch”后会被自然理解为补丁大小。

期望：明确区分 `参数已生成 N KB`、`补丁变更 X files / +A -B` 和 `回执 N B`；参数流式生成时至少显示累计字符或 token。

影响：大型写入期间缺乏进度，完成后指标又低估实际变更，用户无法判断工具规模和卡顿位置。

回归建议：提交 1 B、10 KB 和多文件补丁，断言参数大小、变更统计与回执大小分别标注。

### LUBAN-TUI-005：Inspect 的部分完成错误对用户隐藏了确定性原因

- 严重度：P2
- 状态：未修复
- 范围：Inspect 结果摘要

实际结果：首次 Inspect 将目录 `.` 当普通文件读，调试证据为 `read_failed`；TUI 只显示：

```text
Inspect · 部分完成
来源在生成完整结果前停止；完整证据不可用
```

这段文案容易让人误以为流式输出中断，而不是“目录不能按文件读取”。

期望：显示失败 request ID、kind、path 和可理解原因，例如“root：无法按文件读取目录；另 2 项成功”。原始外部路径应作为参数保留。

影响：模型和用户都需要猜测失败原因；不利于判断是否应该改用 glob、分页或重试。

回归建议：覆盖 read 目录、文件不存在、权限失败、分页截断和 source truncated，确保每种 partial 都有准确原因。

### LUBAN-TUI-006：工具输入约束导致两次可避免的额外循环

- 严重度：P2
- 状态：未修复；运行时均成功恢复
- 范围：工具 schema、调用前校验、模型契约

观察到两类确定性失败：

1. 第二轮同一补丁含两个 `*** Update File: src/main.js`，返回 `duplicate_target`，且 `retryable:false`。
2. 第四轮对 `src/main.js` 做无上下文删除时，返回 `file.apply_patch.read_required`，要求完整 Inspect 后再提交。

工具错误结构清晰，模型也没有原样重试：第二轮合并补丁后成功；第四轮补读并补齐渐缩壳体端盖后成功。这一点是正向表现。

期望：客户端在发送前就检查重复目标；当会话中已有分页/部分读取时，ApplyPatch 卡片或模型提示应提前说明完整读取要求。

影响：额外 provider 往返在本次分别放大为分钟级等待和成本；工具本身未损坏，但不是“无摩擦”。

回归建议：增加补丁预解析器测试；对删除型补丁模拟完整读取、部分读取和未读取三种状态，验证失败前置且修复建议稳定。

### LUBAN-TUI-007：`/compact` 触发的 LLM 摘要压缩约 2 分钟无反馈，应参考 Claude Code 呈现动态压缩进度条

- 严重度：P1
- 状态：未修复
- 范围：上下文压缩生命周期、并发输入

复现：分别在上下文约 42K 和 50K token 时执行 `/compact`。

实际结果：两次提交到完成分别约 2m27s 和约 2 分钟，期间都没有“正在压缩”、耗时、取消说明或 busy 状态，输入框光标仍像空闲界面一样闪烁。第二代复验稳定复现，用户无法确认命令是否被接收，也可能继续输入与压缩并发。

期望：参考 Claude Code 的压缩反馈效果，命令提交后立即在稳定、醒目的位置呈现动态压缩进度条，不得让界面看起来处于空闲。例如：

```text
正在压缩上下文  ██████░░░░  生成 LLM 摘要 · 01:24 · Esc 取消
```

进度条至少区分“准备上下文 → 生成 LLM 摘要 → 安装压缩边界 → 持久化 → 完成”阶段；持续显示已用时间、取消方式，并明确新输入会排队还是拒绝。若 provider 无法提供可信总量，LLM 流式生成阶段应使用持续运动的 indeterminate 动画或按已完成阶段推进，不能伪造精确百分比。完成后在原位置收敛为单一成功回执，显示压缩前后 token、消息数和耗时；失败或取消时同样原位给出明确终态。

影响：这是最明显的上下文 UX 缺陷；长时间无反馈会诱发重复执行、退出或错误输入。

验收标准：

1. 执行 `/compact` 后 100 ms 内出现压缩进度组件，直到成功、失败或取消前不得消失或退化为空闲光标。
2. 2 秒和 120 秒的可控 compactor 都至少展示阶段、动态进度、累计耗时和取消提示；超过 1 秒没有模型 delta 时动画仍持续，但 CPU/ANSI 输出受帧率预算约束。
3. 阶段顺序只能是 accepted → preparing → summarizing → installing → persisting → completed/failed/cancelled，不得在 completed 后再显示“正在压缩”。
4. 不支持可靠百分比时必须使用不定进度样式；任何数值百分比都必须能由真实事件计算并保持单调。
5. 压缩期间提交新输入时，UI 必须明确显示“已排队”或“压缩期间不可输入”，不得静默接受。

回归建议：为生命周期 reducer 增加阶段序列测试；用 2 秒和 120 秒的可控 compactor 做 TUI 快照与逐帧测试，并为动画刷新设置 ANSI 字节/帧率预算，避免修复本问题时加剧 LUBAN-TUI-011。

### LUBAN-TUI-008：压缩完成后的回执顺序和时态互相矛盾

- 严重度：P2
- 状态：未修复
- 范围：命令回执、中文文案

实际显示顺序：

```text
✓ 上下文已压缩（手动）：42904 → 28561 tokens；保留 28561；丢弃 14343 · 已完成
命令 /compact：成功。 正在使用 LLM 摘要压缩上下文…
上下文已压缩：36 → 26 条消息（LLM 摘要）。
```

“正在使用”出现在“已完成”之后，完成事件与过程事件顺序倒置。

期望：过程文案在请求开始时出现，结束时原位更新为完成；最终只保留一份一致回执。

影响：削弱用户对命令是否真的结束的信心，也放大 LUBAN-TUI-007 的空白等待问题。

回归建议：对命令事件做严格序列断言：accepted → running → completed/failed，禁止 completed 后再渲染 present-progress 文案。

### LUBAN-TUI-009：压缩摘要混入错误来源归属和不符合事实的会话状态

- 严重度：P1
- 状态：未修复
- 范围：压缩质量、消息来源完整性

两代调试日志中的 `compact-summary/v2` 都把以下内容列为“All User Messages”：

1. 实际为 developer/meta 的 skill catalog snapshot，被描述成“用户发送”。
2. 一段本轮没有由用户输入的“当前变更已经尝试过验证……立即收敛”被描述成用户指令。
3. 压缩器自身的“停止工具调用并生成当前对话的结构化详细总结”被描述成“最新用户消息”。

压缩后的包装文本还写着“因上下文空间耗尽而中断”，但本次是用户在低占用时执行 `/compact`，不存在空间耗尽。第二代摘要又把内部“生成结构化会话摘要”要求和上一代 continuation 摘要描述成用户消息，说明问题能跨代延续。

积极面：压缩后保留的后续消息补足了摘要边界，回忆测试没有出现事实性任务丢失。但来源归属污染可能在更敏感的任务中改变优先级或让模型误以为用户授权了未授权操作。

期望：摘要保留 role、internal_kind 和来源；压缩提示不得把系统、developer、meta 或 compaction prompt 伪装成用户消息；`/compact` 触发和空间压力阈值触发使用不同包装文案。

影响：上下文连续性“内容大致记得”，但“谁说过什么”不完全可信，属于指令完整性风险。

回归建议：构造 system/developer/meta/user/tool 混合会话，压缩后逐条验证来源；增加“手动低占用压缩不得出现空间耗尽”测试。

### LUBAN-TUI-010：第一代压缩摘要暴露过期的 0 B genesis 审计路径

- 严重度：P2
- 状态：首次复现；第二代路径正确，内部审计链完整
- 范围：压缩可恢复性、审计证据

第一代摘要给出完整会话记录路径：

```text
/Users/blurooo/.luban-code/projects/private-tmp-luban-3d-car-e2e.VpjBki-81767878861d/
b2b4f0f8-5de0-465f-a059-0922ddb1af43/context-v2/audit-transcripts/
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855.jsonl
```

该文件存在但大小为 `0 B`，文件名也是空内容的 SHA-256。进一步核对 manifest 和第二代摘要后确认，这不是当前审计内容丢失，而是第一代 LLM 摘要暴露了过期的 genesis 占位路径。

第二代摘要指向 `253aa8d13c031cf26e1d28c2234134f2708840bc6f1beee3be3ce6268ac540c1.jsonl`，实际为 `174,696 B`；第二代回忆完成后的当前 manifest 指向 `d876a307e86bcfd68722b8d751f1740748d17b2de1f910a4705975e28be8e127.jsonl`，实际为 `178,731 B`、59 条消息。父 manifest 和 8 个 audit segment 也都存在且可解析。

期望：LLM 摘要中的恢复路径必须从已提交且校验过的 manifest 派生，不应由模型猜测或沿用 genesis 占位值；呈现路径前校验文件非空、哈希和消息数一致。

影响：内部持久化设计有效，但第一代摘要给出的用户/模型恢复指引不可用；如果恰好依赖该路径恢复被摘要丢弃的事实，会产生错误结论。

回归建议：每一代压缩后同时断言摘要暴露路径、manifest transcript、父链、行数、字节数和哈希一致，并用新会话读取一个已被摘要丢弃的事实。

### LUBAN-TUI-011：动画和光标重绘产生极高频 ANSI 输出

- 严重度：P2
- 状态：未修复
- 范围：TUI 渲染性能、PTY/远程终端

实际结果：在可见内容几乎未变化的“工作中”阶段，5 秒轮询曾收到约 32,716、35,205、60,299 token 量级的原始 PTY 输出，主要是逐字符颜色动画、光标定位和输入框光标反复重画；第四轮一次 30 秒等待产生约 39,599 token 的 PTY 输出。

这些数值是测试工具对 PTY 文本的 token 近似，不等同于模型 token 或 CPU 采样，但足以证明终端字节流非常密集。

期望：脏区域 diff、帧率上限、动画降级和隐藏/非交互终端检测；状态秒数每秒更新即可，光标不应高频全量闪烁。

影响：本地可能只是额外 CPU，SSH、日志采集、录屏和自动化 PTY 中会显著放大带宽、存储和可读性问题。

回归建议：为静止帧、spinner 帧和一秒计时帧增加 ANSI 字节预算；在 mock terminal 中断言未变化区域不刷新。

### LUBAN-TUI-012：`/exit` 被重复描述并错误标记为高风险

- 严重度：P2
- 状态：未修复
- 范围：命令语义、中文文案

实际结果：

```text
命令 /exit：已请求退出。 已请求退出。 展示：决策。风险：高。
下一步：无需进一步操作。
再见！
```

期望：本地退出是低风险、无需决策解释的普通命令；显示一次“正在退出…”或直接“再见”。

影响：重复且官僚化的措辞破坏收尾体验，“风险：高”会误导用户以为退出可能造成数据损坏。

回归建议：校验 `/exit`、`/context`、`/compact` 的风险分类和最终中文快照，不允许重复句。

### LUBAN-TUI-013：压缩后回忆答复明显超过用户要求的简洁程度

- 严重度：P2
- 状态：未修复或需联合提示词调优
- 范围：回答校准、压缩后行为

用户要求“不调用工具，仅列出”四类事实。模型遵守了不用工具，也全部答对，但用了 40 秒，并展开到多段代码级细节，明显超出核验所需。

期望：在事实召回测试中按四项简短列出；压缩摘要可以详尽，面向用户的最终回答仍应服从当前消息的篇幅和格式要求。

影响：增加等待、输出成本和认知负担；也可能说明长摘要把“详细总结”风格带入了后续回答。

回归建议：对压缩前后同一简短问答做长度和格式对比，确保摘要中的写作要求不会覆盖最新用户要求。

### LUBAN-TUI-014：流式渲染中偶发短暂字符错位，最终帧可恢复

- 严重度：P3
- 状态：未修复
- 范围：TUI 流式文本、宽字符布局

观察到流式中间帧里命令或文本短暂出现字符错位，例如 `/context` 一度看似被拆开，中文与代码片段也有瞬时覆盖；稳定后最终内容正确。

期望：流式 markdown、中文宽字符和状态栏更新互不覆盖；中间帧也应可读。

影响：人工快速浏览或录屏时会误判内容损坏；最终结果不受影响，因此定为 P3。

回归建议：对 CJK、反引号代码、长行换行和状态栏同步更新做逐帧 terminal emulator 测试。

### LUBAN-TUI-015：压缩后 token 来源从 provider 精确值退化为本地估算，解释不足

- 严重度：P3
- 状态：未修复
- 范围：上下文指标语义

压缩前 `/context` 显示 `Provider 精确用量`；第一次压缩后立即显示约 28,561 token，来源变为“完整本地估算”；第四轮结束后又恢复为 provider 精确值 47,453。第二次压缩回执同样先给出本地计算的 30,131，下一次 provider 响应校准为 29,147。

界面已经展示来源，这是优点；但没有解释为什么 `/compact` 完成值不是 provider 精确值、下一次请求为何会校准。

期望：在估算值旁说明“将在下一次 provider 响应后校准”，并让状态栏的近似符号和详情口径一致。

影响：低；主要影响用户对压缩节省量和阈值的精确判断。

回归建议：覆盖压缩前、压缩完成未请求、下一次 provider 响应三个阶段的来源和近似符号。

## 5. 上下文压缩专项结果

### 5.1 验收口径与实现确认

本次不推进到自动阈值点。按用户确认的口径，直接使用 `/compact` 验收“基于 LLM 的自动压缩机制”：命令是唯一人工触发动作，之后的 LLM 摘要、边界安装、provider model view 替换、用量更新、manifest 与审计持久化均须自动完成。

源码路径确认 `/compact` 与空间压力入口复用同一 LLM compactor：

- `commands/builtins.go` 接收 `/compact`；
- `internal/runtime/compact/auto_compact.go` 执行 LLM 摘要；
- `internal/runtime/loop/context_prepare.go` 安装压缩边界并准备后续模型上下文；
- manifest 中 `boundary.trigger` 为 `manual`，它只记录触发来源，不代表摘要是规则截断或人工编写。

调试日志两次都出现与目标 provider/model 对应的 `compaction request/response`，响应 schema 为 `compact-summary/v2`。因此“确实调用 LLM，而不是本地机械裁剪”通过。

### 5.2 两代数量效果

| 代次与检查点 | 消息数 | token | 来源 |
| --- | ---: | ---: | --- |
| 第一代压缩前 | 36 | 41,873 / 353,400（11.8%） | Provider 精确用量 |
| 第一代压缩器内部统计 | 36 → 26 | 42,904 → 28,561 | 压缩回执 |
| 第一代减少量 | -10 | -14,343（-33.4%） | 计算值 |
| 第四轮结束、第二代压缩前 | 45 | 47,453 / 353,400（13.4%） | Provider 精确用量 |
| 第二代压缩器内部统计 | 45 → 25 | 50,098 → 30,131 | 压缩回执 / manifest |
| 第二代减少量 | -20 | -19,967（-39.9%） | 计算值 |
| 第二代压缩后首次回答 | 27 | 29,147 / 353,400 | Provider 精确用量 |

两代都在低占用时有效完成，单代减少 33.4%–39.9%。第二代 compactor 自身用量为 26,359 input / 6,452 output token；这部分是压缩成本，不应与压缩后上下文大小混淆。

### 5.3 两代语义连续性

第一代压缩后要求模型在不调用工具、不读文件的前提下回忆项目/车型、技术栈、前三轮变更、确定性根因与未完成验证。模型全部答对，且没有虚构 TUI 会话之外的浏览器验收。

第二代压缩发生在第四轮完成后；再次明确禁止工具，并要求复述四轮内容、第三轮根因、第四轮精确参数变化和最终资产上限。模型正确回忆：

- 四轮从零搭建、造型打磨、真实 Chrome 阻断修复和 GPU 截图驱动的调光/轮廓优化；
- 根因是 `.webgl-error { display:grid }` 覆盖 UA `[hidden]`，修复为 `[hidden] { display:none !important }`，失败分支仍通过 `errorPanel.hidden = false` 显示，并补 favicon；
- 曝光 `1.05 → 0.82`，Bloom `0.48 → 0.20`、半径 `0.60 → 0.32`、阈值 `0.88 → 0.96`，前/尾灯 emissive `7/8 → 2.1/2.4`，主/轮廓聚光 `560/420 → 175/125`，并准确回忆红光、glow、roughness、clearcoat 和 env map 的变化；
- 明确认可最终仍是程序化低面数资产，没有 TUI 内后改版 Chrome GPU 截图，也没有专业汽车曲面、UV/扫描资产，不能宣称照片级。

第二代回答没有任何工具调用；调试日志只有 1 次 conversation request/response。事实连续性和“压缩后可继续对话”通过，但回答为 1,013 output token，仍明显违背“简洁”，LUBAN-TUI-013 跨两代稳定复现。

### 5.4 多代父链与审计持久化

第二代回忆完成后的 `compaction-manifest/v2` 证据：

| 字段 | 实际值 |
| --- | --- |
| `context_generation` | 9 |
| `compaction_count` | 2（session meta） |
| 当前 manifest digest | `sha256:653ef79440569438a80a261c134d4f611e72825ba3cfb64393f122d5a1be13d5` |
| 父 manifest digest | `sha256:bc30c895da6583a11eb32a47808f8c902e5aca7d6fe7b6a582f12fea78c83ebf`，对应文件存在 |
| summary parent digest | `sha256:21651c505cb762a62d02dd2ee4099b33f8c93ca1ddf1b65d0b85e07a646bbc4f` |
| 审计尾 | 59 条消息，8 个 segment |
| 当前 audit transcript | digest `d876…e127`，178,731 B，59 条消息，文件存在且非空 |
| retained / model view | 25 / 3 |

结论：跨两代父链、当前 transcript、segment 和消息计数均可落盘并恢复，内部审计可恢复性通过。第一代摘要暴露过期的 0 B genesis 路径、第二代摘要路径恢复正常，因此 LUBAN-TUI-010 是“用户可见恢复指引间歇性错误”，不是审计数据整体丢失。

### 5.5 专项验收矩阵

| 验收项 | 结论 | 证据摘要 |
| --- | --- | --- |
| `/compact` 调用目标 LLM | 通过 | 两代均有 `gpt-5.6-sol` compaction request/response |
| 结构化摘要与自动边界安装 | 通过 | `compact-summary/v2`、boundary 和 model view 均落盘 |
| 重复压缩 | 通过 | 同一 session `compaction_count=2` |
| 上下文缩减 | 通过 | 两代分别减少 33.4% 和 39.9% |
| 关键事实与精确参数召回 | 通过 | 两次禁止工具回忆均准确 |
| 压缩后继续对话 | 通过 | 第二代后 provider 正常响应并校准用量 |
| manifest 父链与真实审计文件 | 通过 | 父文件可解析，当前 transcript 178,731 B |
| 压缩等待态 | 不通过 | 两次约 2 分钟均无 running/busy 反馈；需参考 Claude Code 增加动态压缩进度条 |
| 摘要 role/provenance | 不通过 | 两代都把内部/meta 内容归为用户消息 |
| 摘要提供的审计恢复路径 | 部分通过 | 第一代为 0 B genesis，第二代正确 |
| 最新“简洁”要求服从性 | 不通过 | 第二代仍输出 1,013 token |

综合判断：用户指定的“基于 LLM 的自动压缩机制”核心能力通过，包含 LLM 摘要、自动上下文替换、多代连续性和可恢复持久化；但尚不能判定为无摩擦或指令来源完全可信。后续应先修 role/provenance 和压缩 running 状态，再修摘要暴露路径与回答篇幅校准，不能只以 recall 答对为全部通过标准。

## 6. 工具调用专项结果

### 6.1 正向表现

- Inspect 支持批量 glob/read 和分页游标；第二轮在提示“当前结果是分页窗口”后，模型正确继续读取。
- ApplyPatch 的 `duplicate_target` 和 `read_required` 错误都包含结构化 code、path、retryable 和建议动作，模型能够有依据地恢复。
- 模型没有重复提交同一确定性失败指纹。
- Run 能完成语法、Node 测试和 HTTP 冒烟；结果以紧凑卡片展示。
- 用户在第一轮运行中排入第二轮消息后，TUI 显示“1 条消息排队中 · 按 Esc 转为引导”，第一轮结束后自动提交，队列能力通过。

### 6.2 摩擦点

- 一次 Inspect 把目录当文件读，用户层摘要不准确。
- 一次补丁重复目标、一次删除前读取不足，额外增加 provider 往返。
- 大补丁参数生成期间无尺寸或进度。
- 工具结果 `70/71 B` 指标语义不清。
- 工具完成后继续请求的首 token 等待可超过 1 分钟，仍只表现为通用“工作中”。

## 7. TUI UI 与中文措辞验收

### 7.1 通过项

- 150 × 44 下头部、主对话、工具卡片、输入框和底部状态栏层次清楚。
- 模型 `gpt-5.6-sol`、推理强度“高”、上下文约 354K、价格 `$5/$30` 均正确可见。
- 长请求状态包含尝试次数、连接时间、首 token 时间和 `Ctrl+C 中断`。
- 上下文详情同时给出消息数、已用比例、剩余 token 和数据来源。
- 中文 happy path 大部分自然；模型在第三、四轮明确披露“没有真实 GPU 截图”“不能声称照片级”，没有夸大验证。
- 工具卡片紧凑且可展开，队列提示明确。

### 7.2 未通过项

- `/exit` 重复、风险分类错误且措辞官僚。
- `/compact` 完成后仍用“正在”，时序矛盾。
- Inspect partial 文案没有表达真实失败原因。
- ApplyPatch 的 70/71 B 指标误导。
- 长工具参数、`/compact` 触发的 LLM 压缩和等待后续首 token 的反馈不足。
- 流式中间帧有字符错位，高频动画导致大量 ANSI 重绘。

## 8. 生成的 3D 赛车应用缺陷

### CAR-E2E-001：WebGL 正常时全屏错误层仍覆盖页面

- 严重度：P0（原始状态）
- 状态：第三轮已修复并通过真实 Chrome 复验

根因：DOM 上 `#webgl-error` 具有 `hidden`，但作者样式 `.webgl-error { display:grid; }` 覆盖浏览器 UA 的 `[hidden] { display:none; }`，导致 WebGL 正常时仍显示全屏失败面板。

修复：新增全局 `[hidden] { display:none !important; }`；真实失败时 JS 设置 `errorPanel.hidden = false`，选择器不再匹配，错误层仍可显示。

独立复验：桌面和移动端均为 `errorHidden=true`、computed display `none`、WebGL context 存在、赛车正常显示。

防回归缺口：现有测试主要检查源码正则，建议增加真实浏览器 computed style 测试，同时模拟 WebGL 创建失败验证反向分支。

### CAR-E2E-002：最终程序化车型仍未达到“精美”验收标准

- 严重度：P1
- 状态：第四轮部分改善，仍未关闭

第三轮后真实截图表现：车漆大片裁白，前灯、红灯和地面产生巨大 Bloom 光斑，轮毂和灯腔细节被吞没；车体由多个大圆角盒叠放，轮廓方正且像玩具。

第四轮修改：曝光 `1.05 → 0.82`；Bloom 调整为强度 `0.2`、半径 `0.32`、阈值 `0.96`；发光材质改为参与 tone mapping 且强度不超过 2.4；主聚光、红色点光明显下调；两层主车身改为带端盖、平滑法线的纵向渐缩壳体。

独立截图结论：红漆渐变、轮胎、轮毂和前悬细节明显恢复，巨大红色光斑基本消失，轮廓较前版柔和；但地面中心仍有偏强青白光斑，车灯形态、生硬的外露轮组、座舱曲面和车身接缝仍明显是基础几何拼装，不能称为商业级或照片级资产。

建议：如果产品验收目标仍是“精美”，需要允许专业 GLTF/汽车曲面资产、UV/法线/粗糙度贴图或明确把目标降为“风格化程序化概念车”。继续只微调基础几何的边际收益有限。

回归建议：建立固定相机、固定颜色、固定曝光的桌面/移动端截图基线；对高亮区域占比、轮毂局部对比度和溢出做阈值检查，再由人工做造型验收。

### CAR-E2E-003：CDN 失败时永久停在加载页，错误面板永远不出现

- 严重度：P1
- 状态：未修复

复现：拦截 `https://cdn.jsdelivr.net/**` 后访问首页，等待 5 秒。

实际结果：8 个 Three.js/importmap 资源请求失败；页面仍为：

```text
loaderClass: loader
loaderVisibility: visible
loaderText: 正在唤醒性能机器
webgl-error.hidden: true
canvasCount: 1
```

原因：`src/main.js` 的静态 import 在模块执行前失败，应用内 WebGL try/catch 和错误层逻辑没有机会运行。

期望：依赖加载失败时在有限超时内隐藏 loader，显示“3D 资源加载失败”、重试按钮和联网要求；或将依赖本地化，避免运行时 CDN 单点依赖。

影响：离线、企业网络、区域性 CDN 故障或 CSP 限制下，用户看到永久加载，没有任何恢复路径。

建议与回归：优先本地打包固定版本依赖；若仍使用 CDN，增加非 module 的 watchdog 和模块成功握手事件。浏览器测试应拦截 CDN，并断言 5–10 秒内进入可理解的失败态且可以重试。

### CAR-E2E-004：缺少 favicon 导致控制台 404

- 严重度：P3
- 状态：第三轮已修复

初始真实 Chrome 控制台唯一错误为 favicon 404。第三轮新增本地 `favicon.svg` 和显式 `<link rel="icon">`；复验 `/favicon.svg` 为 200，桌面/移动端控制台无 error/warning。

### CAR-E2E-005：回归测试主要验证源码正则，不能证明真实视觉行为

- 严重度：P2
- 状态：未修复

第四轮 4 个 Node 测试覆盖：错误层源码契约、favicon 文件存在、曝光/Bloom/发光参数阈值、渐缩壳体函数和调用存在。这些测试能防参数和结构回退，但不能证明：

- 页面在真实模块加载后可见；
- WebGL 成功/失败两个分支的 computed style 正确；
- CDN 失败能退出 loader；
- 画面不过曝、轮廓确实更好；
- 移动端无重叠和交互可用。

建议：保留廉价源码测试，同时增加 Playwright 浏览器契约测试和少量固定截图视觉回归。源码正则不应被报告为“画质已验证”。本轮模型最终说明没有修后 GPU 截图，这一点措辞是诚实的。

## 9. 独立 Chrome 复验结果

### 9.1 成功路径

| 检查 | 1440 × 900 | 390 × 844 |
| --- | --- | --- |
| WebGL | 成功 | 成功 |
| 错误层 | hidden / display:none | hidden / display:none |
| Loader | done / hidden | done / hidden |
| 横向溢出 | 1440 = 1440，无 | 390 = 390，无 |
| 性能面板 | x=1068.09, y=297.12, 310×364 | x=14, y=636, 362×153 |
| 车漆按钮 | 4 个，点击后 active 正确切换 | 4 个，点击后 active 正确切换 |
| 重置按钮 | 可见并成功点击 | 按设计隐藏 |
| 拖拽 | 成功 | 成功 |
| console error/warning | 0 | 0 |
| request failed | 0 | 0 |

截图：

- 第四轮前：`desktop-red.png`、`mobile-red.png`
- 第四轮后：`desktop-v4-red.png`、`mobile-v4-red.png`
- CDN 故障：`cdn-blocked-v4.png`

以上文件位于临时工作区 `/tmp/luban-3d-car-e2e.VpjBki`。

### 9.2 失败路径

拦截 jsDelivr 后捕获 8 个 `net::ERR_FAILED`，页面 5 秒后仍停在 loader。CAR-E2E-003 稳定复现。

## 10. 自动化测试结果

### 10.1 生成物

```text
npm run check
✔ WebGL error overlay is hidden by default and remains available on failure
✔ page declares a favicon that exists, preventing the browser fallback 404
✔ highlight energy stays within the detail-preserving render profile
✔ main body uses tapered smooth shells instead of stacked full-length boxes
tests 4, pass 4, fail 0
```

另完成本地 HTTP `/`、`src/main.js`、`favicon.svg` 成功响应和未知路径 404 检查。

### 10.2 Luban 仓库相关包

```text
go test ./i18n ./commands ./internal/app ./internal/runtime/compact \
  ./internal/runtime/loop ./internal/store/session ./internal/ui/tui

ok github.com/agent-dance/luban/i18n
ok github.com/agent-dance/luban/commands
ok github.com/agent-dance/luban/internal/app
ok github.com/agent-dance/luban/internal/runtime/compact
ok github.com/agent-dance/luban/internal/runtime/loop
ok github.com/agent-dance/luban/internal/store/session
ok github.com/agent-dance/luban/internal/ui/tui
```

这些包全部通过，但没有覆盖本报告中真实 provider 兼容性、两分钟压缩等待态、摘要来源污染、摘要暴露过期审计路径和 ANSI 输出预算等集成问题。

## 11. 修复优先级建议

### 第一批：上下文可信度和主路径阻断

1. 修复 LUBAN-TUI-009：压缩摘要跨代延续的 role/provenance 污染。
2. 修复 LUBAN-TUI-001、002：provider/API format 预检与可操作错误。
3. 修复 LUBAN-TUI-007、008：参考 Claude Code 增加动态压缩进度条，并修正 running 状态和事件顺序。

### 第二批：长任务无摩擦体验

1. 修复 LUBAN-TUI-003、004：细分长请求和大型工具参数阶段，明确尺寸。
2. 修复 LUBAN-TUI-011：限制重绘帧率和 ANSI 字节量。
3. 修复 LUBAN-TUI-005、006：提前校验工具输入并改善 partial 原因。

### 第三批：文案与细节

1. 修复 LUBAN-TUI-010：保证摘要只暴露已校验、非空且与 manifest 一致的审计路径。
2. 修复 LUBAN-TUI-012、013：退出文案和回答篇幅校准。
3. 修复 LUBAN-TUI-014、015：流式中间帧和 token 来源解释。

### 生成物后续

1. 先修 CAR-E2E-003 的 CDN 永久加载。
2. 若仍要求“精美”，明确允许专业 3D 资产；否则把验收口径调整为风格化程序化概念车。
3. 用浏览器行为测试和截图回归补足 CAR-E2E-005，避免再次出现“静态测试全绿、真实页面被全屏遮住”的问题。

## 12. 最终验收判断

- GPT-5.6 sol high 实际调用：通过，但需要自定义 provider + chat-completions 绕过内置路径。
- 多轮连续开发：通过，共四轮且第二轮成功排队。
- 工具调用正确性：通过；无数据破坏，确定性失败均恢复。
- 工具调用无摩擦：不通过；补丁输入约束、长等待和进度反馈仍有明显摩擦。
- 基于 LLM 的自动压缩机制：核心能力通过；按指定口径由 `/compact` 触发，两代摘要、边界安装、上下文替换和持久化均自动完成。
- 上下文压缩节省量：通过，两代分别减少约 33.4% 和 39.9%。
- 上下文事实连续性：通过；两代均可无工具召回，第二代包含四轮事实和精确调光参数。
- 上下文来源和可恢复性：部分通过；内部父链和真实审计文件完整，但摘要存在角色污染，第一代曾暴露过期的 0 B genesis 路径。
- TUI 主体布局与 happy-path 中文：通过。
- TUI 长任务状态、退出/压缩文案和渲染效率：不通过。
- 真正 3D、拖拽、车漆和响应式：通过。
- “精美 3D 赛车”最终视觉：部分通过，明显改善但未达到商业级验收。
- 生成物网络故障降级：不通过。

建议在 LUBAN-TUI-001、007、009 和 CAR-E2E-003 关闭前，不把本次链路标记为“完整无摩擦 E2E 通过”；LUBAN-TUI-010 应作为恢复指引一致性问题一并修复，但不再视为内部审计数据丢失。
