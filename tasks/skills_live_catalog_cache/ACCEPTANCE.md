# Skills live catalog/cache 最终验收

验收日期：2026-07-16

验收对象：当前共享工作树，`HEAD=12a30d054b79c9e2e074645b9f028fc41bd759db` 叠加未提交的任务实现

本地环境：`go1.26.1 darwin/arm64`，Apple M4 Pro

总结论：**功能与可执行门禁通过，带一项已归因的全仓格式基线例外。**

## 结论摘要

- task01–28、task30、task31 在验收时均为 `accepted`，无 `failed` 任务。task29 仍为 `pending`，因本验收者按所有权约束不修改任务 JSON，待外部协调者根据本报告转为 `accepted`。
- 聚焦、包级、race、全仓 test、vet、build、Windows 交叉编译与 `git diff --check` 全部通过。
- `gofmt -l .` 返回 96 个路径，因此不能宣称“全仓 gofmt 通过”。逐个复核显示 96/96 均是已跟踪且相对 `HEAD` 未修改的既有基线文件；0 个本专项修改/新增文件出现在列表中。
- 禁用/撤销会立即拒绝后续执行，但是**不能擦除已经暴露给模型的 catalog 或 SKILL.md 内容**。敏感内容撤销需要新建或可证明已清洗的 context。

## 需求验收矩阵

| 需求 | 结果 | 主要证据 |
| --- | --- | --- |
| 精确 `/skills` 直接打开列表，没有 List/Manage 中间菜单 | PASS | `TestSkillsMenuDirectListLoadsImmediatelyEnterIsInertAndEscapeCloses`、`TestSkillsSurfaceAcceptanceComposedExactREPLAppAndScreenReaderParity` |
| checkbox 在左侧，`auto`/`name-only`/`manual-only` 为 `[x]`，`off` 为 `[ ]` | PASS | `TestSkillsMenuCheckboxReflectsEveryEffectiveVisibility`、shadowed/off/on 验收用例 |
| Space 在 `off` 与上次非 off 状态间切换，无历史时默认恢复 `auto` | PASS | `TestSpaceSkillToggleUsesStableIDObservedRevisionAndNeverOptimistic`、`TestSkillsToggleRestoresBackendPersistedLastNonOffAfterReopen`、真实 project store 的 off/on 测试 |
| Enter 不执行切换，一次 Esc 关闭，显式子命令走文本路径 | PASS | task30 交互测试、task28 REPL/TUI/screen-reader 组合验收 |
| 每个 skill 项只占一行，按终端 display-cell 宽度省略 | PASS | task31 Unicode/CJK/NFC/combining/emoji/ZWJ/窄屏测试，task28 buffer matrix |
| 只有选中 skill 显示有界详情 | PASS | `TestSkillsMenuSingleLineRowsAndSelectedOnlyDetails`，selection/filter 切换测试 |
| 包含上下边框的 panel 永远在 viewport 预算内 | PASS | `TestSkillsMenuOverflowViewportAndResizeKeepBottomBorderAndSelection`、`TestSkillsMenuFullRootShortViewportDoesNotPushPanelBorderBelowInput` |
| 动态管理在当前 session 立即生效 | PASS | 真实 Manager/override store 的 set/reset/persistence，并发 refresh/toggle/snapshot/invoke，SkillTool 执行时最新 revision 校验 |
| 同名 skill 按稳定 ID 隔离，shadowed 配置与当前激活态分离 | PASS | task26 stable identity 流程，task28 same-name persistence 与 winner removal 测试 |
| 初始快照在当前 user 之前，变更只追加合并 delta | PASS | `TestSkillCatalogIntegrationLifecycleThroughQueryLoop`、`TestSkillCacheProviderRequestShape`、三个 provider 顺序一致性测试 |
| 系统指令、固定 Skill tool schema、model settings、PromptCacheKey 不随 catalog revision 改变 | PASS | task27 byte-prefix/request-shape 验收 |
| 只在调用时载入带 ID/revision/digest 的版本化 SKILL.md envelope | PASS | task08/task16/task26 调用与最新 registry 执行栅栏测试 |
| compact/resume 不复用已不可见的 cursor/body ledger | PASS | task22/task26/task28 epoch 7→8、sidecar-ahead、legacy reminder 测试 |
| MCP add/update/disconnect/reconnect/list_changed 并入同一 catalog 事务 | PASS | task23 MCP workspace/in-flight/hook-owner 测试，task26 MCP lifecycle |
| retained Agent/Team/后台 follow-up 不穿越 session/project/generation | PASS | task23 owner/session-project-dir/generation/lease 与 persisted restore 测试 |
| 并发 refresh/toggle/snapshot/invoke 无 data race | PASS | 聚焦 race 与 task29 无过滤 race 均通过 |

## 执行证据

### 聚焦与集成

| 命令 | 结果 |
| --- | --- |
| `go test ./loop ./engine ./provider -run 'SkillCatalogIntegration' -count=1` | PASS |
| `go test ./loop ./provider -run 'SkillCache\|Cache.*SkillCatalog' -count=1` | PASS |
| `go test ./loop ./provider -run '^$' -bench 'SkillCatalog' -benchmem` | PASS |
| `go test ./commands ./session ./compact ./skills -run 'Skills.*Acceptance\|CatalogRace' -count=1` | PASS |
| `go test . ./tui -run 'Skills.*Acceptance\|Skill.*Surface' -count=1` | PASS |
| `go test ./tui -run 'Skills.*(Layout\|SingleLine\|Details\|Overflow\|Viewport\|Resize\|Unicode\|CJK\|Emoji\|Narrow\|Short)' -count=1` | PASS |
| `go test ./tui -run 'Skills.*Menu\|Skills.*Toggle\|Space.*Skill' -count=1` | PASS |
| task23 的 engine/loop/skills/services-mcp/tools/root/commands 七组聚焦命令 | PASS |

