# Provider 适配性评审（杠精版）

> 本文档从“真实适配不同供应商、不同模型”的角度，对当前项目的 Provider 实现做强批判式审查。
> 目标不是客气建议，而是把抽象层、能力声明、协议兼容和长期演进中的风险点挑明。

---

## 一、总评

一句话总评：

> 当前项目的 Provider 抽象，看起来像“统一适配多供应商”，本质上更像“以 Anthropic / OpenAI 两大语义为中心，对少数兼容厂商做表层兼容”，离“真实适配不同供应商、不同模型”还差一层正式的能力协商、语义降级和模型级特征矩阵。

更狠一点地说：

> 这是一个“工程上很会兼容”的实现，不是一个“抽象上真正完成多供应商适配”的实现。

它的主要优点是：

- 主流路径能跑
- Anthropic / OpenAI 两大阵营的流式协议都接上了
- OpenAI-compatible 生态能以较低成本接入
- 内部事件流统一得不错，利于 UI 和 agent loop 消费

但问题同样明显：

- 公共接口并不真正 provider-agnostic
- 很多能力按 provider 粗暴声明，而不是按 model 精确声明
- 对 OpenAI-compatible 的“兼容”更像乐观假设，而不是契约化适配
- `Capabilities()` 粒度过粗，容易制造“名义兼容”错觉
- 新协议 / 新供应商 / 新模型能力的接入路径还不够稳健

---

## 二、核心结论

### 2.1 Provider 抽象不够底层，公共参数已经泄漏供应商语义

`provider.Params` 里包含了：

- `SystemParts`
- `Thinking`
- `ToolChoice`
- `PreviousResponseID`
- `ReasoningEffort`
- `PromptCacheKey`
- `Truncation`

这些字段并不是真正中性的“统一抽象”，而是：

- 一部分偏 Anthropic 语义
- 一部分偏 OpenAI Responses 语义
- 一部分是为了兼容实际业务需求硬塞进公共层的控制字段

这会带来一个根本问题：

> 当前 Provider 抽象不是“定义了稳定中立的最小协议”，而是“定义了一份 Anthropic/OpenAI 视角下的超集参数集”。

这在短期内提高了复用率，但长期会让公共接口不断长出新的供应商专属字段。

### 2.2 当前更像“按供应商做兼容”，不是“按模型做能力协商”

例如 `OpenAIProvider.Capabilities()` 直接返回：

- `ToolUse: true`
- `Vision: true`
- `SystemParts: true`
- `Thinking: p.dialect == DialectDeepSeek`

问题在于：

> 真实世界里，能力差异往往首先发生在 model 级，而不是 provider 级。

同一供应商下，不同模型是否支持以下能力都可能不同：

- function/tool calling
- vision
- reasoning/thinking
- streaming usage
- parallel tool calls
- strict JSON schema
- 上下文窗口大小
- server-side conversation / chaining

如果只按 provider 给能力标签，就很容易把“某供应商有部分模型支持”误写成“该供应商整体支持”。

### 2.3 统一 StreamEvent 协议，并不等于真正适配了多供应商能力

当前实现把 Anthropic、OpenAI Chat、OpenAI Responses 的流式输出统一成内部 `types.StreamEvent`，这当然是必要的。

但必须指出：

> 统一输出事件流，只说明内部消费层可以共用，并不说明输入能力、参数语义、错误语义、工具契约已经真正统一。

也就是说，当前更接近“统一字幕格式”，不是“统一语言体系”。

---

## 三、分项批判

### 3.1 `Params` 是超集，不是抽象

当前 `Params` 的设计路线，本质上是：

1. 先定义一个尽量大的统一结构
2. 把常见供应商特性往里塞
3. 各 provider 自己挑字段解释

这会造成三个问题：

#### 问题 1：公共层充满供应商偏见

像 `PreviousResponseID`、`ReasoningEffort`、`PromptCacheKey`、`Truncation`，明显更接近 OpenAI Responses API 语义，而不是所有 Provider 的共同基础。

#### 问题 2：同一字段跨 provider 行为差异极大

`SystemParts` 就是典型例子：

- 在 Anthropic 中，它对应结构化 system blocks，且首段有 cache breakpoint 语义
- 在 OpenAI / Responses 中，它只是 `strings.Join(..., "\n\n")`

这不是“统一特性”，而是“同名不同义”。

#### 问题 3：未来接口会继续膨胀

一旦继续增加：

