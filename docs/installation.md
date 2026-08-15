# 安装、升级与卸载

LUBAN Code 提供预编译二进制，不要求用户安装 Go。发布资产以 GitHub Releases 为权威来源。

## Homebrew

macOS 和 Linux 用户可以安装官方 tap：

```bash
brew install agent-dance/tap/luban-code
luban --version
```

升级或卸载：

```bash
brew upgrade luban-code
brew uninstall luban-code
```

## macOS / Linux 安装脚本

安装最新版本：

```bash
curl -fsSL https://raw.githubusercontent.com/agent-dance/luban/main/install.sh | sh
```

在下载前检查脚本：

```bash
curl -fsSLo install.sh https://raw.githubusercontent.com/agent-dance/luban/main/install.sh
less install.sh
sh install.sh
```

固定版本或安装目录：

```bash
sh install.sh --version v0.1.0
sh install.sh --install-dir "$HOME/.local/bin"
```

脚本不会静默调用 `sudo`。如果默认目录不在 `PATH` 中，请根据脚本结束时的提示更新 shell 配置。重新运行脚本即可升级；使用 `--version` 可以安装或回退到指定版本。

## Windows PowerShell

安装最新版本：

```powershell
irm https://raw.githubusercontent.com/agent-dance/luban/main/install.ps1 | iex
```

先检查再执行：

```powershell
Invoke-WebRequest https://raw.githubusercontent.com/agent-dance/luban/main/install.ps1 -OutFile install.ps1
Get-Content .\install.ps1
.\install.ps1
```

固定版本或安装目录：

```powershell
.\install.ps1 -Version v0.1.0
.\install.ps1 -InstallDir "$env:LOCALAPPDATA\Programs\luban-code\bin"
```

重新运行脚本即可升级。脚本下载 Windows ZIP、验证 checksum，并在需要时为当前用户更新 `PATH`。

## 手动安装

从 [GitHub Releases](https://github.com/agent-dance/luban/releases/latest) 下载与平台匹配的文件：

| 平台 | 归档 |
| --- | --- |
| macOS Apple Silicon | `luban-code_Darwin_arm64.tar.gz` |
| macOS Intel | `luban-code_Darwin_x86_64.tar.gz` |
| Linux x86-64 | `luban-code_Linux_x86_64.tar.gz` |
| Linux ARM64 | `luban-code_Linux_arm64.tar.gz` |
| Windows x86-64 | `luban-code_Windows_x86_64.zip` |

解压归档，将 `luban`（Windows 为 `luban.exe`）移动到 `PATH` 中的目录，然后运行：

```bash
luban --version
```

## 校验发布资产

每个 Release 都包含 `checksums.txt`。Linux 可直接验证：

```bash
sha256sum -c checksums.txt --ignore-missing
```

macOS 使用：

```bash
shasum -a 256 luban-code_Darwin_arm64.tar.gz
```

Windows PowerShell 使用：

```powershell
Get-FileHash .\luban-code_Windows_x86_64.zip -Algorithm SHA256
```

将输出与 `checksums.txt` 中对应条目比较。安装脚本会自动完成这一步。

安装 GitHub CLI 后，还可以验证由 GitHub Actions 签发的 artifact attestation：

```bash
gh attestation verify <archive> --repo agent-dance/luban
```

Release 同时附带 SPDX SBOM，用于审计构建产物中的组件。

## 卸载

Homebrew 安装：

```bash
brew uninstall luban-code
```

脚本安装可以使用对应的卸载参数：

```bash
sh install.sh --uninstall
```

```powershell
.\install.ps1 -Uninstall
```

手动安装则删除安装目录中的 `luban` 或 `luban.exe`。Windows 用户如不再使用该目录，可从用户 `PATH` 中移除它。

运行数据和凭据保存在用户主目录下的 `.luban-code`。卸载二进制不会删除这些数据。仅当确定不再需要账号凭据、会话和用户配置时，才手动删除该目录；项目自己的 `.luban-code` 目录也不会自动删除。

## 从源码构建

源码构建面向贡献者，不是普通用户的推荐安装方式：

```bash
git clone https://github.com/agent-dance/luban.git
cd luban
go build -o luban ./cmd/luban-code
./luban --version
```

请使用 `go.mod` 声明的 Go 版本。由于仓库包含本地模块替换，当前不要将 `go install github.com/agent-dance/luban/cmd/luban-code@latest` 作为正式安装入口。
