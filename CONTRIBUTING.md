# Contributing to LUBAN Code

感谢你帮助改进 LUBAN Code。提交改动前，请先搜索现有 Issue 和 Pull Request；较大的行为或架构调整建议先开 Issue 对齐范围。

## 开发环境

```bash
git clone https://github.com/agent-dance/luban.git
cd luban
go mod download
go build ./cmd/luban-code
go test ./...
```

使用 `go.mod` 声明的 Go 版本。仓库包含本地 `pkg/go-tui` 模块替换，请保留该目录结构。

## 提交要求

- 将一个 Pull Request 聚焦在一个问题上。
- 为行为变更补充聚焦测试，并运行受影响包的测试。
- 提交前运行 `go test ./...`、`go vet ./...` 和 `go build ./cmd/luban-code`。
- 不提交二进制、缓存、会话、API key、调试日志或个人配置。
- 更新影响用户的文档和 `CHANGELOG.md`。
- 建议使用 Conventional Commits，例如 `fix(tools): preserve inspect pagination`。

## 国际化

所有新的用户可见文本必须使用语义化 `i18n.Key`，通过 `i18n.Text` 或 `i18n.Format` 输出，并为 `i18n.AllLanguages()` 返回的所有语言提供自然翻译。不要新增 `i18n.T`、`i18n.TString`、强制英文显示或直接传入渲染/日志 API 的英文句子。

涉及用户可见表面时，至少运行：

```bash
go test ./i18n
go test ./path/to/touched/package
```

仓库根目录 [AGENTS.md](AGENTS.md) 是这项策略的完整、权威说明。

## Pull Request 清单

- [ ] 变更目的和用户影响已说明。
- [ ] 新行为有测试覆盖。
- [ ] 受影响测试、全量测试和构建结果已记录。
- [ ] 用户可见文本符合 i18n 策略。
- [ ] 文档和 changelog 已按需更新。
- [ ] 没有提交秘密或本地生成物。

## 安全问题

不要在公开 Issue 或 Pull Request 中披露漏洞。请按照 [SECURITY.md](SECURITY.md) 使用 GitHub private vulnerability reporting。

贡献内容将依据项目的 [MIT License](LICENSE) 分发。
