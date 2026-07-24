# Go Skills Module — 原版对齐分析与改进计划

## 1. 架构定位

TS 原版将 Skills 设计为**两层架构**：

| 层次 | TS 路径 | Go 当前路径 | 职责 |
|------|---------|------------|------|
| **独立模块** | `src/skills/` | `gosrc/skills/` (死代码) | 技能加载、解析、发现、注册 |
| **Tool 消费者** | `src/tools/SkillTool/` | `gosrc/tools/skill.go` (在用) | 模型调用入口、执行、prompt |

Go 端当前的问题：
- `gosrc/skills/` 是完全死代码，未被任何文件 import
- `gosrc/tools/skill.go` 把加载逻辑（SkillManager）和 Tool 执行混在一起
- 需要重构为：`skills/` 负责加载 → `tools/skill.go` 只负责执行

## 2. 差异清单

### 2.1 YAML Frontmatter 解析（🔴 完全缺失）

TS 使用 `src/utils/frontmatterParser.ts` 解析 `---\n...\n---` 分隔的 YAML frontmatter。

支持的字段（TS `FrontmatterData` 类型）：

| 字段 | 类型 | Go 支持 | 说明 |
|------|------|---------|------|
| `description` | string | ❌ | 技能描述 |
| `allowed-tools` | string/string[] | ❌ | 限制技能可用工具列表 |
| `argument-hint` | string | ❌ | 参数提示（UI 显示） |
| `when_to_use` | string | ❌ | 模型何时应使用此技能 |
| `version` | string | ❌ | 技能版本号 |
| `model` | string | ❌ | 模型覆盖 |
| `user-invocable` | string | ❌ | 是否允许用户通过 /name 调用 |
| `hooks` | HooksSettings | ❌ | 技能级钩子 |
| `effort` | string | ❌ | effort 级别覆盖 |
| `context` | 'inline'/'fork' | ❌ | 执行模式：内联或分叉 |
| `agent` | string | ❌ | 分叉执行时的代理类型 |
| `paths` | string/string[] | ❌ | 条件激活路径模式 |
| `shell` | 'bash'/'powershell' | ❌ | Shell 命令执行配置 |

### 2.2 技能加载源（🔴 仅支持 2/7）

| 加载源 | TS | Go | 优先级 |
|--------|----|----|--------|
| 托管策略 `policySettings` | ✅ | ❌ | 最高 |
| 用户全局 `~/.claude/skills` | ✅ | ✅ | 高 |
| 项目级 `.claude/skills` | ✅ | ✅ | 中 |
| CLI `--add-dir` 附加目录 | ✅ | ❌ | 中 |
| 旧版 `/commands/` | ✅ | ❌ | 低 |
| 插件 plugin | ✅ | ❌ | — |
| MCP 远程 | ✅ | ❌ | — |
| 内置 bundled | ✅ | ❌ | — |

### 2.3 文件格式支持（🟡 部分）

| 格式 | TS | Go skills/ | Go tools/skill.go |
|------|----|-----------|--------------------|
| 目录/SKILL.md | ✅ | ✅ | ✅ |
| 单 .md 文件 | ✅ | ✅ | ❌（只扫描子目录） |
| 目录/skill.json | ❌ | ❌ | ✅（Go 独有） |
| YAML frontmatter | ✅ | ❌ | ❌ |

### 2.4 SkillTool 功能（🔴 严重缺失）

| 功能 | TS | Go |
|------|----|----|
| Inline 执行（prompt 注入） | ✅ | ⚠️ 基础版 |
| Fork 执行（子代理） | ✅ | ❌ |
| Remote 加载（实验性） | ✅ | ❌ |
| 修改 allowedTools | ✅ | ❌ |
| 应用 model 覆盖 | ✅ | ❌ |
| 应用 effort 覆盖 | ✅ | ❌ |
| 参数替换 `$arg_name` | ✅ | ❌ |
| 变量替换 `${CLAUDE_SKILL_DIR}` | ✅ | ❌ |
| 变量替换 `${CLAUDE_SESSION_ID}` | ✅ | ❌ |
| Shell 命令嵌入 `` !`cmd` `` | ✅ | ❌ |
| 权限检查 | ✅ | ❌ |
| Strip frontmatter | ✅ | ❌ |
| 使用追踪 | ✅ | ❌ |
| 系统提示中技能列表 | ✅ | ❌ |
| 技能列表预算控制 (1% ctx) | ✅ | ❌ |

