# WebFetch 对齐实施计划

> 对应源码：`tools/web.go`
> 
> 目标：让 `gosrc` 中的 `WebFetch` 工具在接口、行为语义、产品集成、权限体验和可观测性上，尽可能与原版 `../src/tools/WebFetchTool/WebFetchTool.ts` + `prompt.ts` 对齐；同时明确哪些地方无法 1:1 对齐，以及要通过什么架构手段才能真正实现高保真对齐。

---

## 1. 现状与原版差异概览

### 当前 Go 实现的本质
当前 `tools/web.go` 中的 `WebFetchTool` 本质是一个：
- 本地 HTTP GET 抓取器
- 加上 SSRF 防护、域名限制、重定向检查
- 对 HTML 进行简化 strip
- 返回 `Prompt: ...` + 页面文本
- 使用本地 TTL cache

### 原版 TypeScript 实现的本质
原版 `src/tools/WebFetchTool/WebFetchTool.ts` + `prompt.ts` 更接近：
- 由 Claude 原生工具框架承载的网页抓取/提取能力
- `prompt` 参与提取目标定义，而不是简单回显
- 行为受专门 prompt 控制
- 返回结果更偏“与用户问题相关的网页信息”，而不是全文 dump

### 核心结论
要真正对齐，不能只继续微调当前“抓网页 + stripHTML”的路径；必须把 `WebFetch` 从“抓取工具”升级为“受提示词驱动的网页信息提取工具”。

---

## 2. 对齐目标定义

### L1：接口层对齐
确保：
- 输入字段兼容原版
- 错误格式稳定
- 返回结构可被上层工具框架一致消费

### L2：行为语义对齐
确保：
- `prompt` 真正影响提取结果
- 返回的是“相关信息”，而不是无差别原文
- 页面结构、链接、代码块、标题尽量保真

### L3：产品层对齐
确保：
- 与权限系统、TUI 展示、ToolResult block、日志/调试体系对齐
- 与其他工具保持一致的用户体验

### L4：高保真原版对齐
确保：
- 在 provider 支持时，能够走与原版更接近的 provider-backed fetch/extract 流程
- 本地抓取仅作为 fallback，而不是唯一实现

---

## 3. 真正实现对齐所需的目标架构

真正的高保真对齐建议采用“双路径架构”：

### 路径 A：Provider-backed Fetch（高保真路径）
适用场景：
- 当前 provider / model 支持原版式 WebFetch 能力
- 希望获得与原版更一致的结果语义

做法：
1. 将 `WebFetch` 抽象为“网页内容提取能力”，而不是“HTTP 下载能力”
2. 在 provider 层定义可选能力接口，例如：
   - `SupportsWebFetch()`
   - `ExecuteWebFetch(ctx, url, prompt, options)`
3. 在 provider 可用时，优先走 provider-backed 提取
4. 将结果统一转换为 Go 侧工具返回结构

### 路径 B：Local Fetch + Local Extraction（降级路径）
适用场景：
- provider 不支持原生 WebFetch
- 离线或受限环境

做法：
1. 抓取网页原始内容
2. 用更高质量的正文提取器抽正文
3. 再根据 `prompt` 做相关内容提取/摘要
4. 输出与高保真路径尽量一致的结果格式

### 关键原则
- 工具对外接口不变
- 内部执行器可切换
- Provider-backed 是“主路径”，本地抓取是“fallback”

---

## 4. 分阶段完整实施计划

## Phase 0：对齐基线文档与测试固化

### 目标
在改代码前，把现有行为和目标行为固定下来，避免后续改动失控。

### 工作项
1. 为 `WebFetch` 增加差异说明测试用例清单
2. 记录以下行为基线：
   - 空 URL / 空 prompt 报错
   - SSRF 拦截
   - redirect 域名校验
   - HTML strip 行为
   - 缓存命中行为
3. 补充 golden-style 输出样例

