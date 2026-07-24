# WebSearch 对齐实施计划

> 对应源码：`tools/web.go`
> 
> 目标：让 `gosrc` 中的 `WebSearch` 工具在能力来源、行为语义、权限体验、流式反馈、输出格式和 provider 集成上，尽可能与原版 `../src/tools/WebSearchTool/WebSearchTool.ts` + `prompt.ts` 对齐；并明确要怎样做，才能从“DuckDuckGo 替代实现”真正升级为“原版兼容的 WebSearch 工具”。

---

## 1. 现状与原版差异概览

### 当前 Go 实现的本质
当前 `tools/web.go` 中的 `WebSearchTool` 本质是一个：
- 本地 web search 适配器
- 优先调用 DuckDuckGo Instant Answer API
- 失败时 fallback 到 DuckDuckGo Lite HTML
- 本地做 domain filtering
- 返回 markdown link + snippet 列表
- 使用本地 TTL cache

### 原版 TypeScript 实现的本质
原版 `src/tools/WebSearchTool/WebSearchTool.ts` + `prompt.ts` 更接近：
- Claude 官方 server-side web search tool 的封装
- 通过 `queryModelWithStreaming()` 驱动
- 工具调用期间会产生进度事件
- 支持 provider/model gating
- 支持 permission wiring
- 有专门 prompt policy
- 输出带有引用要求和结构化结果语义

### 核心结论
要真正对齐，不能继续把 DDG 版本当作主实现；必须把 `WebSearch` 重构为“以 provider-native web search 为主、以本地 search fallback 为辅”的双路径工具。

---

## 2. 对齐目标定义

### L1：接口层对齐
确保：
- 输入字段与原版兼容
- 输出结构足以表达原版结果语义
- 错误格式与权限行为一致

### L2：行为语义对齐
确保：
- 搜索能力来源在支持场景下与原版一致
- 支持域名约束、来源引用、结果摘要
- 不只是“搜到若干链接”，而是“得到可引用的搜索结果能力”

### L3：产品层对齐
确保：
- 有 streaming/progress 体验
- 有 provider gating
- 有 permissions 集成
- 有 tool-specific prompt

### L4：高保真原版对齐
确保：
- 在支持的 provider/model 下走原生 WebSearch server tool
- 输出与原版 tool result contract 对齐
- DDG 仅作为 fallback，不再是主路径

---

## 3. 真正实现对齐所需的目标架构

真正的对齐建议采用“三层架构”：

## 第一层：统一 WebSearch Contract
在工具内部建立统一结果表示，不论搜索来源是什么，最终都映射到同一种结构。

建议新增内部结构体：
- `webSearchRequest`
- `webSearchResult`
- `webSearchCitation`
- `webSearchExecutionMetadata`

至少应包含：
- query
- results
- citations
- durationSeconds
- searchMethod（provider_native / local_ddg / local_other）
- raw provider events（可选）
- truncated / partial flags

## 第二层：执行器抽象
为 `WebSearch` 定义执行器接口，例如：
- `type webSearchExecutor interface { Execute(ctx, req) (...) }`

至少提供两个实现：
1. `providerNativeWebSearchExecutor`
2. `localFallbackWebSearchExecutor`

## 第三层：调度器
`WebSearchTool.Execute` 只负责：
1. 输入校验
2. 权限检查
3. provider capability 检查
4. 选择执行器
5. 统一格式化结果
6. 上报 progress / telemetry

### 为什么这是必须的
如果没有统一 contract + executor 抽象，后续加入 provider-native path 时，Go 版会出现两套完全不同的行为分叉，无法真正对齐原版。

---

## 4. 分阶段完整实施计划

## Phase 0：先固化现状与目标差异

### 目标
在正式改造前，把“当前 DDG 实现的行为”和“原版预期行为”都固化成文档与测试基线。

### 工作项
1. 记录当前输入校验、缓存、过滤、fallback 行为
2. 为原版关键行为建立待对齐测试清单：
   - allowed/blocked 互斥
   - provider gating
   - progress 事件
   - source citation 要求
   - structured output

### 建议新增测试名称
- `TestWebSearch_CurrentCacheBehavior`
- `TestWebSearch_TargetBehavior_AllowedBlockedExclusive`
- `TestWebSearch_TargetBehavior_ProviderPreferred`
- `TestWebSearch_TargetBehavior_ProgressEvents`

