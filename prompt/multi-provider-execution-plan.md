# 多 Provider 支持 — 完整执行计划

> **目标：** 借鉴 OpenCode 的多 Provider 架构，为 gosrc 实现可靠的运行时 Provider/Model 切换、
> 统一认证管理、以及多 Provider 会话追踪。每个 Phase 独立可编译、可测试、可回滚。

---

## 0. 设计原则

| 原则 | 说明 |
|------|------|
| **可靠** | 每个 Phase 结束时 `go build ./...` 通过 + 全量测试通过，失败可 `git revert` |
| **一致** | 所有 Provider 走同一条路径：Registry → Factory → RetryProvider → Engine |
| **可验证** | 每个 Step 附带具体验证命令和预期输出，不依赖人工目视 |
| **向后兼容** | Phase 1-3 完成前，现有 `--provider` + 环境变量用法 100% 不变 |
| **借鉴 OpenCode** | `provider/model` 格式、Registry Pattern、统一凭证存储、model priority chain |

---

## 1. 当前架构诊断（基线）

### 1.1 Provider 创建链（单次、不可变）

```
main.go:59  p = provider.NewFromEnvWithOverrides(opts.Provider, opts.Model)
  ↓
main.go:108  deps := SetupRegistry(p, ...)        → AgentTool.Provider = p
                                                    → TeamManager.Provider = p
  ↓
main.go:180  eng = engine.New(Config{Provider: p}) → CoreEngine.cfg.Provider = p（终身不变）
  ↓
engine/core.go  conv.ql = loop.New(provider, ...)  → QueryLoop.provider = p（构造后不可变）
  ↓
loop/query.go   provider.CreateStream(ctx, params)   每次查询使用同一个 provider
```

### 1.2 已识别的 9 个修改点

| # | 位置 | 改动 | 风险 |
|---|------|------|------|
| 1 | `provider/registry.go` (新) | Provider Registry + Model Catalog | 无 |
| 2 | `provider/credential_store.go` (新) | 统一凭证存储 | 无 |
| 3 | `provider/env.go` | 工厂读取 Registry + CredStore | 中 |
| 4 | `engine/engine.go` + `core.go` | `SetProvider()` + 热替换 | 高 |
| 5 | `loop/query.go` | provider 查询间原子替换 | 高 |
| 6 | `registry_setup.go` | AgentTool/TeamManager 同步 | 中 |
| 7 | `commands/builtins.go` | `/connect` + 增强 `/model` | 低 |
| 8 | `session/session.go` | SessionMeta 增加 Provider/Model | 低 |
| 9 | `commands/doctor.go` | 多 Provider 诊断 | 低 |

### 1.3 可复用的 6 个现有组件

| 组件 | 文件 | 状态 |
|------|------|------|
| OAuth PKCE 完整流程 | `auth/oauth.go` (219行) | ✅ 完整，0 调用者 |
| 凭证持久化+文件锁 | `auth/store.go` (248行) | ✅ 完整，0 调用者 |
| Auth Middleware | `auth/middleware.go` (72行) | ✅ 完整，0 调用者 |
| RetryProvider.OnAuthError | `provider/retry.go:23` | ✅ 已定义，当前 nil |
| TUI 响应式状态 | `tui/state.go` | ✅ State[string] |
| 模型上下文窗口表 | `provider/models.go` | ✅ 30+ 模型 |

### 1.4 7 个破坏性变更风险点

| 风险 | 描述 | 缓解 |
|------|------|------|
| R1 | `Engine.Provider()` 语义变化 | 返回当前活跃 provider，调用者无感 |
| R2 | `QueryLoop.provider` 不可变 | 新增 SetProvider() + 查询间检测 |
| R3 | AgentTool/TeamManager 同步 | 用 ProviderRef 间接引用 |
| R4 | `os.Setenv("OPENAI_API")` 并发不安全 | 消除全局 env 依赖 |
| R5 | `doctor.go` 硬编码 Anthropic | 按当前 provider 分派检查 |
| R6 | Session Resume 跨 provider | meta 记录 provider，Resume 时警告 |
| R7 | `env.go` auto-detect 逻辑 | 保持向后兼容，Registry 优先链 |

