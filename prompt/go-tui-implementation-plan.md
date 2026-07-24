# Go-TUI 重写实施计划

> 目标：使用 [go-tui](https://github.com/grindlemire/go-tui) 框架重写 `gosrc/` 的终端 UI 层，
> 对标 `./src`（TypeScript/Ink 深度 Fork）的完整使用体验。

---

## 1. 框架选型确认

### go-tui (github.com/grindlemire/go-tui)

| 属性 | 值 |
|------|-----|
| 版本 | v0.11.0 (Apr 3, 2026) |
| 许可证 | MIT |
| Stars | 225 |
| 依赖 | 仅 `golang.org/x/{sys,tools}`，纯 Go 实现 |
| 布局引擎 | 纯 Go Flexbox（基于 Yoga 算法，无 CGO）|
| 渲染器 | 双缓冲 + diff 算法，最小化终端写入 |
| 模板 | `.gsx` 文件 → `tui generate` → 类型安全 Go 代码 |
| 样式 | Tailwind 风格类名（`flex-col`, `text-cyan`, `p-4` 等）|
| 状态 | 泛型 `State[T]` 响应式，自动重绘 |
| 事件 | 键盘/鼠标处理、定时器、通道监视 |
| 组件 | 结构化组件 + `{children...}` 插槽 + 引用 + 焦点管理 |
| 模态 | 内置模态对话框（背景遮罩、焦点捕获、抢占按键）|
| 渲染模式 | 全屏模式 / 内联模式 / 单帧 Print 模式 |
| 滚动 | 内置可滚动容器 + 键盘导航 |
| 动画 | 帧循环旋转器、缓动进度、颜色波浪、脉动边框 |
| 编辑器 | VS Code / Neovim / Helix 扩展 + LSP 代理 |

**选型理由：**
- 声明式 `.gsx` 语法对标 `./src` 的 JSX/React 范式
- 纯 Go Flexbox 布局对标 `./src` 的 Yoga 布局引擎
- 双缓冲 diff 渲染对标 `./src` 的 cell-based blit 优化
- `State[T]` 响应式对标 `./src` 的 React Hooks 状态管理
- 内置模态、焦点、滚动、动画系统覆盖了 `./src` 的核心 UI 能力
- 零外部依赖，纯 Go 实现，与项目技术栈一致

---

## 2. 全面 Gap Analysis：src/ vs gosrc/tui/

### 2.0 规模对比

| 维度 | src/ (TypeScript/Ink) | gosrc/tui/ (Go) | 差距 |
|------|----------------------|-----------------|------|
| 文件数 | 439+ 组件 + 293 命令 | 7 文件 | ~730 vs 7 |
| 代码行数 | ~180K+ 行 TS/TSX | ~2,860 行 Go | ~63:1 |
| 消息类型 | 30+ 种 + 42 个渲染文件 | 10 种 MsgKind | 缺 20+ 种 |
| 权限 UI | 15+ 工具专用 + 66 文件 | 1 通用对话框 | 缺专用 UI |
| 键绑定上下文 | 17 个上下文 / 100+ 绑定 | 2 个 (perm + global) / ~12 绑定 | 缺 15 上下文 |
| 设计系统 | 16 个复用组件 | 0 | 全部缺失 |
| 弹窗/模态 | 10+ 种 | 1 (权限) | 缺 9+ 种 |
| 斜杠命令 | 100+ | 18 | 缺 80+ |
| 主题 | 6 个 / 70+ 颜色键 | 0 (硬编码颜色) | 全部缺失 |
| Markdown | marked + cli-highlight + tree-sitter | goldmark AST (无语法高亮) | 缺高亮 |
| 输入 | 2339 行多功能输入 | 基础 TextArea | 缺大量功能 |

### 2.1 消息渲染 Gap

| 消息类型 | src/ 实现 | gosrc/ 状态 | Gap |
|---------|-----------|------------|-----|
| AssistantText | 30KB 流式 Markdown + 语法高亮 | ✅ 基础流式 Markdown | 缺语法高亮、内联格式 |
| AssistantThinking | 8KB 折叠/展开 + 最小显示时间 2s | ✅ 基础折叠 | 缺计时、展开交互 |
| RedactedThinking | 专用组件 | ❌ | 完全缺失 |
| UserText | 28KB 含 @提及高亮 | ✅ 基础 | 缺提及高亮 |
| UserImage | 图片协议渲染 | ❌ | 完全缺失 |
| ToolUse (Bash) | 74KB 带命令预览+输出流 | ✅ 基础预览 | 缺输出流 |
| ToolUse (Read/Write/Edit) | 专用 diff 预览 | ✅ 通用 diff | 缺文件专用 UI |
| ToolUse (WebFetch) | 22KB URL 预览 | ❌ 通用 | 缺专用 UI |
| ToolUse (Notebook) | 专用 | ❌ | 完全缺失 |
| SystemText | 78KB 多态系统消息 | ✅ 基础 | 缺子类型 |
| APIError | 专用错误格式 | ✅ (MsgError) | 基本够用 |
| RateLimit | 专用提示 + 重试选项 | ❌ | 完全缺失 |
| CompactBoundary | 会话压缩分隔 | ❌ | 完全缺失 |
| TaskAssignment | 任务分配消息 | ❌ | 完全缺失 |
| Attachment | 70KB 附件渲染 | ❌ | 完全缺失 |
| Advisor | AI 建议消息 | ❌ | 完全缺失 |
| CollapsedReadSearch | 76KB 折叠批量搜索 | ❌ | 完全缺失 |
| GroupedToolUse | 批量工具调用 | ✅ 基础分组 | 缺批量折叠 |
| PlanApproval | 25KB 计划审批 | ❌ | 完全缺失 |
| HookProgress | 钩子进度 | ❌ | 完全缺失 |
| TeammateMessage | 24KB Agent 消息 | ❌ | 完全缺失 |
| AgentNotification | Agent 通知 | ❌ | 完全缺失 |
| ChannelMessage | 频道消息 | ❌ | 完全缺失 |

### 2.2 输入系统 Gap

| 功能 | src/ PromptInput (2339行/347KB) | gosrc/ | Gap |
|------|-------------------------------|--------|-----|
| 多行编辑 | ✅ Enter 发送 / Ctrl+J 换行 / Shift+Enter | ✅ 基础 (TextArea 多行) | ✅ 基本够用 |
| 斜杠命令补全 | ✅ typeahead (208KB) | ❌ | 完全缺失 |
| 文件路径补全 | ✅ typeahead | ❌ | 完全缺失 |
| @提及 | ✅ @file/@url/@folder | ❌ | 完全缺失 |
| 历史导航 | ✅ ↑/↓ 浏览历史 | ❌ | 完全缺失 |
| 历史搜索 | ✅ Ctrl+R 反向搜索 (30KB) | ❌ | 完全缺失 |
| 粘贴检测 | ✅ >10K 截断 + 图片粘贴 | ❌ | 完全缺失 |
| 外部编辑器 | ✅ Ctrl+G/$EDITOR | ❌ | 完全缺失 |
| 输入暂存 | ✅ Ctrl+S stash/unstash | ❌ | 完全缺失 |
| Vim 模式 | ✅ 完整 vim 键绑定 (~50KB) | ❌ | 完全缺失 |
| 模式切换 | ✅ Shift+Tab 循环模式 | ❌ | 完全缺失 |
| Shimmer 输入 | ✅ 推测补全动画 | ❌ | 完全缺失 |
| Bash 模式 | ✅ ! 前缀执行 bash | ❌ | 完全缺失 |
| 语音输入 | ✅ Push-to-talk | ❌ | 完全缺失 |
| Footer 信息栏 | ✅ 模式/模型/快捷键提示 (85KB) | ❌ | 完全缺失 |
| 建议面板 | ✅ 命令/文件建议下拉 (33KB) | ❌ | 完全缺失 |
| 帮助菜单 | ✅ 内联帮助 (32KB) | ❌ | 完全缺失 |

### 2.3 布局系统 Gap

| 功能 | src/ | gosrc/ | Gap |
|------|------|--------|-----|
| FullscreenLayout | ✅ 637行 多区域 | ✅ 基础纵向 flex | 缺 overlay/float/modal |
| ScrollBox | ✅ 虚拟滚动 + 粘性 | ✅ 基础可滚动 | 缺虚拟化 |
| 虚拟消息列表 | ✅ 1082行 高度缓存/搜索索引 | ❌ 全量渲染 | 完全缺失 |
| Unseen 消息 pill | ✅ "N new messages" 浮标 | ❌ | 完全缺失 |
| Sticky prompt | ✅ ScrollChromeContext | ❌ | 完全缺失 |
| Modal 叠加层 | ✅ 多层: overlay + float + modal | ❌ (仅权限) | 缺通用 modal |
| 底部浮动区 | ✅ bottomFloat (companion) | ❌ | 完全缺失 |

### 2.4 滚动 & 交互 Gap

| 功能 | src/ ScrollKeybindingHandler (1012行) | gosrc/ | Gap |
|------|--------------------------------------|--------|-----|
| 鼠标滚轮 | ✅ 双曲线加速 (native + xterm.js) | ✅ 固定 ±3 行 | 缺加速曲线 |
| 编码器弹跳检测 | ✅ WHEEL_BOUNCE_GAP_MAX_MS | ❌ | 完全缺失 |
| 设备检测 | ✅ 鼠标 vs 触控板自动切换 | ❌ | 完全缺失 |
| 动量衰减 | ✅ 指数衰减 | ❌ | 完全缺失 |
| 文本选择 | ✅ useCopyOnSelect + shift+arrow | ✅ 鼠标拖选 | 缺键盘选择 |
| NoSelect 区域 | ✅ 行号/gutter 不可选 | ❌ | 完全缺失 |
| 键盘滚动 | ✅ PageUp/Down + Ctrl+Home/End | ✅ 基础 | 基本够用 |
| Vim 滚动 | ✅ j/k/G/gg/Ctrl+u/d | ❌ | 完全缺失 |

### 2.5 键绑定系统 Gap

src/ 定义了 17 个键绑定上下文 (341行 `defaultBindings.ts`):

| 上下文 | src/ 绑定数 | gosrc/ 状态 | Gap |
|--------|-----------|------------|-----|
| Global | 10+ | ✅ 部分 (Ctrl+C/D/Y) | 缺 Ctrl+L/T/O/R, Ctrl+Shift+F/P/B/O |
| Chat | 15+ | ✅ 基础 (Enter 发送) | 缺 Shift+Tab/Meta+M/P/O/T, Ctrl+G/S/V |
| Autocomplete | 8+ | ❌ | 完全缺失 |
| Settings | 5+ | ❌ | 完全缺失 |
| Confirmation | 5+ | ✅ (y/n/a) | 缺 Ctrl+E/D, Tab/Space |
| Tabs | 4+ | ❌ | 完全缺失 |
| Transcript | 5+ | ❌ | 完全缺失 |
| HistorySearch | 5+ | ❌ | 完全缺失 |
| Scroll | 10+ | ✅ 部分 | 缺 Ctrl+Shift+C/Cmd+C |
| Help | 3+ | ❌ | 完全缺失 |
| Attachments | 5+ | ❌ | 完全缺失 |
| Footer | 6+ | ❌ | 完全缺失 |
| MessageSelector | 15+ | ❌ | 完全缺失 |
| MessageActions | 10+ | ❌ | 完全缺失 |
| DiffDialog | 5+ | ❌ | 完全缺失 |
| ModelPicker | 2+ | ❌ | 完全缺失 |
| Select | 5+ | ❌ | 完全缺失 |

### 2.6 权限系统 UI Gap

src/ 有 66 个权限相关文件，15+ 工具专用 UI：

| 工具类型 | src/ 实现 | gosrc/ 状态 | Gap |
|---------|-----------|------------|-----|
| Bash | 74KB 专用 (命令高亮+风险分析) | 通用对话框 | 缺专用 UI |
| FileEdit | 专用 (diff 预览) | 通用对话框 | 缺 diff 预览 |
| FileWrite | 专用 (路径+内容预览) | 通用对话框 | 缺专用 UI |
| WebFetch | 22KB (URL 预览) | 通用对话框 | 缺专用 UI |
| PowerShell | 38KB 专用 | 通用对话框 | 缺专用 UI |
| Notebook | 专用 | 通用对话框 | 缺专用 UI |
| AskUser | 81KB + 7 子文件 | 通用对话框 | 缺用户问答 UI |
| PlanMode | 119KB Exit/Enter | 通用对话框 | 缺计划模式 |
| Skill | 36KB | 通用对话框 | 缺专用 UI |
| Filesystem | 专用 | 通用对话框 | 缺专用 UI |
| 规则管理 | 8 文件 (116KB RuleList) | ❌ | 完全缺失 |

### 2.7 Markdown & 代码渲染 Gap

| 功能 | src/ | gosrc/ | Gap |
|------|------|--------|-----|
| Markdown 解析 | marked lexer + LRU 缓存 (500) | goldmark AST | ✅ 功能相当 |
| 语法高亮 | cli-highlight + tree-sitter (Rust) | ❌ 纯绿色文本 | 完全缺失 |
| 流式渲染 | StreamingMarkdown (单调边界优化) | StreamRenderer (块级增量) | ✅ 相当 |
| Markdown 表格 | MarkdownTable 组件 (46KB) | ❌ | 完全缺失 |
| Diff 展示 | StructuredDiff (24KB) + tree-sitter 高亮 | 基础 +/- 着色 | 缺结构化 diff |
| 代码行号 | ✅ NoSelect gutter | ❌ | 完全缺失 |
| 链接可点击 | ✅ URL 检测 + 打开 | ❌ (仅样式) | 完全缺失 |
| 快速检测 | ✅ hasMarkdownSyntax 正则 (前500字符) | ❌ | 完全缺失 |

### 2.8 Spinner / 加载 Gap

| 功能 | src/ Spinner (562行 + 12 子文件 = ~246KB) | gosrc/ | Gap |
|------|------------------------------------------|--------|-----|
| 基础旋转 | ✅ SpinnerGlyph 多帧 | ✅ 10 帧 braille | ✅ 基本够用 |
| 工具名显示 | ✅ 动态工具名 | ✅ "Running {tool}..." | ✅ 基本够用 |
| Shimmer 效果 | ✅ ShimmerChar (3KB) | ❌ | 完全缺失 |
| Glimmer 消息 | ✅ GlimmerMessage (26KB) | ❌ | 完全缺失 |
| 闪烁字符 | ✅ FlashingChar (6KB) | ❌ | 完全缺失 |
| Teammate 树 | ✅ TeammateSpinnerTree (27KB) + Line (38KB) | ❌ | 完全缺失 |
| Stall 检测 | ✅ useStalledAnimation | ❌ | 完全缺失 |
| Token 计数器 | ✅ verbose 模式显示 | ❌ | 完全缺失 |
| 思考状态追踪 | ✅ 最小 2s 显示 | ❌ | 完全缺失 |
| Verbose 模式 | ✅ 展开详情 | ❌ | 完全缺失 |
| Brief 模式 | ✅ BriefSpinner (Kairos) | ❌ | 完全缺失 |

### 2.9 弹窗 / 模态 / 设置 Gap

| 弹窗 | src/ 大小 | gosrc/ | Gap |
|------|----------|--------|-----|
| Settings | 265KB Config + 18KB Settings + 25KB Status + 39KB Usage | ❌ | 完全缺失 |
| ModelPicker | 53KB | ❌ | 完全缺失 |
| ThemePicker | 35KB | ❌ | 完全缺失 |
| HelpV2 | 3 文件 (~20KB) | ❌ | 完全缺失 |
| GlobalSearch | 43KB | ❌ | 完全缺失 |
| HistorySearch | 19KB | ❌ | 完全缺失 |
| QuickOpen | 28KB | ❌ | 完全缺失 |
| DiffDialog | 42KB + 22KB DiffDetail + 25KB FileList | ❌ | 完全缺失 |
| BackgroundTasks | 114KB + 5 子对话框 | ❌ | 完全缺失 |
| Teams | 92KB TeamsDialog + 7KB Status | ❌ | 完全缺失 |
| MCP | 14 文件 (ElicitationDialog 175KB) | ❌ | 完全缺失 |
| ContextVisualization | 74KB | ❌ | 完全缺失 |
| MessageSelector | 113KB | ❌ | 完全缺失 |
| LogSelector | 196KB | ❌ | 完全缺失 |
| Feedback | 86KB | ❌ | 完全缺失 |

### 2.10 Banner / Logo Gap

| 功能 | src/ LogoV2 (543行 + 16 子文件 ~73KB) | gosrc/ | Gap |
|------|--------------------------------------|--------|-----|
| 基础 banner | ✅ Provider/Model 显示 | ✅ 基础框 | ✅ 基本够用 |
| 布局模式 | ✅ wide/narrow/condensed | ❌ 固定宽度 | 缺自适应 |
| Feed 列 | ✅ 最近活动 / changelog / onboarding | ❌ | 完全缺失 |
| ASCII art | ✅ Clawd (18KB) + AnimatedClawd | ❌ | 完全缺失 |
| 调试信息 | ✅ debug 模式路径显示 | ❌ | 完全缺失 |
| 沙盒状态 | ✅ sandbox 指示 | ❌ | 完全缺失 |
| 公告系统 | ✅ announcements | ❌ | 完全缺失 |
| Agent/Effort 显示 | ✅ | ❌ | 完全缺失 |
| 欢迎页 | ✅ WelcomeV2 (57KB) | ❌ | 完全缺失 |

### 2.11 斜杠命令 Gap

gosrc 现有 18 个命令 via commands.Registry。src/ 有 100+ 命令文件 (293 files in `src/commands/`)。

**已有 (18):** help, clear, model, config, compact, diff, export, memory, plan, fast, effort, theme, mcp, agents, tasks, hooks, permissions, vim

**缺失的重要命令:** resume (36KB), copy (41KB), cost, stats (149KB), status, context, feedback (5KB), bridge (46KB), btw (30KB), branch (9KB), tag (21KB), session (13KB), rename, review, rewind, exit, upgrade, usage, login/logout, keybindings, color, output-style, release-notes, doctor, sandbox-toggle, thinkback (61KB), ultraplan (65KB), insights (113KB), chrome (31KB), commit, version, statusline, plugin (16 sub-files ~800KB+), install-github-app (14 sub-files), install-slack-app, 等。

---

## 3. ./gosrc 现有可复用组件

### 3.1 可直接复用（纯逻辑，无 UI 依赖）

| 模块 | 文件 | 说明 |
|------|------|------|
| Commands | `commands/commands.go` + `builtins.go` | 18 命令 + Registry，通过 `Context.OnEvent` 完全解耦 |
| Permissions | `permissions/*.go` (18 文件) | Checker/Risk/Safety/Store，纯逻辑 |
| CLI 解析 | `cli/*.go` (4 文件) | 30+ 选项解析 |
| Input History | `input/history.go` | 历史记录管理 |
| Clipboard | `input/clipboard.go` | 跨平台剪贴板操作 |
| Token Check | `input/token_check.go` | Token 数量检查 |
| Event Types | `loop/events.go` | 事件类型定义 |
| Image Parse | `input/image.go` | 图片文件解析 |
| Signal Handle | `signals.go` | 终端信号处理 |
| Session Mgmt | `session.go` | 会话管理 |
| Cost Tracker | `cost.go` | 费用追踪 |
| Context Bar | `ui/context_bar.go` | 上下文格式化逻辑 |
| Safety Layer | `permissions/safety.go` | 安全检查层（新增） |

### 3.2 需要替换/重写的组件

| 组件 | 当前实现 | 问题 | 新方案 |
|------|---------|------|--------|
| **TermRenderer** | `fmt.Fprint` 到 `io.Writer` | 无布局、无双缓冲、无组件树 | → go-tui `TuiRenderer` ✅ 已完成 |
| **Spinner** | `\r` 覆写行 | 单行、无动画系统 | → go-tui 动画组件 ✅ 已完成 (基础) |
| **Reader** | chzyer/readline 阻塞 | 同步阻塞、无法集成到事件循环 | → go-tui TextArea ✅ 已完成 |
| **MultilineReader** | readline 依赖 | 同上 | → go-tui TextArea ✅ 已完成 |
| **RunREPL** | 同步 for 循环 | 阻塞主线程、无法并行渲染 | → go-tui App.Run() ✅ 已完成 |
| **RichPrompt** | `bufio.Scanner` 阻塞 | 无法嵌入 TUI | → go-tui Dialog ✅ 已完成 |
| **Markdown 渲染** | `fatih/color` | 应统一到 go-tui 样式系统 | → go-tui goldmark AST walker ✅ 已完成 |

---

## 4. 分阶段实施方案

### Phase 0-4：已完成 ✅

<details>
<summary>Phase 0: 项目初始化 (已完成)</summary>

- [x] 在 worktree (`/Users/buthim/Develop/claude-code-tui`) 中工作
- [x] `go get github.com/grindlemire/go-tui`
- [x] 创建 `gosrc/tui/` 目录结构
</details>

<details>
<summary>Phase 1: 核心骨架 (已完成)</summary>

- [x] 实现 `TuiRenderer` 适配 `ui.Renderer` 接口（20 个方法）
- [x] 将 `main.go` 中 REPL 模式切换到 go-tui App.Run()（通过 `--tui` 标志）
- [x] 基本消息展示（Text/Thinking/Error/Info/Success/Warning/Bold）
- [x] 基本文本输入（单行输入 + Enter 发送）
- [x] ToolCall / ToolResult 渲染
- [x] SpinnerStart 动画适配
- [x] Banner / SessionInfo / Prompt / Goodbye
- [x] Ctrl+C / Ctrl+D 信号处理
</details>

<details>
<summary>Phase 2: 权限 & 命令系统 (已完成)</summary>

- [x] `PermissionDialog` 组件 — 模态对话框 + 风险着色
- [x] `PermissionRequest` 方法 — 同步阻塞
- [x] 斜杠命令集成 — 18 个命令复用 Registry
- [x] 并发安全增强
</details>

<details>
<summary>Phase 3: 消息渲染增强 (已完成)</summary>

- [x] **Markdown 渲染器** — goldmark AST walker，避免 ANSI
- [x] **流式渲染** — StreamRenderer 块级增量 O(n)
- [x] **AssistantThinking** — 折叠/展开
- [x] **ToolUse 预览** — 工具专用摘要
- [x] **Diff 渲染** — +green/-red/@@ cyan
- [x] **Cost/Usage** — 状态栏
- [x] **消息分组** — ToolCall+ToolResult 成对
</details>

<details>
<summary>Phase 4: 滚动 & 布局 (已完成)</summary>

- [x] **粘性滚动** — stickToBottom + OnChange watcher
- [x] **手动滚动** — 暂停自动跟随
- [x] **键盘滚动** — Shift+Up/Down, PageUp/Down, Home/End
- [x] **鼠标滚轮** — ±3 行
- [x] **鼠标拖选复制** — Press/Drag/Release + AttrReverse 高亮 + OSC52 剪贴板
- [x] **?1002h 拖拽追踪** — 手动发送 + OnResume 恢复
</details>

---

### Phase 5: 输入增强 (5-7 天) 🎯 当前重点

**目标：** 对标 src/PromptInput (347KB/2339行) 的核心输入体验

**P0 — 必须实现（日常可用性）:**

- [ ] **历史导航** — ↑/↓ 箭头浏览命令历史
  - 复用 `input/history.go` 历史管理
  - 当输入框为空或光标在首行时，↑ 加载上一条；↓ 加载下一条
  - 保存/恢复当前编辑中的文本（history stash）
  - src 对标: `PromptInput.tsx` 的 ↑/↓ history 逻辑

- [ ] **斜杠命令补全** — 输入 `/` 后显示命令列表
  - 下拉菜单组件（最多显示 8 行），键盘 ↑/↓ 选择，Tab/Enter 确认，Esc 关闭
  - 实时模糊过滤（前缀匹配 → 子串匹配）
  - 显示命令名 + 简短描述
  - src 对标: `useTypeahead.ts` (208KB) 的命令补全部分

- [ ] **模式切换** — Shift+Tab (或 Meta+M) 循环切换
  - 至少支持 "chat" 和 "plan" 两种模式
  - 输入框边框颜色随模式变化（chat=cyan, plan=yellow）
  - 状态栏显示当前模式
  - src 对标: `PromptInputModeIndicator.tsx`, `MODE_CYCLE_KEY`

- [ ] **输入 Footer** — 输入框下方信息栏
  - 左侧: 当前模式 + 模型名
  - 右侧: 关键快捷键提示 (Shift+Enter 换行, /help)
  - src 对标: `PromptInputFooter.tsx` (32KB), `PromptInputFooterLeftSide.tsx` (85KB)

**P1 — 重要（提升效率）:**

- [ ] **@提及补全** — @file / @folder / @url
  - @ 触发文件路径补全（扫描 cwd）
  - 模糊匹配 + 目录递归
  - 补全后插入文件引用格式
  - src 对标: `useTypeahead.ts` 的 @mention 逻辑

- [ ] **历史搜索** — Ctrl+R 反向增量搜索
  - 叠加搜索输入框（类似 shell Ctrl+R）
  - 实时匹配显示候选
  - Enter 选择，Esc 取消
  - src 对标: `HistorySearchInput.tsx`, `useHistorySearch.ts` (30KB)

- [ ] **外部编辑器** — Ctrl+G (或 Ctrl+X Ctrl+E) 打开 $EDITOR
  - 暂停 TUI → 启动 $EDITOR(临时文件) → 恢复后读取内容
  - src 对标: PromptInput 的 external editor 逻辑

**P2 — 锦上添花:**

- [ ] **粘贴检测** — 大文本截断提示 + 图片粘贴
- [ ] **输入暂存** — Ctrl+S stash / Ctrl+S unstash
- [ ] **Bash 模式** — `!` 前缀直接执行 bash 命令

**关键架构决策：**
- 补全下拉使用 go-tui 的 `WithOverlay(true)` 特性覆盖消息区域
- 历史管理复用 `input/history.go`，仅新增 TUI 绑定
- 模式状态新增到 AppState: `Mode *tui.State[string]`

**新增文件：**
```
gosrc/tui/
├── autocomplete.go          # 补全下拉组件 + 过滤逻辑
├── history_nav.go           # 历史导航绑定
├── input_footer.go          # 输入框信息栏组件
└── mode.go                  # 模式管理 (chat/plan)
```

**验收标准：** ↑/↓ 浏览历史流畅，/命令补全精准，模式切换即时反映。

---

### Phase 6: 设计系统 & 主题 (3-4 天)

**目标：** 建立可复用的设计系统组件 + 完整主题支持

src/ 有 16 个设计系统组件 + 6 个主题。当前 gosrc 全部硬编码颜色。

**P0 — 设计基础：**

- [ ] **Theme 类型定义** — 70+ 语义化颜色键
  ```go
  type Theme struct {
      Name           string
      Background     tui.Color
      Foreground     tui.Color
      Primary        tui.Color  // 主色调
      Secondary      tui.Color  // 次色调
      Error          tui.Color
      Warning        tui.Color
      Success        tui.Color
      Info           tui.Color
      Muted          tui.Color  // dim text
      Border         tui.Color
      BorderFocused  tui.Color
      CodeBg         tui.Color
      CodeFg         tui.Color
      DiffAdd        tui.Color
      DiffRemove     tui.Color
      // ... 70+ keys matching src/design-system/ThemeProvider
  }
  ```
- [ ] **6 个主题** — dark, light, light-daltonized, dark-daltonized, light-ansi, dark-ansi
- [ ] **OSC 11 自动检测** — 查询终端背景色 → 自动选择 dark/light
- [ ] **运行时切换** — `/theme` 命令 + ThemePicker
- [ ] **ThemeContext** — 全局 Theme 状态，所有组件引用

**P1 — 设计系统组件：**

- [ ] **Dialog** — 基础对话框 (边框 + 标题 + 关闭键 + 焦点捕获)
  - src 对标: `design-system/Dialog.tsx` (~20KB)
- [ ] **Pane** — 面板容器 (标题 + 边框 + 内容)
  - src 对标: `design-system/Pane.tsx` (~15KB)
- [ ] **Tabs** — 标签页切换 (Tab/Shift+Tab 或 ←/→)
  - src 对标: `design-system/Tabs.tsx` (40KB)
- [ ] **FuzzyPicker** — 模糊搜索选择器 (输入 + 过滤列表 + 高亮匹配)
  - src 对标: `design-system/FuzzyPicker.tsx` (40KB)
- [ ] **ListItem** — 列表项 (选中态 + 图标 + 描述)
  - src 对标: `design-system/ListItem.tsx` (19KB)
- [ ] **ProgressBar** — Unicode 子字符精度进度条
  - src 对标: `design-system/ProgressBar.tsx` (~8KB)
- [ ] **StatusIcon** — 6 种状态图标 (success/error/warning/info/loading/pending)
  - src 对标: `design-system/StatusIcon.tsx` (~5KB)
- [ ] **Divider** — 水平分隔线
- [ ] **KeyboardShortcutHint** — 快捷键提示样式 `[Ctrl+C]`

**新增文件：**
```
gosrc/tui/
├── theme/
│   ├── theme.go             # Theme 类型定义
│   ├── dark.go              # 暗色主题
│   ├── light.go             # 亮色主题
│   ├── daltonized.go        # 色盲友好变体
│   ├── ansi.go              # ANSI 16 色回退
│   └── detect.go            # OSC 11 背景色检测
├── ds/                      # design system
│   ├── dialog.go            # Dialog 组件
│   ├── pane.go              # Pane 组件
│   ├── tabs.go              # Tabs 组件
│   ├── fuzzy_picker.go      # FuzzyPicker 组件
│   ├── list_item.go         # ListItem 组件
│   ├── progress_bar.go      # ProgressBar 组件
│   ├── status_icon.go       # StatusIcon 组件
│   ├── divider.go           # Divider 组件
│   └── shortcut_hint.go     # KeyboardShortcutHint 组件
```

**验收标准：** 所有 UI 元素统一使用 Theme 颜色键，6 主题可切换，设计系统组件可在后续 Phase 复用。

---

### Phase 7: 弹窗系统 (4-6 天)

**目标：** 实现 src/ 中 10+ 核心弹窗

依赖 Phase 6 的 Dialog/Tabs/FuzzyPicker/ListItem 设计系统组件。

**P0 — 核心弹窗：**

- [ ] **HelpDialog** — 快捷键列表 + 命令列表
  - 分 Tab 展示: Keys / Commands / About
  - src 对标: `HelpV2/` (3 文件)
  - 触发: `?` 或 `/help`

- [ ] **ModelPicker** — 模型选择
  - FuzzyPicker 搜索 + 模型列表
  - 显示: 模型名 + provider + context window
  - 触发: Meta+P 或 `/model`
  - src 对标: `ModelPicker.tsx` (53KB)

- [ ] **SettingsDialog** — 设置面板
  - 4 个 Tab: Config / Status / Usage / Permissions
  - Config: 键值对列表 (可编辑)
  - Status: 系统状态检查
  - Usage: 费用统计
  - src 对标: `Settings/` (4 文件, ~347KB 总计)

- [ ] **DiffDialog** — Diff 查看器
  - 文件列表 ← → 详情切换
  - 结构化 diff (不仅仅是 +/- 着色)
  - src 对标: `diff/DiffDialog.tsx` (42KB)

**P1 — 搜索弹窗：**

- [ ] **GlobalSearchDialog** — 对话全文搜索
  - 搜索输入 + 实时匹配 + 跳转到消息
  - 触发: Ctrl+Shift+F
  - src 对标: `GlobalSearchDialog.tsx` (43KB)

- [ ] **HistorySearchDialog** — 会话历史搜索
  - 搜索 + 预览 + 恢复会话
  - 触发: Ctrl+R (从全局)
  - src 对标: `HistorySearchDialog.tsx` (19KB)

- [ ] **QuickOpenDialog** — 快速打开
  - FuzzyPicker 搜索文件
  - 触发: Ctrl+Shift+P
  - src 对标: `QuickOpenDialog.tsx` (28KB)

**P2 — 高级弹窗：**

- [ ] **ThemePicker** — 主题预览切换
- [ ] **ContextVisualization** — 上下文窗口详情
- [ ] **Feedback** — 反馈提交

**新增文件：**
```
gosrc/tui/
├── dialogs/
│   ├── help.go              # HelpDialog
│   ├── model_picker.go      # ModelPicker
│   ├── settings.go          # SettingsDialog
│   ├── diff_dialog.go       # DiffDialog
│   ├── global_search.go     # GlobalSearchDialog
│   ├── history_search.go    # HistorySearchDialog
│   ├── quick_open.go        # QuickOpenDialog
│   ├── theme_picker.go      # ThemePicker
│   ├── context_viz.go       # ContextVisualization
│   └── feedback.go          # Feedback
```

**验收标准：** 弹窗正确打开/关闭/交互，Esc 关闭，Tab 切换，搜索实时过滤。

---

### Phase 8: 消息渲染增强 II (4-5 天)

**目标：** 补齐 src/ 的 20+ 缺失消息类型

**P0 — 核心缺失消息：**

- [ ] **CompactBoundary** — 会话压缩分隔线
  - "—— Context compacted (N messages → M) ——"
  - src 对标: `CompactBoundaryMessage.tsx`

- [ ] **RateLimit** — 速率限制提示
  - 错误信息 + 倒计时 + 重试选项
  - src 对标: `RateLimitMessage.tsx`

- [ ] **PlanApproval** — 计划审批消息
  - 展示计划步骤 + approve/reject 按钮
  - src 对标: `PlanApprovalMessage.tsx` (25KB)

- [ ] **Attachment** — 附件消息
  - 文件/URL 附件渲染 (文件名 + 类型图标 + 大小)
  - src 对标: `AttachmentMessage.tsx` (70KB)

- [ ] **CollapsedReadSearch** — 批量搜索折叠
  - 多个 Read/Search 结果折叠为一行 "Read N files"
  - 可展开查看详情
  - src 对标: `CollapsedReadSearchContent.tsx` (76KB)

**P1 — Agent/Task 消息：**

- [ ] **TaskAssignment** — 子任务分配
  - 显示任务 ID + 描述 + 分配给哪个 agent
  - src 对标: `TaskAssignmentMessage.tsx`

- [ ] **TeammateMessage** — Agent 间通信
  - Agent 名 + 消息内容
  - src 对标: `UserTeammateMessage.tsx` (24KB)

- [ ] **AgentNotification** — Agent 状态通知
  - Agent 启动/完成/错误通知
  - src 对标: `UserAgentNotificationMessage.tsx`

**P1 — ToolResult 增强：**

- [ ] **UserToolResult 子类型** — 细分工具结果
  - RejectedTool, CanceledTool, ToolError 专用样式
  - src 对标: `UserToolResultMessage/` (8 文件)

- [ ] **结构化 Diff** — 替代纯 +/- 着色
  - 文件头 + hunk 头 + 行号 gutter + 着色
  - src 对标: `StructuredDiff.tsx` (24KB)

**P2 — 其他消息类型：**

- [ ] RedactedThinking, Advisor, ChannelMessage, HookProgress
- [ ] UserImage (基础文本占位，后续支持图片协议)

**验收标准：** 所有消息类型均有对应渲染，不再出现"unknown message type"或空白渲染。

---

### Phase 9: 语法高亮 & 代码渲染 (3-4 天)

**目标：** 对标 src/ 的代码高亮质量

src/ 使用 tree-sitter (Rust native) 做语法高亮。Go 侧可选方案：

**方案选择：**
- **方案 A: chroma** (Go 原生) — alecthomas/chroma，纯 Go，支持 200+ 语言
  - 优点：零 CGO，与 goldmark 集成成熟 (goldmark-highlighting)
  - 缺点：高亮质量不如 tree-sitter
- **方案 B: tree-sitter Go 绑定** — smacker/go-tree-sitter
  - 优点：与 src/ 质量一致
  - 缺点：CGO 依赖，编译复杂度上升

**推荐方案 A (chroma)**：纯 Go、零 CGO、够用。

- [ ] **代码块语法高亮** — chroma tokenize → go-tui styled Elements
  - 集成到 `renderFencedCodeBlock()`: 语言 tag → chroma lexer → token 着色
  - 支持 200+ 语言 (Go, Python, JS, TS, Rust, etc.)

- [ ] **行号 gutter** — 代码块左侧行号
  - NoSelect 风格 (鼠标选择时跳过行号)
  - src 对标: `HighlightedCode.tsx` (190行) 的 CodeLine/gutter 逻辑

- [ ] **Markdown 表格** — 对齐的表格渲染
  - 列宽计算 + 边框绘制 + 对齐 (left/center/right)
  - src 对标: `MarkdownTable.tsx` (46KB)

- [ ] **内联格式增强** — 目前 bold/italic 被扁平化为纯文本
  - `**bold**` → Bold 样式
  - `*italic*` → Italic 样式
  - `` `code` `` → BrightGreen 背景 (保持现有行为)
  - `[link](url)` → Blue + Underline (保持现有行为)
  - 需要扩展 `extractInlineText()` 为 `renderInlineElements()` 返回多个 styled segments

- [ ] **快速 Markdown 检测** — 跳过无 Markdown 语法的纯文本
  - 正则预检前 500 字符，避免不必要的 goldmark 解析
  - src 对标: `hasMarkdownSyntax()` 正则

**新增文件：**
```
gosrc/tui/
├── highlight.go             # chroma 语法高亮集成
├── markdown_table.go        # Markdown 表格渲染
└── markdown_inline.go       # 内联格式 (bold/italic/code/link) 渲染
```

**验收标准：** 代码块有语法高亮颜色，表格对齐正确，内联格式可辨识。

---

### Phase 10: Spinner 增强 & 动画 (2-3 天)

**目标：** 对标 src/Spinner (562行 + 12 子文件 = ~246KB) 的视觉质量

**P0 — Spinner 功能增强：**

- [ ] **思考状态追踪** — 最小 2 秒显示
  - 即使思考快速完成，也保持 spinner 至少 2 秒（避免闪烁）
  - src 对标: Spinner 的 thinkingStatusStartTime tracking

- [ ] **Stall 检测** — 长时间无 token 时切换动画
  - 超过 N 秒无 token → 切换为 "Waiting..." 动画
  - src 对标: `useStalledAnimation.ts`

- [ ] **Token 计数器** — verbose 模式显示实时 token 数
  - "Running Tool... (123 tokens)"
  - src 对标: Spinner 的 tokenCount display

- [ ] **动画帧增强** — 更丰富的帧序列
  - 添加反向动画 (正向播放 → 反向播放 → 循环)
  - Shimmer 效果: 字符逐个高亮波浪
  - src 对标: `SpinnerAnimationRow.tsx` (42KB), `SpinnerGlyph.tsx` (10KB)

**P1 — 多 Agent 支持：**

- [ ] **TeammateSpinnerTree** — 多 Agent 并行状态树
  - 树状显示每个 Agent 的当前工具/状态
  - 缩进层级 + 独立动画帧
  - src 对标: `TeammateSpinnerTree.tsx` (27KB), `TeammateSpinnerLine.tsx` (38KB)

**P2 — 视觉润色：**

- [ ] FlashingChar, ShimmerChar, GlimmerMessage 等效果
- [ ] 进度条 Unicode 子字符精度
- [ ] 声音通知 (BEL)

**验收标准：** Spinner 不会短暂闪烁，stall 状态可感知，多 Agent 并行状态清晰。

---

### Phase 11: 权限系统 UI 增强 (3-4 天)

**目标：** 从通用权限对话框升级为工具专用权限 UI

src/ 有 15+ 工具专用权限界面 (66 文件)。当前 gosrc 只有一个通用对话框。

**P0 — 高频工具专用 UI：**

- [ ] **BashPermission** — 命令高亮 + 工作目录 + 风险分析
  - 显示完整命令（语法高亮）
  - 标注工作目录
  - src 对标: `BashPermissionRequest.tsx` (74KB)

- [ ] **FileEditPermission** — Diff 预览
  - 显示修改前/后对比
  - 文件路径 + 行号
  - src 对标: `FileEditPermissionRequest.tsx`

- [ ] **FileWritePermission** — 内容预览
  - 显示将写入的内容（截断 + 行数统计）
  - src 对标: `FileWritePermissionRequest.tsx`

- [ ] **WebFetchPermission** — URL 预览
  - 显示目标 URL + 请求方法
  - src 对标: `WebFetchPermissionRequest.tsx` (22KB)

**P1 — 高级权限功能：**

- [ ] **AskUser** — 用户交互问答权限
  - AI 提问 + 用户回答输入框
  - 选项列表（如果提供选项）
  - src 对标: `AskUserQuestionPermissionRequest/` (81KB + 7 子文件)

- [ ] **PlanMode Permission** — 进入/退出计划模式
  - 显示计划预览
  - src 对标: `ExitPlanModePermissionRequest.tsx` (119KB)

- [ ] **Permission Rules 管理** — 增/删/改规则
  - 规则列表 + 编辑器
  - src 对标: `rules/` (8 文件, PermissionRuleList 116KB)

**P2 — 其他工具权限：**
- [ ] PowerShell, Notebook, Skill, Filesystem, Sandbox 等专用 UI

**验收标准：** Bash/FileEdit/FileWrite 权限有专用预览，不再只是 key-value 列表。

---

### Phase 12: Agent & Task 系统 (3-5 天)

**目标：** 对标 src/ 的 Agent/Task 管理 UI

src/ 有完整的多 Agent 协作 UI：
- `tasks/` (12 文件): BackgroundTasksDialog (114KB), BackgroundTask, AsyncAgentDetailDialog, RemoteSessionDetailDialog 等
- `teams/` (2 文件): TeamsDialog (92KB), TeamStatus (7KB)
- `CoordinatorAgentStatus.tsx` (35KB), `TaskListV2.tsx` (49KB)

**P0 — Agent 状态展示：**

- [ ] **Agent 状态栏** — 活跃 Agent 列表
  - 在 Spinner 区域显示并行 Agent 状态
  - 每个 Agent: 名称 + 当前工具 + 耗时
  - src 对标: `CoordinatorAgentStatus.tsx` (35KB)

- [ ] **TaskList** — 子任务列表
  - 任务 ID + 状态 (pending/running/done/error) + 描述
  - StatusIcon + 进度条
  - src 对标: `TaskListV2.tsx` (49KB)

**P1 — Agent 管理弹窗：**

- [ ] **BackgroundTasksDialog** — 后台任务管理
  - 任务列表 + 详情面板
  - 暂停/恢复/取消操作
  - src 对标: `BackgroundTasksDialog.tsx` (114KB)

- [ ] **TeamsDialog** — 团队管理
  - Agent 列表 + 状态 + 通信日志
  - src 对标: `TeamsDialog.tsx` (92KB)

**P2 — Agent 详情：**
- [ ] AsyncAgentDetail, InProcessTeammateDetail, RemoteSessionDetail, ShellDetail 等

**验收标准：** 多 Agent 并行运行时状态清晰可见，可管理后台任务。

---

### Phase 13: Doctor & Resume & 高级功能 (持续)

**目标：** 非 REPL 屏幕 + 长尾功能

**Doctor 屏幕：**
- [ ] 诊断检查列表 + 状态图标 + 修复建议
- [ ] src 对标: `screens/Doctor.tsx` (72KB)

**ResumeConversation 屏幕：**
- [ ] 会话选择列表 + 预览
- [ ] src 对标: `screens/ResumeConversation.tsx` (58KB), `LogSelector.tsx` (196KB)

**高级功能 (持续)：**

- [ ] **Vim 模式** — 完整 Vim 键绑定 (normal/insert/visual)
  - src 对标: `PromptInput/vim/` (~50KB)

- [ ] **语音输入** — Push-to-talk
  - src 对标: `VoiceIndicator.tsx`, PromptInput voice 逻辑

- [ ] **图片协议** — sixel/kitty/iTerm2 图片内联显示
  - src 对标: UserImageMessage + image 协议检测

- [ ] **MCP UI** — MCP 服务器管理界面
  - ElicitationDialog (175KB), MCPListPanel (57KB) 等
  - src 对标: `mcp/` (14 文件)

- [ ] **虚拟消息列表** — 大对话性能优化
  - 高度缓存 + 只渲染可见区域
  - src 对标: `VirtualMessageList.tsx` (1082行/145KB)

- [ ] **高级滚动** — 滚轮加速曲线 + 弹跳检测 + 动量衰减
  - src 对标: `ScrollKeybindingHandler.tsx` (1012行/146KB)

- [ ] **Banner 增强** — 自适应布局 + Feed 列 + ASCII art
  - src 对标: `LogoV2/` (16 文件, 543行主文件)

- [ ] **StatusLine** — 底部状态栏
  - src 对标: `StatusLine.tsx` (48KB)

- [ ] **MessageActions** — 消息操作菜单 (复制/编辑/重试)
  - src 对标: `messageActions.tsx` (54KB), `MessageSelector.tsx` (113KB)

- [ ] **Stats** — 详细统计面板
  - src 对标: `Stats.tsx` (149KB)

- [ ] **Notifications** — 桌面通知 + Toast 系统
  - src 对标: `Notifications.tsx` (47KB)

- [ ] **AutoUpdater** — 自动更新
  - src 对标: `AutoUpdater.tsx` (30KB)

- [ ] **OAuth 登录流** — 控制台 OAuth
  - src 对标: `ConsoleOAuthFlow.tsx` (78KB)

- [ ] **Onboarding** — 首次使用引导
  - src 对标: `Onboarding.tsx` (31KB)

---

## 5. 架构设计

### 5.1 目录结构 (更新后)

```
gosrc/tui/                          # go-tui 新 UI 层
├── app.go                          # ✅ App 生命周期
├── root.go                         # ✅ RootComponent (主渲染树)
├── state.go                        # ✅ AppState 响应式状态
├── renderer.go                     # ✅ TuiRenderer (ui.Renderer 适配)
├── stream_renderer.go              # ✅ 流式 Markdown 增量渲染
├── markdown.go                     # ✅ goldmark AST → go-tui Elements
├── clipboard.go                    # ✅ OSC52 + 平台剪贴板
│
├── autocomplete.go                 # Phase 5: 补全下拉
├── history_nav.go                  # Phase 5: 历史导航
├── input_footer.go                 # Phase 5: 输入信息栏
├── mode.go                         # Phase 5: 模式管理
│
├── theme/                          # Phase 6: 主题系统
│   ├── theme.go                    # Theme 类型 (70+ 颜色键)
│   ├── dark.go
│   ├── light.go
│   ├── daltonized.go
│   ├── ansi.go
│   └── detect.go                   # OSC 11 自动检测
│
├── ds/                             # Phase 6: 设计系统
│   ├── dialog.go
│   ├── pane.go
│   ├── tabs.go
│   ├── fuzzy_picker.go
│   ├── list_item.go
│   ├── progress_bar.go
│   ├── status_icon.go
│   ├── divider.go
│   └── shortcut_hint.go
│
├── dialogs/                        # Phase 7: 弹窗
│   ├── help.go
│   ├── model_picker.go
│   ├── settings.go
│   ├── diff_dialog.go
│   ├── global_search.go
│   ├── history_search.go
│   ├── quick_open.go
│   ├── theme_picker.go
│   └── context_viz.go
│
├── messages/                       # Phase 8: 消息渲染增强
│   ├── compact_boundary.go
│   ├── rate_limit.go
│   ├── plan_approval.go
│   ├── attachment.go
│   ├── collapsed_read_search.go
│   ├── task_assignment.go
│   ├── teammate.go
│   └── tool_result_variants.go
│
├── highlight.go                    # Phase 9: 语法高亮 (chroma)
├── markdown_table.go               # Phase 9: Markdown 表格
├── markdown_inline.go              # Phase 9: 内联格式
│
├── permissions/                    # Phase 11: 工具专用权限 UI
│   ├── bash.go
│   ├── file_edit.go
│   ├── file_write.go
│   ├── web_fetch.go
│   ├── ask_user.go
│   ├── plan_mode.go
│   └── rules.go
│
├── agents/                         # Phase 12: Agent 管理
│   ├── status.go
│   ├── task_list.go
│   ├── tasks_dialog.go
│   └── teams_dialog.go
│
└── screens/                        # Phase 13: 非 REPL 屏幕
    ├── doctor.go
    └── resume.go
```

### 5.2 Renderer 适配层 (已实现)

```go
// gosrc/tui/renderer.go — 已实现
type TuiRenderer struct {
    app         *tui.App
    state       *AppState
    goodbyeOnce sync.Once
}

// 实现 ui.Renderer 的 20 个方法
// Text → AppendOrStreamText (流式)
// ToolCall/ToolResult → 消息追加
// PermissionRequest → 阻塞等待 PermResp channel
// SpinnerStart → 返回 stop func (sync.Once)
// 所有方法通过 QueueUpdate 保证线程安全
```

### 5.3 事件桥接 (已实现)

```go
// gosrc/repl_tui.go — 已实现 (makeTUIEventHandler)
// loop 事件 → TuiRenderer 方法 → State[T] 更新 → go-tui 自动重绘
```

---

## 6. 风险与缓解

| 风险 | 影响 | 缓解策略 |
|------|------|---------|
| go-tui v0.11 是 Pre-1.0，API 可能变 | 中 | 抽象 adapter 层隔离 API 变化 |
| .gsx 编译器 Bug | 中 | 所有组件保持纯 Go (当前无 .gsx 使用) |
| go-tui 不支持某些高级特性 | 高 | 评估后可用自定义组件或 PR 贡献 |
| 大对话性能 | 高 | 虚拟化 + 增量渲染 + 性能基准 (Phase 13) |
| chroma 语法高亮质量 | 低 | 基本够用；不满意可后续切换 tree-sitter |
| 设计系统组件复杂度 | 中 | 先做简版，迭代增强 |
| Agent/Task UI 复杂度 | 高 | 先做状态展示，后做交互管理 |

---

## 7. 时间线估算 (修订版)

| Phase | 内容 | 状态 | 估时 | 累计 |
|-------|------|------|------|------|
| 0 | 项目初始化 | ✅ 完成 | — | — |
| 1 | 核心骨架 | ✅ 完成 | — | — |
| 2 | 权限 & 命令 | ✅ 完成 | — | — |
| 3 | 消息渲染增强 | ✅ 完成 | — | — |
| 4 | 滚动 & 布局 + 鼠标拖选 | ✅ 完成 | — | — |
| **5** | **输入增强** | 🎯 下一步 | **5-7 天** | **7 天** |
| 6 | 设计系统 & 主题 | 待开始 | 3-4 天 | 11 天 |
| 7 | 弹窗系统 | 待开始 | 4-6 天 | 17 天 |
| 8 | 消息渲染增强 II | 待开始 | 4-5 天 | 22 天 |
| 9 | 语法高亮 & 代码渲染 | 待开始 | 3-4 天 | 26 天 |
| 10 | Spinner 增强 & 动画 | 待开始 | 2-3 天 | 29 天 |
| 11 | 权限系统 UI 增强 | 待开始 | 3-4 天 | 33 天 |
| 12 | Agent & Task 系统 | 待开始 | 3-5 天 | 38 天 |
| 13 | Doctor/Resume + 高级功能 | 持续 | 持续 | — |

**里程碑：**
- **Phase 5 完成 (~7天)**: 日常使用体验大幅提升（历史 + 补全 + 模式）
- **Phase 6-7 完成 (~17天)**: 功能基本齐全（主题 + 弹窗系统）
- **Phase 8-10 完成 (~29天)**: 视觉质量对标 src/（高亮 + 消息类型 + 动画）
- **Phase 11-12 完成 (~38天)**: 完整功能对标（权限 UI + Agent 管理）
- **Phase 13 (持续)**: 长尾功能和优化

**可用于日常开发的完整 TUI：~38 天（从 Phase 5 开始计）**

---

## 8. 优先级依赖图

```
Phase 5 (输入增强)
    ↓
Phase 6 (设计系统+主题)  ← 后续所有 UI 组件的基础
    ↓
Phase 7 (弹窗系统)  ← 依赖 Phase 6 的 Dialog/Tabs/FuzzyPicker
    ↓
Phase 8 (消息增强II)  ← 可与 Phase 7 并行
    ↓
Phase 9 (语法高亮)  ← 独立，可与 Phase 7-8 并行
    ↓
Phase 10 (动画)  ← 独立，可与 Phase 7-9 并行
    ↓
Phase 11 (权限UI)  ← 依赖 Phase 6 的设计系统 + Phase 9 的高亮
    ↓
Phase 12 (Agent/Task)  ← 依赖 Phase 6-7 的弹窗系统
    ↓
Phase 13 (高级功能)  ← 全部
```

**并行机会：**
- Phase 8 + Phase 9 + Phase 10 可并行开发（无相互依赖）
- Phase 11 可在 Phase 6 完成后立即开始（与 Phase 7-10 并行）

---

## 9. 备注

- **Worktree 位置**: `/Users/buthim/Develop/claude-code-tui`
- **分支**: `cl/tui`
- **GitHub 仓库正确地址**: `github.com/grindlemire/go-tui`（不是 `alethi-dev/go-tui`，后者返回 404）
- **go-tui 包含 `ai-chat` 示例**，可作为初始参考
- **现有 `ui.Renderer` 接口**是关键集成点 — 已成功实现 TuiRenderer 适配
- **src/ 分析数据来源**: 2 轮 sub-agent 深度分析，覆盖 30+ 核心文件、所有目录结构
- **当前 Go TUI 文件**: 7 文件 / ~2860 行代码
- **src/ 规模**: 439+ 组件文件 / 293 命令文件 / ~180K+ 行代码
