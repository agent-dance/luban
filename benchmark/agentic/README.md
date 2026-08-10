# Agentic Coding Benchmark Harness v2

> 最新阶段报告：已冻结 20 题目录，并按要求在第 15 题后停止；新增题未执行 Docker 官方判分。详见[根目录摘要](../../README.md)与[15 题 HTML 报告](../../benchmark-results/agentic-2026-07-27/representative15-report.html)。

## 五题 Pilot 实测：Luban 逐项耗时与 Token 均低于 Codex，质量持平

以 Codex 为 `0%` 基准；负值表示 Luban 用量更低。Luban 除 kube 使用 `high` 外，其余任务均使用默认 `medium`。

| 任务 | 耗时：Luban / Codex（变化） | Token：Luban / Codex（变化） | LLM 调用：Luban / Codex（变化） | resolved：Luban / Codex |
| --- | ---: | ---: | ---: | ---: |
| Fabric | 169.3s / 219.5s（−22.9%） | 128,319 / 581,257（−77.9%） | 8 / 14（−42.9%） | 是 / 是 |
| agents-js | 265.3s / 521.9s（−49.2%） | 666,894 / 1,429,141（−53.3%） | 22 / 30（−26.7%） | 是 / 是 |
| kube | 253.7s / 319.1s（−20.5%） | 402,703 / 644,164（−37.5%） | 16 / 19（−15.8%） | 否 / 否 |
| skim | 277.3s / 513.9s（−46.0%） | 582,076 / 1,518,938（−61.7%） | 17 / 28（−39.3%） | 是 / 是 |
| IWYU | 277.3s / 616.0s（−55.0%） | 431,561 / 2,238,240（−80.7%） | 20 / 37（−45.9%） | 否 / 否 |
| **合计** | **1,242.9s / 2,190.4s（−43.3%）** | **2,211,553 / 6,411,740（−65.5%）** | **83 / 128（−35.2%）** | **3/5 / 3/5** |

[查看机器可读结果与逐项评测工件](../../benchmark-results/agentic-2026-07-27/raw/candidates/selected-optimized-20260730.json)。

本目录实现 Luban 与 Codex 的可复现、配对 Agentic Coding 测评。正式目标是 DeepSWE v1.1 的 113 题公开语料；5 题 pilot 只用于优化迭代，不能作为公开总分。下述正式设施独立于仓库根目录下已有的 `benchmark-results/`；快速本机入口则会把每次新报告写入该目录。

## 一键本机测评与 HTML 报告

安装一次仓库内命令后，可直接选择固化代表题集的规模：

```bash
go install ./benchmark/agentic/cmd/benchmark
benchmark --task-size=20 --with-codex
benchmark --task-size=20
```

不安装时也可在仓库内执行：

```bash
./bin/benchmark --task-size=20 --with-codex
./bin/benchmark --task-size=20
```

首次执行或需要更新 Codex 对照时添加 `--with-codex`。该次运行会产出独立的
Codex JSON/HTML 快照，并把它原子更新为后续测评的冻结基线。此后的默认命令只
运行 Luban，不再启动或评测 Codex；对比报告会复用最近一次完整的 Codex 基线。
若新的 Codex 运行或 gold oracle 证据不完整，旧基线不会被替换。较大
`task-size` 不能复用覆盖题目较少的基线，必须通过 `--with-codex` 扩充。

当前代表题库包含 20 道预注册的 SWE-bench-Live MultiLang 题目，每种语言
（C++、Go、Java、Rust、TypeScript）各 4 道，因此 `task-size` 接受 `1..20`。
题目始终按预注册顺序选择前 N 道，不在运行时随机抽样；前 5 道与原 pilot
完全一致。选择规则、数据文件 SHA-256 和最终顺序冻结在
`localbench/catalog/representative20.selection.json`，可使用同目录的
`generate_representative20.py` 从固定 revision 的 parquet 文件复建。

每次执行都会创建一个不覆盖旧数据的新目录：

```text
benchmark-results/agentic-YYYY-MM-DD/run-YYYYMMDD-HHMMSS-nN/
├── benchmark.json
├── codex-baseline.json   # 仅 --with-codex
├── codex-report.html     # 仅 --with-codex
├── report.html
└── raw/
```

