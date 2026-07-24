# Prompt Cache 命中率分析方法论

> 本文档阐述缓存命中率评估的完整方法论，包括缓存原理、实现机制、
> 命中率保持策略，以及数据驱动的对比分析方法。

---

## 一、缓存在 LLM 中的实际价值

### 1.1 缓存是否能降低算力？

**能。Prompt Cache 直接跳过 Transformer 前向传播中最昂贵的 Prefill 阶段。**

LLM 推理分为两个阶段：

```
┌─────────────────────────────────────────────────────────┐
│ 阶段1: Prefill（预填充）                                │
│                                                         │
│ 输入: 完整 prompt（系统提示 + 工具 + 历史消息）           │
│ 过程: 对每个 input token 执行完整的 Transformer 前向传播  │
│       计算所有层的 Key-Value 向量并存入 KV-Cache          │
│ 复杂度: O(n²) — n 为输入序列长度                         │
│ 耗时: 200K tokens 约需 2-5 秒                           │
│ 算力: 占总推理算力的 60-90%（长 prompt 时更高）           │
│                                                         │
│ 这是 GPU 计算密集型操作 — 每个 token 需要通过所有         │
│ Transformer 层（Claude Sonnet 有 ~80 层），每层执行       │
│ Multi-Head Attention + FFN。200K tokens × 80 层 =        │
│ 约 16M 次矩阵乘法。                                     │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│ 阶段2: Decode（解码/生成）                              │
│                                                         │
│ 输入: Prefill 阶段产出的 KV-Cache + 上一步生成的 token   │
│ 过程: 每步生成1个 token，利用已有 KV-Cache 只计算         │
│       新 token 的 attention                              │
│ 复杂度: O(n) per token — 只需 attend 到已有 KV 向量      │
│ 耗时: 每 token 约 10-50ms                               │
│                                                         │
│ 这是内存带宽密集型操作 — GPU 大部分时间在读 KV-Cache。    │
└─────────────────────────────────────────────────────────┘
```

**Prompt Cache 的作用：直接跳过 Prefill。**

当缓存命中时，服务端已经有前一次请求计算好的 KV-Cache 向量。新请求只需要：
1. 从存储中加载已缓存的 KV 向量（内存拷贝，非计算）
2. 对新增的 delta tokens 执行 Prefill
3. 进入 Decode 阶段

```
无缓存:  [====== Prefill 200K tokens ======][== Decode ==]
                    3-5 秒                      1-2 秒

有缓存:  [加载缓存][Prefill 2K][== Decode ==]
          0.1秒     0.05秒       1-2 秒
```

**算力节省是真实的：**
- 缓存命中时，Prefill 的 GPU 算力约**减少 95-99%**（只计算 delta）
- TTFT（Time To First Token）从 3-5 秒降到 **0.2-0.5 秒**
- 这不仅是省钱——是**更快的响应速度**

### 1.2 为什么 cache_read 只收 0.1x 而不是 0x？

缓存读取仍需：
- 从分布式存储加载 KV 向量到 GPU HBM（高带宽内存）
- KV-Cache 体积很大：200K tokens × 80 层 × 2(K+V) × hidden_dim ≈ 数 GB
- 存储和传输有成本，但远低于重新计算

### 1.3 为什么 cache_creation 收 1.25x？

首次缓存写入除了正常计算外还需：
- 将 KV 向量写入持久化存储（分布式 KV 存储或 SSD）
- 维护缓存索引和 TTL 管理
- 额外的 25% 是存储和管理成本

---

## 二、缓存的实现机制

### 2.1 前缀匹配（Prefix Matching）

Anthropic 的 Prompt Cache 基于**严格前缀匹配**：

```
请求 A:  [系统提示] [工具定义] [消息1] [消息2]  ← 断点
                                                  ↓ 缓存

请求 B:  [系统提示] [工具定义] [消息1] [消息2] [消息3]  ← 新断点
         ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
         与缓存前缀字节级一致 → cache_read          ^^^^^^
                                                   新增部分 → cache_creation
```