### 关联文件
- `tools/web_test.go`
- 若涉及 provider 流式响应测试，也可能扩展到 `provider/*_test.go`

---

## Phase 1：修正明显不对齐的正确性问题

## 1.1 输入校验对齐：禁止同时传 `allowed_domains` 与 `blocked_domains`

### 当前问题
Go 版允许两者共存；原版显式禁止。

### 对齐目标
若输入同时包含两者，直接返回 ToolResult error。

### 原因
- 统一调用约束
- 避免 domain filtering 语义歧义
- 与原版调用层保持一致

### 验收标准
- 该输入组合在 Go 与原版表现一致

---

## 1.2 缓存键纳入 domain filters 与 execution mode

### 当前问题
缓存 key 只按 query，会导致：
- 同 query + 不同 allowed_domains 错误复用
- 同 query + 不同 blocked_domains 错误复用
- 将来 provider-native 与 local fallback 结果也可能混淆

### 对齐目标
缓存键至少包含：
- normalized query
- sorted allowed_domains
- sorted blocked_domains
- execution mode / provider mode
- 可选 schema version

### 建议实现
新增 helper：
- `makeWebSearchCacheKey(req webSearchRequest) string`

建议使用：
- 规范化结构序列化 + hash

### 验收标准
- 不同过滤条件不会互相污染缓存
- provider-native 与 fallback 结果不会串缓存

---

## 1.3 结果内部结构化

### 当前问题
直接把最终结果拼成 markdown 字符串，不利于后续对齐。

### 对齐目标
先形成统一结构：
- query
- results[]
- citations[]
- durationSeconds
- method
- partial/truncated info

### 收益
- 可同时适配 provider-native 和 fallback
- 方便 UI、日志、tool_result block、测试断言

---

## Phase 2：让工具具备“原版式”行为语义

这是 WebSearch 真正走向原版兼容的核心阶段。

## 2.1 引入 provider/model gating

### 当前问题
Go 版不区分 provider/model，一律走 DDG。

### 原版行为
原版根据 provider/model 决定 WebSearch 是否启用。

### 对齐目标
建立能力判断逻辑：
- provider 是否支持 server-side web search
- 当前 model 是否在支持列表中
- 若不支持，则 fallback 或禁用

### 建议实现
在 provider capability 层新增：
- `SupportsWebSearch(ctx) bool`
- 或 `WebSearchCapability() CapabilityDescriptor`

### 可能涉及文件
- `provider/*`
- `registry_setup.go`
- `tools/web.go`

### 验收标准
- 支持原生 WebSearch 的 provider/model 会优先走原生路径
- 不支持的环境才 fallback

---

## 2.2 接入 provider-native WebSearch 执行路径

### 当前问题
Go 版完全没有原生 WebSearch 路径。

### 目标
在 provider 支持时，通过 provider 发起与原版等价的 web search 请求。

### 真正对齐需要做到什么
1. 构造与原版一致或尽量接近的 server tool 调用参数
2. 支持增量事件读取
3. 捕获 tool result blocks
4. 组装统一输出结果

### 关键设计
建议新增执行器：
- `providerNativeWebSearchExecutor`

它负责：
- 构造 web search tool request
- 发起 streaming query
- 收集 server tool use 和结果
- 产出结构化 `webSearchResult`

### 这一步为什么是“真正实现对齐”的关键
因为只要主路径还是 DDG，本质上就不是原版 WebSearch，只是语义接近的替代品。

---

## 2.3 保留并重构本地 fallback 搜索路径

### 当前问题
本地 DDG 搜索路径现在是主实现，且是“搜后过滤”。

### 对齐目标
把它降级为 fallback，同时增强其行为，使其更接近原版。

### 重构要求
1. 将 fallback executor 独立出来
2. domain filter 尽量前置，而不只是后置
3. 输出结构与 provider-native path 统一
4. 支持 richer metadata

### 关于 domain 约束前置化
若 fallback 仍基于搜索引擎 query，可以考虑：
- 对 `allowed_domains` 生成 `site:` 查询增强
- 对 `blocked_domains` 做后置过滤，但在结果中标明过滤发生