- parallel tool calls
- strict tool schema
- structured output
- response format profiles
- audio / document / image-first input

那么 `Params` 只会越来越像一个“跨供应商特性垃圾场”。

### 3.2 `SystemParts` 暴露出抽象是围绕 Anthropic 设计的

`SystemParts []string` 的注释已经说明：

- 第一段带 cache_control
- 后续段不带

这很明显是为了 Anthropic prompt cache 的结构设计出来的。

但在 OpenAI 侧，这个字段只是被拼接成字符串。于是出现了严重问题：

> 同一个上层字段，在不同 provider 下既不保结构，也不保语义，只保“输入内容大致相似”。

调用方如果认为 `SystemParts` 表示“多段系统提示”，那就是被接口骗了。

真正发生的是：

- Anthropic：多段结构 + 第一段缓存语义
- OpenAI：纯文本 join
- 其他兼容 provider：可能进一步退化成普通字符串 system message

这类设计会把 provider-specific 优化伪装成公共抽象，从而让跨供应商行为变得不可预测。

### 3.3 `Thinking` 能力声明不诚实，尤其是 DeepSeek 路径

Anthropic 的 thinking 支持是原生 API 能力，这没问题。

但 OpenAI 兼容链路中的 DeepSeek 处理逻辑本质上是：

- 从普通文本流中解析 `<think>...</think>` 标签
- 再把它映射成内部 `ThinkingBlock`

这当然很聪明，但语义上必须说清楚：

> 这不是“provider 原生支持 thinking block”，而是“基于模型输出格式做后处理模拟”。

问题包括：

- 依赖模型是否继续输出 `<think>` 标签
- 依赖代理层 / 网关是否保留这些标签
- 依赖 chunk 边界和文本模式足够稳定
- 一旦输出格式变化，统一 thinking 协议立即失真

所以把 Anthropic 原生 thinking 和 DeepSeek 文本标签解析都塞进 `Capabilities().Thinking = true`，会制造一种错误印象：

> 它们好像是同一种能力，只是不同 provider 的实现细节不同。

其实不是。一个是产品能力，一个更接近输出约定兼容。

### 3.4 `Capabilities()` 过于粗糙，像宣传页，不像契约

当前 `ProviderCapabilities` 包含：

- `Thinking`
- `ToolUse`
- `CacheControl`
- `SystemParts`
- `Vision`
- `MaxContext`

这几个字段的问题不在“有没有”，而在“过于粗、过于乐观”。

#### `ToolUse bool` 太含糊

它没有回答：

- 是否支持 tool choice = any / auto / specific
- 是否支持流式 arguments
- 是否支持 parallel tool calls
- 是否支持严格 schema
- 是否支持 tool result 的多段内容
- 是否支持工具与视觉同时启用

一个 `true` 根本不足以表达真实差异。

#### `Vision bool` 也太含糊

它没有说明：

- 支持 `image_url` 还是 base64 image
- 是否支持 pdf / doc 这类富输入
- 是否支持 mixed content blocks
- 是否支持多图
- 是否支持 vision + tool calling 同请求

#### `Thinking bool` 最危险

因为：

- 原生 thinking config
- 文本标签伪装的思维输出
- 仅支持 reasoning effort 但不外显 thinking
- 完全隐藏 chain-of-thought 的模型

都可能被误归并到一个布尔值里。

这类布尔能力声明会让上层误以为：

> 只要值为 true，语义就足够接近，可以统一使用。

实际上差别很大。

### 3.5 OpenAI-compatible 适配更像“乐观兼容”而非“契约兼容”

`OpenAIProvider` 的注释写着可兼容：

- OpenAI
- DeepSeek
- Ollama
- vLLM
- LiteLLM
- Azure OpenAI
- etc.

这句话很实用，也很危险。

因为所谓“OpenAI-compatible”在真实工程里常常只意味着：

- 某些 endpoint 名字兼容
- 某些基础字段兼容
- 某些 happy path 能回字

它绝不天然意味着以下全部兼容：

- SSE 事件格式
- usage 字段细节
- tool calling 参数结构
- finish_reason 语义
- error body 结构
- model capability matrix
- 请求头要求
- conversation chaining 语义
- cache / reasoning / response format 相关扩展字段

所以当前做法更像：

> 只要看起来像 OpenAI，我就先按 OpenAI 请求打过去；局部坏了再补 dialect patch。

这是工程上常见且有效的方式，但不能吹成“真实适配性强”。

### 3.6 Dialect 机制承认了差异，却没有系统建模差异

