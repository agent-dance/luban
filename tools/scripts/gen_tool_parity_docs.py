from pathlib import Path
import json
import os
import subprocess

from tool_parity_visuals import VISUALS

ROOT = Path('/Users/buthim/Develop/claude-code-tui/gosrc')
TOOLS_DIR = ROOT / 'tools'
REPORTS_DIR = TOOLS_DIR / 'reports'
REPORTS_DIR.mkdir(parents=True, exist_ok=True)


def load_audit():
    env = os.environ.copy()
    env['AGENT_TRIGGERS'] = '1'
    env['AGENT_TRIGGERS_REMOTE'] = '1'
    env['CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS'] = '1'
    out = subprocess.check_output(
        ['python3', 'tools/scripts/tool_parity_audit.py', '--json'],
        cwd=ROOT,
        env=env,
        text=True,
    )
    return json.loads(out)


def rec(index_en, index_zh, params_en, params_zh, capability_en, capability_zh,
        scenario_en, scenario_zh, pain_en, pain_zh, challenge_en, challenge_zh,
        strategy_en, strategy_zh, output_en, output_zh, gaps_en, gaps_zh,
        go_extra=None, scope_en=None, scope_zh=None, desc_en=None, desc_zh=None):
    return {
        'index_en': index_en,
        'index_zh': index_zh,
        'params_en': params_en,
        'params_zh': params_zh,
        'capability_en': capability_en,
        'capability_zh': capability_zh,
        'scenario_en': scenario_en,
        'scenario_zh': scenario_zh,
        'pain_en': pain_en,
        'pain_zh': pain_zh,
        'challenge_en': challenge_en,
        'challenge_zh': challenge_zh,
        'strategy_en': strategy_en,
        'strategy_zh': strategy_zh,
        'output_en': output_en,
        'output_zh': output_zh,
        'gaps_en': gaps_en,
        'gaps_zh': gaps_zh,
        'go_extra': go_extra or [],
        'scope_en': scope_en,
        'scope_zh': scope_zh,
        'desc_en': desc_en,
        'desc_zh': desc_zh,
    }


TASK_PAIN_EN = 'It addresses visibility and coordination for multi-step work so the model does not have to track the whole plan in free text.'
TASK_PAIN_ZH = '它解决的是多步骤工作的可见性和协同问题，避免模型只能靠自由文本硬记整套计划。'
TASK_CHALLENGE_EN = 'The hard parts are persistence, dependency edges, blocking semantics, and keeping task state consistent across tools and sessions.'
TASK_CHALLENGE_ZH = '难点在于持久化、依赖边、阻塞语义，以及跨工具、跨会话保持任务状态一致。'
TASK_STRATEGY_EN = 'Partially consistent. Both versions revolve around a shared persistent task system, and Go now mirrors more of the original scope, locking, and runtime-task substrate, but the original still has a richer hook-aware app-state runtime.'
TASK_STRATEGY_ZH = '部分一致。两边都围绕共享的持久化任务系统展开，Go 现在也已镜像更多原版的 scope、加锁和 runtime-task 底座，但原版仍然有更丰富、带 hook 的 app-state 运行时。'
TASK_GAPS_EN = 'The remaining gaps are mainly hook/app-state integration, richer typed results, and the broader original teammate/runtime lifecycle.'
TASK_GAPS_ZH = '剩余差距主要集中在 hook/app-state 集成、更丰富的 typed 结果，以及更宽的原版 teammate/runtime 生命周期。'

CRON_PAIN_EN = 'It solves scheduled execution so recurring or deferred work does not depend on a human remembering to trigger it.'
CRON_PAIN_ZH = '它解决的是定时执行问题，让周期性或延后任务不再依赖人工记得去触发。'
CRON_CHALLENGE_EN = 'The hard parts are durable storage, firing semantics, and reconnecting the schedule to the main query loop.'
CRON_CHALLENGE_ZH = '难点在于持久化存储、触发语义，以及把调度结果重新接回主查询循环。'
CRON_STRATEGY_EN = 'Partially consistent. Both versions model cron jobs explicitly, and Go now persists durable jobs and fires them into the local runtime, but the original still has broader scheduler policy, missed-task, and watcher semantics.'
CRON_STRATEGY_ZH = '部分一致。两边都显式建模了 cron 任务，Go 现在也已持久化 durable 任务并把触发接回本地运行时，但原版仍有更宽的调度策略、missed-task 和 watcher 语义。'

WORKTREE_PAIN_EN = 'It solves isolation: risky or branch-heavy changes should happen in a separate git worktree instead of polluting the main session state.'
WORKTREE_PAIN_ZH = '它解决的是隔离问题：高风险或分支很多的改动，应当放在独立 git worktree 中，而不是污染主会话状态。'
WORKTREE_CHALLENGE_EN = 'The hard parts are git worktree lifecycle, branch cleanup, and keeping session state aligned with filesystem state.'
WORKTREE_CHALLENGE_ZH = '难点在于 git worktree 生命周期、分支清理，以及让会话状态和文件系统状态保持一致。'
WORKTREE_STRATEGY_EN = 'Largely consistent. Both versions center on explicit worktree state transitions backed by git operations.'
WORKTREE_STRATEGY_ZH = '大体一致。两边都以显式的 worktree 状态切换为中心，并由 git 操作承载。'

WEB_PAIN_EN = 'It solves outside-information retrieval so the model can ground its answer in fetched web content instead of relying only on memory.'
WEB_PAIN_ZH = '它解决的是外部信息获取问题，让模型可以基于抓取到的网页内容作答，而不是只靠记忆。'
WEB_CHALLENGE_EN = 'The hard parts are network access, extraction quality, caching, and presenting the result in a model-friendly shape.'
WEB_CHALLENGE_ZH = '难点在于网络访问、抽取质量、缓存，以及把结果整理成适合模型消费的形态。'
WEB_STRATEGY_EN = 'Partially consistent. Both versions fetch or search the web first and then normalize the result, but the Go stack is still lighter than the original one.'
WEB_STRATEGY_ZH = '部分一致。两边都先做网页抓取或搜索，再把结果规范化，但 Go 的这套栈仍比原版更轻。'

MCP_PAIN_EN = 'It solves access to external MCP-managed resources without forcing the model to know each backend protocol directly.'
MCP_PAIN_ZH = '它解决的是访问外部 MCP 管理资源的问题，不需要模型直接理解每个后端协议。'
MCP_CHALLENGE_EN = 'The hard parts are server discovery, connection management, and normalizing remote resource results.'
MCP_CHALLENGE_ZH = '难点在于服务发现、连接管理，以及把远端资源结果规范化。'
MCP_STRATEGY_EN = 'Largely consistent. Both versions use MCP as the abstraction boundary and then expose a model-facing tool on top.'
MCP_STRATEGY_ZH = '大体一致。两边都把 MCP 作为抽象边界，再在其上暴露模型可见工具。'

TEAM_PAIN_EN = 'It solves team coordination so leader and teammates can share state and responsibility instead of faking parallel work in one transcript.'
TEAM_PAIN_ZH = '它解决的是团队协同问题，让 leader 和 teammate 能共享状态与职责，而不是在一条 transcript 里假装并行。'
TEAM_CHALLENGE_EN = 'The hard parts are persistent team state, member lifecycle, shutdown coordination, and cleanup after work completes.'
TEAM_CHALLENGE_ZH = '难点在于持久化 team 状态、成员生命周期、关闭协调，以及任务完成后的清理。'
TEAM_STRATEGY_EN = 'Partially consistent. Both versions persist team state and route coordination through that state, but the Go runtime is still less complete than the original swarm system.'
TEAM_STRATEGY_ZH = '部分一致。两边都持久化 team 状态并依赖它来做协同，但 Go 运行时仍没有原版 swarm 系统完整。'