### 建议新增测试
- `TestWebFetch_CacheKey_CurrentBehavior`
- `TestWebFetch_PromptAffectsResult_TargetBehavior`
- `TestWebFetch_RedirectPolicy`
- `TestWebFetch_HTMLExtractionQuality`

### 关联文件
- `tools/web_test.go`
- 如有统一测试 helpers，也可能涉及 `tools/helpers_test.go`

---

## Phase 1：修正明显不对齐的正确性问题

### 1.1 缓存键纳入 prompt

#### 当前问题
当前缓存 key 仅为 URL。不同 prompt 请求同一 URL，会错误复用相同内容。

#### 对齐目标
缓存至少区分：
- URL
- prompt（标准化后）
- 可选的提取模式/version

#### 建议实现
新增内部 helper：
- `makeWebFetchCacheKey(url, prompt string, opts fetchExecutionOptions) string`

推荐格式：
- JSON 序列化后 hash
- 不建议直接字符串拼接，避免长度和转义问题

#### 验收标准
- 同 URL 不同 prompt 不共享最终提取结果缓存
- 若采用“两层缓存”，原始页面缓存可仍按 URL 复用

---

### 1.2 拆分“页面抓取缓存”与“提取结果缓存”

#### 原因
真正对齐后，`prompt` 会影响提取结果，但原始页面正文不一定需要重复抓取。

#### 建议设计
- Layer 1: Raw page cache
  - key = normalized URL
  - value = fetched raw/cleaned document
- Layer 2: Extraction cache
  - key = URL + prompt + extraction options
  - value = final extracted answer

#### 收益
- 降低网络请求次数
- 避免 prompt 语义错乱
- 为后续“多种提取器”打基础

---

### 1.3 返回结构从纯字符串向结构化结果演进

#### 当前问题
当前返回是简单字符串拼接：
- `Prompt: ...\n\n---\n\n<content>`

#### 对齐目标
即使对外仍返回文本，也应先在内部生成结构化结果：
- source URL
- title
- extracted summary
- relevant snippets
- citations
- raw text length / truncated flag

#### 建议实现
新增内部结构体：
- `type webFetchResult struct { ... }`

然后由一个 formatter 统一映射为 ToolResult 文本。

#### 收益
- 便于后续接入 TUI richer rendering
- 便于 provider-backed 与 fallback 统一

---

## Phase 2：让 prompt 真正驱动提取结果

这是 WebFetch 真正对齐原版的核心阶段。

### 2.1 引入“网页正文提取”层，而不是直接 stripHTML

#### 当前问题
当前 `stripHTML` 仅删除标签，不是真正的正文提取。

#### 必须补齐的能力
- 识别正文区域
- 降低导航栏/页脚/广告污染
- 保留标题层级
- 保留代码块、列表、表格的基本结构
- 链接保留可追溯性

#### 可选方案

##### 方案 A：Go 本地可用的 readability / boilerplate removal 库
优点：
- 纯本地
- 实现成本可控

缺点：
- 与原版仍不是完全同源

##### 方案 B：自己实现轻量 DOM 清洗 + 语义块提取
优点：
- 可控

缺点：
- 成本高，效果不稳定

#### 推荐
优先采用成熟正文提取库，再补业务规则。

### 2.2 引入“相关片段提取器”

#### 当前问题
页面抓下来后，`prompt` 不影响内容选择。

#### 目标行为
给定：
- URL
- cleaned document
- prompt

输出：
- 与 prompt 最相关的若干片段
- 可选 summary
- 可选 section 标题

#### 可选方案

##### 方案 A：规则/启发式提取
例如：
- 关键词分词
- 段落打分
- 标题邻域扩展
- 最相关 top-k 段落

优点：
- 纯本地、易控

缺点：
- 语义理解弱

##### 方案 B：调用当前模型做本地二次抽取
流程：
1. 工具抓取并清洗网页
2. 将正文 + prompt 交给模型做 extraction summarization
3. 返回结构化结果

