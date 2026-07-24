# 可复用开源库调研报告

> 目标：为 multi-provider 执行计划中每个关键模块寻找成熟的、可直接复用的开源库，降低开发成本和风险。
> 调研时间：2026-04-14

---

## 调研结论总览

| 能力域 | 推荐方案 | 理由 | 预估节省工时 |
|--------|---------|------|------------|
| **统一 LLM Provider 抽象** | ❌ 自建（参考 fantasy） | 当前 provider/ 已高度定制，切换成本 > 收益 | — |
| **模型元数据注册表** | ⚠️ `charm.land/catwalk` | 现成但耦合 Crush 生态；提取子集或自建更务实 | 2-3天 |
| **OAuth2 PKCE + Device Auth** | ✅ `golang.org/x/oauth2` | 项目已依赖 v0.36.0，内置 `DeviceAuth` + `DeviceAccessToken` | 5-7天 |
| **跨平台凭证存储** | ✅ `zalando/go-keyring` | 无 CGo、1.2k stars、API 极简、MIT | 3-4天 |
| **凭证文件存储（fallback）** | ✅ 已有 `auth/store.go` | 完整但死代码，激活即可 | 2天 |
| **重试/弹性** | ✅ 已有 `provider/retry.go` | 功能完善，仅需接入 `OnAuthError` 回调 | 1天 |
| **OpenAI 兼容 SDK** | ✅ 已有 `sashabaranov/go-openai` v1.41.2 | 10.6k stars，已是项目依赖 | — |
| **Anthropic SDK** | ✅ 已有 `anthropic-sdk-go` v1.30.0 | 官方 SDK，已是项目依赖 | — |
| **AWS Bedrock** | ✅ 已有 `aws-sdk-go-v2` | 已是项目依赖 | — |
| **Google Vertex** | ✅ 已有 `cloud.google.com/go/auth` | 已是项目依赖（间接） | — |

---

## 一、统一 LLM Provider 抽象层

### 候选方案对比

| 库 | Stars | Provider 数 | Streaming | Tool Calling | 维护者 | Go 版本 | 评估 |
|----|-------|------------|-----------|-------------|--------|---------|------|
| **charm.land/fantasy** | 711 | 9+ (Anthropic, OpenAI, Google, Azure, Bedrock, OpenRouter...) | ✅ 回调式 | ✅ Agent 级 | Charmbracelet (Crush 背后团队) | ≥1.23 | 功能最强，但 Agent 框架偏重 |
| **mozilla-ai/any-llm-go** | 90 | 10 (Anthropic, OpenAI, Gemini, Mistral, Ollama, DeepSeek...) | ✅ Channel 式 | ✅ | Mozilla AI | ≥1.25 | 轻量纯净，接口设计优雅 |
| **voocel/litellm** | 34 | 8 (OpenAI, Anthropic, Gemini, DeepSeek, Qwen, Bedrock...) | ✅ StreamReader | ✅ | 个人开发者 | ≥1.22 | 内置 cost 计算，但不够成熟 |
| **tmc/langchaingo** | 9.1k | 多 (OpenAI, Anthropic, Ollama, Gemini...) | ✅ | ✅ | 社区 | ≥1.22 | 太重，Chain/Agent 框架整合 |
| **gollmkit/go-llm-kit** | ~50 | 多 | ✅ | 部分 | 社区 | — | API Key 轮转为主，不适合 |

### ❌ 推荐：自建（不引入外部统一抽象）

**核心原因：**

1. **当前 `provider/` 已是成熟的统一抽象**
   - `Provider` 接口（3 方法：`Name()`, `ModelID()`, `CreateStream()`）已经非常精简
   - 10 个 provider 实现（anthropic, openai, responses, bedrock, vertex, deepseek, gemini, groq, mistral, ollama）
   - `RetryProvider` 装饰器、`CapabilityProvider` 可选接口均已实现
   - `OpenAIDialect` 枚举统一了 6 种 OpenAI 兼容方言

2. **引入外部抽象层的成本极高**
   - 需要将所有 `provider.Provider` → 外部库接口的桥接
   - 流式事件类型 `types.StreamEvent` 需要全链路替换
   - `RetryProvider.OnAuthError` 等定制能力难以映射
   - engine → loop → tools 全链路都耦合了 `provider.Provider`

3. **外部库都有不匹配点**
   - `fantasy`: Agent 框架（太重），回调式 streaming 与当前 channel 式不兼容
   - `any-llm-go`: 不支持 Bedrock/Vertex，Go 1.25 最低要求
   - `litellm`: 太年轻（34 stars），API 不够稳定
   - `langchaingo`: 更是一个完整框架，引入即绑定