`benchmark-results/codex-baseline.json` 是当前基线的权威指针副本，
`benchmark-results/codex-baseline.html` 是便于直接浏览的当前 Codex 报告；原始
不可变快照仍保留在其来源运行目录。每份对比报告都记录该快照的相对路径、
SHA-256、来源 run、固化时间、Codex CLI 自报版本和二进制 SHA-256。
仓库当前初始基线来自 `agentic-2026-07-27-local5` 的最后一次完整 5 题运行：
`codex-cli 0.144.6`，二进制 SHA-256
`134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`，严格得分
`2/5`。其结构化快照和独立 HTML 已一并托管。

`benchmark.json` 是结构化事实源。生成器会重新读取它，并套用
`benchmark/agentic/localbench/report.html.tmpl` 生成 `report.html`，不会在 HTML
生成代码中手填分数；报告内的工件链接全部是相对路径。

本机入口会构建当前 Luban 源码、从 Codex 的 `config.toml` 与 `auth.json` 读取
同一网关及凭据、按实际 Responses HTTP POST 统计 LLM 调用，然后执行公开任务
评测容器。只有 `--with-codex` 才会从 `PATH` 定位 Codex、读取 `codex --version`
并在每道题上并发运行 Codex/Luban；默认运行只执行 Luban，并在报告中明确标注
Codex 是历史冻结基线，双方任务耗时不是同一墙钟区间的观测值。
评测器优先使用本机 Docker daemon；若不可用，则自动使用正在运行的
`agentic-deepswe-amd64` Lima VM 内的 Docker。运行前需要上述任一容器入口、
Git、Python 3.11+ 以及有效的 Codex 配置；`--with-codex` 还要求 Codex CLI。

该入口用于快速、非官方的本机对比；正式公开分数协议仍由下文的 v2 harness
承担。

当前协议固定：

- DeepSWE release commit：`8cae5984d5dd0ee37445beff0e928dc10c331116`，任务格式为该 release 实际使用的 Harbor schema 1.1，agent 与 verifier 均无互联网。
- Pier 0.3 peeled commit：`e69a20e4e0ac073ec71fde0274bab3d9f40bac87`。
- Dataset `tasks` tree-v2 SHA-256：`ce6b3f3c7eff0b512d11060976c7f548267755afc26e377f50851b4523db98ea`。
- Pier `src` tree-v2 SHA-256：`600c65f30f803d1a9219432f01dd8637e1bf1c636558b3606b0c957f156af197`；`uv.lock` SHA-256：`8afbae2c8c78ed6eaa3a49656bb4639d77c07cf6cd2b72266e4ad2283d8dc943`。
- Codex 正式基线只接受 2026-07-26 冻结的 npm stable `@openai/codex` 0.145.0；alpha、旧版或仅靠 CLI 自报版本均不合格。bundle v2 同时固定官方 latest/Linux x64 dist-tag、registry 签名、SLSA attestation 地址、完整 8 文件 package tree 和 6 文件 vendor tree，preflight 再验证实际 vendor 文件。
- 两个 agent 的 provider/model/effort 都固定为 `openai/gpt-5.6-sol/xhigh`，禁止模型 fallback。

正式 113 题 v2 lock 已冻结为 `manifests/deepswe-v1.1-release-full-inventory-lock-8cae5984-v2.json`：`HashTaskInventory=85f7f80eb0c48ea3480f95e145d13bacf5782c9aea1c576f79c65a14626d3a7a`，lock 文件本身的 SHA-256 为 `e23cb7c40f696e191122647295d24ef6a4c2e7d2df2dca359acfaebc05e28263`。两者是不同的身份：manifest 的 `dataset.manifest_sha256` 使用前者，artifact/provenance 可另外记录后者，禁止互换。

## 目录与已实现组件