task23 聚焦命令具体为：

```text
go test ./engine -run 'SkillCatalog.*Wiring|SkillRegistry.*Session' -count=1
go test ./loop -run 'SkillLoadedResolver|Explicit.*Skill.*Ledger|ChildSkillLedger|Task23' -count=1
go test ./skills -run 'SessionOverride.*Restore|ReplaceSession|ProjectGeneration|Task23' -count=1
go test ./services/mcp -run 'Task23|Catalog|Reconnect|ListChanged' -count=1
go test ./tools -run 'Task23|RetainedAgent|TeamChild.*Skill|BackgroundFollowUp.*Origin|MCP.*Owner' -count=1
go test . -run 'Task23|Skill.*Runtime|Worktree.*Skill|WorktreeEnter.*Catalog' -count=1
go test ./commands ./loop ./tools ./engine -run 'Skill' -count=1
```

### 包级、race 与全仓

| 命令 | 结果 |
| --- | --- |
| `go test ./skills ./loop ./tools ./engine ./services/mcp -count=1` | PASS；`tools` 约 96 s |
| `go test ./skills ./commands ./tools ./loop ./compact ./provider ./engine ./session ./tui -count=1` | PASS；`tools` 约 97 s |
| `go test -race ./skills ./loop ./tools ./services/mcp -run 'Task23\|ProjectGeneration\|SkillLoadedResolver\|ChildSkillLedger\|RetainedAgent\|TeamChild.*Skill\|ListChanged' -count=1` | PASS |
| `go test -race ./skills ./loop ./tools ./session -run 'Skill\|Catalog' -count=1` | PASS |
| `go test -race ./tui -run 'Skills.*(Layout\|Overflow\|Resize\|Unicode)\|Skills.*Menu\|Skills.*Toggle\|Space.*Skill' -count=1` | PASS |
| `go test -race ./skills ./loop ./tools ./session -count=1` | PASS；`tools` 约 116 s |
| `go test ./... -count=1` | PASS；全仓所有包通过，`tools` 约 98.5 s |
| `go vet ./...` | PASS，exit 0，无输出 |
| `go build ./...` | PASS，exit 0，无输出 |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/skills-task28-windows.test .` | PASS，Windows amd64 根包测试二进制编译成功 |
| `git diff --check` | PASS，exit 0，无输出 |

### 缓存基准

`go test ./loop ./provider -run '^$' -bench 'SkillCatalog' -benchmem` 在本机的主要结果：

| 场景 | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| snapshot/100 | 179,433 | 488,596 | 629 |
| single-update-delta/100 | 297,816 | 328,911 | 2,751 |
| snapshot/1000 | 1,891,861 | 4,806,646 | 6,036 |
| single-update-delta/1000 | 3,279,973 | 3,086,129 | 27,055 |

这些数据是本地回归基准，不是性能 SLA，也不能证明任何供应商的真实 cache-hit 百分比或计费结果。

## gofmt 基线例外

`gofmt -l .` 的输出非空，共 96 个文件，所以本项记为 **BASELINE EXCEPTION**，而不是 PASS。复核结果：

```text
gofmt_count=96
changed_vs_HEAD_count=0
untracked_count=0
baseline_unchanged_count=96
```

复核方法是对 `gofmt -l .` 的每个路径执行 `git ls-files --error-unmatch` 与 `git diff --quiet HEAD -- <path>`。结果说明：

- 96 个全部是已跟踪文件；
- 96 个全部相对 `HEAD` 未修改；
- 本专项的 60 个已跟踪修改文件和 90 个新增代码/测试文件中，没有文件出现在 `gofmt -l .` 列表；
- 额外对 90 个未跟踪的 task-owned 文件逐个执行 `git diff --no-index --check /dev/null <path>`，得到 `whitespace_violation_files=0`。

该归因只证明本专项没有扩大既有 gofmt 债务；它不会把全仓的既有 96 个格式问题伪装成“通过”。

## 平台、供应商与安全限制

- Windows amd64 已做交叉编译，但没有在真实 Windows Console 上运行交互 TUI。task28 的 PTY 端到端验收在 Unix 使用 PTY，在不支持的平台明确 skip；它不代表 Windows 键盘/终端行为已被运行时验证。
- 未在 Linux、Intel macOS 或其他终端模拟器上执行本轮验收。
- provider 证据是本地序列化与 request-shape 对比；未调用真实 Anthropic/OpenAI 网络端点，因此不宣称真实缓存命中率。
- MCP 生命周期使用确定性 fake/in-process 验证，未覆盖所有第三方 MCP server 的网络和协议偏差。
- revoke/off 会立即关闭后续 SkillTool 执行权，但无法从模型已看到的历史中召回文本。这是 context 安全边界，不是 registry 权限检查可以修复的问题。

## 协调器收尾条件

本验收已产生完成 task29 所需的证据，且没有发现需要回退给任务所有者的 P0/P1 缺陷。外部协调器需要在复核本报告后将 task29 从 `pending` 更新为 `accepted`；在该状态变更完成前，不应宣称“31/31 任务已记录为 accepted”。