**正确做法：**
- 保持现有 `provider.Provider` 接口不变
- 在此基础上扩展 `ProviderRegistry` 和 `ProviderRef`（如执行计划 Phase 1/3）
- 参考 `fantasy` 的 `Provider.LanguageModel(ctx, modelID)` 设计改进模型选择

---

## 二、模型元数据注册表（Model Catalog）

### 候选方案

| 方案 | 来源 | 内容 | 评估 |
|------|------|------|------|
| **charm.land/catwalk** | Charmbracelet | 社区维护的 provider/model 数据库，含 cost、context window、capability flags | 数据全但耦合 Crush 生态 |
| **yamanahlawat/llm-registry** | 社区 | JSON 格式的模型注册表 | 小项目，不够成熟 |
| **已有 `provider/models.go`** | 项目内 | `modelContextWindows` map（30+ 模型） | 基础但可扩展 |

### ⚠️ 推荐：参考 catwalk 的数据结构，自建 `ModelCatalog`

**理由：**
- `catwalk` 的数据很好（cost per token, context window, capabilities），但是一个独立的数据库项目
- 直接依赖它会引入 Crush 生态绑定和版本耦合
- 更务实：**从 catwalk 的 `crush.json` 提取数据格式**，自建一个轻量 `ModelCatalog`
- 复用现有 `provider/models.go` 的 `modelContextWindows`，扩展为：

```go
type ModelInfo struct {
    ID            string
    Provider      string
    DisplayName   string
    ContextWindow int
    CostPer1MIn   float64  // 参考 catwalk
    CostPer1MOut  float64  // 参考 catwalk
    CanReason     bool     // 参考 catwalk capabilities
    SupportsTools bool
    SupportsVision bool
    MaxOutputTokens int
}
```

---

## 三、OAuth2 认证（PKCE + Device Authorization Grant）

### ✅ 推荐：`golang.org/x/oauth2` v0.36.0（已有依赖）

**这是最大的发现：项目已经间接依赖了 `golang.org/x/oauth2 v0.36.0`！**

该版本已内置完整的 Device Authorization Grant (RFC 8628) 支持：

```go
// 1. 发起设备授权
func (c *Config) DeviceAuth(ctx context.Context, opts ...AuthCodeOption) (*DeviceAuthResponse, error)

// 2. 轮询令牌（内置 slow_down/pending 处理、超时控制）
func (c *Config) DeviceAccessToken(ctx context.Context, da *DeviceAuthResponse, opts ...AuthCodeOption) (*Token, error)

// DeviceAuthResponse 结构
type DeviceAuthResponse struct {
    DeviceCode              string    // 设备码
    UserCode                string    // 用户码（展示给用户）
    VerificationURI         string    // 验证 URL
    VerificationURIComplete string    // 带码的完整 URL（可生成二维码）
    Expiry                  time.Time // 过期时间
    Interval                int64     // 轮询间隔（秒）
}

// Endpoint 结构已支持 DeviceAuthURL
type Endpoint struct {
    AuthURL       string
    DeviceAuthURL string  // ← 设备授权端点
    TokenURL      string
    AuthStyle     AuthStyle
}
```

**当前项目 `auth/oauth.go` 自己实现的 PKCE 可以保留**（用于 Anthropic 的 Authorization Code + PKCE 流程），同时新增 Device Auth Grant 用于 OpenAI/Codex：

| 流程 | 适用场景 | 实现方案 |
|------|---------|---------|
| Authorization Code + PKCE | Anthropic (Claude Pro/Max), GitHub Copilot | 现有 `auth/oauth.go`（已完整） |
| Device Authorization Grant | OpenAI (ChatGPT Plus/Pro), Codex | `golang.org/x/oauth2` 的 `DeviceAuth` + `DeviceAccessToken` |
| API Key | 大多数 provider | 现有 `provider/env.go` |
| AWS IAM / SigV4 | Bedrock | 现有 `provider/bedrock.go` + `aws-sdk-go-v2` |
| Google ADC | Vertex | 现有 `provider/vertex.go` + `cloud.google.com/go/auth` |

**节省工时：** 不需要自己实现 Device Auth 的轮询逻辑、slow_down 处理、超时控制。直接用标准库约 **5-7 天**。

---

## 四、跨平台安全凭证存储

### 候选方案对比