---

## Phase 1: Provider Registry + Model Catalog 🏗️

**预计：2-3 天 | 新增文件，零破坏性变更**

### 目标
建立 Provider 和 Model 的注册表，对标 OpenCode 的 `internal/llm/models/` + `internal/llm/provider/`。

### Step 1.1: Model Catalog (`provider/model_catalog.go`) — 新建

```go
// ModelInfo 描述一个模型的能力和元数据
type ModelInfo struct {
    ID            string  // "claude-sonnet-4-20250514"
    Name          string  // "Claude Sonnet 4"
    Provider      string  // "anthropic"
    ContextWindow int
    MaxOutput     int
    CostPer1MIn   float64
    CostPer1MOut  float64
    CanReason     bool
    CanUseTools   bool
    CanSeeImages  bool
    CacheControl  bool
    APIFormat     string  // "messages", "chat-completions", "responses"
    IsDefault     bool
}

type ModelCatalog struct { models map[string]ModelInfo }

func NewModelCatalog() *ModelCatalog
func (c *ModelCatalog) Register(m ModelInfo)
func (c *ModelCatalog) Get(id string) (ModelInfo, bool)
func (c *ModelCatalog) ListByProvider(provider string) []ModelInfo
func (c *ModelCatalog) All() []ModelInfo
func (c *ModelCatalog) DefaultForProvider(provider string) string
func DefaultCatalog() *ModelCatalog  // 包含所有已知模型
```

数据来源：从 `modelContextWindows` 迁移，扩展定价和能力字段。

### Step 1.2: Provider Registry (`provider/registry.go`) — 新建

```go
type ProviderInfo struct {
    Name        string   // "anthropic"
    DisplayName string   // "Anthropic"
    EnvKey      string   // "ANTHROPIC_API_KEY"
    Models      []string
    AuthMethods []string // "api_key", "oauth_pkce", "device_code"
    Popularity  int      // 排序权重
}

type ProviderFactory func(cfg Config, modelOverride string) (Provider, error)

type ProviderRegistry struct {
    providers map[string]ProviderInfo
    factories map[string]ProviderFactory
    catalog   *ModelCatalog
}

func NewProviderRegistry() *ProviderRegistry
func (r *ProviderRegistry) Register(info ProviderInfo, factory ProviderFactory)
func (r *ProviderRegistry) Get(name string) (ProviderInfo, bool)
func (r *ProviderRegistry) Create(name string, cfg Config, model string) (Provider, error)
func (r *ProviderRegistry) All() []ProviderInfo
func (r *ProviderRegistry) Available() []ProviderInfo  // 有 API key 的
func (r *ProviderRegistry) Catalog() *ModelCatalog
func DefaultRegistry() *ProviderRegistry  // 内含 10 个 provider
```

### Step 1.3: 注册内置 Providers (`provider/registry_builtins.go`) — 新建

将 `env.go` switch-case 拆分为 10 个独立注册函数：
`registerAnthropic`, `registerOpenAI`, `registerBedrock`, `registerVertex`,
`registerOllama`, `registerDeepSeek`, `registerGemini`, `registerGroq`,
`registerMistral`, `registerOAuth`

### Step 1.4: 集成到 `env.go`（向后兼容）

`NewFromEnvWithOverrides()` 内部改用 Registry，外部行为 100% 不变。

### 验证清单
```bash
go build ./...                                                          # 编译通过
go test ./provider/... -v -count=1                                      # 现有测试通过
go test -run TestDefaultRegistry_AllProviders ./provider/... -v         # 10 个 provider
go test -run TestDefaultCatalog_ModelCount ./provider/... -v            # 所有模型
ANTHROPIC_API_KEY=test go test -run TestNewFromEnv_Anthropic ./provider/... -v  # 行为不变
```