### 2.5 辅助功能（🟡 中低优先级）

| 功能 | TS 文件 | 状态 | 优先级 |
|------|---------|------|--------|
| 技能去重（realpath） | loadSkillsDir.ts | ❌ | P2 |
| 动态技能发现 | loadSkillsDir.ts L861-975 | ❌ | P3 |
| 条件技能（paths） | loadSkillsDir.ts L997-1058 | ❌ | P2 |
| 文件变更监视 | skillChangeDetector.ts | ❌ | P3 |
| 技能改进建议 | skillImprovement.ts | ❌ | P3 |
| 使用追踪/排序 | skillUsageTracking.ts | ❌ | P3 |
| 内置技能 (17+) | bundled/ | ❌ | P3 |
| Compact 后恢复 | compact.go TODO L28 | ❌ | P2 |

## 3. 改进计划

### Phase 1: 核心重构（本次实施）

**目标**: 重建 `gosrc/skills/` 为功能完备的独立模块

#### 1.1 Skill 类型定义 + Frontmatter 解析

新建 `skills/types.go`:
- `Skill` 结构体：完整对齐 TS `Command` + `PromptCommand` 中的 skill 相关字段
- `SkillSource` 枚举：project/user/managed/plugin/mcp/bundled/commands_legacy
- `SkillContext` 枚举：inline/fork

新建 `skills/frontmatter.go`:
- YAML frontmatter 解析（`---` 分隔符）
- 特殊字符处理（glob 模式中的大括号等）
- 各字段的类型转换和校验

#### 1.2 多源加载器

重写 `skills/loader.go`:
- 多目录加载（优先级：project > user > legacy commands）
- 文件格式：单 .md 文件 + 目录/SKILL.md（移除非标准的 skill.json）
- Symlink 去重（`filepath.EvalSymlinks`）
- 线程安全（`sync.RWMutex`）
- 懒加载 + 缓存 + Refresh

#### 1.3 参数替换和变量

新建 `skills/substitute.go`:
- `$arg_name` 命名参数替换（从 frontmatter `arguments` 定义）
- `${CLAUDE_SKILL_DIR}` 替换为技能所在目录
- `${CLAUDE_SESSION_ID}` 替换为当前会话 ID

#### 1.4 Prompt 内 shell 命令执行

