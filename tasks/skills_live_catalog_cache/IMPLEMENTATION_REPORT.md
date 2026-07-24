# Skills live catalog/cache 实施报告

## 交付结果

本专项将 skill 从“对话开始时拼入一份后续难以变更的列表”改造为了一个有 revision、有稳定身份、有分层 override、有执行时权限栅栏的实时 catalog。用户现在可以通过精确 `/skills` 直接进入二级列表式 checklist，在左侧 checkbox 上使用 Space 开关选中 skill；不经过 List/Manage 中间菜单。

实施同时保留了高缓存命中的结构条件：不重写已经进入可见历史的初始 catalog，不为每个 skill 生成一个动态 tool schema，只在变更时追加合并的 developer-role delta。

## 最终设计

### 1. 权威状态与身份

- revisioned Effective Skill Registry 是唯一权威源；prompt 历史只是模型可见投影，不授予执行权。
- SkillID 由 source + locator 稳定定位，不再把名称当作唯一主键。同名不同来源的 skill 可分别管理，winner/shadowed 关系另行计算。
- catalog 条目包含 revision、digest、source、path/locator、visibility、mutability 和有界摘要。
- visibility 统一为 `auto`、`name-only`、`manual-only`、`off` 四态，并按 managed/session/project/user/default 分层求 effective 结果。

### 2. Prompt 顺序与实时生效

初始 turn 的逻辑顺序是：

```text
stable system/model instructions
fixed Skill tool schema
developer-role full catalog snapshot
current user input
```

catalog 改变后的 turn 是：

```text
unchanged prior conversation prefix
developer-role coalesced catalog delta
current user input
```

skill 真正调用时是：

```text
current request / fixed Skill tool call
latest-registry validation
versioned user-role SKILL.md envelope (ID + revision + digest)
continued sampling
```

因此“当前 session 立即生效”包含两层含义：

1. 管理变更会立即更新 Manager 和当前 session 投影，下一次模型请求看到 append-only delta；
2. SkillTool 执行时必须通过最新 registry/revision/digest 验证，所以 off/revoke 不需等到下一个 turn 就能阻断后续执行。

### 3. 缓存保持策略

- system 指令、固定 Skill tool schema、model settings 和 PromptCacheKey 与 catalog revision 解耦。
- catalog 无变化时不生成新消息；add/update/visibility/re-enable 合并成 upsert，disable/delete/permission loss 合并成 revoke。
- 旧历史不被就地改写，所以 catalog 修改之前的 request prefix 能保持字节一致。
- Responses 增量路径只发送新追加项。
- compaction/new context epoch 被明确定义为一次有意识的 prefix rebuild，同时重建 catalog cursor 与 loaded-body ledger，不伪装成“永远不破缓存”。

本地验收证明的是 request shape、字节前缀和 append-only 性质，不是真实供应商缓存命中率或计费折扣。

### 4. `/skills` 二级列表

- 精确 `/skills` 立即打开一个可搜索 checklist；`/skills list/show/set/...` 仍走文本命令后端。
- 列表左侧始终显示 checkbox：配置为非 off 是 `[x]`，off 是 `[ ]`。被同名 winner shadow 的非 off 行仍是 `[x]`，选中详情说明它当前未激活及 winner stable ID。
- Space 只修改选中 stable ID，使用行被渲染时观察到的 catalog revision。变更成功或失败后都从权威结果重绘，不做乐观伪状态。
- Space 仅在 `off` 和已持久化的 last-non-off 状态间切换；没有历史时回到 `auto`。
- 交互快捷开关写 project override；如果当前 session 有更高优先级 override，快捷开关明确拒绝并给出清理方法，而不是悄悄覆盖。
- Enter 不切换，一次 Esc 关闭。

### 5. 列表布局与可访问性

- 每个 skill 列表项严格一个终端行，使用 display-cell 宽度预算和省略。
- 输入会清理 ANSI/control/newline，进行 NFC 归一化。当底层 go-tui 按 rune 计算的模型无法安全表达某个 emoji/ZWJ/variation-selector grapheme 时，整个不安全 cluster 原子性替换为占位符，不会半截或越界。
- 仅选中 skill 渲染详情，而且每行详情都受内宽预算。选择、过滤、resize 会同时清除上一项详情。
- root 与 panel 共享一次布局计算，panel 高度计入 chromeRows，包含 notice 和上下边框。极矮 viewport 使用互斥有界 panel，不依赖事后裁剪遮住溢出。
- TUI 和 screen-reader 使用共享的状态与语义合约，新增用户可见文案经过 semantic i18n key，全语言 catalog 完整性测试通过。

### 6. MCP、session 和保留型子运行时

- 项目文件 skill、MCP prompts 和 `skill://` resources 使用同一 Manager 事务，覆盖 initial connect、late discovery、list_changed、update、disconnect、reconnect 与 shadow/restore。
- MCP invalidator 按 Manager 实例归属，unregister/close 会等待 in-flight callback，旧 client 的延迟回调不能把 workspace A 的 catalog 复活到 workspace B。
- session sidecar 持久化 overlay、visible context epoch、announced cursor 和 loaded-body ledger。resume/compact 只信任当前可见 epoch 中有消息证据的状态。
- retained Agent/Team/background follow-up 按 session ID、session project dir、canonical project root 和 exact project generation 围栏。所有者复核和 enqueue/persist 在同一短生命 generation read lease 内完成，防止 A→B→A 的代际穿越。

## 变更文件与所有权

31 个任务 JSON 的 `owned_files` 去重后共 152 个交付路径，其中包含本 task29 的两份报告。在报告落盘之前，150 个代码/测试交付全部存在且处于专项变更集中：60 个已跟踪文件被修改，90 个是新增文件，0 个缺失。task29 另新增：

