# 原版 `../src` 与当前 Go 复刻版按模块功能差距分析

日期：2026-04-12

## 结论摘要

当前 Go 复刻版不是“底层能力缺失很多”，而更像是：

- **核心执行引擎、Provider、工具底座已经具备较强可用性**
- **用户交互层、命令工作流、TUI/UI、扩展生态与产品化能力明显落后于原版**

换句话说，当前项目更接近：

> **Claude Code engine + 一部分 CLI/TUI 外壳**

而不是原版 TypeScript Claude Code 的完整等价产品。

差距主要不在：

- provider
- query loop
- tool primitives

而在：

- commands
- interactive UI / TUI
- plugin / skills ecosystem
- session / model / project management UX
- analytics / feature flags / remote product flows

---

## 分析口径

本文中的统计不是逐函数精确计数，而是基于下列信息进行的**模块级功能点估算**：

- `../src` 原版目录结构与模块分布
- 当前 Go 复刻版目录结构与已有实现
- 对关键源码的抽查（如 `commands/`、`repl.go`、`repl_tui.go`、`ui/term_renderer.go`、`input/`、`permissions/`、`session/`、`provider/`、`tools/`）
- 仓库内已有审计文档：
  - `AUDIT_HIGH_VALUE_GAPS.md`
  - `SPRINT5_GAPS.md`
  - `CODEBASE_ANALYSIS.md`
  - `EXPLORATION_SUMMARY.md`

### 三个判断维度

#### 1. 未实现功能
原版有完整用户可用能力，Go 版没有对应能力，或只有零散底层代码、没有用户入口。

#### 2. 实现效果较差
Go 版已有对应能力，但在下列方面明显弱于原版：

- 只支持最小子集
- 没有交互 UI
- 没有自动化联动
- 可视化、易用性、状态反馈较弱
- 性能或可恢复性不如原版

#### 3. 差多少
使用模块完成度估计与差距等级表示：

- 小：<20%
- 中：20%~50%
- 大：50%~80%
- 很大：>80%

---

## 按模块分析

---

### 1) 会话 / Query Loop / 执行内核

**原版对应**

- `query.ts`
- `QueryEngine.ts`
- `Tool.ts`
- 一部分 `services/tools/*`

**Go 对应**

- `loop/`
- `engine/`
- `types/`
- `coordinator/`

**结论**

这是 Go 版完成度最高的模块之一。主执行链路、事件流、并发执行、Provider 对接已经成型。

**未实现功能：约 2~4 个**

1. 更完整的运行态机/事件展示联动
2. 与丰富 UI 的深度耦合能力
3. 原版 bridge / remote session 的部分运行路径
4. 更完整的恢复与远端协作链路

**实现效果较差：约 3~5 个**

1. 事件流虽然存在，但展示层利用不足
2. 工具执行进度反馈偏弱
3. 某些 streaming 细粒度体验不如原版
4. 与压缩/统计/远端桥接的自动联动偏弱
5. 错误恢复与状态回显不如原版完整

**差多少**

- 完成度估计：**75%~85%**
- 差距：**中偏小**

---

### 2) 工具系统

**原版对应**

- `tools/`
- `tools.ts`
- `Tool.ts`

**Go 对应**

- `tools/`
- `registry/`

**结论**

底层工具能力已经很强，覆盖面广，包含文件、Git、Web、LSP、MCP、Task、Team、Cron 等大量能力。但和原版相比，仍有生态和交互层面的明显差距。

**未实现功能：约 6~10 个**

1. 一些原版特定工具 UI 配套
2. 部分插件型工具接入能力
3. 更完整的权限特化工具界面
4. 更完整的远端/bridge 场景工具
5. Agent/Tool UI 组合能力
6. 一些平台特定工作流
7. 原版中由插件提供的工具扩展面
8. 某些高级任务编排辅助能力

**实现效果较差：约 8~15 个**

