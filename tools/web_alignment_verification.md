# WebFetch / WebSearch 对齐验证方案

> 适用范围：`tools/web.go` 中 `WebFetch` 与 `WebSearch`
>
> 目标：建立一套可实施、可重复、可审计的验证体系，用于判断 `gosrc` 中的 WebFetch / WebSearch 是否真正对齐 `../src` 原版实现，并明确“何种意义上的对齐可被视为成立”。

---

## 1. 结论先行

### 1.1 是否存在“绝对证明 100% 对齐”的办法？
结论：**通常不存在**。

原因在于原版行为不只由源码决定，还受到以下因素影响：
- provider / model capability
- server-side tool 行为
- prompt policy
- streaming 事件时序
- 权限系统
- 网页内容实时变化
- 搜索结果实时变化
- 上游服务策略变化

因此，对 `WebFetch` / `WebSearch` 这类含外部依赖、含服务端能力、含 prompt 语义的工具，无法仅靠静态代码审查证明“100% 等价”。

### 1.2 工程上可达到的最强结论
可以建立一套高置信度验证体系，使我们能够得出如下结论：

> 在已定义的对齐 contract 下，Go 实现与原版在冻结外部依赖的录制回放环境中逐例一致；在真实环境抽样和用户任务 E2E 场景中无显著行为偏差。

这已经是工程上最接近“证明对齐”的方式。

---

## 2. 先定义“什么叫对齐”

在实施验证前，必须先统一判定标准。建议将“对齐”拆成三层。

## 2.1 实现对齐（Implementation Alignment）
判断对象：
- 输入校验
- 错误 contract
- 缓存语义
- 权限决策
- 执行路径选择
- 结构化结果 contract

判定目标：
- 同输入、同外部依赖、同配置下，原版与 Go 版在结构化 contract 上一致。

## 2.2 行为对齐（Behavioral Alignment）
判断对象：
- prompt 对结果的影响
- 域名过滤语义
- fallback 触发条件
- progress / streaming 事件
- 输出格式
- 引用策略

判定目标：
- 用户可感知的工具行为与原版一致或满足预设容差。

## 2.3 产品对齐（Product Alignment）
判断对象：
- 最终回答质量
- 是否触发同类工具调用
- 是否呈现同类权限提示
- 是否提供同等引用体验
- TUI / renderer 展示是否一致

判定目标：
- 从用户视角看，Go 版与原版没有显著差异。

---

## 3. 最强验证路径：五层验证体系

建议采用五层验证体系，自底向上逐层收紧。

---

## 第 1 层：源码 / Contract 审查

### 目标
先验证是否至少在“定义层”接近原版。

### 检查项

#### WebFetch
- 输入字段是否一致
- `prompt` 是否参与语义，而非仅回显
- cache key 是否包含影响结果的参数
- SSRF / redirect / domain 限制是否与目标规范一致
- 输出是否能映射为统一结构化 contract

#### WebSearch
- 输入字段是否一致
- `allowed_domains` / `blocked_domains` 约束是否一致
- provider gating 是否存在
- 是否具备 provider-native path
- 缓存键是否包含 query + domain filters + mode
- 输出是否支持 citation / method / duration 等元数据

### 通过标准
- 所有核心字段和控制流均有可追溯 contract
- 不存在已知“必然不对齐”的关键缺口

### 局限
- 只能发现明显不一致
- 无法证明运行时等价

---

## 第 2 层：单元级行为对齐

### 目标
对纯函数、分支逻辑、校验和格式化进行逐项对齐验证。

### 推荐验证对象

#### WebFetch
- URL validation
- redirect policy
- domain allow / deny
- raw cache key
- extraction cache key
- HTML cleaning / content extraction
- prompt-guided extraction logic
- formatter

#### WebSearch
- input validation
- allowed/blocked 互斥校验
- cache key
- provider gating
- fallback dispatch
- domain filter application
- formatter / citation mapping
- progress event mapping

### 推荐断言方式
- 严格相等：错误类别、布尔判定、cache key 语义
- 归一化相等：URL 格式、空白、大小写
- 阈值相等：片段覆盖率、结果重合率、摘要相关性

### 通过标准
- 所有纯函数和分支逻辑在测试集上满足既定 contract

---

## 第 3 层：录制回放对拍（核心层）

这是整套方案中最重要的一层。

### 目标
冻结外部依赖，消除网页内容变化、搜索结果变化、上游服务变化带来的噪音，只验证“实现是否对齐”。

### 核心原则
- 原版和 Go 版必须消费同一份录制数据
- 回放时不允许访问真实外部网络
- 对比的是结构化结果和事件流，而不是仅最终字符串

---

## 3.1 WebFetch 录制回放设计

### 录制内容
每条 case 至少记录：
- 请求 URL
- redirect 链
- 最终 resolved URL
- HTTP status code
- response headers
- content-type
- response body
- 超时/连接失败等错误场景

### 用例类型
1. 普通 HTML 页面
2. 带大量导航噪声页面
3. 含代码块页面
4. 纯文本页面
5. markdown 页面
6. redirect 页面
7. 大页面 / 截断页面
8. 非法 URL
9. SSRF 风险 URL
10. disallowed domain 页面
11. 同 URL + 多 prompt
12. 404 / 500 / timeout 页面

