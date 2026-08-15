# LUBAN Code

[![CI](https://github.com/agent-dance/luban/actions/workflows/ci.yml/badge.svg)](https://github.com/agent-dance/luban/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/agent-dance/luban)](https://github.com/agent-dance/luban/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-blue)](#supported-platforms)

[中文](README.md) · [Installation](docs/installation.md) · [Configuration](docs/configuration.md) · [Troubleshooting](docs/troubleshooting.md)

LUBAN Code is an open-source terminal coding agent for understanding and modifying real repositories, running verification commands, managing sessions, and working with multiple model providers.

## Install

### Homebrew (macOS and Linux)

```bash
HOMEBREW_NO_INSTALL_CLEANUP=1 brew install agent-dance/tap/luban-code
```

### Install script (macOS and Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/agent-dance/luban/main/install.sh | sh
```

### PowerShell (Windows)

```powershell
irm https://raw.githubusercontent.com/agent-dance/luban/main/install.ps1 | iex
```

Alternatively, download and extract the Windows ZIP from [GitHub Releases](https://github.com/agent-dance/luban/releases/latest), then add the directory containing `luban-code.exe` to `PATH`.

## Quick start

Set the API key for one provider:

```bash
export DEEPSEEK_API_KEY="your-api-key"
# or: export OPENAI_API_KEY="your-api-key"
# or: export ANTHROPIC_API_KEY="your-api-key"
```

Start LUBAN Code inside a repository:

```bash
cd your-project
luban-code
```

Run a one-shot task with:

```bash
luban-code -p "Explain this repository's architecture"
```

LUBAN Code auto-detects a configured provider. Use `--provider` and `--model` to override the selection. See [configuration](docs/configuration.md) for provider, project, and permission settings.

## Upgrade and uninstall

```bash
brew upgrade luban-code
brew uninstall luban-code
```

Re-run `install.sh` or `install.ps1` to upgrade a script installation. See the [installation guide](docs/installation.md) for uninstall and data-directory details.

## Verify downloads

Every release includes SHA-256 checksums, an SBOM, and GitHub artifact attestations:

```bash
sha256sum -c checksums.txt --ignore-missing
gh attestation verify luban-code_Darwin_arm64.tar.gz --repo agent-dance/luban
```

The install scripts verify checksums automatically. Direct downloads should be verified before execution.

## Supported platforms

| OS | Architecture | Archive | Installation |
| --- | --- | --- | --- |
| macOS | Apple Silicon (`arm64`) | `tar.gz` | Homebrew, `install.sh`, manual |
| macOS | Intel (`x86_64`) | `tar.gz` | Homebrew, `install.sh`, manual |
| Linux | `x86_64` | `tar.gz` | Homebrew, `install.sh`, manual |
| Linux | `arm64` | `tar.gz` | Homebrew, `install.sh`, manual |
| Windows | `x86_64` | `zip` | `install.ps1`, manual |

Prebuilt binaries do not require Go. Building from source requires the Go version declared by `go.mod`.

## Project resources

- [Installation](docs/installation.md)
- [Configuration](docs/configuration.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Contributing](CONTRIBUTING.md)
- [Security](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [MIT License](LICENSE)
- [Benchmarks (Chinese)](README.md#性能与评测)