1. 工具结果展示较原始
2. FileEdit / diff 体验弱
3. AskUser / Permission 类交互不够丰富
4. Agent / Team 工具反馈较弱
5. 长输出处理简化
6. 工具与配置、市场、插件联动不足
7. 高风险工具的 UX 弱于原版
8. 结构化结果展示能力偏弱
9. 结果折叠/展开不够成熟
10. 缺少复杂工具的专用视图

**差多少**

- 完成度估计：**65%~80%**
- 差距：**中**

---

### 3) Slash Commands 命令系统

**原版对应**

- `commands/`
- `commands.ts`

**Go 对应**

- `commands/`

**结论**

这是差距最明显的模块之一。Go 版有一批基础命令，但与 TS 原版相比，**命令数量、交互深度、工作流完整性都明显不足**。

**Go 当前明确已有命令**

- `/help`
- `/exit`
- `/clear`
- `/compact`
- `/model`
- `/cost`
- `/version`
- `/session`
- `/memory`
- `/diff`
- `/config`
- `/status`
- `/init`
- `/resume`
- `/context`
- `/review`
- `/permissions`
- `/paste`

约 **18 个**内建命令。

**未实现功能：约 20~30 个命令级功能**

典型缺失：

1. `/plugin`
2. `/skills`
3. `/reload-plugins`
4. `/plan`
5. `/tasks`
6. `/agents`
7. `/mcp` 完整交互命令
8. `/theme`
9. `/usage`
10. `/stats`
11. `/output-style`
12. `/keybindings`
13. `/voice`
14. `/login` / `/logout`
15. `/install` 相关流
16. `/sandbox-toggle`
17. `/rewind`
18. `/rename`
19. `/export`
20. `/upgrade`
21. `/privacy-settings`
22. `/fast` / `/effort`
23. remote / bridge / mobile / desktop 等产品命令

**实现效果较差：约 6~10 个**

即使已有的命令，很多也只是简化版：

1. `/model` 只有字符串切换，没有浏览/筛选/能力展示
2. `/resume` 只是列会话与 ID 恢复，没有会话选择器
3. `/session` 没有真正的 TUI 列表/预览
4. `/config` 只是 JSON 读写，没有验证/设置界面
5. `/permissions` 体验偏弱
6. `/context` / `/status` 信息维度少
7. `/diff` 展示弱于原版
8. 一些命令只有文本输出，没有交互式引导

**差多少**

- 完成度估计：**35%~50%**
- 差距：**大**

---

### 4) TUI / 渲染 / 交互 UI

**原版对应**

- `main.tsx`
- `components/`
- `screens/`
- `dialogLaunchers.tsx`
- `interactiveHelpers.tsx`
- `ink/`

**Go 对应**

- `tui/`
- `ui/`
- `render.go`
- `repl_tui.go`

**结论**

这是差距最大的模块。Go 版已经开始搭建 TUI，但相对于原版的 Ink 组件体系，仍处于早中期阶段。

**未实现功能：约 15~25 个**

典型缺失：

1. 通用对话框系统
2. 会话恢复选择器
3. 模型选择器
4. 插件市场/插件管理 UI
5. 设置页面/配置校验 UI
6. 全屏 pane 式布局
7. 上下文浏览器
8. 复杂 diff 查看器
9. Agent/Task 详情面板
10. 背景任务对话框
11. MCP server 选择/审批对话框
12. keybinding 帮助 UI
13. theme picker
14. usage/settings 视图
15. onboarding / trust / warning dialog
16. 多面板导航
17. 消息级操作面板
18. 完整的状态栏/统计侧边区

**实现效果较差：约 10~18 个**

1. 文本渲染仍偏终端流式输出
2. thinking block 展示较弱
3. tool result 结构化展示有限
4. 长结果折叠/展开能力弱
5. diff 体验弱
6. 进度条/任务状态反馈弱
7. 成本/上下文展示虽已有，但信息密度低于原版
8. 对复杂工作流的交互承载不足
9. TUI 组件复用层薄
10. 多视图切换能力弱
11. 复杂列表/选择器体验不成熟