当前有：

- `DialectStandard`
- `DialectGemini`
- `DialectMistral`
- `DialectGroq`
- `DialectDeepSeek`
- `DialectOllama`

看起来像在做多后端行为建模，但现有实现中，真正有显著分支逻辑的主要是 DeepSeek `<think>` 解析。

这意味着：

> Dialect 目前更多是“差异标签”，不是“差异契约”。

一个成熟的 dialect/profile 机制，至少应覆盖：

- 请求字段差异
- tool choice 支持矩阵
- system 注入方式
- 流式事件差异
- finish reason 映射
- usage 可用性
- error 结构与 retry hint
- strict schema 支持
- vision 输入格式
- server-side conversation / thread 机制

否则 dialect 只是在为局部 patch 提供枚举名，而不是形成稳定的适配层。

### 3.7 错误处理与重试策略还停留在“通用猜测层”

`provider/errors.go` 的做法是：

- 按 HTTP 状态码做基本分类
- 按少量 `APIError.Type` 做补充判断
- 对网络层错误做 message substring 匹配

这在单一厂商里够用，但对“真实多供应商适配”仍然不够，原因有三：

#### 原因 1：错误语义并不统一

不同 provider / gateway 可能会：

- 把配额问题包装成 400、403、429 或 500
- 把网关错误包装成普通文本错误
- 完全不返回标准 JSON error 结构

#### 原因 2：当前重试判断太依赖 message 文本

例如：

- `connection refused`
- `tls handshake`
- `eof`
- `no such host`

这些只能算弱信号，不是稳定语义层。

#### 原因 3：缺乏 provider/model 特化 retry policy

真实场景里：

- 本地 Ollama 的重试策略应该很短
- 云服务的 429 应该看 retry-after 或 quota window
- 某些 5xx 实际是格式不兼容导致的假性服务端错误
- 某些 provider 支持模型回退或 endpoint 切换，某些不支持

当前实现虽然对 Ollama 给了更短 retry，但整体仍然是“统一重试壳 + 少量特判”，还没有形成 provider-aware 的错误语义层。

### 3.8 `NewFromEnv()` 的扩展方式比较散装

当前环境变量装配逻辑是一个越来越长的 switch：

- anthropic
- bedrock
- vertex
- openai
- ollama
- deepseek
- gemini
- groq
- mistral
- oauth

这当然实用，但从长期演进看问题明显：

#### 问题 1：新增 provider 需要同时改多处隐式规则

包括：

- env 变量命名
- 默认模型
- 默认 baseURL
- key 校验
- retry 策略
- dialect 推断
- provider constructor 路由

#### 问题 2：provider identity 和 transport preset 混在一起

像 deepseek / gemini / groq / mistral，目前主要是：

- 不同 baseURL
- 不同 API key env
- 同一个 `NewOpenAI` 核心实现

那它们到底是独立 provider，还是 OpenAI transport preset？

现在两种概念混在一起，导致对外像“支持很多 provider”，对内其实还是“OpenAI 家族的不同预设”。

#### 问题 3：自动探测可能与用户意图冲突

例如：

- 环境里只要有 `OPENAI_API_KEY` 就默认选 openai
- 否则默认 anthropic

这对新用户很友好，但当多个 key 并存时，系统行为可能只是“猜到了一个能跑的”，不一定是“猜中了用户真正想要的”。

### 3.9 Responses API 的加入，进一步暴露统一抽象的不稳定

当前 OpenAI 体系内部已经明显裂成两条协议线：

- Chat Completions
- Responses API

这本身说明：

> 同一个供应商内部，协议抽象都在快速演化，单一 `Provider` 接口正在承受越来越多协议差异。

`Params` 中的这些字段：

- `PreviousResponseID`
- `Conversation`
- `ReasoningEffort`
- `PromptCacheKey`
- `Truncation`

都在说明统一层已经开始为某条协议路线长出专属字段。

这通常意味着两种后果之一：

1. 公共 `Params` 继续膨胀
2. 未来不得不引入 provider-specific option blocks

不管哪种，都说明当前抽象尚未稳定。

### 3.10 `LookupMaxContext` 有用，但容易制造“假精确”

当前项目为常见模型维护了一个 context window 映射表，这当然比返回 0 好很多。

但如果从真实适配角度审视，它仍然有明显边界：

- 模型 context window 经常变化
- 同名模型在不同网关上的可用上限可能不同
- 推理预算、工具、图像、系统提示、输出上限都会影响真实可用窗口
- 兼容代理有时会人为缩小限制