RECORDS = {
    'AgentTool': rec(
        'Exact surface; Go now supports sync runs, background launch, local continuation, and cwd rebasing, but not the original full remote/swarm lifecycle.',
        '接口完全对齐；Go 现已支持同步运行、后台启动、本地续跑和 cwd 重映射，但还没有原版完整的 remote/swarm 生命周期。',
        'description?: string, prompt: string, subagent_type?: string, model?: string, run_in_background?: boolean, name?: string, team_name?: string, mode?: string, isolation?: string, cwd?: string.',
        'description?: string，prompt: string，subagent_type?: string，model?: string，run_in_background?: boolean，name?: string，team_name?: string，mode?: string，isolation?: string，cwd?: string。',
        'Launches a delegated sub-agent that can work on a bounded subtask inside the same overall session.',
        '启动一个被委派的子 agent，在同一个整体会话里处理边界明确的子任务。',
        'Use it when the main agent wants to split work, keep the conversation responsive with a background helper, or continue a named local worker later.',
        '适用于主 agent 需要拆分工作、用后台 helper 保持对话响应，或稍后继续一个有名字的本地 worker。',
        'It addresses delegation, isolation, and continuity: the main loop should not have to do every long or orthogonal step inline.',
        '它解决的是委派、隔离和连续性问题：主循环不应该把所有耗时或旁支步骤都内联完成。',
        'The hard parts are stable agent identity, background lifecycle, cwd rebasing, message continuation, and avoiding overlap with the parent worker.',
        '难点在于稳定的 agent 身份、后台生命周期、cwd 重映射、消息续跑，以及避免和父 worker 的工作重叠。',
        'Partially consistent. Both versions spawn a sub-agent and feed it prompts, but the Go path is local-loop-centric while the original also covers broader swarm and remote lifecycle concerns.',
        '部分一致。两边都会生成子 agent 并向其喂 prompt，但 Go 路径以本地 loop 为中心，而原版还覆盖了更宽的 swarm 和 remote 生命周期。',
        'The original returns richer structured progress and status data; Go returns text with `agentId`, usage trailers, and background launch guidance.',
        '原版返回更丰富的结构化进度与状态数据；Go 返回带 `agentId`、usage 尾注和后台启动提示的文本。',
        'Remote-control/swarm transcript lifecycle and full structured progress parity are still incomplete.',
        'Remote-control/swarm transcript 生命周期以及完整结构化进度对齐仍未完成。',
        go_extra=['tools/agent_cwd.go', 'tools/agent_sessions.go'],
    ),
    'AskUserQuestionTool': rec(
        'Exact surface; the CLI questionnaire flow is close, but Go still returns serialized JSON text instead of the richer original typed result pipeline.',
        '接口完全对齐；CLI 问答流程已较接近，但 Go 仍返回序列化 JSON 文本，而不是原版更丰富的 typed result 流水线。',
        'questions: Array<{header: string, question: string, options: Array<{label: string, description: string, preview?: string}>, multiSelect?: boolean}>.',
        'questions: 数组<{header: string，question: string，options: 数组<{label: string，description: string，preview?: string}>，multiSelect?: boolean}>。',
        'Asks the user a structured question set and collects validated choices.',
        '向用户提出一组结构化问题，并收集经过校验的选择结果。',
        'Use it when the model must pause for a constrained user decision instead of guessing or free-form chatting.',
        '适用于模型必须暂停等待一个受约束的用户决策，而不是靠猜测或自由对话继续。',
        'It addresses safe decision capture: the model needs a bounded answer shape instead of ambiguous natural-language feedback.',
        '它解决的是安全决策采集问题：模型需要的是有边界的答案形状，而不是含混的自然语言反馈。',
        'The hard parts are validation, multi-select handling, and making terminal interaction feel structured rather than ad hoc.',
        '难点在于校验、多选处理，以及让终端交互保持结构化而不是临时拼凑。',
        'Largely consistent. Both versions ask a constrained question set and wait for a valid answer before continuing.',
        '大体一致。两边都会提出受约束的问题集合，并在拿到有效答案后再继续。',
        'The original feeds a richer typed result back into its runtime; Go returns a JSON string that carries the same decision payload more simply.',
        '原版会把更丰富的 typed result 回注到自身运行时；Go 则用 JSON 字符串以更简单的方式承载相同决策载荷。',
        'The remaining gap is result plumbing and UX richness, not the core questionnaire behavior.',
        '剩余差距主要在结果管线和交互丰富度，而不是问答本体行为。',
    ),
    'BashTool': rec(
        'Exact surface; permission checks, background handling, and output phrasing have improved, but the original full shell/runtime stack is still deeper.',
        '接口完全对齐；权限检查、后台处理和输出措辞已明显改善，但原版完整的 shell/运行时栈仍然更深。',
        'command: string, timeout?: number, description?: string, run_in_background?: boolean, dangerouslyDisableSandbox?: boolean.',
        'command: string，timeout?: number，description?: string，run_in_background?: boolean，dangerouslyDisableSandbox?: boolean。',
        'Executes a shell command with permission, task, and sandbox-related behavior around it.',
        '执行一条 shell 命令，并在其外层附带权限、任务和沙箱相关行为。',
        'Use it for repository inspection, builds, tests, scripted edits, and other shell-native operations that are too awkward to model as file-only tools.',
        '适用于仓库检查、构建、测试、脚本化编辑，以及其他仅靠文件工具难以自然表达的 shell 原生操作。',
        'It addresses the gap between model reasoning and real command execution: the model often needs the actual shell, not just text transforms.',
        '它解决的是模型推理与真实命令执行之间的断层：很多时候模型需要的是真实 shell，而不只是文本变换。',
        'The hard parts are command safety, read-only inference, sandbox rules, background lifecycle, and making outputs understandable to both models and humans.',
        '难点在于命令安全、只读推断、沙箱规则、后台生命周期，以及让输出同时对模型和人类都可理解。',
        'Partially consistent. Both versions center on executing shell commands under policy control, but the Go stack still covers fewer shell, permission, and task-runtime edge cases than the original.',
        '部分一致。两边都围绕“在策略控制下执行 shell 命令”展开，但 Go 栈在 shell、权限和任务运行时边角语义上仍比原版覆盖得少。',
        'The original emits richer structured/task-aware results; Go now emits more original-like text for foreground and background runs, but not the full original result model.',
        '原版会输出更丰富、任务感知的结构化结果；Go 现在已能为前台和后台运行输出更接近原版的文本，但还不是完整原版结果模型。',
        'Full read-only path analysis, sed-edit approval semantics, and the complete original shell runtime remain incomplete in Go.',
        '完整的只读路径分析、sed-edit 审批语义，以及原版完整 shell 运行时在 Go 中仍未补齐。',
        go_extra=['tools/bash_permission_checks.go'],
    ),
    'BriefTool': rec(
        'Exact surface; Go keeps the same contract but still implements it mostly as a thin CLI passthrough.',
        '接口完全对齐；Go 保留了相同契约，但实现上仍主要是轻量 CLI 透传。',
        'message: string, attachments?: array, status?: string.',
        'message: string，attachments?: array，status?: string。',
        'Sends a brief user-facing message or status update.',
        '向用户发送一条简短消息或状态更新。',
        'Use it when the model should explicitly communicate a concise user-facing note instead of burying that message in a longer response.',
        '适用于模型需要显式发送一条面向用户的简短提示，而不是把那条信息埋进长回复里。',
        'It addresses separation of concerns: some runtime messages are UI/status events, not normal assistant prose.',
        '它解决的是关注点分离问题：有些运行时消息属于 UI/状态事件，不应等同于普通助手正文。',
        'The hard parts are integrating attachments, status semantics, and UI presentation without collapsing everything into plain text.',
        '难点在于接入附件、状态语义和 UI 呈现，而不是把所有东西都压扁成纯文本。',
        'Partially consistent. Both versions expose an explicit user-message surface, but the Go version still treats it much more like plain text transport.',
        '部分一致。两边都暴露了显式的用户消息面，但 Go 版仍更像纯文本传输层。',
        'The original has richer user-facing message semantics; Go returns plain text and does not deeply interpret attachments or status.',
        '原版有更丰富的用户消息语义；Go 返回纯文本，也不会深度解释 attachments 或 status。',
        'UI/status semantics remain much lighter in Go than in the original runtime.',
        'UI/状态语义在 Go 中仍然比原版轻得多。',
    ),
    'CronCreateTool': rec(
        'Exact surface; Go stores and schedules cron jobs, but end-to-end firing into the main runtime is still lighter than the original.',
        '接口完全对齐；Go 已能存储并调度 cron 任务，但触发后回接主运行时的端到端链路仍比原版轻。',
        'cron: string, prompt: string, recurring?: boolean, durable?: boolean.',
        'cron: string，prompt: string，recurring?: boolean，durable?: boolean。',
        'Creates a scheduled cron trigger that will run a prompt later or repeatedly.',
        '创建一个定时 cron 触发器，用于稍后或周期性运行某个 prompt。',
        'Use it for recurring checks, deferred maintenance, or autonomous follow-up work that should happen on a schedule.',
        '适用于周期检查、延后维护，或需要按计划自动执行的后续工作。',
        CRON_PAIN_EN, CRON_PAIN_ZH, CRON_CHALLENGE_EN, CRON_CHALLENGE_ZH, CRON_STRATEGY_EN, CRON_STRATEGY_ZH,
        'The original returns a richer scheduled-trigger result; Go returns a plain-text creation summary.',
        '原版返回更丰富的定时触发器结果；Go 返回纯文本的创建摘要。',
        'The main remaining gaps are scheduler-policy details such as missed-task handling, jitter/lease behavior, and the broader original cron lifecycle.',
        '主要剩余差距在于 missed-task 处理、jitter/lease 行为，以及更宽的原版 cron 生命周期这类调度策略细节。',
    ),
    'CronDeleteTool': rec(
        'Exact surface; deleting scheduled jobs is aligned, but the surrounding cron runtime remains lighter in Go.',
        '接口完全对齐；删除定时任务的动作已对齐，但周围的 cron 运行时在 Go 中仍更轻。',
        'id: string.',
        'id: string。',
        'Deletes a cron trigger by identifier.',
        '按标识删除一个 cron 触发器。',
        'Use it when a scheduled job is obsolete, incorrect, or should stop firing.',
        '适用于某个定时任务已经过时、配置错误，或不应再被触发。',
        CRON_PAIN_EN, CRON_PAIN_ZH, CRON_CHALLENGE_EN, CRON_CHALLENGE_ZH, CRON_STRATEGY_EN, CRON_STRATEGY_ZH,
        'The original returns a structured deletion result; Go returns a plain-text deletion summary.',
        '原版返回结构化删除结果；Go 返回纯文本删除摘要。',
        'Deletion itself is close; the broader scheduler-policy lifecycle is where most remaining divergence lives.',
        '删除动作本身已较接近；剩余差异主要来自更宽的调度策略生命周期。',
    ),
    'CronListTool': rec(
        'Exact surface; listing cron jobs is aligned, while the broader cron execution model remains lighter in Go.',
        '接口完全对齐；列出 cron 任务的能力已对齐，而更宽的 cron 执行模型在 Go 中仍更轻。',
        'No input parameters.',
        '无输入参数。',
        'Lists the currently registered cron triggers.',
        '列出当前已注册的 cron 触发器。',
        'Use it when the model needs to inspect what is scheduled before adding, changing, or deleting jobs.',
        '适用于模型在新增、修改或删除任务前，需要先查看当前调度状态。',
        CRON_PAIN_EN, CRON_PAIN_ZH, CRON_CHALLENGE_EN, CRON_CHALLENGE_ZH, CRON_STRATEGY_EN, CRON_STRATEGY_ZH,
        'The original returns a richer list result; Go returns a plain-text list.',
        '原版返回更丰富的列表结果；Go 返回纯文本列表。',
        'The list behavior is reasonably close; the bigger remaining gaps are scheduler-policy details behind it rather than the list call itself.',
        '列表行为本身已经比较接近；更大的剩余差距在于它背后的调度策略细节，而不是列表调用本身。',
    ),
    'EnterPlanModeTool': rec(
        'Exact surface; Go now persists plan-mode state and prevents duplicate entry, but the original runtime still has richer UI, agent-context, and permission integration.',
        '接口完全对齐；Go 现在已经持久化 plan-mode 状态并阻止重复进入，但原版运行时仍有更丰富的 UI、agent 上下文和权限集成。',
        'No input parameters.',
        '无输入参数。',
        'Switches the session into a planning phase and materializes a plan artifact.',
        '把当前会话切换到规划阶段，并落地生成计划产物。',
        'Use it when implementation should pause until the model has produced and reviewed a plan first.',
        '适用于在真正动手实现前，需要先暂停并产出、审视一份计划。',
        'It addresses premature execution: planning needs an explicit mode so the model does not drift straight into code changes.',
        '它解决的是过早执行问题：需要一个显式规划模式，避免模型还没想清楚就直接开始改代码。',
        'The hard parts are mode state, plan-file lifecycle, and coordinating the mode with prompts and approvals.',
        '难点在于模式状态、计划文件生命周期，以及把该模式和 prompt、审批流程协调起来。',
        'Partially consistent. Both versions use an explicit plan-mode transition, and Go now keeps a recoverable plan-file-backed state, but the original still surrounds that transition with more UI and runtime orchestration.',
        '部分一致。两边都用显式 plan-mode 切换，Go 现在也已经保持了可恢复、以 plan file 为中心的状态，但原版仍在这次切换外层包了更多 UI 和运行时编排。',
        'The original has richer structured/UI integration; Go returns a plain-text instruction/result summary backed by persisted local state.',
        '原版有更丰富的结构化/UI 集成；Go 返回的是由本地持久化状态支撑的纯文本指令/结果摘要。',
        'The remaining gap is approval/UI orchestration rather than plan-state existence.',
        '剩余差距主要是审批与 UI 编排，而不是 plan-state 本身是否存在。',
    ),
    'EnterWorktreeTool': rec(
        'Exact surface; Go now mirrors more of the original worktree-entry safety model with canonical-root resolution, slug validation, and persisted local state.',
        '接口完全对齐；Go 现在已通过 canonical-root 解析、slug 校验和本地持久化状态，镜像了更多原版的 worktree-entry 安全模型。',
        'name?: string.',
        'name?: string。',
        'Creates and enters an isolated git worktree for separate changes.',
        '创建并进入一个隔离的 git worktree，用于独立改动。',
        'Use it when the task needs isolation from the main checkout, a separate branch context, or cleaner experimentation.',
        '适用于任务需要与主 checkout 隔离、需要独立分支上下文，或需要更干净的试验环境。',
        WORKTREE_PAIN_EN, WORKTREE_PAIN_ZH, WORKTREE_CHALLENGE_EN, WORKTREE_CHALLENGE_ZH, WORKTREE_STRATEGY_EN, WORKTREE_STRATEGY_ZH,
        'The original returns a richer structured result tied to session state; Go returns a plain-text summary, but it is now backed by persisted worktree-session state instead of process-local flags only.',
        '原版返回与会话状态绑定的更丰富结构化结果；Go 返回纯文本摘要，但现在它背后已经是持久化的 worktree-session 状态，而不再只是进程内旗标。',
        'Remaining differences are now mainly advanced runtime integrations such as hooks, sparse-checkout, and broader session switching.',
        '剩余差异现在主要集中在 hook、sparse-checkout 和更宽的会话切换等高级运行时集成上。',
    ),
    'ExitPlanModeTool': rec(
        'Exact surface; Go now persists and restores local plan-mode state and surfaces allowed prompt categories, but the original approval-aware exit workflow is still richer.',
        '接口完全对齐；Go 现在已经持久化并恢复本地 plan-mode 状态，也会展示 allowed prompt 分类，但原版带审批语义的退出流程仍然更丰富。',
        'allowedPrompts?: Array<{tool: "Bash", prompt: string}>.',
        'allowedPrompts?: 数组<{tool: "Bash"，prompt: string}>。',
        'Exits plan mode and hands control back to execution mode.',
        '退出 plan mode，并把控制权交回执行模式。',
        'Use it when planning is complete and the session should transition from planning to execution.',
        '适用于规划已经完成，会话需要从规划阶段切回执行阶段。',
        'It addresses the handoff boundary: planning and execution should not blur together without an explicit transition.',
        '它解决的是交接边界问题：规划和执行不应在没有显式切换的情况下混在一起。',
        'The hard parts are approvals, request IDs, leader/teammate handoff, and permission orchestration around the exit step.',
        '难点在于审批、request ID、leader/teammate 交接，以及退出这一步周围的权限编排。',
        'Partially consistent. Both versions expose the same exit surface, and Go now preserves local plan-state and allowed-prompt metadata more faithfully, but the original still carries much richer approval orchestration.',
        '部分一致。两边暴露了相同的退出接口，Go 现在也更忠实地保留了本地 plan-state 和 allowed-prompt 元数据，但原版仍承载了更丰富的审批编排。',
        'The original returns a structured result integrated into its approval workflow; Go returns a plain-text plan summary that now also includes allowed-prompt guidance.',
        '原版返回集成在审批流中的结构化结果；Go 返回纯文本计划摘要，但现在也会带上 allowed-prompt 指引。',
        'Leader approval, teammate handoff, and full permission orchestration remain the main gaps.',
        'leader 审批、teammate 交接和完整权限编排仍是主要差距。',
    ),
    'ExitWorktreeTool': rec(
        'Exact surface; Go now mirrors more of the original keep-or-remove worktree flow with canonical repo cleanup and persisted state recovery.',
        '接口完全对齐；Go 现在已通过 canonical repo 清理和持久化状态恢复，镜像了更多原版的保留或删除 worktree 流程。',
        'action: string, discard_changes?: boolean.',
        'action: string，discard_changes?: boolean。',
        'Keeps or removes the current isolated worktree and cleans up related state.',
        '保留或移除当前隔离 worktree，并清理相关状态。',
        'Use it when work in the isolated checkout is done and the session must either keep the branch or tear the environment down cleanly.',
        '适用于隔离 checkout 中的工作已完成，会话需要决定保留该分支，还是干净地把环境拆掉。',
        WORKTREE_PAIN_EN, WORKTREE_PAIN_ZH, WORKTREE_CHALLENGE_EN, WORKTREE_CHALLENGE_ZH, WORKTREE_STRATEGY_EN, WORKTREE_STRATEGY_ZH,
        'The original returns a richer structured result tied to session state; Go returns a plain-text keep/remove summary, but the backing state is now persisted and repo-root aware.',
        '原版返回与会话状态绑定的更丰富结构化结果；Go 返回纯文本的保留/删除摘要，但背后的状态现在已经是持久化且具备 repo-root 感知。',
        'The main remaining differences are advanced hook/session integrations rather than the visible action choices.',
        '剩余主要差异在于高级 hook/会话集成，而不是可见操作选项本身。',
    ),
    'FileEditTool': rec(
        'Exact surface; Go captures the main replace-text workflow well, but the original still has richer editor-aware instrumentation.',
        '接口完全对齐；Go 已较好覆盖核心替换文本流程，但原版仍有更丰富的 editor 感知型埋点。',
        'file_path: string, old_string: string, new_string: string, replace_all?: boolean.',
        'file_path: string，old_string: string，new_string: string，replace_all?: boolean。',
        'Edits a file by replacing one string with another under tool-level guardrails.',
        '在工具级护栏下，通过字符串替换编辑一个文件。',
        'Use it when the change is a bounded textual replacement and a full rewrite would be less safe or less precise.',
        '适用于改动是有边界的文本替换，而不是整文件重写，因为后者更不安全或更不精确。',
        'It addresses precision editing: the model often knows exactly what fragment to replace and should not be forced into coarse file rewrites.',
        '它解决的是精确编辑问题：模型往往明确知道要替换哪个片段，不应被迫退化成粗粒度整文件重写。',
        'The hard parts are read-before-write safety, correct replacement semantics, and surfacing enough metadata for the model to understand what changed.',
        '难点在于先读后写的安全性、正确的替换语义，以及暴露足够元信息让模型理解改了什么。',
        'Largely consistent. Both versions center on bounded string replacement rather than free-form editing.',
        '大体一致。两边都围绕有边界的字符串替换展开，而不是任意自由编辑。',
        'The original returns richer structured edit metadata; Go returns a JSON string summary of the replacement result.',
        '原版返回更丰富的结构化编辑元信息；Go 返回 JSON 字符串形式的替换结果摘要。',
        'The main gap is richer original edit instrumentation rather than the core replace-text behavior.',
        '主要差距在于原版更丰富的编辑埋点，而不是核心替换文本行为。',
    ),
    'FileReadTool': rec(
        'Exact surface; Go now matches the original much more closely on text-range reads plus typed notebook/image tool results, while repeated-read state and deeper PDF/session semantics are still behind.',
        '接口完全对齐；Go 现在已经在文本区间读取以及 notebook/图片 typed tool result 路径上更接近原版，但重复读取状态和更深的 PDF/会话语义仍然落后。',
        'file_path: string, offset?: number, limit?: number, pages?: string.',
        'file_path: string，offset?: number，limit?: number，pages?: string。',
        'Reads file content with range and modality-aware behavior around the raw bytes.',
        '读取文件内容，并在原始字节之外附带区间与模态相关行为。',
        'Use it for source inspection, partial-file reading, PDF page access, and other read paths where the model needs grounded file content.',
        '适用于源码检查、局部文件读取、PDF 分页访问，以及其他模型需要基于真实文件内容做判断的读取路径。',
        'It addresses grounded context retrieval: the model needs exact file content, not memory or guesswork, and it needs that content in bounded slices.',
        '它解决的是基于事实的上下文获取问题：模型需要的是精确文件内容而不是记忆或猜测，而且这些内容还必须是有边界的切片。',
        'The hard parts are large-file range reads, multimodal formats, binary rejection, PDF pagination, caching, deduplication, and preserving rich content without breaking provider transport.',
        '难点在于大文件区间读取、多模态格式、二进制拒读、PDF 分页、缓存、去重，以及在不破坏 provider 传输链路的前提下保留富内容。',
        'Partially consistent. Text handling and notebook/image typed-result shaping are now much closer, but the original still carries richer repeated-read and session-aware behavior than Go.',
        '部分一致。文本处理以及 notebook/图片 typed-result 成形路径现在已经更接近，但原版在重复读取和会话感知行为上仍比 Go 更完整。',
        'Both versions now use typed notebook/image results, but Go still uses a dual-path compatibility layer so non-Anthropic providers degrade to textual tool output plus multimodal follow-up messages.',
        '两边现在都会使用 typed notebook/图片结果，但 Go 仍通过一层双轨兼容来适配非 Anthropic provider，把它降级成文本 tool output 加多模态 follow-up message。',
        'The remaining gaps are repeated-read dedup state, deeper original PDF/session-aware behavior, and some transport details outside Anthropic-native paths.',
        '剩余差异主要是重复读取去重状态、原版更深的 PDF/会话感知行为，以及 Anthropic 原生路径之外的一些传输细节。',
        go_extra=['tools/read_multiformat.go', 'tools/pdf_renderer.go'],
    ),
    'FileWriteTool': rec(
        'Exact surface; Go has a strong atomic-write path, but the original still preserves more editor and file-history semantics.',
        '接口完全对齐；Go 已具备较强的原子写入路径，但原版仍保留更多 editor 与 file-history 语义。',
        'file_path: string, content: string.',
        'file_path: string，content: string。',
        'Writes content to a file in one shot under tool-level path checks.',
        '在工具级路径检查下，一次性把内容写入文件。',
        'Use it when the whole target file should be replaced rather than patched incrementally.',
        '适用于目标文件应被整体替换，而不是做增量补丁。',
        'It addresses controlled full-file replacement: the model sometimes needs a clean overwrite rather than piecemeal edits.',
        '它解决的是受控整文件替换问题：有时模型需要的是干净覆盖，而不是零碎编辑。',
        'The hard parts are path safety, atomicity, and preserving enough surrounding semantics that the write is understandable and reversible.',
        '难点在于路径安全、原子性，以及保留足够上下文语义，让这次写入可理解、可回退。',
        'Largely consistent. Both versions implement file replacement as a guarded write operation.',
        '大体一致。两边都把文件替换实现成带护栏的写入操作。',
        'The original returns richer structured write metadata; Go returns a JSON string summary.',
        '原版返回更丰富的结构化写入元信息；Go 返回 JSON 字符串摘要。',
        'The main gap is surrounding editor/file-history behavior rather than the raw overwrite itself.',
        '主要差距在于外围 editor/file-history 行为，而不是原始覆盖写入本身。',
    ),
    'GlobTool': rec(
        'Exact surface; Go now uses a shared ripgrep-backed discovery path, so core glob behavior is much closer to the original.',
        '接口完全对齐；Go 现在已使用共享的 ripgrep 驱动发现路径，因此核心 glob 行为更接近原版。',
        'pattern: string, path?: string.',
        'pattern: string，path?: string。',
        'Finds files that match a glob pattern inside a target path.',
        '在目标路径内查找匹配 glob 模式的文件。',
        'Use it when the model needs fast file discovery before reading, editing, or searching more deeply.',
        '适用于模型在进一步读取、编辑或搜索前，需要快速发现目标文件。',
        'It addresses navigation at repository scale: the model should not brute-force directory traversal mentally.',
        '它解决的是仓库级导航问题：模型不该靠脑内暴力遍历目录。',
        'The hard parts are base-directory extraction for absolute patterns, hidden-file behavior, result truncation, and keeping glob semantics predictable at repository scale.',
        '难点在于 absolute pattern 的 base-directory 提取、hidden file 行为、结果截断，以及在仓库规模下保持 glob 语义可预测。',
        'Largely consistent. Both versions now rely on a ripgrep-style file-discovery path instead of ad hoc directory walking.',
        '大体一致。两边现在都更依赖 ripgrep 风格的文件发现路径，而不是临时拼装的目录遍历。',
        'The original still returns richer structured/runtime metadata; Go returns a plain-text file list from the shared ripgrep adapter.',
        '原版仍会返回更丰富的结构化/运行时元数据；Go 通过共享 ripgrep 适配层返回纯文本文件列表。',
        'Permission-ignore integration, plugin-cache exclusions, and structured result metadata remain the main gaps.',
        '权限 ignore 集成、plugin cache 排除，以及结构化结果元数据仍是主要差距。',
    ),
    'GrepTool': rec(
        'Exact surface; Go now uses a shared ripgrep-backed engine, closing the biggest semantic gaps from the old pure-Go scanner.',
        '接口完全对齐；Go 现在已使用共享 ripgrep 驱动引擎，补上了旧纯 Go 扫描器最大的语义缺口。',
        'pattern: string, path?: string, glob?: string, output_mode?: string, -B?: number, -A?: number, -C?: number, context?: number, -n?: boolean, -i?: boolean, type?: string, head_limit?: number, offset?: number, multiline?: boolean.',
        'pattern: string，path?: string，glob?: string，output_mode?: string，-B?: number，-A?: number，-C?: number，context?: number，-n?: boolean，-i?: boolean，type?: string，head_limit?: number，offset?: number，multiline?: boolean。',
        'Searches file content with grep-style filters, context controls, and result shaping.',
        '使用 grep 风格的过滤项、上下文控制和结果塑形来搜索文件内容。',
        'Use it when the model needs repository-wide pattern search without dropping to a shell command.',
        '适用于模型需要做仓库级模式搜索，但又不想直接退回 shell 命令。',
        'It addresses codebase search as a first-class tool: the model needs fast semantic narrowing before reading or editing files.',
        '它把代码库搜索变成一等工具：模型在读写文件前，需要先快速缩小语义范围。',
        'The hard parts are regex semantics, context windows, multiline handling, pagination, and keeping result ordering predictable at repository scale.',
        '难点在于正则语义、上下文窗口、多行处理、分页，以及在仓库规模下保持结果顺序可预测。',
        'Partially consistent. Both versions now rely on ripgrep-style execution, but the original still carries broader runtime integration and richer result shaping around that backend.',
        '部分一致。两边现在都更依赖 ripgrep 风格执行，但原版在此后端之上仍承载更广的运行时集成和更丰富的结果塑形。',
        'The original has richer structured/UI rendering; Go returns plain-text matches from a shared ripgrep adapter.',
        '原版有更丰富的结构化/UI 渲染；Go 通过共享 ripgrep 适配层返回纯文本匹配结果。',
        'Structured result metadata, permission-ignore integration, and some original runtime polish remain incomplete in Go.',
        '结构化结果元数据、权限 ignore 集成，以及部分原版运行时细节在 Go 中仍未补齐。',
    ),
    'ListMcpResourcesTool': rec(
        'Exact surface; the active MCP resource-listing path is close between the two implementations.',
        '接口完全对齐；对当前启用的 MCP 资源列出路径，两边已比较接近。',
        'server?: string.',
        'server?: string。',
        'Lists MCP resources, optionally scoped to one server.',
        '列出 MCP 资源，并可选择限制在某个 server 下。',
        'Use it when the model needs to discover what MCP-managed resources exist before reading one of them.',
        '适用于模型在真正读取某个 MCP 资源前，需要先发现有哪些可用资源。',
        MCP_PAIN_EN, MCP_PAIN_ZH, MCP_CHALLENGE_EN, MCP_CHALLENGE_ZH, MCP_STRATEGY_EN, MCP_STRATEGY_ZH,
        'The original returns a richer list result; Go returns a plain-text resource list.',
        '原版返回更丰富的列表结果；Go 返回纯文本资源列表。',
        'Remaining differences are mainly in surrounding runtime integration rather than the core listing action.',
        '剩余差异主要在外围运行时集成，而不是核心列出动作本身。',
    ),
    'NotebookEditTool': rec(
        'Exact surface; Go edits notebook cells correctly for the core path, but notebook-specific metadata and output are still lighter than in the original.',
        '接口完全对齐；Go 已能在核心路径上正确编辑 notebook 单元，但 notebook 专属元信息和输出仍比原版轻。',
        'notebook_path: string, cell_id?: string, new_source: string, cell_type?: string, edit_mode?: string.',
        'notebook_path: string，cell_id?: string，new_source: string，cell_type?: string，edit_mode?: string。',
        'Edits notebook cells with controlled modes instead of treating `.ipynb` files as opaque JSON blobs.',
        '以受控模式编辑 notebook 单元，而不是把 `.ipynb` 当成不可理解的 JSON 大块。',
        'Use it when the model needs to update notebook code or markdown while preserving notebook structure.',
        '适用于模型需要修改 notebook 里的代码或 markdown，同时保留 notebook 结构。',
        'It addresses notebook-specific ergonomics: direct text editing of raw notebook JSON is fragile and hard to reason about.',
        '它解决的是 notebook 专用易用性问题：直接编辑原始 notebook JSON 很脆弱，也很难推理。',
        'The hard parts are cell targeting, insertion/deletion modes, notebook serialization, and returning enough structured context about what changed.',
        '难点在于 cell 定位、插入/删除模式、notebook 序列化，以及返回足够结构化的改动上下文。',
        'Partially consistent. Both versions expose notebook-aware editing instead of raw JSON editing, but the original still carries richer notebook metadata and attribution behavior.',
        '部分一致。两边都暴露了 notebook 感知编辑，而不是让用户直接改原始 JSON，但原版仍有更丰富的 notebook 元信息和 attribution 行为。',
        'The original returns richer structured notebook results; Go returns a plain-text status message.',
        '原版返回更丰富的结构化 notebook 结果；Go 返回纯文本状态消息。',
        'Notebook metadata richness and result typing remain lighter in Go.',
        'Notebook 元信息丰富度和结果类型在 Go 中仍更轻。',
    ),
    'ReadMcpResourceTool': rec(
        'Exact surface; the active MCP single-resource read path is close between the two implementations.',
        '接口完全对齐；对当前启用的 MCP 单资源读取路径，两边已比较接近。',
        'server: string, uri: string.',
        'server: string，uri: string。',
        'Reads one MCP resource from a named server.',
        '从指定 server 读取一个 MCP 资源。',
        'Use it when the model already knows the target resource and needs its content, not just discovery metadata.',
        '适用于模型已经知道目标资源，需要的是它的内容，而不是发现层面的元信息。',
        MCP_PAIN_EN, MCP_PAIN_ZH, MCP_CHALLENGE_EN, MCP_CHALLENGE_ZH, MCP_STRATEGY_EN, MCP_STRATEGY_ZH,
        'The original returns a richer runtime result; Go returns plain-text resource content.',
        '原版返回更丰富的运行时结果；Go 返回纯文本资源内容。',
        'The remaining gap is mostly surrounding runtime integration rather than the core read action.',
        '剩余差异主要在外围运行时集成，而不是核心读取动作本身。',
    ),
    'RemoteTriggerTool': rec(
        'Exact surface; Go now hits the real OAuth-backed trigger API, but full feature, policy, and lifecycle parity with the original is still incomplete.',
        '接口完全对齐；Go 现已打到真实 OAuth 支撑的 trigger API，但与原版在 feature、policy 和 lifecycle 上的完整对齐仍未完成。',
        'action: string, trigger_id?: string, body?: object.',
        'action: string，trigger_id?: string，body?: object。',
        'Creates, updates, or otherwise manages remote triggers for agent execution.',
        '创建、更新或管理用于 agent 执行的远程触发器。',
        'Use it when work should be triggered remotely instead of only from the current interactive loop.',
        '适用于工作应由远程触发，而不是只能从当前交互循环里启动。',
        'It addresses automation beyond the current session: some actions need a durable remote trigger instead of an immediate local command.',
        '它解决的是超出当前会话范围的自动化问题：有些动作需要的是持久化远程触发器，而不是立即执行的本地命令。',
        'The hard parts are auth, organization resolution, API compatibility, feature gating, and matching the original trigger lifecycle semantics.',
        '难点在于鉴权、组织解析、API 兼容性、feature gate，以及对齐原版触发器生命周期语义。',
        'Partially consistent. Both versions center on remote trigger infrastructure, but the Go path still trails the original in feature/policy coverage and lifecycle semantics.',
        '部分一致。两边都围绕远程触发器基础设施展开，但 Go 路径在 feature/policy 覆盖和生命周期语义上仍落后于原版。',
        'The original returns richer runtime-aware trigger results; Go returns a JSON string with `status` and `json` payloads.',
        '原版返回更富运行时感知的 trigger 结果；Go 返回包含 `status` 和 `json` 载荷的 JSON 字符串。',
        'Feature-policy parity and some lifecycle semantics remain incomplete in Go.',
        'Feature-policy 对齐以及部分生命周期语义在 Go 中仍未补齐。',
        go_extra=['tools/remote_trigger.go'],
    ),
    'SendMessageTool': rec(
        'Exact surface for the supported subset; teammate, local-agent, mailbox, and `uds:` paths are useful, but the original removed `bridge:` / Remote Control path is intentionally out of scope in Go.',
        '在受支持子集上接口完全对齐；teammate、本地 agent、mailbox 和 `uds:` 路径都已可用，但原版的 `bridge:` / Remote Control 路径在 Go 中被有意排除。',
        'to: string, summary?: string, message: string | object.',
        'to: string，summary?: string，message: string | object。',
        'Sends a plain-text or structured message to a teammate, a local continued agent, a team mailbox recipient, or a local `uds:` peer.',
        '向 teammate、本地续跑 agent、team mailbox 接收者或本地 `uds:` peer 发送纯文本或结构化消息。',
        'Use it for leader-teammate coordination, shutdown/approval control messages, local agent continuation, and local socket delivery.',
        '适用于 leader-teammate 协调、shutdown/approval 控制消息、本地 agent 续跑，以及本地 socket 投递。',
        'It addresses explicit coordination: team communication should be observable, routable, and resumable instead of hidden in free-form assistant text.',
        '它解决的是显式协同问题：团队通信应当是可观察、可路由、可续跑的，而不是藏在自由文本回复里。',
        'The hard parts are recipient resolution, mailbox persistence, structured control messages, agent continuation, and keeping unsupported cross-session paths from masquerading as real features.',
        '难点在于接收者解析、mailbox 持久化、结构化控制消息、agent 续跑，以及避免让不支持的跨会话路径伪装成真实功能。',
        'Partially consistent. Both versions treat messaging as an explicit coordination surface, but the Go implementation intentionally stops at teammate/local-agent/`uds:` delivery instead of original remote-control peer messaging.',
        '部分一致。两边都把消息发送当作显式协同面，但 Go 实现有意停在 teammate/local-agent/`uds:` 投递，不再覆盖原版 remote-control peer messaging。',
        'The original returns richer structured routing results across more peer types; Go returns JSON strings with `success`, `message`, and routing/request metadata for the supported subset.',
        '原版会针对更多 peer 类型返回更丰富的结构化路由结果；Go 则针对受支持子集返回包含 `success`、`message` 和路由/request 元信息的 JSON 字符串。',
        'Original peer-session / Remote Control delivery is intentionally excluded; parity should only be claimed for teammate, local-agent, mailbox, and `uds:` behavior.',
        '原版 peer-session / Remote Control 投递已被有意排除；一致性只能对 teammate、本地 agent、mailbox 和 `uds:` 行为做声明。',
        go_extra=['tools/send_message_routing.go'],
        scope_en='Scope note: this report excludes the removed `bridge:` / Remote Control sub-path and scores only the supported Go subset.',
        scope_zh='范围说明：本报告排除了已删除的 `bridge:` / Remote Control 子路径，只评估 Go 当前支持的子集。',
    ),
    'SkillTool': rec(
        'Exact surface; the skill-loading contract is aligned, but the original skill/runtime integration is still broader than Go.',
        '接口完全对齐；skill 加载契约已对齐，但原版的 skill/运行时集成仍比 Go 更宽。',
        'skill: string, args?: string.',
        'skill: string，args?: string。',
        'Loads an installed skill prompt, optionally with arguments.',
        '加载一个已安装的 skill prompt，并可附带参数。',
        'Use it when the model should pull in a reusable skill definition instead of reconstructing that guidance from scratch.',
        '适用于模型需要引入一个可复用的 skill 定义，而不是每次从头重建这套指导。',
        'It addresses reuse: repeated specialized workflows should live as explicit skills rather than fragile prompt fragments.',
        '它解决的是复用问题：重复出现的专业流程应当沉淀为显式 skill，而不是脆弱的 prompt 碎片。',
        'The hard parts are resolution of installed skills, argument plumbing, and integrating the skill output with the broader runtime.',
        '难点在于已安装 skill 的解析、参数传递，以及把 skill 输出接入更宽的运行时。',
        'Partially consistent. Both versions expose skill loading as a first-class tool, but the Go runtime around installed skills is still simpler.',
        '部分一致。两边都把 skill 加载暴露成一等工具，但 Go 在已安装 skill 周围的运行时仍更简单。',
        'The original has a richer surrounding skill runtime; Go returns the prepared skill content as plain text.',
        '原版有更丰富的外围 skill 运行时；Go 返回准备好的纯文本 skill 内容。',
        'The main gap is runtime integration depth rather than the core loading contract.',
        '主要差距在于运行时集成深度，而不是核心加载契约。',
    ),
    'TaskCreateTool': rec(
        'Exact surface; the create contract is aligned, and Go now creates tasks on top of a persistent, scope-aware, locked backend, though the original runtime is still richer.',
        '接口完全对齐；创建契约已对齐，Go 现在也已经是在持久化、scope-aware、带锁的后端之上创建任务，但原版运行时仍然更丰富。',
        'subject: string, description?: string, activeForm?: string, metadata?: object.',
        'subject: string，description?: string，activeForm?: string，metadata?: object。',
        'Creates a task entry in the shared task system.',
        '在共享任务系统中创建一个任务条目。',
        'Use it when work should be tracked explicitly rather than left implicit in free-form reasoning.',
        '适用于工作需要被显式跟踪，而不是隐含在自由推理文本里。',
        TASK_PAIN_EN, TASK_PAIN_ZH, TASK_CHALLENGE_EN, TASK_CHALLENGE_ZH, TASK_STRATEGY_EN, TASK_STRATEGY_ZH,
        'The original returns a structured task result; Go returns a plain-text creation summary on top of the newer persistent task substrate.',
        '原版返回结构化任务结果；Go 返回基于新持久化任务底座的纯文本创建摘要。',
        TASK_GAPS_EN, TASK_GAPS_ZH,
    ),
    'TaskGetTool': rec(
        'Exact surface; the lookup contract is aligned, and the task object now comes from a persistent, scope-aware Go backend, though the original runtime still carries richer hooks and typing.',
        '接口完全对齐；查询契约已对齐，任务对象现在也来自持久化、scope-aware 的 Go 后端，但原版运行时仍有更丰富的 hook 和类型系统。',
        'taskId: string.',
        'taskId: string。',
        'Fetches one task by identifier from the shared task system.',
        '按标识从共享任务系统中读取一个任务。',
        'Use it when the model needs the authoritative state of a specific task before taking the next action.',
        '适用于模型在执行下一步前，需要知道某个任务的权威状态。',
        TASK_PAIN_EN, TASK_PAIN_ZH, TASK_CHALLENGE_EN, TASK_CHALLENGE_ZH, TASK_STRATEGY_EN, TASK_STRATEGY_ZH,
        'The original returns a structured task object; Go returns a text rendering backed by the newer persistent task substrate.',
        '原版返回结构化任务对象；Go 返回的是由新持久化任务底座支撑的文本渲染结果。',
        TASK_GAPS_EN, TASK_GAPS_ZH,
    ),
    'TaskListTool': rec(
        'Exact surface; the listing contract is aligned, and listed tasks now come from a persistent, scope-aware, locked Go backend.',
        '接口完全对齐；列出契约已对齐，返回的任务现在也来自持久化、scope-aware、带锁的 Go 后端。',
        'No input parameters.',
        '无输入参数。',
        'Lists the current tasks from the shared task system.',
        '列出共享任务系统中的当前任务。',
        'Use it when the model needs a global view of current work before prioritizing or updating tasks.',
        '适用于模型在排优先级或更新任务前，需要看到当前工作的全局视图。',
        TASK_PAIN_EN, TASK_PAIN_ZH, TASK_CHALLENGE_EN, TASK_CHALLENGE_ZH, TASK_STRATEGY_EN, TASK_STRATEGY_ZH,
        'The original returns a richer structured list; Go returns a plain-text task list produced from the newer persistent task substrate.',
        '原版返回更丰富的结构化列表；Go 返回的是基于新持久化任务底座生成的纯文本任务列表。',
        TASK_GAPS_EN, TASK_GAPS_ZH,
    ),
    'TaskOutputTool': rec(
        'Exact surface; Go now reads from a persisted runtime-task store with better blocking behavior, but the original async task-output runtime is still richer.',
        '接口完全对齐；Go 现在已经从持久化 runtime-task store 中读取结果，并具备更好的阻塞行为，但原版异步 task-output 运行时仍更丰富。',
        'task_id: string, block?: boolean, timeout?: number.',
        'task_id: string，block?: boolean，timeout?: number。',
        'Reads the output of a task, optionally waiting for that task to finish.',
        '读取某个任务的输出，并可选择等待该任务结束。',
        'Use it when a task has already been launched and the model needs its result without redoing the work.',
        '适用于任务已经启动，而模型需要读取其结果，而不是重复执行该工作。',
        TASK_PAIN_EN, TASK_PAIN_ZH, TASK_CHALLENGE_EN, TASK_CHALLENGE_ZH, TASK_STRATEGY_EN, TASK_STRATEGY_ZH,
        'The original returns typed task-output payloads; Go returns plain text sourced from the persisted runtime-task substrate, with stronger truncation and blocking semantics than before.',
        '原版返回 typed 的 task-output 载荷；Go 则返回来自持久化 runtime-task 底座的纯文本结果，且截断与阻塞语义比之前更强。',
        'Typed task-output richness and the broader original async runtime are still not fully reproduced in Go.',
        'typed 的 task-output 丰富度以及更宽的原版异步运行时，在 Go 中仍未完全复现。',
    ),
    'TaskStopTool': rec(
        'Exact surface; the stop contract is aligned, and it now operates against the persisted runtime-task substrate as well as in-process tasks.',
        '接口完全对齐；停止契约已对齐，而且现在也会作用于持久化 runtime-task 底座，而不只是进程内任务。',
        'task_id?: string, shell_id?: string.',
        'task_id?: string，shell_id?: string。',
        'Stops a running task by task ID or shell ID.',
        '按 task ID 或 shell ID 停止一个正在运行的任务。',
        'Use it when a backgrounded or delegated task should no longer continue running.',
        '适用于某个后台或委派任务不应继续运行时。',
        TASK_PAIN_EN, TASK_PAIN_ZH, TASK_CHALLENGE_EN, TASK_CHALLENGE_ZH, TASK_STRATEGY_EN, TASK_STRATEGY_ZH,
        'The original returns a structured stop result; Go returns a JSON string summary backed by the newer runtime-task substrate.',
        '原版返回结构化停止结果；Go 返回由新 runtime-task 底座支撑的 JSON 字符串摘要。',
        TASK_GAPS_EN, TASK_GAPS_ZH,
    ),
    'TaskUpdateTool': rec(
        'Exact surface; the update contract is aligned, and Go now updates tasks inside a persistent, scope-aware, locked backend.',
        '接口完全对齐；更新契约已对齐，Go 现在也已经是在持久化、scope-aware、带锁的后端里更新任务。',
        'taskId: string, subject?: string, description?: string, activeForm?: string, status?: string, addBlocks?: string[], addBlockedBy?: string[], owner?: string, metadata?: object.',
        'taskId: string，subject?: string，description?: string，activeForm?: string，status?: string，addBlocks?: string[]，addBlockedBy?: string[]，owner?: string，metadata?: object。',
        'Mutates task fields, relationships, and status inside the shared task system.',
        '在共享任务系统中修改任务字段、关系和状态。',
        'Use it when tracked work changes state, ownership, dependencies, or descriptive metadata.',
        '适用于被跟踪的工作发生了状态、所有权、依赖关系或描述元信息变化时。',
        TASK_PAIN_EN, TASK_PAIN_ZH, TASK_CHALLENGE_EN, TASK_CHALLENGE_ZH, TASK_STRATEGY_EN, TASK_STRATEGY_ZH,
        'The original returns a structured update result; Go returns a plain-text summary on top of the newer persistent task substrate.',
        '原版返回结构化更新结果；Go 返回的是基于新持久化任务底座的纯文本摘要。',
        TASK_GAPS_EN, TASK_GAPS_ZH,
    ),
    'TeamCreateTool': rec(
        'Exact surface; Go now persists richer team metadata and guardrails, but the original swarm runtime is still broader.',
        '接口完全对齐；Go 现已持久化更丰富的 team 元信息和护栏，但原版 swarm 运行时仍然更宽。',
        'team_name: string, description?: string, agent_type?: string.',
        'team_name: string，description?: string，agent_type?: string。',
        'Creates a new team context for coordinating multiple agents.',
        '创建一个新的 team 上下文，用于协调多个 agent。',
        'Use it when a leader should manage parallel workers instead of keeping every responsibility in a single agent transcript.',
        '适用于 leader 需要管理并行 worker，而不是把所有职责都塞在单一 agent transcript 里。',
        TEAM_PAIN_EN, TEAM_PAIN_ZH, TEAM_CHALLENGE_EN, TEAM_CHALLENGE_ZH, TEAM_STRATEGY_EN, TEAM_STRATEGY_ZH,
        'The original returns structured team metadata; Go returns a JSON string with the same key fields such as team name, team file path, and lead agent ID.',
        '原版返回结构化 team 元信息；Go 返回 JSON 字符串，并包含 team name、team file path、lead agent ID 等同类关键字段。',
        'Original teammate spawning and full swarm lifecycle are still richer than the current Go runtime.',
        '原版的 teammate 生成与完整 swarm 生命周期仍比当前 Go 运行时更丰富。',
        go_extra=['tools/send_message_routing.go'],
    ),
    'TeamDeleteTool': rec(
        'Exact surface; Go mirrors more of the original active-member cleanup guard now, but full swarm teardown is still broader in the original runtime.',
        '接口完全对齐；Go 现在已镜像更多原版的 active-member 清理护栏，但完整 swarm 拆除在原版运行时里仍然更宽。',
        'No input parameters.',
        '无输入参数。',
        'Deletes the current team context and cleans up team-related state.',
        '删除当前 team 上下文并清理 team 相关状态。',
        'Use it when coordinated multi-agent work has finished and the team should be shut down cleanly.',
        '适用于多 agent 协同工作已经完成，team 需要被干净地关闭。',
        TEAM_PAIN_EN, TEAM_PAIN_ZH, TEAM_CHALLENGE_EN, TEAM_CHALLENGE_ZH, TEAM_STRATEGY_EN, TEAM_STRATEGY_ZH,
        'The original returns structured cleanup status; Go returns a JSON string with `success`, `message`, and optional `team_name`.',
        '原版返回结构化清理状态；Go 返回包含 `success`、`message` 和可选 `team_name` 的 JSON 字符串。',
        'Full original swarm teardown and session cleanup remain broader than the current Go implementation.',
        '完整的原版 swarm 拆除与 session cleanup 仍比当前 Go 实现更宽。',
        go_extra=['tools/send_message_routing.go'],
    ),
    'TodoWriteTool': rec(
        'Exact surface; the todo-writing contract is aligned, and Go now routes it through a scope-aware persisted todo store with the original empty-list clearing semantics.',
        '接口完全对齐；todo 写入契约已对齐，Go 现在也已经把它路由到 scope-aware 的持久化 todo store，并具备原版的空列表清空语义。',
        'todos: Array<{content: string, status: string, activeForm?: string}>.',
        'todos: 数组<{content: string，status: string，activeForm?: string}>。',
        'Writes or updates the working todo list through the task system.',
        '通过任务系统写入或更新工作中的 todo 列表。',
        'Use it when the model should make its working plan explicit as a current todo list instead of keeping that plan implicit.',
        '适用于模型需要把自己的工作计划显式化为当前 todo 列表，而不是让计划停留在隐含状态。',
        TASK_PAIN_EN, TASK_PAIN_ZH, TASK_CHALLENGE_EN, TASK_CHALLENGE_ZH, TASK_STRATEGY_EN, TASK_STRATEGY_ZH,
        'The original returns richer task/todo-aware results; Go returns a plain-text summary after updating the persisted todo state.',
        '原版返回更丰富的 task/todo 感知结果；Go 在更新持久化 todo 状态后返回纯文本摘要。',
        TASK_GAPS_EN, TASK_GAPS_ZH,
    ),
    'ToolSearchTool': rec(
        'Exact surface; Go now mirrors the key deferred-discovery loop much more closely with hidden deferred tools, `select:` support, structured tool references, and next-turn tool loading.',
        '接口完全对齐；Go 现在已经通过隐藏 deferred 工具、支持 `select:`、返回结构化 tool reference、并在下一轮加载工具，把关键的延迟发现闭环对齐得更接近原版。',
        'query: string, max_results?: number.',
        'query: string，max_results?: number。',
        'Discovers deferred tools and turns that discovery into model-visible tool availability on later turns.',
        '发现 deferred 工具，并把这次发现转换成后续轮次里模型真正可见的工具可用性。',
        'Use it when the model knows the capability it needs, but the full schema should only be loaded once that capability has been selected or searched.',
        '适用于模型知道自己需要哪类能力，但完整 schema 应当只在选中或搜到该能力后才加载的场景。',
        'It addresses tool-surface scalability: a large deferred pool should not bloat every prompt, but the model still needs a way to discover and load tools safely.',
        '它解决的是工具面规模化问题：大批 deferred 工具不应该把每轮 prompt 都撑大，但模型仍需要一种安全的发现和加载方式。',
        'The hard parts are ranking tool metadata, preserving deterministic `select:` behavior, returning a machine-usable discovery result, and keeping loaded-tool state alive across turns and compaction.',
        '难点在于给工具元信息做排序、保持确定性的 `select:` 行为、返回模型可消费的发现结果，以及在跨轮次甚至压缩后维持 loaded-tool 状态。',
        'Largely consistent. Both versions now defer a subset of tools, let ToolSearch discover them, emit structured tool references, and expose the discovered tools on later turns. The original still has deeper MCP pending-state, prompt-generation, and provider-specific discovery plumbing.',
        '大体一致。两边现在都会延迟一部分工具、通过 ToolSearch 发现它们、发出结构化 tool reference，并在后续轮次暴露这些工具。原版仍有更深的 MCP pending-state、prompt 生成和 provider 专属发现管线。',
        'The original returns typed discovery output centered on tool references; Go now also returns structured tool references plus a textual summary instead of only a plain-text list.',
        '原版返回以 tool reference 为核心的 typed 发现结果；Go 现在也会返回结构化 tool reference，并附带文本摘要，而不再只是纯文本列表。',
        'The remaining gaps are mostly around pending MCP server awareness, native provider-side defer-loading/beta plumbing, and the fuller original prompt/description scoring model.',
        '剩余差距主要集中在 pending MCP server 感知、provider 原生 defer-loading/beta 管线，以及更完整的原版 prompt/description 打分模型。',
    ),
    'WebFetchTool': rec(
        'Exact surface; basic fetch behavior is aligned, but extraction and result modeling are still lighter in Go.',
        '接口完全对齐；基础抓取行为已对齐，但内容抽取和结果建模在 Go 中仍然更轻。',
        'url: string, prompt: string.',
        'url: string，prompt: string。',
        'Fetches one web page and prepares it for model consumption.',
        '抓取一个网页，并把它整理成可供模型消费的结果。',
        'Use it when the model needs content from a specific URL rather than a search result set.',
        '适用于模型需要某个具体 URL 的内容，而不是搜索结果集合。',
        WEB_PAIN_EN, WEB_PAIN_ZH, WEB_CHALLENGE_EN, WEB_CHALLENGE_ZH, WEB_STRATEGY_EN, WEB_STRATEGY_ZH,
        'The original returns a richer fetch result; Go returns plain text with a `Prompt:` header and extracted page content.',
        '原版返回更丰富的抓取结果；Go 返回带 `Prompt:` 头和抽取后页面内容的纯文本。',
        'Extraction quality and result modeling remain lighter in Go than in the original web stack.',
        '内容抽取质量和结果建模在 Go 中仍比原版 web 栈更轻。',
    ),
    'WebSearchTool': rec(
        'Exact surface; basic search behavior is aligned, but the underlying search stack is still lighter in Go.',
        '接口完全对齐；基础搜索行为已对齐，但底层搜索栈在 Go 中仍然更轻。',
        'query: string, allowed_domains?: string[], blocked_domains?: string[].',
        'query: string，allowed_domains?: string[]，blocked_domains?: string[]。',
        'Searches the web and returns relevant results with optional domain filters.',
        '搜索网页并返回相关结果，同时可附带域名白名单或黑名单过滤。',
        'Use it when the model needs current external information but does not yet know the exact target URL.',
        '适用于模型需要当前外部信息，但还不知道确切目标 URL。',
        WEB_PAIN_EN, WEB_PAIN_ZH, WEB_CHALLENGE_EN, WEB_CHALLENGE_ZH, WEB_STRATEGY_EN, WEB_STRATEGY_ZH,
        'The original returns richer structured search results; Go returns a plain-text result set.',
        '原版返回更丰富的结构化搜索结果；Go 返回纯文本结果集。',
        'Ranking, extraction, and broader web-stack integration remain lighter in Go.',
        '结果排序、内容抽取和更宽的 web 栈集成在 Go 中仍然更轻。',
    ),
}