**差多少**

- 完成度估计：**20%~35%**
- 差距：**很大**

---

### 5) 输入系统

**原版对应**

- `PromptInput/*`
- `hooks/useTextInput.ts`
- `useGlobalKeybindings.tsx`
- `keybindings/*`
- paste / history 相关实现

**Go 对应**

- `input/`
- `repl.go`
- `tui/clipboard.go`

**结论**

Go 版基础可用，但比原版的输入编辑体验差很多。

**未实现功能：约 5~8 个**

1. 完整 keybinding 定制系统
2. 真正成熟的输入框编辑体验
3. 历史搜索 UI
4. 更丰富的大块粘贴处理
5. 更完善的 token warning 联动
6. 类似原版 textarea 的输入体验
7. 用户自定义快捷键体系
8. 多模式输入辅助能力

**实现效果较差：约 5~7 个**

1. `input/multiline.go` 是启发式多行，不如原版稳定自然
2. readline 模式体验有限
3. 历史能力弱于原版
4. 粘贴检测/处理能力较简化
5. 控制键行为不如原版丰富
6. 输入区状态反馈弱
7. 编辑体验连续性差于原版

**差多少**

- 完成度估计：**40%~55%**
- 差距：**中到大**

---

### 6) 权限与安全交互

**原版对应**

- `components/permissions/*`
- `hooks/toolPermission/*`

**Go 对应**

- `permissions/`
- `commands/permissions.go`
- `permissions/rich_prompt.go`

**结论**

权限引擎本身不差，但权限交互 UX 比原版差很多。

**未实现功能：约 6~10 个**

1. 多种工具类型专属权限界面
2. 更完整的规则编辑 UI
3. 工作区目录授权流
4. 更复杂的 permission explanation
5. IDE diff / file permission dialog 深度支持
6. worker / remote permission handling
7. plan mode / ask user 专门审批视图
8. 更完整的权限回顾与管理入口

**实现效果较差：约 5~8 个**

1. 主要还是 terminal prompt / rich prompt
2. “Always allow”能力弱于原版可视化规则体系
3. 风险分类虽有，但展示层较轻
4. 不同工具场景的审批解释不够细
5. 权限决策回顾/管理能力弱
6. 与复杂工作流的联动不足
7. 权限提示上下文较少

**差多少**

- 完成度估计：**55%~70%**
- 差距：**中**

---

### 7) Session 持久化 / 压缩 / 同步

**原版对应**

- `history.ts`
- `state/*`
- `services/compact/*`
- bridge / remote session 相关

**Go 对应**

- `session/`
- `compact/`
- `commands/resume.go`
- `commands/session.go`

**结论**

本地持久化和 compaction 基础已经有，但和原版相比，缺在自动化、同步与交互体验。

**未实现功能：约 5~8 个**

1. 自动压缩触发策略的完整接线
2. 远端会话同步
3. 会话预览/恢复 UI
4. 更丰富的 metadata 管理
5. Claude.ai / bridge 会话能力
6. 增量追加式存储优化
7. 更完整的会话重命名/管理流
8. 会话级统计/摘要能力

**实现效果较差：约 4~6 个**

1. `session.Save` 仍是整份重写，不是增量 append
2. `/resume`、`/session list` 只是文本列举
3. 压缩能力有，但运行时联动较弱
4. 会话浏览体验弱
5. 崩溃恢复与远端恢复能力弱于原版
6. 缺少会话级可视化管理

**差多少**

- 完成度估计：**50%~65%**
- 差距：**中**

---

### 8) Provider / 模型能力

**原版对应**

- `commands/model/model.tsx`
- 状态栏/模型切换/hooks 中分散实现

**Go 对应**

- `provider/`
- `commands /model`
- `engine.SetModel`