所以 `MaxContext` 更适合被理解为：

- 经验值
- 默认值
- 提示值

而不是“稳定契约”。

如果缺少以下机制：

- runtime probe
- provider override
- model metadata refresh
- server-reported limits

那么这个能力字段只能算“粗略辅助信息”。

---

## 四、为什么说它现在是“可运行兼容”，不是“可信赖适配”

这里需要区分两个层级。

### 4.1 可运行兼容

含义是：

- 主流模型大多能跑通
- 常见 happy path 都有人工 patch 覆盖
- 输出可以归一化成统一事件流
- 出问题时可以持续打补丁修

### 4.2 可信赖适配

含义是：

- 明确知道每个模型支持什么、不支持什么
- 不支持的能力会提前拒绝或自动降级
- 参数映射有清晰规则，不会同名异义
- 错误和重试具备 provider-aware 语义
- 新增 provider / model 的接入方式可验证、可扩展

当前项目明显更接近第一种，而不是第二种。

这并不是在否定工程价值，而是在强调：

> 不要把“主流路径能跑”误当成“抽象层已经做好了真实多供应商适配”。

---

## 五、评分（批判视角）

如果满分 10 分代表“对真实不同供应商 / 不同模型的适配非常成熟”，当前实现大致可以这样打分：

| 维度 | 评分 | 说明 |
|------|------|------|
| 工程实用性 | 7.5/10 | Anthropic / OpenAI 主流路径已具备较高可用性 |
| 主流厂商可跑性 | 8/10 | 常见 happy path 基本覆盖 |
| 抽象纯度 | 5.5/10 | 公共参数层已经泄漏较多供应商语义 |
| 模型级能力适配严谨性 | 4.5/10 | 仍偏 provider-level 粗粒度声明 |
| 长期可扩展性 | 6/10 | 可继续加 patch，但架构张力已经出现 |
| OpenAI-compatible 稳健性 | 5/10 | 更像乐观兼容，不是严格契约兼容 |

综合评价：

> 6/10：能打，但远没到“适配性强得可以放心吹”的程度。

---

## 六、最毒舌的总结

最后用一句最毒舌的话概括：

> 现在这套 Provider 实现，不像“真正面向多供应商设计的统一抽象”，更像“围绕 Anthropic 和 OpenAI 两极建立的兼容层，再给其他厂商贴上 OpenAI-compatible 标签后尽量接进去”。

或者更简化成一句：

> 能跑是事实，但把“能跑”包装成“高适配”就有点自信过头了。

---

## 七、附：如果要把它从“能跑”升级到“真适配”，应该补什么

虽然本文主基调是批判，但为了后续行动方便，这里给出一个最小建设性清单。

### 7.1 能力声明从 provider-level 升级到 model-profile / backend-profile

建议把粗粒度布尔能力拆成更明确的契约，例如：

- ToolUse.Mode = none / basic / streaming / strict / parallel
- Thinking.Mode = none / native_blocks / reasoning_only / text_tag_emulated
- Vision.Inputs = image_url / base64 / pdf / mixed_blocks
- Conversation.Mode = stateless / previous_response_id / server_thread
- Cache.Mode = none / provider_cache_control / server_prefix_cache / sticky_affinity

### 7.2 区分“公共抽象参数”与“协议特有参数”

例如：

- 保留最小公共参数：model、messages、tools、tool choice、max tokens 等
- 把 Responses API 特有字段放入专属 option block
- 把 Anthropic prompt cache / thinking 特性放入专属 option block

### 7.3 把 dialect 从枚举升级为真正的 profile 策略对象

至少应能集中定义：

- 请求改写策略
- 流式事件解析策略
- finish_reason 映射
- usage 解释方式
- 错误解析与 retry hint
- 降级规则

### 7.4 对“不支持的能力”显式失败或降级

不要只在 happy path 里假设支持，应该在请求前就能判断：

- 当前 model 是否支持工具
- 是否支持 vision + tools 共存
- 是否支持 system parts
- 是否支持强制 tool choice
- 是否支持 reasoning 配置

### 7.5 建立 provider × model 的验证矩阵

不是只有单测，还包括：

- 请求格式兼容性
- stream 事件一致性
- tool calling 路径
- usage 字段可用性
- 错误码和 retry 行为
- 不支持能力时的失败模式

只有这样，适配性才能从“经验”变成“证据”。