- `schema/manifest.schema.json`：严格 manifest schema，拒绝未知字段。
- `manifests/deepswe-v1.1.template.json`：113 题、4 次重复的正式模板。
- `manifests/deepswe-v1.1-pilot.template.json`：固定 5 题、1 次重复的 pilot 模板。
- `manifests/deepswe-v1.1-release-full-inventory-lock-8cae5984-v2.json`：113 题 full v2 lock。
- `manifests/deepswe-v1.1-release-pilot-inventory-lock.json`：这 5 题的 v2 partial lock；inventory SHA-256 为 `0d76a2c978a96350d1dc8468746e56ce25f34526aeffe85094d720979bf6a96b`。
- `pier/codex-0.145.0-linux-x64.bundle.json`：官方 npm provenance 快照和完整 content-addressed Codex Linux x64 runtime 清单。
- `evidenceproxy`：controller-only provider proxy；固定模型请求、替换真实 credential、拒绝 provider storage/stateful chaining，并生成无内容证据。
- `pierbackend`：可运行的 Pier full-trial backend；一次 `RunAgent` 物理启动一次 Pier trial，trial 内依次运行 agent 和独立 verifier。
- `harness`：preflight、source snapshot、oracle、配对计划、state-v2、原始 attempt、证据聚合、公开计分和 artifact ledger。
- `report`：从冻结 artifact bundle 生成 HTML；诊断输入不能冒充正式结果。

## 不可协商的证据

| 层 | 必须冻结或验证的内容 |
| --- | --- |
| Dataset | URL、40 位 commit、任务目录树 SHA-256、inventory coverage/count/hash |
| Task | `task.toml` 与 `instruction.md` SHA-256、解析后的完整 base commit、OCI 名称与不可变 digest |
| Evaluator | Pier URL/commit/tree、lock hash、实际二进制 SHA-256 和最低版本 |
| Codex | stable 0.145.0 registry 快照、签名/attestation 证据、package/vendor tree、每个 vendor 文件 mode/size/SHA-256 和实际 binary SHA-256 |
| Luban | binary SHA-256；source base commit、临时 index 产生的 tree OID/patch/archive；独立 build receipt |
| Model | 每个真实 HTTP request 的 run identity、开始序号、request/response ID 哈希、实际 model/effort、usage 和 completion proof |
| Runtime | CPU/内存/存储/timeout、pair 顺序、无网络 verifier、固定 egress proxy image digest 与本机 image ID |
| Submission | tracked/staged/unstaged/deleted/untracked/binary 的完整 temporary-index patch，且与 verifier 输入逐字节一致 |

Luban 的开发 worktree 不要求干净。`SnapshotAgentAt` 使用隔离的临时 Git index 和临时 object directory 构造完整 source tree，不读取真实 index 作为权威，也不修改真实 index/object database。它把 `source.patch`、确定性 `source.tar` 和 `build-receipt.json` 冻结到 artifact 中；任何 source 或 binary 漂移都会拒绝恢复。

## 执行和隔离

```text
manifest + immutable inventory lock
              |
              v
backend preflight -- commit/tree/image/bundle/proxy 不符 --> INVALID
              |
              v
all selected gold solutions -- separate pristine verifier --> 任一非满分：INVALID
              |
              v
deterministic adjacent pairs: Codex/Luban or Luban/Codex
              |
              +--> public instruction + isolated agent workspace
              |          | provider only via controller proxy
              |          v
              |     complete binary workspace patch
              |          |
              +--> separate no-network verifier (same Pier trial)
                         |
                         v
provider receipts + verifier reward + sealed raw attempt
              |
              v
state-v2 --> public DeepSWE score --> ledger --> HTML report
```

`PublicTaskView` 不包含 tests、solution、verifier 或 reward。Oracle 在任何 agent 启动前运行。agent sandbox 不持有 provider credential，只能访问 controller proxy；provider proxy 的 access path 也不会被写入公开日志。Pier 的 auxiliary Squid 必须是：

```text
ubuntu/squid@sha256:93d2d581a961f475ca5b23fe47fc3c3afadbe5849a6925a5b5435068502d7051
```

compose 使用精确 digest 和 `pull_policy: never`，禁止运行时 build/pull。preflight 同时记录本机 image ID；当前验证过的 amd64 image ID 是 `sha256:ff30461a9d2980e7584f3b9602c5bda1edd97e5c1b6ccb97996d6dcf2afebe8d`，不同主机必须记录自己的实际 ID，不能伪造复用。