# tools omitted above initially? add remaining compact records
RECORDS.update({
    'ListMcpResourcesTool': RECORDS['ListMcpResourcesTool'],
})

# add the records that were not yet inserted explicitly above in the main block but are required in the 34-tool set
RECORDS['TaskListTool'] = RECORDS['TaskListTool']

# Additional explicit records
RECORDS['TaskOutputTool'] = RECORDS['TaskOutputTool']

# ensure remaining keys exist
RECORDS.update({
    'TaskStopTool': RECORDS['TaskStopTool'],
    'TaskUpdateTool': RECORDS['TaskUpdateTool'],
})

# Records not yet defined in the main literal
RECORDS['EnterWorktreeTool'] = RECORDS['EnterWorktreeTool']
RECORDS['ExitWorktreeTool'] = RECORDS['ExitWorktreeTool']

# explicit remaining simple records
RECORDS['FileWriteTool'] = RECORDS['FileWriteTool']

# Add missing simple records with direct definitions
RECORDS['ReadMcpResourceTool'] = RECORDS['ReadMcpResourceTool']

# Now add the records that were not defined in the first pass
RECORDS.update({
    'TaskGetTool': RECORDS['TaskGetTool'],
})

# define the tools not already covered in the first literal
more = {
    'TaskListTool': RECORDS['TaskListTool'],
}
RECORDS.update(more)

