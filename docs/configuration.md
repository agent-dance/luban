# 配置指南

## 最小配置

至少配置一个模型提供商的凭据：

```bash
export DEEPSEEK_API_KEY="your-api-key"
# 或 OPENAI_API_KEY
# 或 ANTHROPIC_API_KEY
```

如果存在多个 key，建议显式指定提供商，避免依赖自动选择顺序：

```bash
luban-code --provider deepseek
luban-code --provider openai --model gpt-5.6-sol
luban-code --provider anthropic
```

不要将 API key 写入仓库、提交到 Git，或粘贴到问题报告和调试日志中。可把环境变量放入操作系统密钥管理工具或只在本地加载的 shell 配置。

## 常用提供商环境变量

| 提供商 | API key | 模型覆盖 | Endpoint 覆盖 |
| --- | --- | --- | --- |
| DeepSeek | `DEEPSEEK_API_KEY` | `DEEPSEEK_MODEL` | `DEEPSEEK_BASE_URL` |
| OpenAI | `OPENAI_API_KEY` | `OPENAI_MODEL` | `OPENAI_BASE_URL` |
| Anthropic | `ANTHROPIC_API_KEY` | `ANTHROPIC_MODEL` | `ANTHROPIC_BASE_URL` |
| Ollama | 不需要 | `OLLAMA_MODEL` | `OLLAMA_BASE_URL` |

还支持 Gemini、Groq、xAI、Mistral、智谱、MiniMax、Kimi、Amazon Bedrock 和 Google Vertex；运行 `luban-code --help` 查看 CLI 选项，并通过交互式模型设置选择已配置的提供商。

## 配置文件层级

LUBAN Code 使用 JSON 配置文件：

1. 用户配置：`~/.luban-code/settings.json`
2. 项目配置：`<project>/.luban-code/settings.json`
3. 项目本地配置：`<project>/.luban-code/settings.local.json`

项目配置适合团队共享；包含机器路径、个人偏好或敏感信息的配置应放入 `settings.local.json` 并排除 Git 跟踪。凭据由用户级凭据存储管理，不应写进项目配置。

## 常用命令行选项

```text
--provider NAME        指定提供商
--model MODEL          指定模型
-p, --print            执行一次性任务
--allowed-dir PATH     允许访问额外目录，可重复
--sandbox              对命令启用操作系统沙箱
--force-sandbox-tools  无法沙箱时拒绝执行
--allowed-tools LIST   工具白名单
--disallowed-tools LIST 工具黑名单
--language LANG        指定界面语言
--screen-reader        使用追加式无障碍界面
--no-color             禁用 ANSI 颜色
```

以本机安装版本的 `luban-code --help` 为完整且权威的选项列表。

## 权限边界

默认保留交互式权限确认。只允许代理访问完成任务所需的最小目录：

```bash
luban-code --allowed-dir ../shared-library
```

CI 或受控环境可以使用 `--allow-all`，但这会跳过交互式权限提示；不要在不可信仓库中默认开启。需要强制命令沙箱时使用：

```bash
luban-code --force-sandbox-tools
```

## 一次性与无障碍模式

一次性输出：

```bash
luban-code -p "运行测试并解释失败原因"
```

屏幕阅读器模式：

```bash
luban-code --screen-reader
```

也可以设置 `LUBAN_CODE_SCREEN_READER=1`。该模式需要交互式终端，不能与 `--print` 或 SDK 输入共用标准输入。