**关键约束：字节级一致。** 任何差异——包括空格、字段顺序、数字格式——都会导致缓存失效。

### 2.2 cache_control 断点

API 请求中通过 `cache_control: {type: "ephemeral"}` 标记缓存边界：

```json
{
  "system": [
    {"type": "text", "text": "You are...", "cache_control": {"type": "ephemeral"}}
  ],
  "tools": [
    {"name": "Bash", ...},
    {"name": "Read", ..., "cache_control": {"type": "ephemeral"}}
  ],
  "messages": [
    {"role": "user", "content": [{"type": "text", "text": "hello"}]},
    {"role": "assistant", "content": [{"type": "text", "text": "hi", "cache_control": {"type": "ephemeral"}}]}
  ]
}
```

服务端缓存断点之前的所有内容。多个断点创建嵌套缓存边界。

### 2.3 缓存生命周期

| TTL | 费率 | 适用场景 |
|-----|------|---------|
| 5分钟（默认） | 1.25x write | 活跃对话（每轮刷新 TTL） |
| 1小时 | 2.0x write | 不频繁交互、后台任务 |

每次缓存命中（cache_read）都会**刷新 TTL**——活跃对话的缓存不会过期。

### 2.4 缓存失效原因

| 原因 | 触发条件 |
|------|---------|
| 前缀内容变更 | 系统提示修改、工具增删、消息编辑 |
| TTL 过期 | >5分钟无请求（默认）或 >1小时 |
| 模型变更 | 切换模型导致缓存键不匹配 |
| API 参数变更 | Beta headers、extra body params |
| 服务端驱逐 | 缓存存储满时 LRU 淘汰 |

---

## 三、保持高命中率的策略

### 3.1 前缀稳定性（最重要）

**原则：断点之前的内容在连续轮次间必须字节级一致。**

```
✅ 好的做法:
   系统提示固定不变 → 工具集固定不变 → 消息只追加不修改
   
❌ 坏的做法:
   系统提示包含时间戳 → 每轮都变 → 缓存永远失效
   工具描述包含动态列表 → 每轮不同 → 工具缓存失效
   旧消息被修改/删除 → 前缀变更 → 全部缓存失效
```

### 3.2 断点位置策略

```
最优策略（原版 TS 实现）:
  
  [系统提示-静态部分]  ← 断点1（global scope，跨组织共享）
  [系统提示-动态部分]  ← 无断点（动态变更不影响静态缓存）
  [工具定义-最后一个]  ← 断点2（系统+工具整体缓存）
  [消息-最后一条]      ← 断点3（全部历史缓存）

Go 简化版（当前实现）:
  
  [系统提示-完整]  ← 断点1
  [工具-最后一个]  ← 断点2
  [消息-最后一条]  ← 断点3
  
差异: 系统提示未分离静态/动态 → 动态变更会废弃整个系统提示缓存
```

### 3.3 消息体积控制

缓存命中率 = `cache_read / total_input`。降低 `total_input` 中非缓存部分的比例也能提高命中率：

| 手段 | 效果 |
|------|------|
| 工具输出持久化（>50K → 磁盘） | 减少消息体积 → 降低 total_input |
| ToolResultBudget（>30K → 截断） | 减少消息体积 |
| Microcompact（清理旧结果） | API 层面减少发送的 token 数 |
| 消息压缩（LLM 摘要） | 大幅减少总 token，但会重建缓存 |

### 3.4 避免缓存中断

| 实践 | 说明 |
|------|------|
| 会话参数锁存 | Beta headers、TTL 设置一旦确定不再翻转 |
| 工具 schema 缓存 | 工具定义一次序列化，不随配置漂移 |
| 压缩保持前缀 | 压缩时优先保留前缀消息不变 |
| 缓存中断检测 | 监控 cache_read 下降，归因到具体原因 |