**结论**

底层 provider 支持其实很强，但模型管理 UX 很弱。

**未实现功能：约 3~5 个**

1. 模型浏览器
2. 按能力/价格筛选
3. 会话级模型元信息展示
4. 模型说明/成本提示 UI
5. 更丰富的 provider 切换工作流

**实现效果较差：约 3~5 个**

1. `/model` 只是“输入名字切换”
2. 没有浏览/搜索/推荐
3. 模型能力展示弱
4. 成本与模型联动展示不够丰富
5. 切换后的状态反馈简化

**差多少**

- 底层 provider 完成度：**80%~90%**
- 模型交互完成度：**30%~45%**
- 综合完成度估计：**60%~75%**
- 差距：**中**

---

### 9) 插件 / Skills / 扩展生态

**原版对应**

- `commands/plugin/*`
- `commands/skills/*`
- `commands/reload-plugins/*`
- `plugins/*`
- `hooks/useManagePlugins.ts`

**Go 对应**

- `skills/`
- `tools/skill.go`
- `registry_setup.go`

**结论**

这是功能存在一些底层残片，但用户级能力明显未成型的模块。

**未实现功能：约 8~12 个**

1. `/plugin`
2. `/skills`
3. `/reload-plugins`
4. 插件发现
5. 插件安装/删除/更新
6. marketplace 浏览
7. 信任/校验工作流
8. skills 管理界面
9. 扩展设置界面
10. 动态热重载工作流
11. 更完整的插件状态管理
12. 市场来源管理

**实现效果较差：约 2~4 个**

1. skill invoke 有底层支持，但管理不可见
2. skills loader 有，但用户体验不足
3. registry 层与扩展生态联动不完整
4. 扩展结果与配置回显弱

**差多少**

- 完成度估计：**20%~35%**
- 差距：**很大**

---

### 10) Hooks / MCP / 协调执行

**原版对应**

- `hooks/`
- `services/mcp/*`
- `commands/mcp/*`
- coordinator 相关

**Go 对应**

- `hooks/`
- `mcp/`
- `coordinator/`
- `tools/mcp_tools.go`
- `tools/team.go`
- `tools/tasks.go`

**结论**

这是 Go 版相对优秀的一块。底层能力明显比 UI 模块成熟得多。

**未实现功能：约 4~7 个**

1. 完整的 MCP 管理命令 UI
2. 某些远端/bridge 场景联动
3. 更完整的 approval / server management
4. 更复杂的 background task UI
5. 原版一些 transport / remote 协作能力
6. 更强的协调执行可视化
7. 更完善的运营级接线

**实现效果较差：约 3~5 个**

1. hooks 有，但可视化管理弱
2. MCP 生命周期有，但用户管理入口弱
3. team/task 的展示反馈不如原版
4. 协调执行的“看得见的状态”较弱
5. 复杂流程的调试体验弱

**差多少**

- 完成度估计：**65%~80%**
- 差距：**中偏小**

---

### 11) 项目上下文 / Git / 环境感知

**原版对应**

- project detection / git context / file suggestions / onboarding 逻辑
- `hooks/fileSuggestions.ts`
- 相关 utils / services

**Go 对应**

- `prompt/`
- `tools/git_operations.go`
- `commands/diff.go`

**结论**

命令级 Git 工具不错，但“项目理解能力”明显弱于原版。

**未实现功能：约 5~8 个**

1. 项目类型自动检测
2. framework-aware 上下文增强
3. `.gitignore` 更深度参与
4. 分支/remote/fork 关系感知
5. 自动关键文件纳入上下文
6. 更完整的 repo onboarding
7. 文件建议与项目结构联动
8. 更智能的上下文裁剪来源识别

**实现效果较差：约 3~5 个**

1. Git context 更偏原始信息
2. prompt 上下文构建较简单
3. diff / context / file suggestion 体验不如原版
4. 缺少框架/语言感知增强
5. 项目入口识别能力弱