优点：
- 语义更接近原版

缺点：
- 增加 token 和复杂度

#### 真正对齐建议
要“真正实现对齐”，推荐采用分层策略：
- 默认：启发式筛选降低上下文量
- 高质量模式：模型二次抽取

这才接近原版“prompt-guided web extraction”的语义。

### 2.3 增加提取模式配置

建议引入内部 options：
- `ExtractionModeRawText`
- `ExtractionModeRelevantSnippets`
- `ExtractionModeStructuredSummary`
- `ExtractionModeProviderNative`

最终 `WebFetch` 对外仍叫 `WebFetch`，但内部执行模式可根据 provider 与配置切换。

---

## Phase 3：补齐原版 prompt wiring

### 当前问题
Go 版没有 `prompt.ts` 对应物，缺少工具专用提示工程。

### 对齐目标
建立 Go 侧的 WebFetch prompt policy，使其在：
- provider-backed 路径
- 本地模型二次提取路径
中都使用统一的“工具专用系统提示”。

### 建议实现
新增同名文档/模板来源，例如：
- `tools/web_fetch_prompt.go`
- 或配置化模板文件

建议包含：
1. 只提取与 prompt 相关的信息
2. 优先保留原文事实，不编造
3. 返回时尽量保留来源上下文
4. 如页面信息不足，明确说明不足
5. 如内容过长，优先返回最相关片段与摘要

### 为什么这是“真正对齐”所必须的
因为原版并不是一个裸 HTTP 工具，而是一个“带工具专用行为约束的能力”。没有专用 prompt policy，就只是模仿接口，不是对齐语义。

---

## Phase 4：补齐产品集成层

## 4.1 权限与策略系统对齐

### 当前
WebFetch 仅有工具内部的 SSRF / 域名限制，没有完整产品层权限体验。

### 目标
接入现有 permissions 系统，让 WebFetch：
- 能被规则允许/拒绝
- 能在交互式场景中提示用户
- 能在非交互式场景中按照策略自动处理

### 可能涉及文件
- `permissions/*`
- `registry_setup.go`
- 与 tool policy 相关的 wiring 文件

### 实施要点
- 工具名级别策略：`WebFetch`
- URL 风险等级评估
- 是否允许外网访问、指定域名访问、重定向访问
- 审计记录中记录 URL 与最终落点域名

## 4.2 ToolResult / UI 展示对齐

### 当前
返回纯文本，UI 无 richer semantics。

### 目标
支持：
- 展示“正在抓取 URL”
- 展示“已提取正文/相关片段”
- 若 provider-backed，展示与原版类似的阶段状态

### 可能涉及文件
- `tui/renderer.go`
- `tui/state.go`
- `provider/responses.go`
- Tool result 映射逻辑相关文件

### 建议
即使第一阶段不做流式，也至少让结果文本包含：
- Source URL
- Title
- Summary
- Relevant sections
- Truncated indicator

---

## Phase 5：实现 provider-backed 高保真对齐

这是“真正实现对齐”的关键阶段。

### 5.1 在 provider 层定义 WebFetch 能力抽象

建议新增抽象，例如：
- `type WebFetchProvider interface { ... }`
- 或在现有 provider capability 中增加 web fetch support

需要表达的能力：
- 是否支持 provider-native WebFetch
- 是否支持 prompt-guided extraction
- 是否支持 structured citations

### 5.2 在工具层增加调度器

`WebFetchTool.Execute` 不应直接只有一条本地执行路径，而应：
1. 解析输入
2. 权限校验
3. 根据 provider capability 决定：
   - provider-native path
   - local fallback path
4. 统一格式化输出

### 5.3 对齐输出 contract

无论执行路径如何，最终结果都应统一字段：
- `url`
- `resolvedURL`
- `title`
- `summary`
- `snippets[]`
- `citations[]`
- `truncated`
- `fetchMethod`（provider/local）