### 新增文件
```
gosrc/provider/model_catalog.go, model_catalog_test.go
gosrc/provider/registry.go, registry_test.go
gosrc/provider/registry_builtins.go
```

### 回滚策略
全部新增文件 + env.go 内部重构。`git checkout provider/env.go && rm provider/{model_catalog,registry,registry_builtins}*.go`

---

## Phase 2: 统一凭证存储 🔐

**预计：2 天 | 新增文件 + 激活死代码**

### 目标
建立统一凭证存储 `~/.claude-go/auth.json`，激活 `auth/` 死代码包。

### Step 2.1: CredentialStore (`provider/credential_store.go`) — 新建

```go
type CredentialEntry struct {
    Provider     string    `json:"provider"`
    AuthMethod   string    `json:"auth_method"`  // "api_key", "oauth", "env"
    APIKey       string    `json:"api_key,omitempty"`
    AccessToken  string    `json:"access_token,omitempty"`
    RefreshToken string    `json:"refresh_token,omitempty"`
    ExpiresAt    time.Time `json:"expires_at,omitempty"`
    BaseURL      string    `json:"base_url,omitempty"`
    LastUsed     time.Time `json:"last_used"`
}

type CredentialStore struct { path string; entries map[string]CredentialEntry; mu sync.RWMutex }

func NewCredentialStore() (*CredentialStore, error)
func (s *CredentialStore) Get(provider string) (CredentialEntry, bool)
func (s *CredentialStore) Set(entry CredentialEntry) error
func (s *CredentialStore) Delete(provider string) error
func (s *CredentialStore) All() []CredentialEntry
func (s *CredentialStore) HasCredentials(provider string) bool
func (s *CredentialStore) MigrateFromEnv() int  // 从环境变量导入
```

文件位置：`~/.claude-go/auth.json`（600 权限），JSON 格式。

### Step 2.2: 集成到 Registry

`Available()` 同时检查环境变量和 CredentialStore。

### Step 2.3: 激活 auth 死代码

修改 `provider/env.go` 的 `case "oauth"` — 使用 `auth.AuthMiddleware` 而非裸 token，
并设置 `RetryConfig.OnAuthError` 回调实现 401 自动刷新。

### 验证清单
```bash
go build ./...
go test ./provider/... -v -count=1
go test -run TestCredentialStore ./provider/... -v
ANTHROPIC_API_KEY=test go test -run TestMigrateFromEnv ./provider/... -v
grep -r '"github.com/agent-adaptor/luban/auth"' gosrc/provider/ | wc -l  # >= 1
go test ./... -count=1
```

### 新增/修改文件
```
新增: provider/credential_store.go, credential_store_test.go
修改: provider/env.go (OAuth case), provider/registry.go (Available)
```

### 回滚策略
`git checkout provider/env.go provider/registry.go && rm provider/credential_store*.go`

---

## Phase 3: Engine 层 Provider 热替换 🔄

**预计：3-4 天 | 核心架构变更，⚠️ 高风险**

### 目标
让 Engine + QueryLoop 支持运行时 Provider 替换，消除"Provider 终身不变"限制。

### Step 3.1: ProviderRef (`provider/ref.go`) — 新建

```go
// ProviderRef 是线程安全的、可交换的 Provider 引用。
// 本身实现 Provider + CapabilityProvider 接口（完全透明代理）。
type ProviderRef struct {
    mu       sync.RWMutex
    current  Provider
    onChange []func(Provider)
}

func NewProviderRef(p Provider) *ProviderRef
func (r *ProviderRef) Get() Provider
func (r *ProviderRef) Swap(p Provider) Provider  // 原子替换，通知 listeners
func (r *ProviderRef) OnChange(fn func(Provider))

// 实现 Provider 接口 — 代理到 current
func (r *ProviderRef) Name() string
func (r *ProviderRef) ModelID() string
func (r *ProviderRef) CreateStream(ctx, params) (<-chan StreamEvent, error)
func (r *ProviderRef) Capabilities() ProviderCapabilities
```