### 对拍方式
对于每个用例：
1. 原版执行
2. Go 执行
3. 归一化输出为统一结构
4. 比较以下字段：
   - success / error
   - error class
   - resolved URL
   - title
   - extracted snippets
   - summary
   - truncated
   - method
   - prompt 是否影响结果

### 判定标准
- 安全/权限/输入校验：必须一致
- 结构化字段：应完全一致或归一化一致
- 文本提取质量：允许近似一致，但需满足阈值

---

## 3.2 WebSearch 录制回放设计

### 录制内容
根据执行路径不同，录制对象不同。

#### 若是 provider-native path
应录制：
- 请求参数
- streaming event 序列
- server tool use blocks
- tool result blocks
- 最终结构化结果
- provider capability / model info

#### 若是 fallback path
应录制：
- 搜索请求 URL / 参数
- HTTP response body
- 解析后的结果集
- 过滤前结果
- 过滤后结果

### 用例类型
1. 普通 query
2. 冷门 query
3. 零结果 query
4. 同 query + `allowed_domains`
5. 同 query + `blocked_domains`
6. provider 支持场景
7. provider 不支持场景
8. native path 失败 → fallback 场景
9. timeout / upstream error
10. 多事件 progress 场景
11. 同 query 不同 filters
12. 引用要求场景

### 对拍方式
归一化结果为统一结构后比较：
- validation result
- selected execution mode
- provider gating result
- progress events
- result count
- top-k URLs
- snippets
- citation fields
- duration metadata
- fallback reason

### 判定标准
- 执行模式选择必须一致
- 输入约束必须一致
- 事件语义必须一致
- 结构化结果应一致或满足预设阈值

---

## 第 4 层：真实环境抽样对拍

### 目标
验证录制回放没覆盖到的真实世界偏差。

### 方法
定期在真实网络和真实 provider 环境下，对一批稳定样本执行原版和 Go 版，比较：
- 返回结果
- 引用来源
- fallback 率
- 错误率
- 响应时间

### 抽样池建议

#### WebFetch
- 官方文档页
- 稳定博客页
- 静态帮助页
- 常见重定向页
- 含代码示例页

#### WebSearch
- 常见技术查询
- 限定官方域名查询
- 错误排查查询
- API 能力验证查询
- 对冷门主题的查询

### 指标建议
- 结果 URL 集合重合率
- 引用重合率
- 摘要相似度
- fallback 比例
- 错误率差异
- 进度事件完整率

### 说明
这一层不能用于“证明”，但可以用于发现真实世界退化。

---

## 第 5 层：用户任务 E2E 对齐

### 目标
直接验证最终用户体验是否一致。

### 方法
构建任务集，让用户问题分别在原版和 Go 版完整跑通，记录：
- 工具调用序列
- 工具参数
- 进度事件
- 权限提示
- 最终回答
- 引用表现

### WebFetch E2E 示例任务
- “读取这个安装文档并总结步骤”
- “找出这个页面关于 rate limits 的描述”
- “提取这篇文章中的代码样例”
- “这个页面是否提到了 Windows 支持？”

### WebSearch E2E 示例任务
- “搜索官方文档确认这个 API 是否支持流式”
- “仅在 golang.org 内搜索 context cancellation”
- “搜索这个错误信息的修复方法并引用来源”
- “确认 Anthropic 文档里是否支持某项能力并给出处”

### 评估维度
- 是否调用相同工具
- 是否使用相近参数
- 是否得到相近来源
- 是否产出相近回答
- 是否保留相近引用
- 权限体验是否一致

---

## 4. 统一比较模型：不要只比字符串

为了让验证可执行，必须先定义统一的结构化结果模型。

---

## 4.1 WebFetch 统一结果模型

建议统一为：

```json
{
  "input": {
    "url": "...",
    "prompt": "..."
  },
  "execution": {
    "method": "provider_native|local_fallback",
    "resolvedUrl": "...",
    "truncated": false,
    "durationMs": 1234,
    "cacheHit": false
  },
  "content": {
    "title": "...",
    "summary": "...",
    "snippets": ["..."],
    "links": ["..."],
    "rawTextLength": 12345
  },
  "error": null
}
```

### 字段比较规则
- `input`: 必须一致
- `method`: 必须一致（目标是对齐原版路径选择）
- `resolvedUrl`: 归一化一致
- `truncated`: 必须一致
- `title`: 归一化一致
- `summary/snippets`: 允许近似，但需满足阈值
- `error`: 错误类别必须一致

---

## 4.2 WebSearch 统一结果模型

建议统一为：