---

## 四、命中率评估方法论

### 4.1 指标定义

```
缓存命中率:
  hit_rate = cache_read / (cache_read + cache_creation + uncached)
  
有效费率（相对于无缓存）:
  eff_cost = (uncached × 1.0 + creation × 1.25 + read × 0.1) / total_input
  
费用节省率:
  savings = 1 - eff_cost
  
缓存利用率（缓存覆盖了多少前缀）:
  util_rate = cache_read / (total_input - new_tokens_this_turn)
```

### 4.2 数据来源

**运行时指标（来自 API 响应）：**
- `usage.input_tokens`：总 input token 数（含缓存）
- `usage.cache_read_input_tokens`：从缓存读取的 token 数
- `usage.cache_creation_input_tokens`：写入缓存的 token 数
- 无缓存 token 数 = `input - cache_read - cache_creation`

**Go 实现的指标获取路径：**
```
API 响应 → provider/anthropic.go (填充 Usage)
        → loop/query.go (传播到 EventTurnEnd)
        → render.go (显示 [cache: XK read / YK created / ZK uncached])
        → compact/compact.go (ContextWindow.CacheRead/CacheCreated)
```

### 4.3 模拟评估方法

当无法获取真实 API 数据时，可用以下模型估算：

**输入参数：**
- `S`：系统提示 token 数（默认 8,000）
- `T`：工具定义 token 数（默认 15,000）
- `D`：每轮新增消息 token 数（默认 2,500）
- `N`：对话轮数

**无缓存模型：**
```
第 n 轮 total_input = S + T + (n-1) × D + D = S + T + n × D
累计 total = Σ(n=1→N) (S + T + n × D) = N×(S+T) + D×N×(N+1)/2
```

**3 断点缓存模型：**
```
第1轮: creation = S + T + D, read = 0
第n轮 (n>1): read = S + T + (n-1) × D, creation = D
累计 read = Σ(n=2→N) (S + T + (n-1)×D) = (N-1)×(S+T) + D×(N-1)×N/2
累计 creation = (S + T + D) + (N-1) × D = S + T + N × D
```

**费用计算：**
```
无缓存费用 = total × base_rate
缓存费用 = read × 0.1 × base_rate + creation × 1.25 × base_rate
```

详见 `scripts/cache_metrics.py` 脚本实现。

---

## 五、对比分析结果

### 5.1 三种方案 20 轮模拟对比

| 指标 | 无缓存 | Go 3断点 | TS 完整版 |
|------|--------|---------|----------|
| 总 input tokens | 955,000 | 955,000 | 784,000 |
| cache_read | 0 | 883,500 | 730,500 |
| cache_creation | 0 | 71,500 | 53,500 |
| 缓存命中率 | 0.0% | 92.5% | 93.2% |
| 有效费率 | 1.000x | 0.186x | 0.178x |
| 节省率 | 0% | 81.4% | 82.2% |
| 费用（Sonnet input） | $2.865 | $0.533 | $0.420 |

### 5.2 逐轮明细

| 轮次 | 总input | Go命中率 | Go费率 | TS命中率 | TS费率 |
|------|---------|---------|--------|---------|--------|
| 1 | 24K | 0.0% | 1.250x | 0.0% | 1.250x |
| 2 | 26.5K | 90.6% | 0.208x | 90.6% | 0.208x |
| 5 | 34K | 92.6% | 0.185x | 95.2% | 0.156x |
| 10 | 46.5K | 94.6% | 0.162x | 96.1% | 0.145x |
| 15 | 59K | 95.8% | 0.149x | 96.7% | 0.138x |
| 20 | 71.5K | 96.5% | 0.140x | 97.2% | 0.132x |

### 5.3 关键结论