## 配对、raw attempt 与恢复

`BuildPlan` 用 manifest hash、seed、repetition、task ID 和 agent ID 的 SHA-256 排序，不依赖 Go 伪随机实现。每个 task/repetition 的两个 agent 相邻，task 顺序与 pair 内先后次序可复现；正式 full run 固定 113 题 × 4 次，`max_parallel_pairs=1`。

state-v2 为每个 plan entry 预留且仅预留一个 raw slot：`attempt-001`。规则如下：

- agent timeout/context failure 是计分失败，不是 infra 排除。
- 只有结构化 `provider_infrastructure`、`verifier_infrastructure`、`network_infrastructure` 可以排除；日志字符串、异常文案或猜测不能触发排除。
- 被排除的 raw attempt 不生成替代 attempt；其已发生的时间、token、费用和工具活动仍保留在 all-executed 效率统计中。
- 成功/排除的 trial 写 `sealed-attempt.json`；恢复只读取该 sealed receipt，不重新调用 Pier。
- 只有在 sealed receipt 不存在且能证明 provider 请求数为零时，才允许在同一个 raw slot 安全重启。已发生付费请求但 seal 缺失时 fail closed，禁止静默重跑和重复计费。
- 未分类错误保持实验未完成并要求人工审计；不能把它自动变成 agent 0 分或 infra 排除。

公开分数由 `ScoreExperimentForManifest` 按 `deepswe-v1.1-public-ci` 规则计算。只有完整、合法的 state 才能产出 final scorecard；pilot、缺题、缺重复、无效 evidence 或 all-excluded 数据不能伪装为公开 113 题分数。

## Provider、缓存与工具证据

proxy raw schema 为 `agentic-bench/provider-http-v6`，规范化后每行是一条真实 provider request。`run_identity` 将并发完成顺序中的记录绑定回唯一 trial；`round` 在请求开始时分配，normalizer 按它排序，不能按完成顺序推断会话轨迹。v6 还证明：

- `store=false`、无 `previous_response_id`、`reasoning.context=all_turns`；
- encrypted reasoning 与完整 output-item replay 都来自同一 prompt-blind lineage；
- 合法 epoch reset 只接受旧序列的有序子序列，未知、乱序、跨 lineage 重放均被拒绝；
- 必须看到 terminal `response.completed`、`status=completed`、实际 response model 和原子 usage receipt；
- input、cached input、cache-write input、billed output 与 reasoning-output subset 分开记录。缺失值保持 `null`，绝不补零。

工具统计不得再用一个含糊的“调用数”。至少并列展示：provider requests、tool-bearing responses、provider response-committed logical calls、agent trace proposals、execution-matched logical calls、未匹配/中止、以及 physical child operations。Codex `item.started/completed` 与 Luban `machine-event/v2` 都要从无内容 trace 关联。Luban 的 `Run` 物理 step 只有在真实 `exec.Start` 成功后才计数；普通 legacy 工具的 `physical_child_operations` 仍只是 top-level operation proxy，报告必须标注 `ordinary_proxy`，不能把它解释为进程启动次数。

## 费用口径

正式价格源和生效时间冻结在 manifest。GPT-5.6 Sol 基础目录价为每百万 token：uncached input `$5`、cached input `$0.50`、output `$30`。单个请求的 input tokens **严格大于** 272,000 时，对该请求整体应用 input/cached ×2、output ×1.5；不能按整场 aggregate token 触发。cache write 仅在 provider 实际返回 `cache_write_tokens` 时按 uncached input 的 1.25 倍写入价计费：

```text
request cost =
  uncached_input / unit * input_rate * request_input_multiplier
  + cached_input / unit * cached_rate * request_cached_multiplier
  + output / unit * output_rate * request_output_multiplier
  + cache_write / unit * input_rate * request_input_multiplier * (1.25 - 1)
```

任一 usage receipt 缺失时，目录价只能标记为 partial；cache-write 字段未报告时只能给 lower bound，不能宣称完整费用。provider-reported cost 若存在则单列，不能用订阅额度或促销抵扣改写目录价。