### 为什么这是必须的
如果不引入 provider-backed path，Go 版永远只是“一个像 WebFetch 的替代工具”，而不是对齐原版的 WebFetch。

---

## 5. 完整文件级实施清单

## 核心修改文件
- `tools/web.go`
  - 拆分 WebFetch 执行逻辑
  - 新增 cache key helper
  - 引入 structured result
  - 引入 execution mode dispatch
  - 引入 raw/extraction 双层缓存

## 建议新增文件
- `tools/web_fetch_alignment.md`（本文档）
- `tools/web_fetch_prompt.go`
- `tools/web_fetch_extract.go`
- `tools/web_fetch_provider.go`
- `tools/web_fetch_types.go`
- `tools/web_fetch_cache.go`（如果想把 cache 从 `web.go` 中拆分）

## 可能需要修改的周边文件
- `registry_setup.go`
  - 注入 provider capability / config
- `provider/responses.go`
  - 如需接 provider-backed fetch 结果事件
- `permissions/*`
  - 对齐工具权限控制
- `tui/*`
  - richer rendering / progress display
- `tools/web_test.go`
  - 大量新增测试

---

## 6. 测试计划

## 单元测试
1. 输入校验
2. cache key 区分 prompt
3. raw cache 与 extraction cache 行为
4. redirect 域名限制
5. provider-backed dispatch 优先级
6. fallback path 输出结构
7. 正文提取质量基本断言
8. 相关片段提取稳定性

## 集成测试
1. provider 不可用 → local fallback
2. provider 可用 → native fetch path
3. 权限拒绝时的工具输出
4. TUI / renderer 对结构化结果的展示

## 回归测试
- SSRF 保护不退化
- 超大页面截断仍生效
- 非 HTML 内容仍可安全返回

---

## 7. 风险与取舍

### 风险 1：过度追求 1:1 还原导致架构复杂化
应对：
- 先统一 contract，再统一 execution path

### 风险 2：本地启发式提取质量不稳定
应对：
- 增加模型二次提取模式
- 保留 provider-native path 作为高保真路线

### 风险 3：权限、UI、provider 三层同时改动过大
应对：
- 分阶段推进，先修正确性，再补集成

### 风险 4：性能与 token 成本上升
应对：
- raw cache + extraction cache
- 默认启发式，必要时模型抽取

---

## 8. 最小可落地路线图（推荐）

### Milestone 1：修正确性
- prompt 纳入 cache key
- raw/extraction 双层缓存
- structured result 内部表示

### Milestone 2：修语义
- 正文提取器
- 相关片段提取器
- prompt policy

### Milestone 3：修产品集成
- permissions wiring
- richer result rendering

### Milestone 4：高保真对齐
- provider-backed WebFetch
- 统一输出 contract

---

## 9. 完成标准（Definition of Done）

只有满足以下条件，才可认为 `WebFetch` 已“真正实现对齐”：

1. `prompt` 会显著影响返回内容，而不是仅被回显
2. 返回结果以“相关信息提取”为主，而不是全文 dump
3. 本地 fallback 与 provider-native path 输出 contract 一致
4. 具备与原版相近的 prompt policy / 行为约束
5. 接入权限系统和可观测工具结果展示
6. 缓存策略不会因 prompt 差异产生错误复用
7. SSRF 与域名策略仍不退化

---

## 10. 结论

`WebFetch` 的真正对齐，不是简单改几个字段或调一下返回格式，而是要完成一次能力升级：

- 从“本地网页下载器”
- 升级为“带 prompt 语义、可 provider-backed、带 fallback 的网页信息提取工具”

如果后续实施时资源有限，建议优先顺序为：
1. cache 正确性
2. prompt 驱动提取
3. 正文提取质量
4. provider-backed path
5. 权限与 UI 对齐