1. **Go 3 断点 ≈ 原版 95% 收益**：命中率差距 0.7%，费用差距 $0.11/会话
2. **TS 优势主要来自体积优化**：微压缩减少 171K tokens（不是命中率更高）
3. **第 2 轮起即有 90%+ 命中率**：前缀匹配机制天然高效
4. **系统提示 + 工具定义**贡献固定 23K/轮的缓存节省
5. **缓存中断检测**是 Go 最大的可观测性差距（无法诊断命中率下降原因）

---

## 六、codex-lb 实测数据（2026-04-06，最终修正版）

### 6.1 测试概要

使用自建 codex-lb 代理（`http://192.168.31.83:2455`）路由到 OpenAI 官方 API，三组对照实验测试不同 API 路径的缓存命中率。模型 `gpt-5.4`。

**codex-lb 的角色：** 多账号负载均衡路由代理。缓存发生在 OpenAI 服务端，codex-lb 通过 sticky routing（基于 `prompt_cache_key`）确保同一会话路由到同一账号。

### 6.2 三组对照实验

#### 实验 A：Chat Completions（无 sticky routing）

| 轮次 | Input | Cached | Hit% |
|------|-------|--------|------|
| 1 | 39 | 0 | 0.0% |
| 2 | 3,993 | 0 | 0.0% |
| 3 | 9,616 | 0 | 0.0% |
| 4 | 15,031 | 0 | 0.0% |
| 5 | 21,457 | 0 | 0.0% |

#### 实验 B：Responses API + `prompt_cache_key`（无 `previous_response_id`）

| 轮次 | Input | Cached | Hit% |
|------|-------|--------|------|
| 1 | 36 | 0 | 0.0% |
| 2 | 4,978 | 4,736 | **95.1%** |

#### 实验 C：Responses API + `prompt_cache_key` + `previous_response_id`

| 轮次 | Input | Cached | Hit% |
|------|-------|--------|------|
| 1 | 36 | 0 | 0.0% |
| 2 | 4,848 | 4,608 | **95.0%** |

### 6.3 结论

| API 路径 | `prompt_cache_key` | `prev_response_id` | Round 2 缓存 |
|----------|:-:|:-:|:-:|
| Chat Completions | ❌ | N/A | **0%** |
| Responses API | ✅ | ❌ | **95.1%** |
| Responses API | ✅ | ✅ | **95.0%** |

**关键发现：**

1. **`prompt_cache_key` 是缓存命中的决定性因素**——它触发 codex-lb 的 sticky routing，确保请求路由到同一 OpenAI 账号。
2. **`previous_response_id` 不影响缓存命中率**——实验 B 和 C 结果几乎相同。其价值在于省网络带宽（只发增量）和 server-side state 管理。
3. **Chat Completions 在 codex-lb 上 0% 缓存**——因为 codex-lb 对此路径无 sticky routing。
4. **此前的调研结论已修正**：不再需要 190K+ tokens 才能命中缓存。Responses API + `prompt_cache_key` 在 ~5K tokens 时即可 95% 命中。

### 6.4 与模拟评估对比

| 指标 | 模拟估算（第 5.1 节） | codex-lb 实测 | 差异原因 |
|------|-------------------|--------------|---------|
| 首次命中轮次 | 第 2 轮 | 第 2 轮 | 一致 |
| 稳定命中率 | 92-97% | 95% | 实测略优 |
| Chat Completions 缓存 | 未测试 | 0%（codex-lb 无 sticky） | 路由层限制 |

### 6.5 对不同后端的适用性

| 后端 | Chat Completions 缓存 | Responses API 缓存 | 关键因素 |
|------|:-:|:-:|------|
| OpenAI 官方 | 90%+（自动 prefix cache） | 95%+ | 无需 sticky routing |
| codex-lb → OpenAI | 0%（无 sticky） | 95%（有 sticky） | 必须用 Responses API |
| vLLM/SGLang 自建 | 0-90%+（取决于配置） | N/A（不支持） | `--enable-prefix-caching` |
| Anthropic | N/A | N/A | 显式 `cache_control` 断点 |
