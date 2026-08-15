# 故障排查

## Homebrew 显示安装成功后又报告 Permission denied

如果输出已经包含 `luban-code was successfully installed`，随后才在
`brew cleanup` 阶段报告其他路径的 `Permission denied`，LUBAN Code 通常已经
安装完成。先验证：

```bash
brew list --cask --versions luban-code
luban-code --version
```

这是 Homebrew 安装结束后执行的全局清理失败，不是 LUBAN Code 下载、校验或
安装失败。重新安装时可禁止这次附带清理：

```bash
HOMEBREW_NO_INSTALL_CLEANUP=1 brew install agent-dance/tap/luban-code
```

该变量不会关闭 checksum 或 tap trust。错误路径属于其他软件时，应单独检查其
所有者和符号链接目标，不要为了安装 LUBAN Code 递归修改整个 `/usr/local` 的
权限。

Homebrew 6 还可能列出机器上其他未受信任的第三方 taps。只要输出明确显示
`Trusted cask agent-dance/tap/luban-code`，这些警告就不表示 LUBAN Code Cask
未受信任；不要全局设置 `HOMEBREW_NO_REQUIRE_TAP_TRUST=1`。

## 找不到 `luban-code`

先重新打开终端并运行：

```bash
command -v luban-code
luban-code --version
```

脚本安装后仍找不到命令，通常是安装目录不在 `PATH`。重新运行安装脚本并查看结束提示，或用 `--install-dir` 指定已有的 `PATH` 目录。Windows 可用：

```powershell
Get-Command luban-code
$env:Path -split ';'
```

## 缺少 API key 或无法选择模型

确认环境变量存在，但不要把实际值打印到日志或问题报告：

```bash
test -n "$DEEPSEEK_API_KEY" && echo configured
test -n "$OPENAI_API_KEY" && echo configured
test -n "$ANTHROPIC_API_KEY" && echo configured
```

存在多个凭据时显式运行 `luban-code --provider <name> --model <id>`。检查自定义 endpoint 是否包含提供商要求的 API 基础路径。

## checksum 不匹配

不要运行该文件。删除归档和 `checksums.txt`，从同一个 GitHub Release 重新下载后再验证。代理、下载缓存或版本混用都可能导致不匹配；必须确保归档与 checksum 来自同一 Release。

如果问题可重复出现，请在 GitHub 提交问题并附上：Release 版本、资产文件名、操作系统、架构和实际 checksum。不要附 API key。

## macOS 阻止运行

先确认下载来自官方 Release，验证 checksum 和 GitHub artifact attestation。正式发布产物如果尚未完成 Apple notarization，macOS 可能显示来源确认提示。不要通过网络上的未知命令全局关闭 Gatekeeper；优先使用 Homebrew 安装或在确认发布来源后通过系统界面批准该单个应用。

## Windows Defender 或 SmartScreen 提示

确认文件来自 `agent-dance/luban` 的官方 Release，并验证 SHA-256。未经代码签名的早期版本可能触发信誉提示。不要关闭 Defender；如果 checksum 或来源无法核验，请不要运行并提交安全报告。

## 终端显示异常

确认终端支持 ANSI 和交互式输入。可尝试：

```bash
luban-code --no-color
luban-code --screen-reader
```

通过管道执行任务时使用 `--print`；屏幕阅读器模式必须连接交互式终端。

## 网络超时或流中断

检查提供商状态、网络代理、endpoint 和模型名称。先用一次性、最小请求排除项目因素：

```bash
luban-code --provider <name> -p "回复 OK"
```

需要收集诊断时，可以临时使用 `--debug-file <path>`。提交前必须检查并移除 API key、提示词、源码片段和其他敏感数据。

## 如何报告问题

普通缺陷请提交 GitHub Issue，并包含：

- `luban-code --version` 输出；
- 操作系统和架构；
- 安装方式；
- 可复现的最小步骤；
- 已脱敏的错误信息。

安全漏洞不要公开提交，按照 [SECURITY.md](../SECURITY.md) 私下报告。