```json
{
  "input": {
    "query": "...",
    "allowedDomains": ["..."],
    "blockedDomains": ["..."]
  },
  "execution": {
    "method": "provider_native|local_fallback",
    "durationMs": 1200,
    "fallbackReason": "",
    "cacheHit": false,
    "provider": "...",
    "model": "..."
  },
  "progress": [
    {"type": "started"},
    {"type": "query_issued", "query": "..."},
    {"type": "results_received", "count": 5}
  ],
  "results": [
    {
      "title": "...",
      "url": "...",
      "snippet": "...",
      "sourceType": "web"
    }
  ],
  "citations": [
    {"title": "...", "url": "..."}
  ],
  "error": null
}
```

### 字段比较规则
- 输入校验结果：必须一致
- execution method：必须一致
- progress 事件集合：必须一致
- top-k URLs：应一致或达到阈值
- citation：必须可追溯到结果 URL
- error class：必须一致

---

## 5. 判定规则：哪些要严格一致，哪些允许近似

建议把比较规则分为三档。

## 5.1 必须严格一致
- 输入校验是否通过
- 错误类别
- permission 决策
- provider gating 结果
- 是否 fallback
- domain filtering 语义
- cache key 语义
- progress 事件类型集合

## 5.2 允许归一化后一致
- URL 大小写 / slash 归一化
- 文本空白
- markdown 格式差异
- duration 的小幅波动
- 结果顺序的微小变化（如规范允许）

## 5.3 允许近似一致
- 摘要文本
- snippet 截断位置
- 正文提取边界
- 相关片段选取顺序

### 对近似一致的建议判定方式
- token overlap
- URL 集合重合率
- 片段覆盖率
- 语义相似度
- 引用完整性

---

## 6. 为真正接近“百分百”所需的前置条件

若要把验证强度做到最高，必须满足以下前提。

## 6.1 原版必须可测试化
需要能够：
- 固定依赖版本
- 注入 mock provider / mock network
- 导出结构化结果
- 导出 progress events
- 运行标准样本集

## 6.2 Go 版必须暴露同样可观察面
至少导出：
- 结构化结果
- 执行 method
- fallback 原因
- cache 命中信息
- progress events
- permission 决策

## 6.3 必须建立对齐规范文档
不能只说“看起来像”。必须定义：
- 必须一致字段
- 允许归一化字段
- 允许近似字段
- 每类阈值
- 失败时如何归类

---

## 7. 推荐实施方案

建议按以下顺序落地。

## Step 1：先补结构化结果导出
为 WebFetch / WebSearch 各自建立统一 normalize 层，使原版与 Go 版都能输出同一结构。

## Step 2：先做录制回放框架
这是最关键的基础设施。没有 replay，就无法排除外部变化噪音。

## Step 3：建立黄金样本集
分别为：
- WebFetch 页面集
- WebSearch query 集
- 错误 / fallback / 权限场景集

## Step 4：先做结构化对拍
先不比最终回答，只比：
- 执行路径
- 事件流
- 结构化结果
- 错误

## Step 5：再做 E2E 对拍
对标准任务集比较最终体验。

## Step 6：建立持续验证机制
每次改动 WebFetch / WebSearch 后：
- 跑单元对齐
- 跑 replay 对拍
- 跑部分真实抽样
- 生成对齐报告

---

## 8. 可量化的最终通过标准

建议采用如下通过标准，而不是写“100% 对齐原版”。

### 标准 A：实现对齐通过
- 录制回放环境中，所有 must-match 字段 100% 一致
- normalized fields 100% 一致
- approx fields 达到阈值要求

### 标准 B：行为对齐通过
- 标准场景集上，无高优先级行为偏差
- provider gating / permission / fallback / citation 语义全通过

### 标准 C：产品对齐通过
- E2E 任务集中，用户视角无显著差异
- 结果质量、引用与进度体验达到约定标准

### 建议不要使用的表述
- “彻底证明 100% 完全一致”

### 建议使用的表述
- “在定义的对齐 contract 和冻结依赖环境下，逐例一致”
- “在标准任务集和真实抽样场景中达到产品对齐标准”

---

## 9. 针对 WebFetch / WebSearch 的专门建议

## 9.1 WebFetch 的额外难点
WebFetch 最大难点不在抓取，而在：
- prompt 是否真正驱动提取
- 提取结果是否保留原版相关性
- 页面正文抽取质量是否足够接近原版

### 建议
对 WebFetch 增加人工标注或 LLM-as-judge 评估，辅助判断：
- 相关性
- 信息覆盖率
- 误提取率

## 9.2 WebSearch 的额外难点
WebSearch 最大难点不在 schema，而在：
- provider-native 行为
- streaming 事件
- 搜索结果实时变化
- 引用契约

### 建议
对 WebSearch 以 replay 对拍为主、真实网络抽样为辅；不要试图仅靠真实网络结果直接断言对齐。

---

## 10. 最终结论

如果问题是：

> 有没有真正可以验证是否百分百对齐原版的办法？

最准确的回答是：

### 没有绝对静态证明办法；但有一套工程上最强的验证方案：
1. 定义统一对齐 contract
2. 建立原版与 Go 版双实现对拍
3. 使用录制回放冻结外部依赖
4. 比较结构化结果、事件流、权限行为和最终体验
5. 用真实抽样与 E2E 任务补充验证

当这套体系全部通过时，就已经达到了工程上最接近“证明对齐”的结论。
