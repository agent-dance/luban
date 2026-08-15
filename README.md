# LUBAN Code

[![CI](https://github.com/agent-dance/luban/actions/workflows/ci.yml/badge.svg)](https://github.com/agent-dance/luban/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/agent-dance/luban)](https://github.com/agent-dance/luban/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-blue)](#支持平台)

[English](README.en.md) · [安装指南](docs/installation.md) · [配置指南](docs/configuration.md) · [故障排查](docs/troubleshooting.md)

LUBAN Code 是一款运行在终端中的开源 AI 编程代理，面向真实代码库提供代码理解、修改、命令验证、会话管理和多模型接入能力。

## 安装

### Homebrew（macOS / Linux）

```bash
HOMEBREW_NO_INSTALL_CLEANUP=1 brew install agent-dance/tap/luban-code
```

### 安装脚本（macOS / Linux）

```bash
curl -fsSL https://raw.githubusercontent.com/agent-dance/luban/main/install.sh | sh
```

脚本会下载 GitHub Release 中与当前平台匹配的归档并校验 SHA-256；默认安装到用户可写目录，不会静默使用 `sudo`。需要固定版本或安装目录时，请参阅[安装指南](docs/installation.md)。

### PowerShell（Windows）

```powershell
irm https://raw.githubusercontent.com/agent-dance/luban/main/install.ps1 | iex
```

也可以从 [GitHub Releases](https://github.com/agent-dance/luban/releases/latest) 下载 Windows ZIP，解压后将 `luban-code.exe` 所在目录加入 `PATH`。

## 快速开始

选择一个模型提供商并设置 API key：

```bash
# DeepSeek
export DEEPSEEK_API_KEY="your-api-key"

# 或 OpenAI
export OPENAI_API_KEY="your-api-key"

# 或 Anthropic
export ANTHROPIC_API_KEY="your-api-key"
```

进入项目并启动交互界面：

```bash
cd your-project
luban-code
```

也可以执行一次性任务：

```bash
luban-code -p "解释这个仓库的架构"
```

LUBAN Code 会根据可用的 key 自动选择提供商；可用 `--provider` 和 `--model` 显式指定。完整环境变量、项目配置和权限选项见[配置指南](docs/configuration.md)。

## 升级与卸载

```bash
# Homebrew
brew upgrade luban-code
brew uninstall luban-code

# 安装脚本安装的版本
curl -fsSL https://raw.githubusercontent.com/agent-dance/luban/main/install.sh | sh
```

Windows 用户可重新运行 `install.ps1` 升级；卸载方式和数据目录说明见[安装指南](docs/installation.md#卸载)。

## 校验下载

Release 同时发布 SHA-256 校验文件、SBOM 和 GitHub artifact attestation。下载归档后可验证：

```bash
sha256sum -c checksums.txt --ignore-missing
gh attestation verify luban-code_Darwin_arm64.tar.gz --repo agent-dance/luban
```

macOS 可使用 `shasum -a 256 <archive>` 与 `checksums.txt` 中对应条目比较。安装脚本会自动执行 checksum 校验，但直接下载的用户应手动校验。

## 支持平台

| 操作系统 | 架构 | 发布格式 | 安装方式 |
| --- | --- | --- | --- |
| macOS | Apple Silicon (`arm64`) | `tar.gz` | Homebrew、`install.sh`、手动下载 |
| macOS | Intel (`x86_64`) | `tar.gz` | Homebrew、`install.sh`、手动下载 |
| Linux | `x86_64` | `tar.gz` | Homebrew、`install.sh`、手动下载 |
| Linux | `arm64` | `tar.gz` | Homebrew、`install.sh`、手动下载 |
| Windows | `x86_64` | `zip` | `install.ps1`、手动下载 |

运行时不要求安装 Go。源码构建需要仓库 `go.mod` 声明的 Go 版本。平台归档名称和离线安装步骤见[安装指南](docs/installation.md)。

## 文档与项目治理

- [安装、升级与卸载](docs/installation.md)
- [模型与项目配置](docs/configuration.md)
- [故障排查](docs/troubleshooting.md)
- [贡献指南](CONTRIBUTING.md)
- [安全政策](SECURITY.md)
- [更新日志](CHANGELOG.md)
- [MIT License](LICENSE)

## 性能与评测

## 15 题优化实测：Luban 总耗时低 28.8%，Token 低 61.7%，LLM 调用低 30.8%

以 Codex 为 `0%` 基准；负值表示 Luban 更低。每格按“优化前 → 优化后 / Codex（优化后相对 Codex）”展示。针对原先落后的 7 道题完成优化复测后，Luban 的 15 题合计耗时从高于 Codex `96.3%` 变为低 `28.8%`，超时从 2 次降为 0 次；7 道复测题的耗时与 Token 均已低于 Codex。

20 题目录已经冻结，但按要求在第 15 题后停止，最后 5 题未运行。复测沿用原有本地代理方式，没有下载 Docker 镜像；新增 10 题未做官方判分，因此 resolved 只展示原 5 题结果。

| 任务 | 耗时：优化前 → Luban / Codex（变化） | Token：优化前 → Luban / Codex（变化） | LLM 调用：优化前 → Luban / Codex（变化） | resolved / 状态 |
| --- | ---: | ---: | ---: | --- |
| Fabric | 169.3s → 169.3s / 219.5s（−22.9%） | 128,319 → 128,319 / 581,257（−77.9%） | 8 → 8 / 14（−42.9%） | 是 / 是；沿用基线 |
| agents-js | 265.3s → 265.3s / 521.9s（−49.2%） | 666,894 → 666,894 / 1,429,141（−53.3%） | 22 → 22 / 30（−26.7%） | 是 / 是；沿用基线 |
| kube | 253.7s → 253.7s / 319.1s（−20.5%） | 402,703 → 402,703 / 644,164（−37.5%） | 16 → 16 / 19（−15.8%） | 否 / 否；沿用基线 |
| skim | 277.3s → 277.3s / 513.9s（−46.0%） | 582,076 → 582,076 / 1,518,938（−61.7%） | 17 → 17 / 28（−39.3%） | 是 / 是；沿用基线 |
| IWYU | 277.3s → 277.3s / 616.0s（−55.0%） | 431,561 → 431,561 / 2,238,240（−80.7%） | 20 → 20 / 37（−45.9%） | 否 / 否；沿用基线 |
| ninja | 547.0s → 547.0s / 611.1s（−10.5%） | 1,435,423 → 1,435,423 / 2,859,848（−49.8%） | 33 → 33 / 39（−15.4%） | 未判分；沿用基线 |
| crush | 153.7s → 153.7s / 230.0s（−33.2%） | 74,322 → 74,322 / 374,868（−80.2%） | 7 → 7 / 13（−46.2%） | 未判分；沿用基线 |
| floci | 246.6s → 169.4s / 191.9s（−11.7%） | 372,478 → 168,545 / 410,507（−58.9%） | 15 → 10 / 14（−28.6%） | 未判分；正式复测 |
| eza | 1,800.2s → 245.3s / 470.8s（−47.9%） | — → 394,477 / 2,234,769（−82.3%） | 4 → 15 / 37（−59.5%） | 未判分；超时 → 完成 |
| assistant-ui | 632.2s → 123.9s / 194.6s（−36.3%） | 459,912 → 74,139 / 322,474（−77.0%） | 23 → 7 / 12（−41.7%） | 未判分；正式复测 |
| actor-framework | 1,439.4s → 282.8s / 312.6s（−9.5%） | 1,070,509 → 349,663 / 977,723（−64.2%） | 31 → 16 / 22（−27.3%） | 未判分；正式复测 |
| lima | 372.6s → 372.6s / 392.6s（−5.1%） | 548,817 → 548,817 / 1,313,747（−58.2%） | 18 → 18 / 23（−21.7%） | 未判分；沿用基线 |
| springdoc | 1,800.2s → 218.3s / 274.0s（−20.3%） | — → 612,031 / 911,769（−32.9%） | 2 → 20 / 18（+11.1%） | 未判分；超时 → 完成 |
| napi-rs | 1,354.1s → 292.8s / 340.2s（−13.9%） | 1,287,671 → 372,716 / 660,232（−43.5%） | 37 → 14 / 18（−22.2%） | 未判分；正式复测 |
| G2 | 1,490.6s → 371.8s / 436.2s（−14.8%） | 2,651,755 → 615,804 / 1,411,342（−56.4%） | 45 → 22 / 30（−26.7%） | 未判分；正式复测 |
| **合计** | **11,079.5s → 4,020.6s / 5,644.5s（−28.8%）** | **10,112,440* → 6,857,490 / 17,889,019（−61.7%）** | **298 → 245 / 354（−30.8%）** | **超时 2 → 0 / 0；patch 15/15 / 15/15** |

\* 优化前 `eza`、`springdoc` 超时且无可解析 Token，因此优化前 Token 合计只有 13 题；优化后与 Codex 的百分比按完整 15 题计算。新增任务未判分，不能用于比较解决率。每项工程优化、原因、关联任务和实际效果均沉淀在完整报告中。

[查看完整 HTML 报告](benchmark-results/agentic-2026-07-27/representative15-report.html) · [查看机器可读结果](benchmark-results/agentic-2026-07-27/raw/candidates/selected-15task-20260731.json) · [查看测评协议](benchmark/agentic/README.md)