| 库 | Stars | CGo | macOS | Linux | Windows | API | 维护状态 |
|----|-------|-----|-------|-------|---------|-----|---------|
| **zalando/go-keyring** | 1.2k | ❌ 无 | ✅ `/usr/bin/security` | ✅ Secret Service dbus | ✅ Credential Manager | `Set/Get/Delete(service, user, password)` | 活跃 (v0.2.8, 2026.03) |
| **99designs/keyring** | 647 | ⚠️ 部分需要 | ✅ | ✅ (多后端) | ✅ | `Open(Config) → Set/Get/Keys` | 停滞 (v1.2.2, 2022.12) |
| **keybase/go-keychain** | ~300 | ⚠️ 需要 | ✅ | ✅ | ❌ | 底层 Keychain 操作 | 低活跃 |

### ✅ 推荐：`zalando/go-keyring`

**核心优势：**
1. **无 CGo 依赖** — 可以静态编译，与项目交叉编译策略兼容
2. **API 极简** — 只有 3 个函数：`Set`, `Get`, `Delete`
3. **MockInit()** — 内置测试 mock，CI/CD 无需真实 keyring
4. **维护活跃** — 2026 年 3 月还在更新
5. **1.2k stars** — 社区验证充分

**集成设计（与现有 `auth/store.go` 互补）：**

```go
// 分层凭证存储策略
type CredentialStore interface {
    Save(provider string, creds Credentials) error
    Load(provider string) (Credentials, error)
    Delete(provider string) error
    List() ([]string, error)
}

// 实现 1：系统 Keyring（首选）— 使用 go-keyring
type KeyringStore struct { serviceName string }

// 实现 2：文件存储（fallback）— 复用现有 auth/store.go
type FileStore struct { path string }  // ~/.claude-go/auth.json

// 自动选择策略
func NewCredentialStore() CredentialStore {
    if keyringAvailable() {
        return &KeyringStore{serviceName: "claude-code-go"}
    }
    return &FileStore{path: defaultAuthPath()}
}
```

**与 OpenCode/Crush 的对比：**
- OpenCode 使用纯文件存储 `~/.local/share/opencode/auth.json`（无加密）
- Crush 通过各 Provider SDK 自身处理 + 配置文件
- 我们的方案更安全：优先系统 Keyring，文件存储仅作 fallback

---

## 五、已有可直接复用的项目内代码

### 5.1 auth/ 包（死代码，激活即可用）

| 文件 | 行数 | 功能 | 状态 |
|------|------|------|------|
| `auth/oauth.go` | 219 | PKCE (code_verifier + challenge)、localhost callback、authorization code exchange | ✅ 完整 |
| `auth/store.go` | 248 | `~/.claude/.credentials.json`、文件锁、原子写入、auto-refresh | ✅ 完整 |
| `auth/middleware.go` | 72 | `AuthMiddleware` 装饰器、`EnsureValid()` → inject `Bearer` token | ✅ 完整 |
| `auth/auth_test.go` | — | 单元测试覆盖 | ✅ 完整 |

**激活路径：**
1. `provider/env.go` case "oauth" → 创建 `auth.AuthMiddleware` 包裹 provider
2. `provider/retry.go` `OnAuthError` → 调用 `store.EnsureValid()` 刷新 token
3. 这两步即可将死代码变为生产代码

### 5.2 provider/ 包（已有完善的基础设施）

| 组件 | 可复用程度 | 扩展方向 |
|------|----------|---------|
| `Provider` interface (3 methods) | 100% 保留 | 无需修改 |
| `RetryProvider` + `OnAuthError` hook | 100% 保留 | 接入 auth middleware |
| `Config` struct | 100% 保留 | — |
| `ProviderCapabilities` | 100% 保留 | 扩展字段 |
| `NewFromEnvWithOverrides()` | 改造 | 拆分为 Registry + Factory |
| `modelContextWindows` map | 改造 | 扩展为 `ModelCatalog` |
| `OpenAIDialect` | 100% 保留 | 新 provider 通过 dialect 接入 |
| `errors.go` | 100% 保留 | — |

### 5.3 TUI 层（已有响应式状态）

| 组件 | 可复用程度 |
|------|----------|
| `AppState.Provider *tui.State[string]` | 100% — `.Set()` 触发自动重绘 |
| `AppState.Model *tui.State[string]` | 100% |
| `TuiRenderer.Banner(provider, model)` | 100% |
| Title bar `provider/model` 格式 | 100% — 已经是 `%s/%s` 格式 |

---

## 六、Crush (原 OpenCode) 的依赖参考

Crush 的 `go.mod` 为我们提供了"生产验证"的库选择参考：