## 配置和命令

backend config 是非 secret JSON；credential 只通过 `provider_credential_env` 指定的环境变量读取。示例：

```json
{
  "pier_binary": "/abs/pier-venv/bin/pier",
  "dataset_repository_root": "/abs/deep-swe",
  "evaluator_repository_root": "/abs/pier",
  "evaluator_manifest_path": "/abs/pier/uv.lock",
  "inventory_lock_path": "/abs/luban/benchmark/agentic/manifests/deepswe-v1.1-release-pilot-inventory-lock.json",
  "python_module_root": "/abs/luban",
  "private_work_root": "/abs/private-agentic-work",
  "registry_gate_path": "/abs/private-agentic-work/registry-gate.json",
  "egress_proxy_image": "ubuntu/squid@sha256:93d2d581a961f475ca5b23fe47fc3c3afadbe5849a6925a5b5435068502d7051",
  "proxy_listen_address": "0.0.0.0:0",
  "proxy_advertise_host": "host.docker.internal",
  "provider_upstream": "https://api.openai.com",
  "provider_credential_env": "AGENTIC_SUB_API_KEY"
}
```

先将 template 中所有 `${...}` 替换为审核过的绝对路径和哈希，再运行：

```bash
go run ./benchmark/agentic/cmd/agenticbench preflight \
  --manifest /abs/pilot.json --backend-config /abs/backend.json --work-dir /abs/work

go run ./benchmark/agentic/cmd/agenticbench oracle --execute \
  --manifest /abs/pilot.json --backend-config /abs/backend.json --work-dir /abs/work

go run ./benchmark/agentic/cmd/agenticbench run --execute \
  --manifest /abs/pilot.json --backend-config /abs/backend.json --work-dir /abs/work

go run ./benchmark/agentic/cmd/agenticbench resume --execute \
  --manifest /abs/pilot.json --backend-config /abs/backend.json --work-dir /abs/work

go run ./benchmark/agentic/cmd/agenticbench score \
  --manifest /abs/pilot.json --backend-config /abs/backend.json --work-dir /abs/work

go run ./benchmark/agentic/cmd/agenticbench ledger \
  --manifest /abs/pilot.json --backend-config /abs/backend.json --work-dir /abs/work
```

`run` 拒绝已有 state，`resume` 要求已有 state。`oracle --execute` 只跑完所有 gold oracle 后停止。`lock --execute` 会访问 registry/API 并只在目标 lock 不存在时写入；若已存在且内容不同则拒绝覆盖。生成 full lock 时必须使用 full selection/template，生成 pilot lock 时必须使用显式 5 题 selection，二者的 coverage/hash 不可互换。

## Artifact 与验证门

正式 artifact 至少包括：

```text
manifest.json
plan.json
state.json
backend-snapshot.json
agent-snapshots.json
sources/luban/source.patch
sources/luban/source.tar
sources/luban/build-receipt.json
oracle/<task>/verifier/*
runs/<pair>/<agent>/attempt-001/sealed-attempt.json
runs/<pair>/<agent>/attempt-001/submission.patch
runs/<pair>/<agent>/attempt-001/metrics/provider-http.jsonl
runs/<pair>/<agent>/attempt-001/metrics/provider-requests.jsonl
runs/<pair>/<agent>/attempt-001/verifier/*
scorecard.json
ledger.json
```

ledger 记录每个普通文件的相对路径、mode、大小和 SHA-256，拒绝 symlink/特殊文件。它证明归档未被事后替换，不代表 raw 内容自动适合公开。

无付费的本地完成门：

```bash
go test ./benchmark/agentic/evidenceproxy
go test ./benchmark/agentic/harness
go test ./benchmark/agentic/pierbackend
go test ./benchmark/agentic/report
go test ./i18n
```

正式运行还必须通过真实 Docker/Pier oracle、controller-only provider canary、0.145.0 Codex bundle preflight、5 题 pilot 以及 artifact/report 严格加载。任何占位符、旧 bundle、缺失 full inventory lock、部分费用或 protocol failure 都必须在报告中显式阻断“全面超越”结论。
