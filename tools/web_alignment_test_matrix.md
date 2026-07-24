# WebFetch / WebSearch 对拍测试矩阵

> 对应源码：`tools/web.go`
>
> 目标：定义一套可落地的对拍测试矩阵，用于比较 `gosrc` 与 `../src` 的 WebFetch / WebSearch 实现行为差异，并为后续 replay、双实现对拍、E2E 对齐验证提供统一样本来源。

---

## 1. 使用说明

本矩阵服务于三类验证：

1. **结构化对拍**
   - 比较执行路径、错误、事件流、结构化字段
2. **录制回放验证**
   - 冻结外部依赖后比较实现差异
3. **E2E 用户任务验证**
   - 比较最终用户感知体验

每条 case 建议具备以下元信息：
- Case ID
- Tool Name
- Scenario
- Input
- Fixture / Replay Source
- Must-Match Fields
- Approx-Match Fields
- Notes / Risk

---

## 2. 比较规则分级

### A. Must Match
必须严格一致：
- 输入校验是否通过
- 是否报错
- 错误类别
- provider gating 结果
- fallback 是否触发
- permission 决策
- progress 事件类型集合
- domain filtering 语义

### B. Normalized Match
归一化后应一致：
- URL 归一化结果
- markdown 细节
- 空白字符
- 大小写
- duration 小幅浮动

### C. Approx Match
允许近似：
- 摘要
- snippet 截断边界
- 正文提取片段边界
- 相关片段排序
- top-k 搜索结果中非前几项排序

---

## 3. WebFetch 测试矩阵

## WF-001 基本纯文本抓取
- **Scenario**: 纯文本页面
- **Input**:
  - `url`: plain text fixture URL
  - `prompt`: `What does the page say?`
- **Fixture**: 静态 `text/plain`
- **Must Match**:
  - success
  - 无错误
  - 包含页面正文
- **Approx Match**:
  - 输出格式细节
- **目的**:
  - 验证最基础抓取能力

## WF-002 基本 HTML 抓取
- **Scenario**: 简单 HTML 页面
- **Input**:
  - `prompt`: `extract the content`
- **Fixture**: 含 `<p>` 文本
- **Must Match**:
  - script/style 不泄漏
  - 可见文本被提取
- **Approx Match**:
  - 标签清洗后的空白与换行

## WF-003 链接保留语义
- **Scenario**: 页面含超链接
- **Fixture**: `<a href="https://example.com">Docs</a>`
- **Must Match**:
  - 链接信息未完全丢失
- **Approx Match**:
  - 呈现为 `Docs (https://example.com)` 或原版等价表达
- **目的**:
  - 检查链接上下文保留能力

## WF-004 script/style 去除
- **Scenario**: 页面含大量脚本和样式
- **Must Match**:
  - script/style 内容不应出现在提取文本中
- **目的**:
  - 验证 HTML 清洗基础能力

## WF-005 噪声页面正文提取
- **Scenario**: 页面有导航栏、页脚、广告、正文
- **Fixture**: 自建噪声 HTML
- **Must Match**:
  - 正文存在
- **Approx Match**:
  - 正文覆盖率
  - 噪声比例
- **目的**:
  - 验证是否接近原版正文提取能力

## WF-006 含代码块页面
- **Scenario**: 页面包含 code/pre
- **Must Match**:
  - 代码内容不丢失
- **Approx Match**:
  - 代码块边界、格式保真度
- **目的**:
  - 检查技术文档抓取质量

## WF-007 Markdown-like 页面
- **Scenario**: 页面主体接近 markdown 结构
- **Must Match**:
  - 标题/列表文本保留
- **Approx Match**:
  - 层级结构保留程度

## WF-008 Redirect 正常跳转
- **Scenario**: 302 → 最终页面
- **Must Match**:
  - redirect 能成功跟随
  - resolved URL 一致
- **目的**:
  - 验证跳转路径一致性

## WF-009 Redirect 到被阻止域名
- **Scenario**: 初始 URL 合法，跳转到 disallowed domain
- **Must Match**:
  - 必须被拒绝
  - 错误类别一致
- **目的**:
  - 验证安全策略与 redirect 复检

