# LUBAN Code

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [Deutsch](README.de.md)

LUBAN Code 是一款用 Go 编写的终端编程 Agent，重点处理耗时长、上下文大的仓库任务。它不会为了省 Token 改写会话原始记录，也不会因为换了代理地址，就悄悄改掉 Provider 的原生协议。

> 当前是 `v0.1.0` 源码预览版。请从源码构建；尚未发布二进制包和包管理器安装方式。

![LUBAN Code TUI 正在使用 OpenAI gpt-5-6 模型运行](docs/assets/screenshots/luban-tui.png)

_2026-08-12 取自当前 Windows 源码构建的真实 TUI。画面没有显示 API key 或 endpoint 地址。_

## 真正不同的地方

### 压缩模型看到的内容，不动原始证据

长会话经常逼人二选一：要么把所有旧工具结果继续塞进提示词，要么用一段难以核对的摘要替换历史。LUBAN 保留完整 transcript。在经过审查的有限生产范围内，它只会把 Provider 视图中的旧 `Inspect` 结果换成确定性投影，路径、行号范围、分页、digest 和 proof 都留着。

投影之前，运行时会估算完整请求，并把冷缓存和后续恢复成本算进去。价格未知、usage 不完整、证据失败或节省不够，全部不投影。投影异常会回滚；同一会话连续出现 3 次异常，就触发熔断。

目前的默认生产范围很窄：仅 `openai/gpt-5.6-sol*`、`deepseek/deepseek-v4-flash*`，工具仅限 `Inspect`。实现和限制写在[设计文档](docs/design/progressive-context-compaction.md)与[80k 配对实跑记录](benchmark-results/progressive-context-compaction-v7-80k-2026-08-10/README.md)里。

### 代理只改去向，不改 Provider 身份

`BaseURL` 只是传输配置。OpenAI、DeepSeek、Anthropic、Vertex、Bedrock 即使走自定义地址，认证、缓存控制、Responses/Chat 语义和 Provider 专属字段仍由原生身份决定，不会被统一降格成泛化的 OpenAI-compatible 方言。

只有明确选择 compatible Provider 时，才会自动协商 Responses 到 Chat。当前实现只把 `404`、`405`、`501` 当成 endpoint 不存在；认证、限流和 schema 问题会作为错误返回，不会触发协议降级。

### 模型工具少，运行层并不简陋

默认生产配置中，模型面对的编程内核是 `Inspect`、`ApplyPatch`、`Run`；开启 `ContextUpdate` shadow 路径后，还会加入这个内部工具。外围运行时负责会话恢复、并行子 Agent、可选 Git worktree、权限确认、生命周期 hooks、MCP 连接，以及 NDJSON/Go SDK 边界。子 Agent 启动时会拿到一份不可变的权限快照；父会话之后放宽权限，不会顺带扩大已运行子 Agent 的权限。

终端 UI 会显示上下文、缓存、费用、压缩和子 Agent 状态。`--screen-reader` 提供不控制光标、不捕获鼠标、没有动画的线性模式。运行时文案实际覆盖英、中、德、日、韩、俄六种语言，可用 `Ctrl+L` 或 `/language` 切换。

## 数字有出处，也有边界

在冻结的 15 题本地样本中，入选的 LUBAN 运行比入选的 Codex 运行耗时更短，Token 和模型调用也更少：

| 观测合计 | LUBAN | Codex | 差值 |
| --- | ---: | ---: | ---: |
| 耗时 | 4,020.6 秒 | 5,644.5 秒 | -28.8% |
| Token | 6,857,490 | 17,889,019 | -61.7% |
| LLM 调用 | 245 | 354 | -30.8% |
| 生成 patch | 15/15 | 15/15 | 相同 |

这组数不是“总体击败”的结论。只有最初 5 题有官方判分，双方都是 3/5；新增 10 题没有判分。入选运行是在优化后筛选的，模型也没有固定 seed。要做更广的判断，请先看[完整 HTML 报告](benchmark-results/agentic-2026-07-27/representative15-report.html)、[入选机器数据](benchmark-results/agentic-2026-07-27/raw/candidates/selected-15task-20260731.json)和[测评协议](benchmark/agentic/README.md)。

