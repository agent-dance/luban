# Changelog

LUBAN Code 的重要用户可见变更记录在此文件中。格式参考 [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)，版本遵循 [Semantic Versioning](https://semver.org/spec/v2.0.0.html)。

## [Unreleased]

## [0.2.0] - 2026-08-16

### Added

- `Run` 步骤新增 `image_output`，支持在当前模型具备视觉能力时把受限大小的图片 data URI 返回给模型进行真实视觉检查。

### Changed

- Agentic V2 默认工作模式改为按用户目标、风险、时效与投入回报分配精力：MVP 缩减范围而不牺牲核心完成度。
- UI 与视觉任务将感知质量视为正确性的一部分，优先打磨核心体验，并在环境允许时从用户可见边界直接观察、批判和迭代产物。
- 精简系统提示词中重复的工具说明，保留与工具无关、可跨场景泛化的工程判断和验证契约。

## [0.1.1] - 2026-08-15

### Changed

- 安装后的唯一可执行命令改为 `luban`；发布归档和 Homebrew Cask 名称保持兼容。
- 安装器升级时移除旧的 `luban-code` 可执行文件，避免残留两个启动命令。

## [0.1.0] - 2026-08-15

### Added

- LUBAN Code 首个公开版本。
- 正式分发文档、MIT License、安全政策和贡献指南。
- macOS、Linux 和 Windows 的安装入口及发布资产验证说明。
- 面向代码库的终端交互与一次性任务模式。
- DeepSeek、OpenAI、Anthropic 及其他兼容模型提供商接入。
- 工具调用、权限控制、会话管理与多语言界面。

[Unreleased]: https://github.com/agent-dance/luban/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/agent-dance/luban/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/agent-dance/luban/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/agent-dance/luban/releases/tag/v0.1.0