### 验收标准
- fallback 不再是主路径
- fallback 输出可被统一 formatter 处理

---

## Phase 3：补齐流式进度与事件语义

## 3.1 增加 WebSearch progress 事件模型

### 当前问题
Go 版是同步返回，无进度体验。

### 原版行为
原版 WebSearch 在执行中会产生：
- query 更新
- 搜索结果接收
- 可能多轮工具调用

### 对齐目标
定义 Go 侧事件模型，例如：
- `WebSearchStarted`
- `WebSearchQueryIssued`
- `WebSearchResultsReceived`
- `WebSearchCompleted`
- `WebSearchFallbackActivated`

### 实施建议
- 在 tools 执行上下文中提供 progress emitter
- executor 执行过程中上报事件
- TUI / renderer 消费这些事件

### 可能涉及文件
- `tools/web.go`
- `provider/responses.go`
- `tui/renderer.go`
- `tui/state.go`
- 可能还有 tool event 类型定义处

---

## 3.2 TUI 展示与 tool result rendering 对齐

### 当前
只有最终字符串结果。

### 目标
至少支持展示：
- 正在搜索的 query
- 是否走 provider-native 还是 fallback
- 已收到多少结果
- 最终结果摘要与引用来源

### 高保真方向
若响应体系允许，应尽量映射到与原版相近的 tool progress UX。

---

## Phase 4：补齐权限与工具策略系统

## 4.1 permissions wiring

### 当前问题
WebSearch 还没有原版式 permission check 体验。

### 对齐目标
让 `WebSearch` 接入现有权限系统：
- 工具是否允许调用
- 是否允许访问外部网络搜索
- 是否对某些域名/类别做策略限制
- 交互模式下是否询问用户

### 可能涉及文件
- `permissions/*`
- `registry_setup.go`
- `tools/web.go`

### 建议
权限系统中至少支持：
- 工具级规则：`WebSearch`
- 网络搜索级规则：external web search
- 域名限制策略复用

---

## 4.2 审计与可观测性

### 目标
记录：
- query
- 是否使用 provider-native
- fallback 原因
- 过滤规则
- 结果数
- 耗时

### 价值
- 调试对齐偏差
- 分析 provider 支持率
- 发现 fallback 质量问题

---

## Phase 5：补齐原版 prompt wiring 与引用约束

## 5.1 引入 WebSearch 专用 prompt policy

### 当前问题
Go 版没有 `getWebSearchPrompt()` 对应逻辑。

### 对齐目标
建立工具专用提示策略，用于：
- provider-native path 的 tool 调用上下文
- fallback path 的结果格式化/二次摘要

### prompt policy 至少应约束
1. 优先返回与用户问题最相关的来源
2. 引用必须可追溯到 URL
3. 不要把搜索结果当事实，需以来源为准
4. 若结果不足，明确说明不确定性
5. 回答中优先使用 markdown hyperlink 引用来源

### 为什么这是必须的
原版 WebSearch 不只是拿回链接，而是通过 prompt policy 约束模型如何使用搜索结果。没有这层策略，就只是“做了搜索”，不是“对齐了 WebSearch”。

---

## 5.2 补齐引用输出 contract

### 当前问题
Go 版只是输出 markdown 链接列表，没有强约束模型引用来源。

### 对齐目标
结果结构中显式保留：
- title
- url
- snippet
- citation-ready text
- source ranking / relevance

并在 tool result 文本中加入类似原版的引用提醒。

### 建议
新增统一 formatter：
- `formatWebSearchToolResult(result webSearchResult) types.ToolResult`

它负责：
- 输出摘要
- 输出引用来源
- 加入“最终回答应引用来源”的约束提示

---

## Phase 6：输出结构与调用契约对齐

## 6.1 统一内部结果结构

建议内部结果最少包含：
- `Query string`
- `Results []WebSearchItem`
- `DurationSeconds float64`
- `Method string`
- `Partial bool`
- `FallbackReason string`
- `AppliedAllowedDomains []string`
- `AppliedBlockedDomains []string`

每个 result item 建议包含：
- `Title`
- `URL`
- `Snippet`
- `SourceType`
- `RelevanceScore`（可选）

