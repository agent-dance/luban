# LUBAN Code

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [Deutsch](README.de.md)

LUBAN Code は、長時間に及ぶリポジトリ作業を想定した Go 製のコーディングエージェントです。トークンを減らすために元のセッション記録を書き換えることはありません。また、プロキシの URL が変わっても、Provider 固有のプロトコルは維持されます。

> 現在は `v0.1.0` のソースプレビューです。バイナリやパッケージマネージャー向けの配布はまだありません。ソースからビルドしてください。

![OpenAI gpt-5-6 モデルで動作中の LUBAN Code TUI](docs/assets/screenshots/luban-tui.png)

_2026-08-12、Windows 上で現在のソースをビルドし、実際の TUI を撮影しました。API key と endpoint のアドレスは表示していません。_

## 何が違うのか

### 証拠を消さず、Provider に渡す表示だけを圧縮する

長いセッションでは、古いツール結果をすべてプロンプトに残すか、確認しにくい要約で履歴を置き換えるか、という選択になりがちです。LUBAN は元の transcript を保持します。現在の限定された本番ポリシーでは、古い `Inspect` 結果だけを Provider 向けの決定的な投影に置き換えます。パス、行範囲、ページ情報、digest、proof は残ります。

投影を適用する前に、リクエスト全体のコストを見積もります。コールドキャッシュと回復分を含めても節約になる場合だけ適用します。価格が不明、usage が不完全、証拠が失敗、節約量が不足。このどれかに当てはまれば投影しません。異常な投影はロールバックされ、同じセッションで 3 回続くとサーキットブレーカーが作動します。

本番での対象は意図的に絞っています。`openai/gpt-5.6-sol*` と `deepseek/deepseek-v4-flash*`、ツールは `Inspect` だけです。仕組みと制限は[設計資料](docs/design/progressive-context-compaction.md)と[80k ペア実行の記録](benchmark-results/progressive-context-compaction-v7-80k-2026-08-10/README.md)で確認できます。

### プロキシが変えるのは経路だけ

`BaseURL` は通信先の設定です。OpenAI、DeepSeek、Anthropic、Vertex、Bedrock を独自 URL 経由で使っても、認証、キャッシュ制御、Responses と Chat の意味、Provider 固有のリクエスト項目は元の契約に従います。汎用的な OpenAI-compatible 方言へ暗黙に変換しません。

Responses から Chat への自動ネゴシエーションは、compatible Provider を明示的に選んだ場合だけです。現在は `404`、`405`、`501` を endpoint 不在として扱います。認証、レート制限、schema の問題はエラーとして返り、プロトコルのフォールバックは起きません。

### モデルに見せる道具は少なく、運用機能は深く

標準の本番設定でモデルに公開されるコーディングカーネルは、`Inspect`、`ApplyPatch`、`Run` の 3 つです。`ContextUpdate` の shadow 経路を有効にすると、この内部ツールも加わります。その周囲に、再開可能なセッション、並列サブエージェント、任意の Git worktree、権限確認、ライフサイクル hooks、MCP 接続、NDJSON/Go SDK 境界があります。サブエージェントは起動時の権限スナップショットを保持します。親セッションの権限を後から緩めても、実行中の子には広がりません。

TUI では、コンテキスト、キャッシュ、費用、圧縮、サブエージェントの状態を確認できます。`--screen-reader` はカーソル制御、マウス捕捉、アニメーションを使わない追記型モードです。実行時の表示は英語、簡体字中国語、ドイツ語、日本語、韓国語、ロシア語に対応し、`Ctrl+L` または `/language` で切り替えられます。

## 数値と、その読み方

固定した 15 タスクのローカル比較では、選定された LUBAN の実行は、選定された Codex の実行よりも経過時間、トークン、モデル呼び出しが少ない結果でした。

| 観測値の合計 | LUBAN | Codex | 差分 |
| --- | ---: | ---: | ---: |
| 経過時間 | 4,020.6 秒 | 5,644.5 秒 | -28.8% |
| トークン | 6,857,490 | 17,889,019 | -61.7% |
| LLM 呼び出し | 245 | 354 | -30.8% |
| patch を生成 | 15/15 | 15/15 | 同じ |