**关键设计：** ProviderRef 实现 Provider 接口 → 所有现有代码可零改动接收。

### Step 3.2: Engine 层

**engine/engine.go** — 新增 `SetProvider(p Provider) error` 和 `ProviderRef() *ProviderRef`
**engine/config.go** — 新增 `ProviderRef *provider.ProviderRef` 字段
**engine/core.go** — `CoreEngine` 持有 `providerRef`；`New()` 自动将 `Config.Provider` 包装为 `ProviderRef`；
`Provider()` 代理到 `providerRef.Get()`；`SetProvider()` 调用 `providerRef.Swap()`

### Step 3.3: QueryLoop

**loop/query.go** — `providerRef *provider.ProviderRef` 替代 `provider Provider`；
`New()` 接收 `*ProviderRef`；每次 query 开始时 `p := ql.providerRef.Get()` 获取快照。

**关键约束：** Provider 替换在查询**之间**生效。进行中的流全程使用同一 provider。

### Step 3.4: 消费者迁移

**registry_setup.go** — 函数签名改为 `SetupRegistry(pRef *provider.ProviderRef, ...)`
**tools/agent_tool.go** — `ProviderRef *provider.ProviderRef` 替代 `Provider provider.Provider`
**tools/team_manager.go** — 同上

### Step 3.5: main.go 迁移

```go
p, err := provider.NewFromEnvWithOverrides(opts.Provider, opts.Model)
pRef := provider.NewProviderRef(p)
deps := SetupRegistry(pRef, cwd, ...)
eng, err := engine.New(engine.Config{ProviderRef: pRef, ...})
```

### Step 3.6: 消除 os.Setenv 并发风险

`opts.API` 通过新增 `FactoryOpts` 参数传入工厂，不再写全局环境。

### 验证清单
```bash
go build ./...
go test ./... -count=1
go test -run TestProviderRef_ConcurrentSwap -race ./provider/... -v
go test -run TestEngine_SetProvider ./engine/... -v
go test -run TestAgentTool_ProviderRef ./tools/... -v
grep -rn 'os\.Setenv' gosrc/ --include='*.go' | grep -v _test.go | grep -v NO_COLOR  # 预期 0
go test -race ./... -count=1
```

### 新增/修改文件
```
新增: provider/ref.go, ref_test.go
修改: provider/env.go, engine/engine.go, engine/config.go, engine/core.go,
      loop/query.go, registry_setup.go, tools/agent_tool.go,
      tools/team_manager.go, main.go, repl.go, repl_tui.go
```

### 回滚策略
改动面大。开始前 `git tag phase3-baseline`。回滚 = `git reset --hard phase3-baseline`。

---

## Phase 4: `/connect` + 增强 `/model` 🔌

**预计：2-3 天 | 中等复杂度**

### 目标
OpenCode 风格的 `/connect` 统一认证入口 + `provider/model` 格式的 `/model` 命令。

### Step 4.1: `/connect` 命令 (`commands/connect.go`) — 新建

无参数：列出所有 provider + 连接状态 (✅/❌)
有参数：`/connect anthropic` → 输入 API key → 保存到 CredentialStore
         `/connect oauth`     → 启动 OAuth PKCE 流程

```
> /connect
  ✅ anthropic  — Connected (API key)
  ✅ openai     — Connected (API key)
  ❌ gemini     — Not connected
  ❌ ollama     — Not connected (local)
Use /connect <provider> to set up authentication.
```

### Step 4.2: 增强 `/model`

支持 3 种格式：
1. `provider/model` → 切换 provider + model（如 `/model openai/o3`）
2. `model-name` → 当前 provider 内切换
3. 无参数 → 列出所有可用模型

```
> /model
Current: anthropic/claude-sonnet-4-20250514
Available:
  anthropic/claude-sonnet-4  [200K, $3/$15]  ← current
  openai/gpt-4o              [128K, $2.5/$10]
  openai/o3                  [200K, $15/$60]  (reasoning)
  gemini/gemini-2.5-pro      [1M,  $1.25/$5]

> /model openai/o3
✅ Switched to openai/o3 (200K context, reasoning)
```