**差多少**

- 完成度估计：**40%~55%**
- 差距：**中到大**

---

### 12) 遥测 / Analytics / Feature Flags

**原版对应**

- `services/analytics/*`
- GrowthBook
- datadog
- first-party event logging
- metadata / sink

**Go 对应**

- 几乎没有同等级体系

**结论**

这是基本缺失的模块。

**未实现功能：约 6~10 个**

1. event tracking
2. analytics pipeline
3. GrowthBook / feature flags
4. A/B testing
5. metadata logging
6. exporter / sink
7. 实验开关控制
8. 产品指标采集
9. 行为事件治理
10. 灰度发布联动

**实现效果较差：约 0~2 个**

因为很多不是“差”，而是“没有”。

**差多少**

- 完成度估计：**5%~15%**
- 差距：**很大**

---

## 汇总表

| 模块 | 未实现功能数 | 实现较差功能数 | 完成度估计 | 差距 |
|---|---:|---:|---:|---|
| 会话/查询执行内核 | 2~4 | 3~5 | 75%~85% | 中偏小 |
| 工具系统 | 6~10 | 8~15 | 65%~80% | 中 |
| 命令系统 | 20~30 | 6~10 | 35%~50% | 大 |
| TUI/渲染/UI | 15~25 | 10~18 | 20%~35% | 很大 |
| 输入系统 | 5~8 | 5~7 | 40%~55% | 中到大 |
| 权限与安全交互 | 6~10 | 5~8 | 55%~70% | 中 |
| Session 持久化/压缩/同步 | 5~8 | 4~6 | 50%~65% | 中 |
| Provider/模型能力 | 3~5 | 3~5 | 60%~75% | 中 |
| 插件/Skills/扩展 | 8~12 | 2~4 | 20%~35% | 很大 |
| Hooks/MCP/协调执行 | 4~7 | 3~5 | 65%~80% | 中偏小 |
| 项目上下文/Git感知 | 5~8 | 3~5 | 40%~55% | 中到大 |
| Analytics/Feature Flags | 6~10 | 0~2 | 5%~15% | 很大 |

---

## 按“差距最大”排序

1. **TUI / 渲染 / 交互 UI**
2. **插件 / Skills / 扩展生态**
3. **Analytics / Feature Flags**
4. **命令系统**
5. **输入系统 / 项目上下文感知**

---

## 按“最影响日常可用性”排序

### 1. TUI / 交互 UI
不是不能用，而是信息密度、反馈完整性、操作效率与原版差距很大。

### 2. 命令系统
原版很多核心工作流依赖 slash command + dialog，Go 版当前命令面明显偏窄。

### 3. 模型 / 会话管理交互
Go 版虽然有 `/model`、`/resume`，但都明显简化，缺少选择器、预览、状态化管理。

### 4. 插件 / skills 生态
原版扩展性强很多，Go 版用户几乎感知不到完整生态。

### 5. 输入体验
多行、快捷键、历史、粘贴等体验未追上原版。

---

## 总体数量级估算

如果把当前项目按模块总计，粗略可认为：

- **未实现功能总量**：约 **85~137 个模块级子功能点**
- **实现较差功能总量**：约 **52~90 个模块级子功能点**

注意：

这不是精确到函数级别的机械计数，而是面向用户可感知能力的模块级粗估。

换句话说，当前 Go 版更接近：

- **底层能力层：70% 左右**
- **用户工作流层：40% 左右**
- **完整产品层：35%~50% 左右**

---

## 最准确的一句判断

**当前复刻版更像“Claude Code engine + 一部分 CLI/TUI 外壳”，而不是原版完整产品的等价替身。**

差距主要不在：

- provider
- loop
- tool primitives

而在：

- **commands**
- **interactive UI**
- **plugin/skills ecosystem**
- **session/model/project management UX**
- **product infrastructure（analytics/feature flags/remote flows）**
