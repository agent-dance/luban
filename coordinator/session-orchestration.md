# 会话编排层：Coordinator · Session · Hooks

> **模块路径**：`gosrc/coordinator/`、`gosrc/session/`、`gosrc/hooks/`  
> **对应原版**：`src/coordinator/coordinatorMode.ts`、`src/utils/sessionState.ts`、`src/types/hooks.ts`  
> **文档日期**：2026-04-05  
> **适用版本**：Go 复刻版 v0.x（开发中）

---

## 目录

1. [概述](#1-概述)
2. [原版（TS）设计详情](#2-原版ts设计详情)
3. [Go 实现现状](#3-go-实现现状)
4. [关键知识背景](#4-关键知识背景)
5. [评估指标](#5-评估指标)
6. [与原版的差距及后续规划](#6-与原版的差距及后续规划)

---

## 1. 概述

会话编排层由三个相互协作的模块构成，共同负责**多智能体任务调度**、**会话状态持久化**与**生命周期钩子执行**。

```
用户输入
   │
   ▼
┌─────────────────────────────────────────────────────┐
│                    main.go (REPL)                    │
│  loadHooks() ──► hookRunner                         │
│  session.FileStore ──► sessionID                    │
│  compact.ResultStore ──► ql.SetResultStore()        │
└──────────────┬──────────────────────────────────────┘
               │ loop.New(p, registry, Config{HookRunner})
               ▼
┌─────────────────────────────────────────────────────┐
│                 loop.QueryLoop.Run()                  │
│  ┌──────────────────────────────────────────────┐   │
│  │  executeToolsConcurrently()                  │   │
│  │    runPreToolHooks()  ◄── hooks.Runner       │   │
│  │    [execute tools]                           │   │
│  │    runPostToolHooks() ◄── hooks.Runner       │   │
│  └──────────────────────────────────────────────┘   │
└──────────────┬──────────────────────────────────────┘
               │ (Multi-agent: agent tools spawn sub-loops)
               ▼
┌─────────────────────────────────────────────────────┐
│              coordinator.Coordinator                  │
│  Task{BlockedBy[]} ──► DAG resolution               │
│  Agent{AgentFunc} ──► goroutine dispatch            │
│  MessageBus{chan} ──► pub-sub 消息传递               │
└─────────────────────────────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────────────┐
│               session.FileStore / MemoryStore         │
│  JSONL 原子写入 (tmp→fsync→rename)                  │
│  Memory{Fact, Category} → ForPrompt() 注入          │
└─────────────────────────────────────────────────────┘
```

### 三模块职责速览

| 模块 | Go 包 | 核心职责 |
|------|--------|---------|
| **Coordinator** | `coordinator` | 多智能体任务调度、DAG 依赖解析、并发派发 |
| **Session** | `session` | 对话历史持久化 (FileStore)、上下文记忆注入 (MemoryStore) |
| **Hooks** | `hooks` | 生命周期钩子：PreToolUse / PostToolUse 等，外部脚本 I/O 协议 |

---

## 2. 原版（TS）设计详情

### 2.1 Coordinator 模式（`coordinatorMode.ts`）

TS 版 Coordinator 不是一个任务调度器，而是一种**LLM 对话模式**——通过环境变量激活后注入专用系统提示词，让 Claude 自主完成多步任务规划与调度。

**激活方式**

```typescript
// 读取环境变量
export function isCoordinatorMode(): boolean {
  return process.env.CLAUDE_CODE_COORDINATOR_MODE === '1'
}
```

**四阶段工作流**（系统提示词中定义，约 300 行）

```
Phase 1: Research
  └─ 分析任务、探索代码库、识别相关文件和模式

Phase 2: Synthesis
  └─ 综合信息、制定策略、处理依赖和权衡

Phase 3: Implementation
  └─ 执行计划、使用工具完成任务、应对意外情况

Phase 4: Verification
  └─ 验证实现正确性、测试、确认所有需求已满足
```

**任务通知协议**（XML 格式）

```xml
<task-notification>
  <type>subtask_started|subtask_completed|error</type>
  <task_id>unique-id</task_id>
  <description>任务描述</description>
  <result>执行结果</result>
</task-notification>
```

**会话模式匹配**

```typescript
export function matchSessionMode(session: Session): void {
  if (session.coordinatorMode) {
    process.env.CLAUDE_CODE_COORDINATOR_MODE = '1'
  }
}
```

### 2.2 Hooks 系统（`src/types/hooks.ts`）

TS 版 Hooks 定义了 **16+ 个 hookEventName 类型**，覆盖工具调用前后、权限请求、文件变更、工作树操作等完整生命周期。

**`hookSpecificOutput` 判别联合类型（精选）**

| hookEventName | 触发时机 | 特有输出字段 |
|---------------|---------|-------------|
| `PreToolUse` | 工具调用前 | `permissionBehavior` |
| `PostToolUse` | 工具调用后 | — |
| `SubagentStart` | 子智能体启动前 | `allowedToolNames` |
| `SubagentStop` | 子智能体结束后 | — |
| `PermissionRequest` | 权限提示弹出时 | `permissionBehavior` |
| `FileChanged` | 文件系统变更后 | `changedFiles[]` |
| `WorktreeCreate` | 工作树创建时 | `worktreePath` |
| `WorktreeRemove` | 工作树删除时 | `worktreePath` |
| `UserPromptSubmit` | 用户提交输入后 | `modifiedPrompt` |
| `SessionStart` | 会话初始化时 | — |
| `SessionEnd` | 会话终止时 | `exitReason` |
| `Stop` | 智能体停止时 | — |
| `Notification` | 系统通知产生时 | `message` |
| `MCPToolStart` | MCP 工具开始时 | `serverName` |
| `MCPToolEnd` | MCP 工具结束时 | `serverName`, `duration` |
| `PreCompact` | 上下文压缩前 | `compressionTarget` |

**Hook 结果类型**

```typescript
type HookResult = {
  outcome: 'success' | 'blocking' | 'non_blocking_error' | 'cancelled'
  permissionBehavior?: PermissionBehavior
  additionalContext?: string        // 注入到系统提示的额外上下文
  updatedInput?: Record<string, unknown>  // 修改工具输入
}
```

**异步钩子协议**

```json
// Hook 输出中包含 async: true，表示继续轮询
{ "async": true, "pollIntervalMs": 500, "timeoutMs": 30000 }
```

**双类型钩子**
- **Shell Script Hook**：外部进程，JSON stdin/stdout 协议
- **In-process Callback Hook**：TypeScript 函数直接注册（`HookCallback` 类型）

### 2.3 会话状态机（`src/utils/sessionState.ts`）

TS 版会话维护一个**三态有限状态机**：

```
          用户提交输入
idle ──────────────────► running
 ▲                          │
 │     工具执行完成         │ 需要权限/用户确认
 └──────────────────────────┤
                            ▼
                     requires_action
                            │
                    用户确认/拒绝
                            │
                            └──► running / idle
```

**`RequiresActionDetails` 载荷**

```typescript
type RequiresActionDetails = {
  tool_name: string
  action_description: string
  tool_use_id: string
  request_id: string
  input: Record<string, unknown>
}
```

**SDK 事件发射**（`CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1` 时启用）

```typescript
SessionExternalMetadata = {
  permission_mode: PermissionMode
  pending_action?: RequiresActionDetails
  task_summary?: string
  post_turn_summary?: string
}
```

**监听器模式**（模块级单例）

```typescript
// 注册状态变更回调
addSessionStateListener((state, metadata) => { ... })
// 触发状态转换
setSessionState('running')
setSessionState('requires_action', details)
setSessionState('idle')
```

### 2.4 会话存储（TS 原版）

TS 版会话使用 SQLite（通过 `better-sqlite3`）进行持久化，同时维护内存缓存。

**核心表结构**

```sql
-- messages 表：对话历史（分 session 存储）
CREATE TABLE messages (
  id INTEGER PRIMARY KEY,
  session_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL,   -- JSON-serialized ContentBlock[]
  created_at INTEGER NOT NULL
);

-- sessions 表：会话元数据
CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  coordinator_mode INTEGER DEFAULT 0,
  title TEXT
);
```

---

## 3. Go 实现现状

### 3.1 功能覆盖状态表

| 功能点 | TS 原版 | Go 实现 | 状态 | 备注 |
|--------|--------|---------|------|------|
| **Coordinator 模式** | LLM 对话模式（env var 激活） | 程序化任务调度器 | 🔄 重新设计 | 架构差异，非直接移植 |
| Coordinator 四阶段提示词 | `getCoordinatorSystemPrompt()` | 无 | ❌ 缺失 | |
| `<task-notification>` XML 协议 | 全面支持 | 无 | ❌ 缺失 | |
| 会话模式匹配（resume） | `matchSessionMode()` | `ResolveSession()` | ✅ 已实现 | 不含 coordinator 模式 |
| **DAG 任务依赖** | 无 | `Task.BlockedBy []string` | ✅ Go 特有 | |
| **并发任务派发** | 无 | `Coordinator.Dispatch()` goroutine | ✅ Go 特有 | |
| **MessageBus 消息总线** | 无 | `MessageBus{chan, cap=32}` | ✅ Go 特有 | |
| **Agent 注册** | 无 | `RegisterAgent(name, AgentFunc)` | ✅ Go 特有 | |
| **PreToolUse hook** | ✅ 支持 | ✅ 支持 | ✅ 已实现 | `runPreToolHooks()` |
| **PostToolUse hook** | ✅ 支持 | ✅ 支持 | ✅ 已实现 | `runPostToolHooks()` |
| SessionStart hook | ✅ 支持 | 已定义未调用 | ⚠️ 部分实现 | 常量存在，逻辑缺失 |
| SessionEnd hook | ✅ 支持 | 已定义未调用 | ⚠️ 部分实现 | 同上 |
| UserPromptSubmit hook | ✅ 支持 | 已定义未调用 | ⚠️ 部分实现 | 同上 |
| SubagentStart/Stop hook | ✅ 支持 | ❌ 缺失 | ❌ 缺失 | |
| PermissionRequest hook | ✅ 支持 | ❌ 缺失 | ❌ 缺失 | |
| FileChanged hook | ✅ 支持 | ❌ 缺失 | ❌ 缺失 | |
| WorktreeCreate/Remove hook | ✅ 支持 | ❌ 缺失 | ❌ 缺失 | |
| Notification hook | ✅ 支持 | ❌ 缺失 | ❌ 缺失 | |
| MCPToolStart/End hook | ✅ 支持 | ❌ 缺失 | ❌ 缺失 | |
| PreCompact hook | ✅ 支持 | ❌ 缺失 | ❌ 缺失 | |
| **异步钩子协议** | `{async: true}` 轮询 | ❌ 缺失 | ❌ 缺失 | |
| **In-process 回调钩子** | `HookCallback` 函数类型 | ❌ 缺失 | ❌ 缺失 | |
| Hook Block（exit 2） | ✅ 支持 | ✅ 支持 | ✅ 已实现 | |
| Hook ModifiedInput | ✅ 支持 | ✅ 支持 | ✅ 已实现 | |
| Hook SystemReminder | ✅ 支持 | ✅ 支持 | ✅ 已实现 | |
| Hook OOM 保护 | 有上限 | `limitedBuffer{1MB}` | ✅ 已实现 | |
| **FileStore 会话持久化** | SQLite | JSONL 原子写入 | 🔄 重新实现 | 格式差异 |
| FileStore 原子写入 | SQLite 事务 | tmp→fsync→rename | ✅ 已实现 | POSIX 崩溃安全 |
| FileStore 损坏恢复 | 无需（SQLite） | 按行跳过损坏 | ✅ 已实现 | |
| **MemoryStore 记忆系统** | 无直接对应 | `Memory{Fact,Category}` | ✅ Go 特有 | |
| MemoryStore 上限控制 | — | `MaxPromptMemories=50` | ✅ 已实现 | |
| MemoryStore 提示词注入 | — | `ForPrompt()` markdown 格式 | ✅ 已实现 | |
| **三态会话 FSM** | idle/running/requires_action | ❌ 缺失 | ❌ 缺失 | |
| SDK 事件发射 | `CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS` | ❌ 缺失 | ❌ 缺失 | |
| 权限请求拦截 | `RequiresActionDetails` | ❌ 缺失 | ❌ 缺失 | |

### 3.2 端到端数据流

```
用户输入 "修复 bug X"
    │
    ▼
main.go: RunREPL()
    │  loop.Run("修复 bug X", onEvent)
    │
    ▼
loop/query.go: QueryLoop.Run()
  │
  ├─ [1] 追加用户消息到 q.messages
  │
  ├─ [2] Microcompact（清理旧 tool results）
  │
  ├─ [3] provider.CreateStream(params)
  │       │  System prompt + messages + tools
  │       ▼
  │    Anthropic API ──► StreamEvent channel
  │
  ├─ [4] processStream()
  │       │  EventContentBlockDelta → text/thinking/tool_input 累积
  │       │  EventContentBlockStop  → 组装 ContentBlock
  │       └─► assistantMsg{TextBlock|ToolUseBlock|ThinkingBlock}
  │
  ├─ [5] 若有 ToolUseBlock：
  │       │
  │       └─► loop/concurrent.go: executeToolsConcurrently()
  │               │
  │               ├─ [5a] runPreToolHooks(runner, toolUse)
  │               │         │  JSON stdin: {type, toolName, toolInput, sessionId}
  │               │         │  bash -c <script>
  │               │         │  JSON stdout: {action, modifiedInput, message}
  │               │         └─► HookResult{Action: Block/ModifiedInput/SystemReminder}
  │               │
  │               ├─ [5b] 并发执行（Read/Glob/Grep）或顺序执行
  │               │         registry.Execute(toolName, input)
  │               │         └─► ToolResultBlock{content}
  │               │
  │               └─ [5c] runPostToolHooks(runner, toolUse, result)
  │                         └─► reminders []string
  │
  ├─ [6] resultStore.ProcessResult()（超大结果持久化到磁盘）
  │
  ├─ [7] toolBudget.Apply()（截断超大 tool results）
  │
  ├─ [8] 注入 <system-reminder> 消息（来自 hook reminders）
  │
  ├─ [9] ctxWindow.ShouldCompact() → compactor.Compact()
  │
  └─ [10] onEvent(EventTurnEnd) → 循环下一 turn

               ↕ （Agent 工具触发子循环时）
coordinator.Coordinator.Dispatch()
    │
    ├─ findAssignment(): 遍历 tasks，找未阻塞+未运行的任务
    ├─ 检查 BlockedBy 依赖（所有前置任务已完成？）
    ├─ goroutine: agent.AgentFunc(ctx, task, bus)
    │               └─► 子 loop.QueryLoop（独立实例）
    ├─ wg.Wait()（等待本批次所有任务完成）
    └─ 重复直到 dispatched == 0

session.FileStore
    │
    ├─ Save(messages): 写临时文件 → fsync → os.Rename
    └─ Load(id): 读 JSONL，按行解析，跳过损坏行
```

### 3.3 Coordinator 调度算法详解

```go
// 伪代码：Coordinator.Dispatch() 核心逻辑
func (c *Coordinator) Dispatch(ctx context.Context) error {
    for {
        c.mu.Lock()
        assignments := c.findAssignment()  // O(n×m) 扫描

        if len(assignments) == 0 && c.allDone() {
            c.mu.Unlock()
            break  // 所有任务完成
        }

        var wg sync.WaitGroup
        for _, (agent, task) := range assignments {
            agent.busy = true        // 标记 agent 忙碌
            task.status = "running"  // 标记任务运行中
            wg.Add(1)
            go func() {
                defer wg.Done()
                result, err := agent.AgentFunc(ctx, task, c.bus)
                c.mu.Lock()
                agent.busy = false
                task.status = "done" (or "failed")
                task.result = result
                c.mu.Unlock()
            }()
        }
        c.mu.Unlock()

        wg.Wait()  // 等待本批次全部完成再调度下一批
    }
}

// findAssignment: 匹配可用 agent 和待执行任务
func (c *Coordinator) findAssignment() []Assignment {
    for _, agent := range c.agents {
        if agent.busy { continue }
        for _, task := range c.tasks {
            if task.status != "pending" { continue }
            if !c.depsResolved(task) { continue }  // 检查 BlockedBy
            return append(result, Assignment{agent, task})
        }
    }
}
```

**调度时序图**

```
Task A (无依赖)  ──[Agent-1]──► Done
Task B (无依赖)  ──[Agent-2]──► Done
Task C (BlockedBy: A,B)         ──[Agent-1]──► Done
Task D (BlockedBy: C)                           ──[Agent-2]──► Done
                 │              │               │              │
                 t0             t1              t2             t3
                 第一批 (A+B 并发)   第二批 (C)      第三批 (D)
```

### 3.4 Hooks 执行协议

**PreToolUse 钩子 I/O 流**

```
loop/concurrent.go                    hooks/hooks.go           外部脚本
      │                                     │                      │
      │ runPreToolHooks(runner, toolUse)     │                      │
      ├────────────────────────────────────►│                      │
      │                                     │ HookInput{           │
      │                                     │   Type: PreToolUse,  │
      │                                     │   ToolName: "Bash",  │
      │                                     │   ToolInput: {...}   │
      │                                     │ }                    │
      │                                     │──── JSON stdin ─────►│
      │                                     │     bash -c script   │
      │                                     │◄─── JSON stdout ─────│
      │                                     │ {                    │
      │                                     │   action: "block"    │  exit 2 → block
      │                                     │   | "modifiedInput"  │
      │                                     │   | "systemReminder" │
      │                                     │   message: "..."     │
      │                                     │   modifiedInput: {}  │
      │                                     │ }                    │
      │◄────────────────────────────────────│                      │
      │ HookResult:                         │                      │
      │   Block      → 返回错误结果，跳过工具执行
      │   ModifiedInput → 替换 toolUse.Input 后继续
      │   SystemReminder → 收集 reminder，工具正常执行
```

**limitedBuffer OOM 保护机制**

```go
// hooks/hooks.go
type limitedBuffer struct {
    buf []byte
    max int   // 1 << 20 = 1 MB
}

func (lb *limitedBuffer) Write(p []byte) (int, error) {
    remaining := lb.max - len(lb.buf)
    if remaining <= 0 {
        return len(p), nil  // 静默丢弃超出部分，不返回错误
    }
    if len(p) > remaining {
        p = p[:remaining]
    }
    lb.buf = append(lb.buf, p...)
    return len(p), nil
}
```

### 3.5 Session 持久化机制

**FileStore 原子写入流程**

```
Save(messages) 调用时：
  1. 生成临时文件路径：<id>.tmp
  2. os.Create(tmpPath) → 创建临时文件
  3. 遍历 messages，每条 json.Marshal() 后写入 "\n"
  4. f.Sync() → 强制刷入磁盘（防止掉电丢失）
  5. f.Close()
  6. os.Rename(tmpPath, finalPath) → 原子替换
     （POSIX 保证：同一文件系统内 rename 是原子操作）

Load(id) 调用时：
  1. 读取 <id>.jsonl
  2. 按 "\n" 分割为行
  3. 每行 json.Unmarshal() 为 types.Message
  4. 解析失败的行：记录 warning，跳过（容错）
  5. 返回成功解析的消息列表
```

**MemoryStore 提示词注入**

```go
// ForPrompt() 输出格式（注入到系统提示词末尾）
## Memory
- [architecture] 项目使用 Go 实现，遵循 Clean Architecture 原则
- [preference] 用户偏好简洁的代码风格，避免过度抽象
- [context] 当前在修复 session 模块的并发 bug
  ...（最多 50 条）
```

---

## 4. 关键知识背景

### 4.1 事件驱动架构与 goroutine 安全

Go 版本的并发模型基于 **goroutine + channel + mutex** 三件套，与 TS 的 Promise/async-await 模型有本质差异。

**工具并发执行模型**

```go
// 并发工具（ReadFile, Glob, Grep）使用 channel 串行化回调
type toolResult struct {
    index  int
    result types.ToolResultBlock
}
resultCh := make(chan toolResult, len(concurrentTools))

for i, tool := range concurrentTools {
    go func(idx int, tu types.ToolUseBlock) {
        result := registry.Execute(tu)
        resultCh <- toolResult{idx, result}
    }(i, tool)
}

// 主 goroutine 顺序消费结果，保证回调顺序
for range concurrentTools {
    r := <-resultCh
    onResult(r.index, r.result)
}
```

**Coordinator 的 mutex 使用模式**

```
c.mu.Lock() 保护的共享状态：
  - c.tasks[].status / .result
  - c.agents[].busy

goroutine 内：修改状态前必须重新加锁
主循环：持锁期间不执行 AgentFunc（避免死锁）
```

### 4.2 POSIX 原子文件写入

`os.Rename(src, dst)` 在 POSIX 系统上是原子操作（单一文件系统内），配合 `f.Sync()` 可实现崩溃安全的持久化：

```
关键保证：
  - rename 前 Sync：确保数据在磁盘上
  - rename 原子性：读者永远看到完整文件或旧文件，不存在中间状态
  - 损坏恢复：Load 时按行解析 + 跳过损坏行，而非整体失败
```

### 4.3 钩子安全模型

Go 版钩子的**故障转移策略**：宁可阻止操作，也不静默忽略错误。

```
钩子执行结果判定规则：

exit code 2     → Block（明确拒绝）
JSON 解析失败   → 将 stdout 作为 SystemReminder（降级）
非 ExitError    → Block（fail-safe：未知错误视为阻止）
超出 1MB stdout → 截断（保护内存，不崩溃）
超时            → ctx 取消（父 ctx 超时传播）
```

### 4.4 上下文窗口与记忆压力

`MemoryStore.MaxPromptMemories = 50` 限制了每次请求注入的记忆条数。当记忆积累过多时，策略是保留最近 50 条，而非全量注入：

```
影响：长会话中早期记忆可能丢失
缓解：File-backed 存储保留全部历史，仅 ForPrompt() 截断
```

### 4.5 MessageBus 背压控制

```go
type MessageBus struct {
    channels map[string]chan Message
    capacity int  // 默认 32
}

// 发布时：容量满则阻塞（自然背压）
bus.channels[topic] <- msg

// 订阅时：goroutine 消费，避免积压
go func() {
    for msg := range bus.channels[topic] {
        handler(msg)
    }
}()
```

Channel 容量 32 是平衡吞吐量与内存的经验值。若 Agent 生产消息速度远超消费速度，发布操作会阻塞，起到自然限流作用。

### 4.6 JSONL 格式选择

FileStore 选择 JSONL（每行一个 JSON 对象）而非单一 JSON 数组的原因：

```
优势：
  - 追加写入：无需读取整个文件即可追加新消息
  - 部分恢复：单行损坏不影响其他行的解析
  - 流式读取：可逐行处理，无需全量加载到内存

劣势（相比 SQLite）：
  - 无索引：无法高效按条件查询
  - 无事务：多条消息的批量写入不是单一原子操作（虽然 rename 保证整体文件原子性）
  - 无结构校验：类型约束完全依赖应用层
```

---

## 5. 评估指标

### 5.1 Hook 覆盖率

**Hook 类型覆盖**

| 指标 | 数值 | 计算方式 |
|------|------|---------|
| TS 原版定义的 Hook 类型总数 | 16 | 统计 `hookSpecificOutput` 判别联合的 case 数 |
| Go 版已定义的 HookType 常量数 | 5 | `PreToolUse, PostToolUse, SessionStart, SessionEnd, UserPromptSubmit` |
| Go 版实际触发的 HookType 数 | **2** | `PreToolUse`（`runPreToolHooks`）、`PostToolUse`（`runPostToolHooks`） |
| **Hook 定义覆盖率** | **31%** (5/16) | 已定义 / TS 总数 |
| **Hook 触发覆盖率** | **13%** (2/16) | 实际触发 / TS 总数 |

```
Hook 覆盖状态（按触发频率降序）：

██████████ PreToolUse        ✅ 已实现（每次工具调用前）
██████████ PostToolUse       ✅ 已实现（每次工具调用后）
▒▒▒▒▒▒▒▒▒▒ SessionStart     ⚠️  已定义，未在 RunREPL 中触发
▒▒▒▒▒▒▒▒▒▒ SessionEnd       ⚠️  已定义，未在 RunREPL 中触发
▒▒▒▒▒▒▒▒▒▒ UserPromptSubmit ⚠️  已定义，未在 Run() 中触发
░░░░░░░░░░ SubagentStart     ❌ 未实现
░░░░░░░░░░ SubagentStop      ❌ 未实现
░░░░░░░░░░ PermissionRequest ❌ 未实现
░░░░░░░░░░ FileChanged       ❌ 未实现（无文件监控）
░░░░░░░░░░ WorktreeCreate    ❌ 未实现（无 worktree 支持）
░░░░░░░░░░ WorktreeRemove    ❌ 未实现
░░░░░░░░░░ Notification      ❌ 未实现
░░░░░░░░░░ MCPToolStart      ❌ 未实现
░░░░░░░░░░ MCPToolEnd        ❌ 未实现
░░░░░░░░░░ PreCompact        ❌ 未实现
░░░░░░░░░░ Stop              ❌ 未实现
```

### 5.2 Session 功能完整性

| 功能维度 | TS 原版 | Go 实现 | 完整度 |
|---------|--------|---------|--------|
| 消息持久化 | SQLite | JSONL | 80%（功能等价，格式不同）|
| 会话查询（按 ID） | SQL WHERE | 文件名映射 | 90% |
| 会话列表 | SQL SELECT | 目录扫描 | 70% |
| 会话删除 | SQL DELETE | os.Remove | 90% |
| 状态机（FSM） | 三态 + 监听器 | 无 | 0% |
| 权限拦截 | RequiresActionDetails | 无 | 0% |
| SDK 事件发射 | SessionExternalMetadata | 无 | 0% |
| 记忆注入 | 无对应 | MemoryStore（Go 特有）| — |
| **整体 Session 完整度** | | | **~55%** |

### 5.3 Coordinator 功能完整性

| 功能维度 | TS 原版 | Go 实现 | 完整度 |
|---------|--------|---------|--------|
| 多智能体协调 | LLM 对话模式 | 程序化任务调度 | 50%（不同架构） |
| 四阶段工作流 | 系统提示词内置 | 无 | 0% |
| 任务通知协议 | XML `<task-notification>` | 无 | 0% |
| 会话模式 resume | `matchSessionMode()` | `ResolveSession()` | 70% |
| DAG 依赖解析 | 无 | `BlockedBy []string` | Go 特有（超越 TS） |
| 并发派发 | 无（LLM 串行） | goroutine 并发 | Go 特有（超越 TS） |
| MessageBus | 无 | 缓冲 channel | Go 特有（超越 TS） |
| **整体 Coordinator 完整度** | | | **~40%**（架构差异导致） |

### 5.4 性能与延迟指标

**Hook 执行延迟目标**

| 操作 | 目标 P50 | 目标 P99 | 当前状态 |
|------|---------|---------|---------|
| PreToolUse hook 脚本启动 | < 50ms | < 200ms | 未测量 |
| PostToolUse hook 执行 | < 100ms | < 500ms | 未测量 |
| Hook JSON 解析 | < 1ms | < 5ms | 未测量 |
| Hook 超时保护 | 30s 强制取消 | — | ✅ 已实现 |

**Session I/O 延迟目标**

| 操作 | 目标 P50 | 目标 P99 | 当前状态 |
|------|---------|---------|---------|
| FileStore.Save()（100 条消息）| < 10ms | < 50ms | 未测量 |
| FileStore.Load()（100 条消息）| < 5ms | < 20ms | 未测量 |
| MemoryStore.ForPrompt()（50 条）| < 1ms | < 5ms | 未测量 |

**Coordinator 调度延迟目标**

| 操作 | 目标 | 当前状态 |
|------|------|---------|
| Task 派发延迟（findAssignment） | < 1ms | 未测量 |
| 10 个并发 Agent 完成时间（含 I/O）| 依赖任务 | 未测量 |

### 5.5 可靠性矩阵

| 故障场景 | 期望行为 | Go 实现 | 验证状态 |
|---------|---------|---------|---------|
| Hook 脚本输出超 1MB | 截断，不 OOM | `limitedBuffer` | ✅ 代码验证 |
| Hook exit code 非 0/2 | fail-safe Block | `if exitErr.ExitCode() != 2` | ✅ 代码验证 |
| Hook JSON 解析失败 | 降级为 SystemReminder | fallback 路径 | ✅ 代码验证 |
| FileStore 写入中断 | 保留旧文件（rename 原子性）| tmp→fsync→rename | ✅ 理论验证 |
| JSONL 行损坏 | 跳过损坏行，其余正常 | 逐行解析 | ✅ 代码验证 |
| Coordinator context 取消 | goroutine 传播取消 | `ctx.Err()` 检查 | ✅ 代码验证 |
| Agent panic | wg.Wait 永久阻塞 | ❌ 无 recover | ⚠️ 已知问题 |

---

## 6. 与原版的差距及后续规划

### 6.1 差距汇总表

| 差距类别 | 严重程度 | 影响范围 | 修复难度 |
|---------|---------|---------|---------|
| Hook 类型覆盖（仅 2/16 触发）| 高 | 所有使用额外 hooks 的用户 | 中 |
| SessionStart/End/UserPromptSubmit 未触发 | 高 | Hook 脚本开发者 | 低 |
| 三态会话 FSM 缺失 | 高 | 权限请求、UI 集成 | 高 |
| SDK 事件发射缺失 | 中 | 外部工具集成 | 中 |
| Coordinator 系统提示词缺失 | 高 | 多步自主任务 | 中 |
| 异步 Hook 协议缺失 | 中 | 长时间运行的 hook | 中 |
| In-process 回调 hook 缺失 | 低 | 库使用者 | 中 |
| Agent panic 未恢复 | 中 | 生产稳定性 | 低 |
| SQLite 会话存储缺失 | 低 | 性能（万级历史） | 高 |
| 文件监控（FileChanged hook）缺失 | 低 | 文件变更感知 | 高 |

### 6.2 优先级路线图

#### P1（阻塞性，立即实施）

**1.1 触发已定义的 SessionStart/End/UserPromptSubmit Hook**

```go
// 在 main.go RunREPL() 中添加：
hookRunner.Run(ctx, hooks.HookInput{
    Type:      hooks.HookSessionStart,
    SessionID: sessionID,
})
defer hookRunner.Run(ctx, hooks.HookInput{
    Type:      hooks.HookSessionEnd,
    SessionID: sessionID,
})

// 在 loop/query.go Run() 开始处：
hookRunner.Run(ctx, hooks.HookInput{
    Type:    hooks.HookUserPromptSubmit,
    Message: userMessage,
})
```

**估算工作量**：2-4 小时，风险极低

---

#### P2（高优先级，下一迭代）

**2.1 扩展 Hook 类型至完整覆盖**

需要新增 HookType 常量和触发点：
- `SubagentStart` / `SubagentStop`：在 Agent 工具创建/销毁子 QueryLoop 时触发
- `Notification`：在 EventText 发射时触发（通知用户）
- `PreCompact`：在 `compactor.Compact()` 调用前触发

**估算工作量**：1-2 天

**2.2 修复 Agent goroutine panic 恢复**

```go
// coordinator/coordinator.go
go func() {
    defer wg.Done()
    defer func() {
        if r := recover(); r != nil {
            c.mu.Lock()
            task.status = "failed"
            task.err = fmt.Errorf("agent panic: %v", r)
            c.mu.Unlock()
        }
    }()
    // ... AgentFunc 调用
}()
```

**估算工作量**：1-2 小时

---

#### P3（中优先级，季度规划）

**3.1 实现三态会话 FSM**

```go
// session/state.go（新文件）
type SessionState string
const (
    StateIdle           SessionState = "idle"
    StateRunning        SessionState = "running"
    StateRequiresAction SessionState = "requires_action"
)

type StateManager struct {
    state     SessionState
    mu        sync.RWMutex
    listeners []func(SessionState, *ActionDetails)
}

func (sm *StateManager) Transition(next SessionState, details *ActionDetails) {
    sm.mu.Lock()
    sm.state = next
    sm.mu.Unlock()
    for _, l := range sm.listeners {
        l(next, details)
    }
}
```

**估算工作量**：3-5 天（含测试）

**3.2 Coordinator 四阶段系统提示词**

实现 `getCoordinatorSystemPrompt()` 的 Go 版本，参考 TS 原版四阶段设计（Research→Synthesis→Implementation→Verification），注入到子 Agent 的 QueryLoop.Config.System。

**估算工作量**：2-3 天

---

#### P4（中低优先级，半年规划）

**4.1 异步 Hook 协议支持**

```go
// 扩展 HookOutput 结构
type HookOutput struct {
    Action         string `json:"action"`
    Message        string `json:"message"`
    Async          bool   `json:"async"`            // 新增
    PollIntervalMs int    `json:"pollIntervalMs"`   // 新增
    TimeoutMs      int    `json:"timeoutMs"`        // 新增
}

// executeHook() 中：若 output.Async == true，
// 启动轮询循环，持续调用脚本直到 !async 或超时
```

**估算工作量**：2-3 天

**4.2 PermissionRequest Hook 与权限拦截**

需要在工具执行前增加权限检查层，集成 `HookPermissionRequest` 触发，并将结果注入到工具执行决策中。

**估算工作量**：3-5 天

---

#### P5（低优先级，年度规划）

**5.1 SDK 事件发射**

实现 `CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS` 环境变量支持，通过 HTTP SSE 或 UNIX socket 发射 `SessionExternalMetadata` 事件，供外部工具集成。

**估算工作量**：1-2 周

**5.2 SQLite 会话存储（可选替代）**

为超大历史（>1万条消息）的场景提供 SQLite 后端，与现有 JSONL FileStore 接口兼容：

```go
type Store interface {
    Save(id string, messages []types.Message) error
    Load(id string) ([]types.Message, error)
    List() ([]SessionMeta, error)
    Delete(id string) error
}
// FileStore 和 SQLiteStore 均实现此接口
```

**估算工作量**：1-2 周

---

#### P6（研究性，按需实施）

**6.1 文件变更监控（FileChanged Hook）**

使用 `fsnotify` 库实现文件系统监控，在工作目录文件变更时触发 `FileChanged` hook。适用于需要响应外部文件变更的自动化场景。

**估算工作量**：1 周（含测试）

**6.2 Worktree 支持**

实现 `WorktreeCreate` / `WorktreeRemove` hook 触发点，与 git worktree 操作集成。依赖 git 工具的完整实现。

**估算工作量**：2-3 周（含 git worktree 集成）

---

### 6.3 快速得分机会（Quick Wins）

下列改动可在**一个工作日内**完成，显著提升 Hook 覆盖率：

```
1. 触发 SessionStart hook（RunREPL 入口）   → +1 hook type
2. 触发 SessionEnd hook（RunREPL defer）    → +1 hook type
3. 触发 UserPromptSubmit hook（Run() 入口） → +1 hook type
4. 修复 Agent panic recover                 → 提升稳定性
5. 添加 Hook 触发覆盖率单元测试             → 防止退化

执行后 Hook 触发覆盖率：2/16 → 5/16 = 31%（+18%）
```

---

> **文档维护说明**：本文档应随 `coordinator/`、`session/`、`hooks/` 模块的重大变更同步更新。评估指标（第 5 节）建议每季度运行 `scripts/orchestration_metrics.py` 更新数值。