### Step 4.3: commands.Context 增加 Provider 信息

```go
type Context struct {
    // ... 现有字段 ...
    CurrentProvider  string
    ProviderRef      *provider.ProviderRef
    ProviderRegistry *provider.ProviderRegistry
    CredentialStore  *provider.CredentialStore
}
```

### Step 4.4: QueryLooper 新增 SetProvider

```go
type QueryLooper interface {
    // ... 现有方法 ...
    SetProvider(p provider.Provider)
}
```

repl.go / repl_tui.go 的 `engineQueryLooper` 实现：调用 `eng.SetProvider(p)` + 更新 model。

### 验证清单
```bash
go build ./...
go test -run TestRegisterBuiltins_HasConnect ./commands/... -v
go test -run TestModelCmd_ProviderSlashModel ./commands/... -v
go test ./... -count=1
```

### 新增/修改文件
```
新增: commands/connect.go, connect_test.go, commands/models.go
修改: commands/commands.go, commands/builtins.go, repl.go, repl_tui.go
```

---

## Phase 5: Session Provider/Model 追踪 📝

**预计：1-2 天 | 低风险**

### 目标
SessionMeta 记录 Provider/Model，支持 Resume 时感知，历史列表显示。

### Step 5.1: 扩展 SessionMeta

```go
type SessionMeta struct {
    // ... 现有字段 ...
    Provider string `json:"provider,omitempty"` // 新增
    Model    string `json:"model,omitempty"`    // 新增
}
```

### Step 5.2: Query 后更新 meta

engine/core.go 的 Query() 结束时保存当前 provider/model 到 session meta。

### Step 5.3: Resume 时跨 provider 警告

如果 session 的 provider ≠ 当前 provider，记录日志 + 提示用户。
不自动切换（避免意外行为）。

### Step 5.4: Session 列表显示

`/session list` 输出增加 provider/model 列。

### 验证清单
```bash
go build ./...
go test -run TestSessionMeta_ProviderModel ./session/... -v
go test -run TestSessionMeta_BackwardCompat ./session/... -v
go test ./... -count=1
```

### 修改文件
```
session/session.go, engine/core.go, commands/builtins.go
```

**回滚安全：** JSON `omitempty` 保证向后兼容，旧 meta 文件可正常读取。

---

## Phase 6: Doctor 多 Provider 🩺

**预计：1-2 天 | 低风险**

### 目标
`/doctor` 从 Anthropic 硬编码改为多 Provider 感知。

### Step 6.1: 重构检查框架

```go
type healthCheck struct {
    Name     string
    Fn       func(ctx *Context) (bool, string)
    Provider string // "" = 通用, "anthropic" = 仅 Anthropic
}
```

通用检查（Go version, Network, Disk, Config）对所有 provider 执行。
Provider-specific 检查（API key 验证、server 可达性）按当前 provider 过滤。

### Step 6.2: 新增检查

- `checkOpenAIKey` — 验证 OPENAI_API_KEY（或 CredentialStore）
- `checkGeminiKey` — 验证 GEMINI_API_KEY
- `checkOllamaServer` — `http://localhost:11434/api/tags` 可达性
- `checkModelAvailable` — 当前 model 是否在 Catalog 中

### 验证清单
```bash
go build ./...
OPENAI_API_KEY=test go test -run TestDoctor_OpenAI ./commands/... -v    # 不再 panic
go test -run TestDoctor_FilterByProvider ./commands/... -v
go test ./... -count=1
```

---

## Phase 7: OAuth 流程集成 🔑

**预计：3-4 天 | 中等复杂度**

### 目标
`/connect` 支持 OAuth PKCE 登录 + Device Authorization Grant。

### Step 7.1: Anthropic OAuth 配置 (`auth/providers.go`) — 新建