## 6.2 统一对外输出映射

即便对外仍使用 `types.ToolResult{Content: ...}`，内部也应统一由 formatter 输出。
这样后续：
- UI
- agent
- tests
- structured logging
都能共享同一 contract。

---

## 5. 完整文件级实施清单

## 核心修改文件
- `tools/web.go`
  - 拆出 WebSearch 逻辑
  - 输入校验对齐
  - cache key 改造
  - dispatch / executor 模式引入
  - 结构化结果与 formatter

## 建议新增文件
- `tools/websearch.md`（本文档）
- `tools/web_search_types.go`
- `tools/web_search_executor.go`
- `tools/web_search_provider.go`
- `tools/web_search_fallback.go`
- `tools/web_search_prompt.go`
- `tools/web_search_format.go`

## 可能需要修改的周边文件
- `registry_setup.go`
  - 注入 provider capability / config
- `provider/*`
  - 支持 provider-native web search
- `provider/responses.go`
  - 若需流式 tool event 对接
- `permissions/*`
  - WebSearch 权限规则
- `tui/*`
  - progress rendering / richer result display
- `tools/web_test.go`
  - 扩展测试

---

## 6. 测试计划

## 单元测试
1. query 为空报错
2. allowed/blocked 互斥校验
3. cache key 包含 domains 与 mode
4. fallback executor 的 query 构造
5. provider-native executor 的 dispatch 条件
6. formatter 输出引用提示
7. domain filtering 与前置 site: 注入逻辑

## 集成测试
1. provider 支持 → native path
2. provider 不支持 → fallback path
3. provider native 失败 → fallback path（若策略允许）
4. 权限拒绝行为
5. progress event 上报行为
6. TUI 渲染行为

## 回归测试
- DDG fallback 仍能工作
- 缓存不会因 query 相同而错误复用不同域过滤结果
- provider 不可用时不影响整体稳定性

---

## 7. 风险与取舍

### 风险 1：provider-native 路径接入复杂
应对：
- 先建立 executor 抽象与统一 contract，再逐步接 provider

### 风险 2：fallback 与 native 结果差异较大
应对：
- 输出中标记 method
- 使用统一 formatter 降低表现差异

### 风险 3：流式事件会牵动 TUI 和 provider 多层改造
应对：
- 先定义事件模型，再逐步接线

### 风险 4：搜索结果引用规范落实不到最终回答
应对：
- 在 tool result 中显式加入引用约束提示
- 在上层回答链路中增加 citation checks（如有能力）

---

## 8. 最小可落地路线图（推荐）

### Milestone 1：修正确性
- allowed/blocked 互斥校验
- cache key 纳入 domains + mode
- 结构化内部结果

### Milestone 2：修架构
- executor 抽象
- provider gating
- fallback executor 独立

### Milestone 3：修体验
- progress events
- richer result formatting
- prompt policy
- 引用约束

### Milestone 4：高保真对齐
- provider-native WebSearch 主路径上线
- DDG 降级为 fallback
- 权限与 TUI 全链路对齐

---

## 9. 完成标准（Definition of Done）

只有满足以下条件，才可认为 `WebSearch` 已“真正实现对齐”：

1. 在支持的 provider/model 下，默认走 provider-native web search，而不是 DDG
2. provider 不支持时才走 fallback，本地 fallback 结果 contract 与 native 一致
3. 输入约束与原版一致，包括 allowed/blocked 互斥
4. 结果中显式保留来源与引用信息
5. 有 tool-specific prompt policy，约束如何使用搜索结果
6. 具备 progress / streaming 事件或等价体验
7. 接入权限系统与策略控制
8. 缓存策略不会因过滤条件或执行模式差异产生错误复用

---

## 10. 结论

`WebSearch` 的真正对齐，本质上不是“把 DDG 搜索再打磨一下”，而是要完成一次架构转换：

- 从“本地搜索引擎适配器”
- 升级为“以 provider-native web search 为主、fallback 为辅、具备 prompt/权限/UI/引用体系的统一搜索工具”

如果后续实施资源有限，建议优先顺序为：
1. 输入约束与缓存正确性
2. executor 抽象与 provider gating
3. provider-native path
4. 进度事件与引用格式
5. permissions / TUI 全链路对齐