この固定したローカル標本だけで、一般的な優劣は判断できません。公式 grader の結果があるのは当初の 5 タスクだけで、解決数は両者とも 3/5 でした。追加の 10 タスクは未採点です。実行の選定は最適化後に行われ、モデルの seed も固定されていません。広い結論を出す前に、[HTML レポート全文](benchmark-results/agentic-2026-07-27/representative15-report.html)、[選定済みの機械可読データ](benchmark-results/agentic-2026-07-27/raw/candidates/selected-15task-20260731.json)、[評価プロトコル](benchmark/agentic/README.md)を確認してください。

段階的圧縮の実験も同じく限定的です。1 回の 80k ペア実行では、固定 evaluator の結果は両方とも `2/2 + 455/455` でした。total token は `1,362,070` から `444,419`、推定費用は `$5.207999` から `$1.004185` になりました。ただし seed を固定できず、2 本のトレースは最初の投影前に分岐しています。これは実際の 2 トレースの測定値と固定レートによる費用推定であり、投影効果の因果平均ではありません。

## ソースからビルドする

Git と、[`go.mod`](go.mod) に記載された Go が必要です。現在のバージョンは `1.26.1` です。`Run` の shell-form は Bash を呼び出します。Windows では Git Bash、WSL Bash などの `bash` を `PATH` に追加してください。

macOS または Linux：

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

現在の module にはローカル `replace` があるため、`go install github.com/agent-dance/luban/cmd/luban-code@latest` はサポートしていません。

## Provider に接続して実行する

認証情報は環境変数で設定できます。複数の認証情報がある場合は、Provider も明示してください。

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

DeepSeek は `PROVIDER=deepseek` と `DEEPSEEK_API_KEY` で使えます。既定の Provider も DeepSeek です。Ollama は既定で `http://localhost:11434/v1` とモデル `llama3.1` を使います。TUI を起動して `Alt+P` を押し、Provider、モデル、利用可能な認証方式を選ぶこともできます。

TUI を開かずに 1 回だけ実行する例：

```sh
./luban-code -p "このリポジトリを確認し、最も危険な問題を報告してください"
```

![LUBAN Code v0.1.0 が実際に LUBAN READY を返した 1 回実行の画面](docs/assets/screenshots/luban-live-run.png)

_2 番目のコマンドは、ローカルで設定済みの OpenAI endpoint に実際のリクエストを送り、終了コード 0 で完了しています。画像内のローカルプロンプトパスは伏せています。確認できるのは、この 1 回の動作だけです。Provider の互換性や性能を測ったものではありません。_

TUI 内の `/init` は、既存ファイルを上書きせずに `LUBAN.md` とプロジェクト設定を追加します。認証情報は設定しません。

## 利用前に確認しておく制限

- Linux の OS sandbox には Bubblewrap が必要です。macOS は `sandbox-exec` を使います。Windows には現在 OS sandbox backend がありません。検証済み backend がない状態で `--force-sandbox-tools` を指定すると、処理は失敗します。
- Agent Teams は実験的な opt-in 機能です。並列サブエージェントや worktree 分離は、遠隔分散 swarm を意味しません。
- Provider の登録とプロトコルテストは、すべてのモデルや第三者 gateway の認証試験ではありません。
- ローカルの認証情報は平文 JSON です。Unix 系では `0600` で書き込みますが、Windows には現時点で同等の ACL 保証がありません。暗号化 vault や OS keychain ではありません。
- Node.js が必要なのは Node.js 製 MCP server を使う場合だけです。CLI 本体には不要です。
- リポジトリ直下には、まだライセンスがありません。Owner が公開するまでは通常の著作権が適用されます。

## 根拠資料

- [段階的コンテキスト圧縮の設計](docs/design/progressive-context-compaction.md)
- [段階的圧縮のロールアウト報告](docs/reports/progressive-context-compaction-rollout-2026-08-11.md)
- [15 タスクのベンチマーク報告](benchmark-results/agentic-2026-07-27/representative15-report.html)
- [Agentic ベンチマークのプロトコル](benchmark/agentic/README.md)

コントリビューションは [CONTRIBUTING.md](CONTRIBUTING.md)、セキュリティ報告は [SECURITY.md](SECURITY.md) を確認してください。
5 言語で行った 3 回の編集・実行確認は [README リリースレビュー](docs/release/readme-review-2026-08-12.md)に記録しています。

セキュリティ上の問題は、公開 issue ではなく GitHub の[非公開脆弱性報告](https://github.com/agent-dance/luban/security/advisories/new)から送ってください。必要な情報は [SECURITY.md](SECURITY.md) に記載しています。