```go
func AnthropicOAuthConfig() OAuthConfig {
    return OAuthConfig{
        ClientID: "claude-code-go",
        AuthURL:  "https://console.anthropic.com/oauth/authorize",
        TokenURL: "https://console.anthropic.com/oauth/token",
        Scopes:   []string{"user:inference"},
    }
}
```

### Step 7.2: Device Authorization Grant (`auth/device_auth.go`) — 新建

RFC 8628 Device Auth 流程：请求 device code → 显示 user code + verification URI → 轮询 token。

```go
type DeviceAuthConfig struct {
    ClientID      string
    DeviceAuthURL string
    TokenURL      string
    Scopes        []string
    PollInterval  time.Duration
}

func StartDeviceAuthFlow(ctx context.Context, cfg DeviceAuthConfig) (*TokenResponse, error)
```

### Step 7.3: `/connect` OAuth 路径

- `/connect anthropic` + OAuth → 启动 PKCE → 自动打开浏览器
- `/connect codex` → 启动 Device Auth → 显示 user code

### Step 7.4: RetryProvider 401 钩子

OAuth 认证的 provider 自动设置 `OnAuthError` 回调 → 401 时尝试 refresh → 成功则重试一次。

### 验证清单
```bash
go build ./...
go test -run TestDeviceAuth ./auth/... -v
go test -run TestOAuthPKCE_MockServer ./auth/... -v
go test -run TestRetryProvider_OnAuthError ./provider/... -v
go test ./... -count=1
```

### 新增文件
```
auth/providers.go, auth/device_auth.go, auth/device_auth_test.go
```

---

## Phase 8: TUI 增强 🖥️

**预计：2-3 天 | 低风险**

### Step 8.1: 标题栏增强

```
LUBAN Code — deepseek/deepseek-v4-flash [1.0M] [$0.3/$1.2]
```

### Step 8.2: Model Picker 快捷键

Meta+P → FuzzyPicker 弹窗，从 ProviderRegistry.Catalog() 获取模型列表。

### Step 8.3: 连接状态指示

新增 `tui/provider_status.go` — 显示各 provider 连接状态。

### Step 8.4: `/model` 切换联动 TUI

切换后 `r.Banner()` 更新标题栏。

### 验证清单
```bash
go build ./...
go test ./... -count=1
# 手动验证: --tui 标题栏、/model 切换、Meta+P 弹窗
```

---

## Phase 9: Cost Tracker 多 Provider 💰

**预计：1 天 | 低风险**

### Step 9.1: ModelInfo 驱动定价

`CalculateCostFromCatalog(model, catalog, usage)` — 使用 ModelInfo 的 CostPer1M 字段。

### Step 9.2: 多 Model 累计

CostTracker 按 model 分组累计，`/cost` 输出按 provider/model 分组。

### 验证清单
```bash
go build ./...
go test -run TestCostTracker_MultiModel ./ui/... -v
go test -run TestCostCmd_MultiProvider ./commands/... -v
```

---

## 总体依赖图

```
Phase 1 (Registry + Catalog)
    ↓
Phase 2 (Credential Store)
    ↓
Phase 3 (ProviderRef 热替换)  ← 核心基础
    ↓
    ├──→ Phase 4 (/connect + /model)
    ├──→ Phase 5 (Session tracking)  ← 可并行
    └──→ Phase 6 (Doctor)            ← 可并行
           ↓
    Phase 7 (OAuth)   ← 依赖 Phase 4
           ↓
    Phase 8 (TUI)     ← 依赖 Phase 4
           ↓
    Phase 9 (Cost)    ← 依赖 Phase 1
```

**并行机会：**
- Phase 4 + 5 + 6 可同时开工（Phase 3 完成后）
- Phase 7 + 9 可与 Phase 8 并行

---

## 时间线估算