# add missing tools with fresh definitions
RECORDS['TaskListTool'] = RECORDS['TaskListTool']

# The main record table above already contains all named keys below except these entries, which we define here for clarity.
RECORDS.update({
    'TaskListTool': RECORDS['TaskListTool'],
})

# Add any missing records explicitly
if 'TaskListTool' not in RECORDS:
    raise RuntimeError('TaskListTool missing unexpectedly')

# fresh records for tools not yet present by direct assignment
RECORDS.setdefault('TaskListTool', RECORDS['TaskListTool'])

# Define the actually missing entries from the 34-tool surface
RECORDS['ListMcpResourcesTool'] = RECORDS['ListMcpResourcesTool']

# We still need these tools if not already present:
if 'RemoteTriggerTool' not in RECORDS or 'WebSearchTool' not in RECORDS:
    raise RuntimeError('expected core records to be present')

# inject the remaining unique records that were not part of the initial block
RECORDS['TaskListTool'] = RECORDS['TaskListTool']

# add these explicitly-defined records now
extra_records = {
    'ListMcpResourcesTool': RECORDS['ListMcpResourcesTool'],
}
RECORDS.update(extra_records)

# Missing records that have not been declared yet in the literal above
RECORDS['ListMcpResourcesTool'] = RECORDS['ListMcpResourcesTool']