渐进压缩实验同样不能泛化。一次 80k 配对运行中，冻结评测两边都是 `2/2 + 455/455`；total token 从 `1,362,070` 降到 `444,419`，估算费用从 `$5.207999` 降到 `$1.004185`。由于没有固定采样 seed，两条轨迹在首次投影前就已经分叉。这些是两条真实轨迹的测量数据和按固定费率得到的估算，不是投影效果的因果均值。

## 从源码构建

需要 Git，以及 [`go.mod`](go.mod) 声明的 Go 版本，当前为 `1.26.1`。`Run` 的 shell-form 步骤会调用 Bash；Windows 用户需要把 Git Bash、WSL Bash 或其他 `bash` 放进 `PATH`。

macOS 或 Linux：

```sh
git clone https://github.com/agent-dance/luban.git
cd luban
go build -o luban-code ./cmd/luban-code
./luban-code --version
```

Windows PowerShell：

```powershell
git clone https://github.com/agent-dance/luban.git
Set-Location luban
go build -o .\luban-code.exe .\cmd\luban-code
.\luban-code.exe --version
```

模块目前带有本地 `replace`，所以暂不支持 `go install github.com/agent-dance/luban/cmd/luban-code@latest`。

## 连接 Provider 并开始运行

可以用环境变量配置凭据。电脑上有多组凭据时，请明确选择 Provider：

```sh
export PROVIDER=openai
export OPENAI_API_KEY="..."
./luban-code
```

```powershell
$env:PROVIDER = "openai"
$env:OPENAI_API_KEY = "..."
.\luban-code.exe
```

DeepSeek 使用 `PROVIDER=deepseek` 和 `DEEPSEEK_API_KEY`，它也是默认 Provider。Ollama 默认连接 `http://localhost:11434/v1`，默认模型为 `llama3.1`。也可以先启动 TUI，再按 `Alt+P` 选择 Provider、模型和可用的认证方式。

不打开 TUI，直接执行一次任务：

```sh
./luban-code -p "审查当前仓库，报告风险最高的问题"
```

![LUBAN Code v0.1.0 实际单次运行并返回 LUBAN READY](docs/assets/screenshots/luban-live-run.png)

_第二条命令通过本机已配置的 OpenAI endpoint 发出了真实请求，并以退出码 0 结束；截图中的本地提示符路径已经脱敏。这只能证明本次运行成功，不是 Provider 兼容性或性能测评。_

进入 TUI 后可以用 `/init` 创建 `LUBAN.md` 和项目设置；已有文件不会被覆盖。这个命令不负责配置凭据。

## 使用前要知道的边界

- Linux 的系统级沙箱依赖 Bubblewrap，macOS 使用 `sandbox-exec`。Windows 目前没有系统级沙箱后端；没有可信后端时，`--force-sandbox-tools` 会直接失败。
- Agent Teams 仍是实验功能，需要主动开启。并行子 Agent 和 worktree 隔离不等于远程分布式 swarm。
- Provider 已注册、协议测试通过，不代表每个模型或第三方网关都经过线上认证。
- 本地凭据以明文 JSON 保存。类 Unix 系统写入时使用 `0600`；Windows 当前没有等价的 ACL 保证。它不是加密保险库，也不是操作系统钥匙串。
- 只有使用 Node.js 实现的 MCP server 才需要 Node.js；核心 CLI 不需要。
- 仓库根目录暂时没有许可证。Owner 正式发布许可证前，适用默认版权规则。

## 证据入口

- [渐进上下文设计](docs/design/progressive-context-compaction.md)
- [渐进压缩上线报告](docs/reports/progressive-context-compaction-rollout-2026-08-11.md)
- [15 题测评报告](benchmark-results/agentic-2026-07-27/representative15-report.html)
- [Agentic 测评协议](benchmark/agentic/README.md)

提交代码请先看 [CONTRIBUTING.md](CONTRIBUTING.md)，安全问题请按 [SECURITY.md](SECURITY.md) 报告。
五种语言的三轮编辑与运行核查记录在 [README 发布复核](docs/release/readme-review-2026-08-12.md)中。

安全漏洞请通过 GitHub 的[私密漏洞报告](https://github.com/agent-dance/luban/security/advisories/new)提交，不要发送到公开 issue。所需信息见 [SECURITY.md](SECURITY.md)。