## WF-010 SSRF 风险 URL
- **Scenario**: localhost / private IP / metadata URL
- **Must Match**:
  - 必须拒绝
  - 错误语义一致
- **目的**:
  - 验证 SSRF 策略

## WF-011 非法 URL
- **Scenario**: malformed URL
- **Must Match**:
  - 输入错误
  - 错误类型一致

## WF-012 缺失 URL
- **Scenario**: `url` 为空
- **Must Match**:
  - 必须返回参数错误

## WF-013 缺失 Prompt
- **Scenario**: `prompt` 为空
- **Must Match**:
  - 必须返回参数错误

## WF-014 同 URL 不同 Prompt
- **Scenario**: 相同 URL，不同提取意图
- **Input A**: `Summarize installation steps`
- **Input B**: `Find rate limits`
- **Must Match**:
  - 输出不应完全相同（目标行为）
- **Approx Match**:
  - 各自与 prompt 的相关性
- **目的**:
  - 验证 prompt 是否真正驱动提取

## WF-015 超大页面
- **Scenario**: 页面内容超出 body limit / output limit
- **Must Match**:
  - 不崩溃
  - 截断行为稳定
- **Approx Match**:
  - 截断标记位置

## WF-016 非 2xx 状态码
- **Scenario**: 404 / 500
- **Must Match**:
  - 返回错误
  - 错误类别与文案语义一致

## WF-017 超时场景
- **Scenario**: 服务端长时间不返回
- **Must Match**:
  - 超时错误
  - 错误类别一致

## WF-018 纯文本缓存命中
- **Scenario**: 同输入重复调用
- **Must Match**:
  - 第二次命中缓存（若目标设计要求）
- **目的**:
  - 验证缓存行为

## WF-019 同 URL 不同 Prompt 缓存隔离
- **Scenario**: 同 URL + 不同 prompt 连续调用
- **Must Match**:
  - 不应错误复用提取结果
- **目的**:
  - 验证未来对齐后的 cache key 语义

## WF-020 权限拒绝场景
- **Scenario**: 产品层禁用 WebFetch 或禁用外网访问
- **Must Match**:
  - 权限结果一致
- **目的**:
  - 验证 permissions 集成对齐

---

## 4. WebSearch 测试矩阵

## WS-001 基本 query
- **Scenario**: 简单搜索
- **Input**: `golang`
- **Must Match**:
  - success
  - 至少一个结果
- **Approx Match**:
  - 结果文本格式

## WS-002 空 query
- **Scenario**: `query` 为空
- **Must Match**:
  - 输入错误

## WS-003 allowed_domains 限定
- **Scenario**: 限定结果域名
- **Input**:
  - `query`: `context cancellation`
  - `allowed_domains`: `golang.org`
- **Must Match**:
  - 结果只来自允许域名
- **目的**:
  - 验证域名白名单语义

## WS-004 blocked_domains 限定
- **Scenario**: 屏蔽指定域名
- **Must Match**:
  - 被屏蔽域名不应出现

## WS-005 allowed 与 blocked 同时传入
- **Scenario**: 两者同时出现
- **Must Match**:
  - 若目标对齐原版，则必须报错
- **目的**:
  - 验证输入 contract 对齐

## WS-006 Instant Answer 命中
- **Scenario**: 第一搜索路径返回结果
- **Must Match**:
  - 不触发 fallback
- **Approx Match**:
  - 结果展示格式

## WS-007 Fallback 命中
- **Scenario**: 第一搜索路径为空，fallback 返回结果
- **Must Match**:
  - fallback 被触发
  - 最终结果成功

## WS-008 双路径都无结果
- **Scenario**: 无搜索结果
- **Must Match**:
  - 返回 no results 语义

## WS-009 provider 支持场景
- **Scenario**: 当前 provider/model 支持原生 WebSearch
- **Must Match**:
  - 选择 provider-native path
- **目的**:
  - 验证真正对齐的主路径

## WS-010 provider 不支持场景
- **Scenario**: 当前 provider/model 不支持原生 WebSearch
- **Must Match**:
  - 走 fallback 或禁用策略

## WS-011 provider native 失败后降级
- **Scenario**: provider-native 执行失败
- **Must Match**:
  - 降级行为符合策略
  - fallback reason 可见