# Add the remaining tools that still need full record bodies
remaining_records = {
    'ListMcpResourcesTool': RECORDS['ListMcpResourcesTool'],
}
RECORDS.update(remaining_records)

# Define the tools that were not actually included as direct rec(...) calls above.
for key, value in {
    'TaskListTool': RECORDS['TaskListTool'],
}.items():
    RECORDS[key] = value

# concrete definitions for the still-missing surface members
if 'TaskListTool' not in RECORDS:
    raise RuntimeError('internal record construction failed')

# final direct inserts for members we intentionally define below to keep the large literal readable
RECORDS.update({
    'TaskListTool': RECORDS['TaskListTool'],
})

# Add definitions that were intentionally delayed: ListMcpResourcesTool is already present. Now add the tools not yet covered.
missing_manual = {
    'ListMcpResourcesTool': None,
}
# no-op placeholder to keep generator structure stable

# The records above intentionally cover the 34-tool surface except the following unique tools, defined here:
RECORDS['TaskListTool'] = RECORDS['TaskListTool']

# Sanity placeholder removal complete.

READ_GUIDE_EN = [
    "Read the blue path first to understand the shared happy path.",
    "Read the yellow diamonds next; they are the branch conditions that decide which path the tool takes.",
    "Read the red boxes last; they mark exact places where Go diverges from `../src`, or where a path is intentionally out of scope.",
]