| Phase | 内容 | 预计 | 累计 | 风险 |
|-------|------|------|------|------|
| 1 | Registry + Catalog | 2-3 天 | 3 天 | 低 |
| 2 | 凭证存储 | 2 天 | 5 天 | 低 |
| 3 | ProviderRef 热替换 | 3-4 天 | 9 天 | **高** |
| 4 | /connect + /model | 2-3 天 | 12 天 | 中 |
| 5 | Session 追踪 | 1-2 天 | 14 天 | 低 |
| 6 | Doctor 多 Provider | 1-2 天 | 16 天 | 低 |
| 7 | OAuth 流程 | 3-4 天 | 20 天 | 中 |
| 8 | TUI 增强 | 2-3 天 | 23 天 | 低 |
| 9 | Cost 多 Provider | 1 天 | 24 天 | 低 |

**里程碑：**
- **Phase 3 完成 (~9天)**: 基础架构就绪，Provider 可运行时切换
- **Phase 4 完成 (~12天)**: 用户可通过 `/connect` + `/model provider/model` 操作
- **Phase 7 完成 (~20天)**: OAuth 认证完整可用
- **Phase 9 完成 (~24天)**: 全功能多 Provider 支持

---

## 附录 A: OpenCode 设计对标

| OpenCode 概念 | 本项目对应 | Phase |
|--------------|-----------|-------|
| `Provider` interface | `provider.Provider` (已有) | — |
| `NewProvider()` factory | `ProviderRegistry.Create()` | 1 |
| `SupportedModels` map | `ModelCatalog` | 1 |
| `auth.json` credential store | `CredentialStore` | 2 |
| `setProviderDefaults()` | `resolveProviderType()` (增强) | 1 |
| `/connect` command | `/connect` | 4 |
| `/models` command | `/model` (无参数) | 4 |
| `provider/model` format | `/model` 解析器 | 4 |
| OAuth for Anthropic | `auth.AnthropicOAuthConfig()` | 7 |
| OAuth for Codex | `auth.DeviceAuth` | 7 |
| `ProviderPopularity` sorting | `ProviderInfo.Popularity` | 1 |
| `Model.CostPer1M{In,Out}` | `ModelInfo.CostPer1M{In,Out}` | 1 |
| `Model.CanReason` | `ModelInfo.CanReason` | 1 |
| config agent model selection | `CredentialStore.IsDefault` | 2 |

## 附录 B: 全量验证脚本

每个 Phase 完成后执行：

```bash
#!/bin/bash
set -e
echo "=== 编译检查 ===" && go build ./...
echo "=== 全量测试 ===" && go test ./... -count=1
echo "=== Race 检测 ===" && go test -race ./provider/... ./engine/... ./loop/... ./auth/... -count=1
echo "=== auth 死代码检查 ==="
grep -r '"github.com/agent-adaptor/luban/auth"' gosrc/ --include='*.go' | grep -v _test.go | wc -l
echo "=== os.Setenv 检查 ==="
grep -rn 'os\.Setenv' gosrc/ --include='*.go' | grep -v _test.go | grep -v NO_COLOR | wc -l
echo "=== ProviderRef 使用检查 ==="
grep -rn 'ProviderRef' gosrc/ --include='*.go' | grep -v _test.go | wc -l
echo "✅ 全部通过"
```

## 附录 C: 文件变更总汇

| Phase | 新增文件 | 修改文件 |
|-------|---------|---------|
| 1 | 5 (model_catalog, registry, builtins, tests) | 1 (env.go) |
| 2 | 2 (credential_store, test) | 2 (env.go, registry.go) |
| 3 | 2 (ref.go, test) | 11 (env, engine×3, loop, registry_setup, tools×2, main, repl×2) |
| 4 | 3 (connect, models, test) | 4 (commands×2, repl×2) |
| 5 | 0 | 3 (session, core, builtins) |
| 6 | 0 | 1 (doctor.go) |
| 7 | 3 (providers, device_auth, test) | 1 (connect.go) |
| 8 | 1 (provider_status.go) | 2 (root.go, repl_tui.go) |
| 9 | 0 | 2 (cost.go, builtins) |
| **总计** | **16 个新文件** | **~27 处修改** |