新建 `skills/shellexec.go`:
- `` !`command` `` 内联 shell 命令执行
- ` ```! ` 多行 shell 块执行
- 命令输出替换回 prompt 内容

### Phase 2: Tool 层重构

#### 2.1 重构 `tools/skill.go`

- 移除 SkillManager 和 loadSkillDir（迁移到 skills 包）
- SkillTool 改为引用 `skills.Manager`
- 添加 `allowedTools` 限制
- 添加 `model`/`effort` 覆盖传递
- Strip frontmatter 再返回 prompt

#### 2.2 系统提示中的技能列表

新增 `skills/prompt.go`:
- 生成 `<system-reminder>` 中的技能列表
- 预算控制：上下文窗口的 1%（默认 8000 字符）
- 描述截断（内置技能优先保留）
- 集成到 `prompt/system.go` 的 BuildSystemPromptParts

### Phase 3: 后续迭代（不在本次范围）

- P2: 条件技能（paths frontmatter + 文件操作触发）
- P2: Compact 后技能恢复（createSkillAttachmentIfNeeded）
- P3: Fork 执行模式
- P3: 内置技能移植
- P3: 文件变更监视
- P3: MCP 远程技能
- P3: 使用追踪和排序

## 4. 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `skills/types.go` | 新建 | Skill 结构体、Source 枚举、Context 枚举 |
| `skills/frontmatter.go` | 新建 | YAML frontmatter 解析 |
| `skills/frontmatter_test.go` | 新建 | frontmatter 解析测试 |
| `skills/loader.go` | 新建 | 多源加载器（替代旧 skills.go 的 Loader） |
| `skills/loader_test.go` | 新建 | 加载器测试 |
| `skills/substitute.go` | 新建 | 参数/变量替换 |
| `skills/substitute_test.go` | 新建 | 替换测试 |
| `skills/prompt.go` | 新建 | 技能列表生成 + 预算控制 |
| `skills/prompt_test.go` | 新建 | prompt 测试 |
| `skills/skills.go` | 删除 | 旧实现（死代码） |
| `skills/skills_test.go` | 删除 | 旧测试 |
| `tools/skill.go` | 重写 | 瘦身为纯 Tool，引用 skills 模块 |
| `tools/skill_test.go` | 更新 | 适配新接口 |
| `prompt/system.go` | 修改 | 集成技能列表到系统提示 |
| `registry_setup.go` | 修改 | 传入 skills.Manager 到 SkillTool |

## 5. 受影响的功能

| 功能 | 影响说明 | 对齐后 |
|------|---------|--------|
| 模型技能发现 | ❌ 当前模型不知道有哪些技能 | ✅ 系统提示包含技能列表 |
| 技能元数据 | ❌ 所有 frontmatter 被忽略 | ✅ 完整解析 16 个字段 |
| 多源加载 | ⚠️ 仅 2 个源 | ✅ 支持 project/user/legacy |
| 参数传递 | ⚠️ 只有追加模式 | ✅ 命名参数 $arg + 变量替换 |
| 工具限制 | ❌ 技能无法限制工具 | ✅ allowedTools 生效 |
| 模型覆盖 | ❌ | ✅ model/effort 传递 |
| 去重 | ❌ | ✅ symlink 去重 |

---

## 6. 实施记录

### Phase 1 — 核心骨架（✅ 完成）

| 文件 | 状态 | 测试数 | 说明 |
|------|------|--------|------|
| `skills/types.go` | ✅ | — | Skill 结构体 20+ 字段，SkillSource/SkillContext 常量 |
| `skills/frontmatter.go` | ✅ | 9 | YAML frontmatter 16 字段解析，yamlStrings 自定义反序列化器 |
| `skills/frontmatter_test.go` | ✅ | 9 | 覆盖基本/无 frontmatter/数组/glob/布尔/参数/完整应用 |
| `skills/loader.go` | ✅ | 13 | Manager 多源加载器，DirSource 优先级，symlink 去重 |
| `skills/loader_test.go` | ✅ | 13 | 目录/文件/优先级/刷新/空目录/symlink 去重等 |

### Phase 2 — 替换管道 + 提示列表（✅ 完成）

| 文件 | 状态 | 测试数 | 说明 |
|------|------|--------|------|
| `skills/substitute.go` | ✅ | 36 | 5 步替换链：命名参数→索引→简写→$ARGUMENTS→追加回退 |
| `skills/substitute_test.go` | ✅ | 36 | ParseArguments/SubstituteArguments/Variables/PrepareSkillContent/HasShell |
| `skills/prompt.go` | ✅ | 17 | 预算控制（1%上下文×4字符/token），bundled 永不截断策略 |
| `skills/prompt_test.go` | ✅ | 17 | GetCharBudget/FormatSkillsWithinBudget/Filter/WrapInSystemReminder |

### Phase 3 — 集成 + 清理（✅ 完成）

| 文件 | 操作 | 说明 |
|------|------|------|
| `skills/skills.go` | 🗑️ 删除 | 旧死代码（Loader/Skill/SkillTool 类型冲突） |
| `skills/skills_test.go` | 🗑️ 删除 | 旧测试引用已删除类型 |
| `tools/skill.go` | ✅ 重写 | 消费 skills.Manager，使用 PrepareSkillContent 完整替换管道 |
| `tools/skill_test.go` | ✅ 重写 | 10 个测试覆盖 prompt 执行/参数替换/错误路径/schema |

### 测试汇总

- `go test ./skills/...` — **75 tests PASS** ✅
- `go test ./tools/... -run TestSkillTool` — **10 tests PASS** ✅
- `go build ./skills/... ./tools/... ./engine/... ./loop/...` — **编译通过** ✅