| Crush 使用的库 | 我们项目的状态 | 行动建议 |
|---------------|--------------|---------|
| `charm.land/fantasy` (统一 LLM) | 不需要（自有 provider/ 层） | 参考接口设计 |
| `charm.land/catwalk` (模型注册) | 不需要（自建轻量版） | 参考数据结构 |
| `charmbracelet/openai-go` (OpenAI fork) | 用 `sashabaranov/go-openai` v1.41.2 | 保持不变 |
| `charmbracelet/anthropic-sdk-go` | 用 `anthropic-sdk-go` v1.30.0 | 保持不变 |
| `aws-sdk-go-v2` | ✅ 已有 v1.41.5 | 保持不变 |
| `golang.org/x/oauth2` | ✅ 已有 v0.36.0（间接） | **提升为直接依赖** |
| `spf13/cobra` (CLI) | 不需要（自有 CLI） | — |
| `ncruces/go-sqlite3` (存储) | 不需要 | — |
| `pkg/browser` (打开浏览器) | **需要新增** | OAuth 流程打开浏览器 |
| `denisbrodbeck/machineid` | 可选 | 设备标识 |

---

## 七、新增依赖建议清单

基于调研，执行 multi-provider 计划需要新增的外部依赖：

| 依赖 | 版本 | 用途 | 优先级 | Phase |
|------|------|------|--------|-------|
| `golang.org/x/oauth2` | v0.36.0 | Device Auth Grant (RFC 8628) | **已有**（间接→直接） | Phase 2 |
| `github.com/zalando/go-keyring` | v0.2.8 | 跨平台安全凭证存储 | **新增** | Phase 2 |
| `github.com/pkg/browser` | latest | OAuth 流程中打开系统浏览器 | **新增** | Phase 7 |

**总计：仅 2 个新增依赖**（另有 1 个已存在的间接依赖提升为直接依赖）

---

## 八、对执行计划的影响

### 修订建议

| 原执行计划 Phase | 调研影响 | 修订内容 |
|-----------------|---------|---------|
| **Phase 1: Provider Registry** | 无变化 | 保持自建 `ProviderRegistry` |
| **Phase 2: 凭证存储** | **重大简化** | 引入 `go-keyring`，分层存储：Keyring 优先 + 文件 fallback；复用 `auth/store.go` |
| **Phase 3: Engine 热替换** | 无变化 | `ProviderRef` 方案不变 |
| **Phase 4: /connect + /model** | 无变化 | — |
| **Phase 5: Session 追踪** | 无变化 | — |
| **Phase 6: Doctor** | 无变化 | — |
| **Phase 7: OAuth** | **重大简化** | PKCE 用现有 `auth/oauth.go`；Device Auth 用 `golang.org/x/oauth2`，无需自实现轮询逻辑 |
| **Phase 8: TUI** | 无变化 | — |
| **Phase 9: Cost** | 参考 `catwalk` 数据 | `ModelInfo` 结构加入 cost per token 字段 |

### 工时影响估算

| 模块 | 原估算 | 调研后估算 | 节省 |
|------|--------|----------|------|
| 凭证存储 | 3天 | 1天 | 2天 |
| Device Auth Grant | 4天 | 1天 | 3天 |
| PKCE OAuth | 3天 | 0天（已有） | 3天 |
| 模型元数据 | 3天 | 2天 | 1天 |
| **总计** | — | — | **~9天** |

---

## 九、风险评估

| 风险 | 可能性 | 影响 | 缓解措施 |
|------|--------|------|---------|
| `go-keyring` 在 headless Linux 无 keyring 时 crash | 中 | 中 | fallback 到文件存储；`keyring.MockInit()` 用于 CI |
| `golang.org/x/oauth2` DeviceAuth 某些 provider 不兼容 | 低 | 中 | 可传 `AuthCodeOption` 自定义参数 |
| `auth/store.go` 的文件锁在 NFS/网络文件系统失败 | 低 | 低 | 已有 20 次重试逻辑 |
| `catwalk` 数据过时（需要手动更新价格） | 中 | 低 | 定期从 catwalk 或在线源拉取 |

---

## 十、最终结论

> **我们的项目已经具备了 80% 的基础设施**，大部分能力以"死代码"或"不完全集成"的形式存在。
>
> 执行 multi-provider 计划只需：
> 1. **激活死代码**（auth/ 包）
> 2. **新增 2 个轻量依赖**（go-keyring + pkg/browser）
> 3. **提升 1 个间接依赖**（golang.org/x/oauth2 → 直接）
> 4. **自建 3 个新模块**（ProviderRegistry, ProviderRef, ModelCatalog）
>
> 不需要引入重量级的外部 LLM 抽象层。当前的 `provider.Provider` 接口已经是最适合项目的抽象。