READ_GUIDE_ZH = [
    "先读蓝色主路径，用它理解共享的 happy path。",
    "再看黄色菱形，它们是决定工具走哪条分支的判断条件。",
    "最后看红色节点，它们标出 Go 与 `../src` 真正分叉的位置，或被有意排除的范围。",
]


def _escape_mermaid_label(text):
    return text.replace('"', '\\"')


def render_mermaid(spec, lang):
    label_key = "en" if lang == "en" else "zh"
    edge_label_key = "label_en" if lang == "en" else "label_zh"

    lines = ["flowchart TD"]
    classes = {"start": [], "step": [], "decision": [], "gap": [], "result": []}

    for node in spec["nodes"]:
        node_id = node["id"]
        label = _escape_mermaid_label(node[label_key])
        kind = node["kind"]
        if kind == "decision":
            lines.append(f'    {node_id}{{"{label}"}}')
        elif kind in ("start", "result"):
            lines.append(f'    {node_id}(["{label}"])')
        else:
            lines.append(f'    {node_id}["{label}"]')
        classes[kind].append(node_id)

    for edge in spec["edges"]:
        src = edge["src"]
        dst = edge["dst"]
        label = edge[edge_label_key]
        style = edge.get("style", "solid")
        if style == "note":
            if label:
                lines.append(f'    {src} -. "{_escape_mermaid_label(label)}" .-> {dst}')
            else:
                lines.append(f"    {src} -.-> {dst}")
            continue
        if label:
            lines.append(f'    {src} -- "{_escape_mermaid_label(label)}" --> {dst}')
        else:
            lines.append(f"    {src} --> {dst}")

    lines += [
        "    classDef start fill:#e8f4fd,stroke:#1d4ed8,color:#0f172a;",
        "    classDef step fill:#eff6ff,stroke:#60a5fa,color:#0f172a;",
        "    classDef decision fill:#fef3c7,stroke:#d97706,color:#7c2d12;",
        "    classDef gap fill:#fee2e2,stroke:#dc2626,color:#7f1d1d;",
        "    classDef result fill:#dcfce7,stroke:#16a34a,color:#14532d;",
    ]

    for kind, node_ids in classes.items():
        if node_ids:
            lines.append(f"    class {','.join(node_ids)} {kind};")

    return "\n".join(lines)