## WS-012 progress 事件最小集合
- **Scenario**: 一次标准搜索
- **Must Match**:
  - started
  - query_issued
  - results_received / completed
- **目的**:
  - 验证事件语义

## WS-013 多结果引用
- **Scenario**: 搜索返回多个来源
- **Must Match**:
  - citation 可追溯
- **Approx Match**:
  - 引用排序

## WS-014 同 query 不同 allowed_domains
- **Scenario**: 相同 query，切换 allowed_domains
- **Must Match**:
  - 结果不同
  - 不能错用缓存
- **目的**:
  - 验证 cache key 对齐

## WS-015 同 query 不同 blocked_domains
- **Scenario**: 相同 query，切换 blocked_domains
- **Must Match**:
  - 被 block 的站点应被排除
  - 不能错用缓存

## WS-016 工具级 allowed 与输入级 allowed 交集
- **Scenario**: tool-level policy 与 query-level filters 同时存在
- **Must Match**:
  - 生效集合符合交集语义（若这是目标设计）

## WS-017 搜索超时
- **Scenario**: provider 或 fallback 请求超时
- **Must Match**:
  - 超时错误类别一致

## WS-018 搜索解析错误
- **Scenario**: 返回非预期 JSON / HTML
- **Must Match**:
  - 错误语义一致

## WS-019 权限拒绝
- **Scenario**: 产品层禁用外部 web search
- **Must Match**:
  - 权限决策一致

## WS-020 引用约束输出
- **Scenario**: 搜索后需要在最终回答中引用来源
- **Must Match**:
  - tool result 中具备引用提示或结构化 citation
- **目的**:
  - 验证产品语义对齐

---

## 5. E2E 任务矩阵

这些 case 不只比较工具结果，还比较“用户体验是否一致”。

## E2E-FETCH-001 安装步骤提取
- 用户问题：`读取这个安装文档并总结安装步骤`
- 关注点：
  - WebFetch 是否被调用
  - prompt 是否驱动提取安装部分
  - 最终回答是否聚焦步骤

## E2E-FETCH-002 限制项提取
- 用户问题：`这个页面有没有提到 rate limits？给我原文依据`
- 关注点：
  - 提取相关段落
  - 引用保留

## E2E-SEARCH-001 官方文档检索
- 用户问题：`只在 golang.org 搜索 context cancellation`
- 关注点：
  - domain filter
  - 引用
  - 最终结果是否来自目标域名

## E2E-SEARCH-002 错误排查
- 用户问题：`搜索这个错误信息的修复方法并引用来源`
- 关注点：
  - WebSearch 调用
  - 结果质量
  - 引用完整性

## E2E-SEARCH-003 能力确认
- 用户问题：`确认某官方文档中是否支持流式，并给出处`
- 关注点：
  - 搜索 → 抓取 / 回答链路
  - 最终用户体验

---

## 6. 推荐样本实现方式

### WebFetch Fixture 来源
建议三类：
1. 本地静态 HTML/text fixtures
2. `httptest` 动态服务端
3. replay 录制文件

### WebSearch Fixture 来源
建议三类：
1. provider-native event replay
2. fallback HTTP response replay
3. 结构化 normalized result fixtures

---

## 7. 建议的落库形式

建议后续将样本分为三层目录：

```text
tools/testdata/web_alignment/
  webfetch/
    fixtures/
    replay/
    normalized/
  websearch/
    fixtures/
    replay/
    normalized/
```

其中：
- `fixtures/`: 原始页面或响应
- `replay/`: 录制的请求/响应/事件流
- `normalized/`: 统一结构下的期望结果

---

## 8. 推荐优先级

若资源有限，建议优先做以下 8 个 case：

### WebFetch 优先
1. WF-002 基本 HTML 抓取
2. WF-008 Redirect 正常跳转
3. WF-009 Redirect 到被阻止域名
4. WF-014 同 URL 不同 Prompt
5. WF-019 同 URL 不同 Prompt 缓存隔离

### WebSearch 优先
6. WS-005 allowed 与 blocked 同时传入
7. WS-007 Fallback 命中
8. WS-014 / WS-015 query 相同但 domains 不同缓存隔离

这些 case 覆盖当前最明显的不对齐风险。