- `tasks/skills_live_catalog_cache/ACCEPTANCE.md`
- `tasks/skills_live_catalog_cache/IMPLEMENTATION_REPORT.md`

功能变更按所有权分组如下：

- 消息/协议：`types/messages.go`、`types/tools.go`、`skills/catalog_contract.go`、`skills/catalog_identity.go`、`skills/catalog_policy.go`、`skills/catalog_render.go`、`skills/catalog_diff.go`、`skills/invocation_envelope.go`。
- registry/override/MCP：`skills/invocation_origin.go`、`skills/override_store.go`、`skills/loader.go`、`skills/mcp_prompt_discovery.go`、`skills/mcp_runtime_generation.go`、`skills/mcp_skills.go`、`services/mcp/catalog_lifecycle.go` 及 manager/cache/notifications/reconnect 集成。
- prompt/provider：`loop/skill_catalog.go`、`loop/query.go`、`loop/query_state.go`、`loop/context_prepare.go`、`provider/anthropic.go`、`provider/openai.go`、`provider/responses.go`。
- compact/session：`session/skill_catalog_state.go`、`session/session.go`、`compact/compact.go`、`compact/invariants.go`、microcompact/content-replacement/post-compact/recovery 相关文件。
- 命令/执行：`commands/skills.go`、`commands/commands.go`、`tools/skill.go`、`tools/file_edit_settings_validator.go`。
- runtime 组装：`engine/config.go`、`engine/core.go`、`engine/session_adapter.go`、`registry_setup.go`、`main.go`、`mcp_skill_runtime.go`、`worktree_session_runtime.go`、`session_switcher.go`。
- 子运行时隔离：`tools/agent.go`、`tools/agent_sessions.go`、`tools/team.go`、`tools/background_tasks.go`、`tools/runtime_task_store.go`、`tools/skill_project_generation.go` 与 worktree enter/exit/session-state 文件。
- UI：`repl_tui.go`、`repl_screen_reader.go`、`tui/app.go`、`tui/state.go`、`tui/root.go`、`tui/skills_surface.go`、`tui/skills_toggle_view.go`、`tui/skills_layout.go`、`tui/slash_commands.go`、`tui/session_projection.go`。
- i18n：`i18n/session_command_keys.go`、`i18n/skill_tool_keys.go`、`i18n/skills_menu_keys.go`、`i18n/skills_screen_reader_keys.go`、`i18n/skills_tui_routing_keys.go` 及各自的 catalog 完整性测试。
- 验收：每个实施 task 的 `*_taskNN_test.go`，以及 task26 集成、task27 缓存、task28 生命周期/race/表面、task31 布局测试。

完整的逐文件所有权清单以 `tasks/skills_live_catalog_cache/task_01.json` 至 `task_31.json` 的 `owned_files` 字段为权威源，`INDEX.json.hot_file_owners` 记录了共享热点文件的唯一任务所有者。

## 简化点

- 一个固定 Skill tool 取代“每个 skill 一个 tool definition”，tool schema 和 cache key 不再随 catalog 抖动。
- 一个 Manager + 一个 override store + 一个 session overlay 承担项目/MCP/UI/执行的同一权威事务，减少了表面之间各自复制状态。
- 一个 stable SkillID 贯穿列表、持久化、delta 和执行验证，删除了以 name 为权限主键的歧义。
- 一条 append-only catalog coordinator 代替就地重写 prompt snapshot，变更集只有 upsert/revoke 两种模型可见事件。
- 一个权威 redraw 结果同时处理成功、stale、普通回滚与 degraded-refresh-required，避免 UI 自己猜测后端状态。
- 一次 skills panel 布局计算同时产生宽度、可见范围、详情/通知行数和总高，不再把 panel height 二次回馈导致布局漂移。
- 用预算与有界单行渲染取代事后 `OverflowHidden` 裁剪 workaround，底边框是布局合约的一部分。

## 已知风险与非支持/未验证行为

1. **已暴露 prompt 无法撤回。** off/revoke 能阻断未来执行，却不能删除模型已经看到的 snapshot 或 body。对敏感文本，唯一安全边界是新建或可证明已清洗的 context。
2. **真实 provider cache 命中率未测量。** 当前证明了精确前缀稳定和增量 request shape，不代表 Anthropic/OpenAI 线上计费或命中率承诺。
3. **Windows 仅交叉编译。** `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c ... .` 通过，但 Unix PTY 验收在 Windows 使用明确 skip stub；真实 Windows Console 的 Space/Esc/resize 仍需平台测试。
4. **其他平台未运行。** 本次主机为 darwin/arm64，没有实机验证 Linux、Intel macOS 或特定终端模拟器。
5. **MCP 第三方偏差。** 当前使用 fake/in-process 对 connect/reconnect/list_changed/disconnect 做了确定性覆盖，不可能代表所有第三方 server 的异常时序。
6. **全仓 gofmt 基线债务仍在。** `gofmt -l .` 列出 96 个相对 HEAD 未修改的既有文件。本专项未增加该债务，但也没有在所有权外清理它。
7. **超复杂 grapheme 的降级。** 当 renderer 的 per-rune cell 模型不能保证整个 cluster 安全呈现时，UI 会宁可使用原子占位符也不半截/越界；这保证布局安全，但不保证每个组合 emoji 的完全视觉保真。

## 验证结论

聚焦、包级、race、全仓 test、vet、build、Windows cross-compile 和 diff-check 全部通过；详细命令、基准数据、gofmt 例外归因与收尾状态见 `tasks/skills_live_catalog_cache/ACCEPTANCE.md`。