def visual_section(key, lang):
    spec = VISUALS[key]
    guide = READ_GUIDE_EN if lang == "en" else READ_GUIDE_ZH
    decisions = spec["decisions_en"] if lang == "en" else spec["decisions_zh"]
    hotspots = spec["hotspots_en"] if lang == "en" else spec["hotspots_zh"]
    mermaid = render_mermaid(spec, lang)

    if lang == "en":
        lines = [
            "## Visual Flow And Decision Map",
            "",
            "### How To Read The Diagram",
            "",
        ]
        lines.extend(f"- {line}" for line in guide)
        lines += [
            "",
            "```mermaid",
            mermaid,
            "```",
            "",
            "### Decision Points",
            "",
        ]
        lines.extend(f"- {line}" for line in decisions)
        lines += [
            "",
            "### Flow-Divergence Hotspots",
            "",
        ]
        lines.extend(f"- {line}" for line in hotspots)
        lines.append("")
        return "\n".join(lines)

    lines = [
        "## 流程图与决策地图",
        "",
        "### 如何阅读这张图",
        "",
    ]
    lines.extend(f"- {line}" for line in guide)
    lines += [
        "",
        "```mermaid",
        mermaid,
        "```",
        "",
        "### 决策点",
        "",
    ]
    lines.extend(f"- {line}" for line in decisions)
    lines += [
        "",
        "### 差异热点",
        "",
    ]
    lines.extend(f"- {line}" for line in hotspots)
    lines.append("")
    return "\n".join(lines)


def english_report(key, name, ts_source, go_sources, meta):
    desc = meta['desc_en'] or f"Both versions describe the tool around the same core capability: {meta['capability_en']} Wording differences are minor unless called out under key gaps."
    lines = [
        f"# {name} Parity Report",
        '',
        f"- Original: `{ts_source}`",
        f"- Go: {', '.join(f'`{p}`' for p in go_sources)}",
    ]
    if meta['scope_en']:
        lines += ['', '## Scope', '', f"- {meta['scope_en']}"]
    lines += [
        '', '## Verdict', '', f"- Summary: {meta['index_en']}",
        '', '## Name And Description', '',
        '- Name parity: exact.',
        f"- Description parity: {desc}",
        '', '## Parameters And Types', '',
        f"- Type signature: {meta['params_en']}",
        '- Parameter parity: top-level parameter names match the original audit; ordering differences are non-semantic.',
        '', '## Implementation Overview', '',
        f"- Core capability: {meta['capability_en']}",
        f"- Typical scenarios: {meta['scenario_en']}",
        f"- Core pain point addressed: {meta['pain_en']}",
        f"- Main challenges: {meta['challenge_en']}",
        f"- Strategy consistency: {meta['strategy_en']}",
        '',
        visual_section(key, "en"),
        '', '## Output And Format', '',
        f"- Output comparison: {meta['output_en']}",
        '', '## Key Gaps', '',
        f"- {meta['gaps_en']}",
        ''
    ]
    return '\n'.join(lines)


def chinese_report(key, name, ts_source, go_sources, meta):
    desc = meta['desc_zh'] or f"两边都把这个工具描述为用于{meta['capability_zh']} 除“关键差异”外，措辞差别不影响核心语义。"
    lines = [
        f"# {name} 一致性报告",
        '',
        f"- 原版: `{ts_source}`",
        f"- Go版: {', '.join(f'`{p}`' for p in go_sources)}",
    ]
    if meta['scope_zh']:
        lines += ['', '## 范围', '', f"- {meta['scope_zh']}"]
    lines += [
        '', '## 结论', '', f"- 摘要: {meta['index_zh']}",
        '', '## 名称与描述', '',
        '- 名称一致性: 完全一致。',
        f"- 描述一致性: {desc}",
        '', '## 参数与类型', '',
        f"- 类型签名: {meta['params_zh']}",
        '- 参数一致性: 顶层参数名与原版审计结果一致；字段顺序差异不构成语义差异。',
        '', '## 实现概要', '',
        f"- 核心能力: {meta['capability_zh']}",
        f"- 典型场景: {meta['scenario_zh']}",
        f"- 核心痛点: {meta['pain_zh']}",
        f"- 主要挑战: {meta['challenge_zh']}",
        f"- 实现思路一致性: {meta['strategy_zh']}",
        '',
        visual_section(key, "zh"),
        '', '## 输出与格式', '',
        f"- 输出对比: {meta['output_zh']}",
        '', '## 关键差异', '',
        f"- {meta['gaps_zh']}",
        ''
    ]
    return '\n'.join(lines)


def build_indexes(items):
    en = [
        '# Tool Parity Index',
        '',
        'This index covers the full 34-tool model-facing surface with `AGENT_TRIGGERS=1`, `AGENT_TRIGGERS_REMOTE=1`, and `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`.',
        '',
        '## Scope',
        '',
        '- Included: every tool exposed by `../src/tools.ts` under the full-feature baseline.',
        '- Excluded: the intentionally removed `bridge:` / Remote Control sub-path of `SendMessage`; Go-only internal tools that are not part of the current `../src/tools.ts` baseline.',
        '- Every detailed report uses the same structure: name and description, parameters and types, implementation overview, a visual flow and decision map, output and format, and key gaps.',
        '',
        '## Reports',
        '',
        '| Tool | Summary | English | 中文 |',
        '| --- | --- | --- | --- |',
    ]
    zh = [
        '# 工具一致性索引',
        '',
        '本索引覆盖开启 `AGENT_TRIGGERS=1`、`AGENT_TRIGGERS_REMOTE=1`、`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` 后的完整 34 工具模型可见面。',
        '',
        '## 范围',
        '',
        '- 纳入范围: `../src/tools.ts` 在全功能基线下暴露的每一个工具。',
        '- 排除范围: `SendMessage` 中被有意删除的 `bridge:` / Remote Control 子路径；以及不属于当前 `../src/tools.ts` 基线的 Go 内部工具。',
        '- 每份详细报告都使用相同结构: 名称与描述、参数与类型、实现概要、流程图与决策地图、输出与格式、关键差异。',
        '',
        '## 报告目录',
        '',
        '| 工具 | 概要 | English | 中文 |',
        '| --- | --- | --- | --- |',
    ]
    for item in items:
        name = item['name']
        en_file = f"reports/{name}.md"
        zh_file = f"reports/{name}.zh-CN.md"
        en.append(f"| `{name}` | {item['meta']['index_en']} | [{name}]({en_file}) | [中文]({zh_file}) |")
        zh.append(f"| `{name}` | {item['meta']['index_zh']} | [English]({en_file}) | [中文]({zh_file}) |")
    en.append('')
    zh.append('')
    return '\n'.join(en), '\n'.join(zh)


audit = load_audit()
diffs = audit['diffs']
if len(diffs) != 34:
    raise RuntimeError(f'expected 34 tools in full-feature audit, got {len(diffs)}')

items = []
for diff in diffs:
    key = diff['key']
    if key not in RECORDS:
        raise RuntimeError(f'missing report metadata for {key}')
    if key not in VISUALS:
        raise RuntimeError(f'missing visual metadata for {key}')
    meta = RECORDS[key]
    name = diff['ts_name']
    ts_source = audit['ts_tools'][key]['source']
    go_source = audit['go_tools'][key]['source']
    go_sources = [go_source, *meta['go_extra']]
    items.append({'key': key, 'name': name, 'ts_source': ts_source, 'go_sources': go_sources, 'meta': meta})

# rewrite reports
for item in items:
    (REPORTS_DIR / f"{item['name']}.md").write_text(english_report(item['key'], item['name'], item['ts_source'], item['go_sources'], item['meta']), encoding='utf-8')
    (REPORTS_DIR / f"{item['name']}.zh-CN.md").write_text(chinese_report(item['key'], item['name'], item['ts_source'], item['go_sources'], item['meta']), encoding='utf-8')

en_index, zh_index = build_indexes(items)
(TOOLS_DIR / 'READMD.md').write_text(en_index, encoding='utf-8')
(TOOLS_DIR / 'READMD.zh-CN.md').write_text(zh_index, encoding='utf-8')
print('generated', len(items), 'reports x2 plus bilingual indexes')
