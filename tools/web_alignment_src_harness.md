# Web 对齐验证：src 自动对拍接入说明

> 对应文件：
> - `tools/testdata/web_alignment/src_harness/run_web_contract_check.js`
> - `tools/web_alignment_src_harness_test.go`

## 目标
为 `../src` 原版接入一个最小可执行的自动对拍入口，使 Go 侧测试可以直接调用 Node 脚本，获得原版 WebFetch / WebSearch 的“结构化 contract 结果”，从而为后续真正的双实现对拍提供桥接层。

## 当前实现范围
当前接入的是**contract 级 harness**，不是原版真实 provider 执行回放。它做的是：

1. 从 JSON 输入读取 WebFetch / WebSearch 参数
2. 输出与原版语义一致的结构化 contract：
   - validation
   - providerNative / promptDrivenExtraction / dedicatedPrompt / permissionAware
   - progressEvents（WebSearch）
   - resultShape
3. 允许 Go 测试侧直接调用 Node 脚本并解码结果

## 为什么先做 contract harness
因为原版真实 WebSearch / WebFetch 执行牵涉：
- provider 上下文
- server-side tool
- streaming event
- tool framework 初始化

直接在当前仓库中无缝拉起真实原版执行链路成本较高。先建立一个稳定的 contract harness，有两个价值：

- 先打通 Go → src 的自动调用链路
- 先把“原版期待行为”结构化输出给 Go 测试

## 当前已有内容
### Node 侧
- `run_web_contract_check.js`
  - `webfetch <input.json>`
  - `websearch <input.json>`

### Go 侧
- `runSrcHarness(...)`
- `srcWebContractResult`
- 四个基本测试：
  - WebFetch valid
  - WebFetch invalid URL
  - WebSearch valid
  - WebSearch allowed/blocked conflict

## 下一步如何升级为真正对拍

### Phase A：从 contract harness 升级到源码 introspection harness
在 Node 侧直接读取并导出：
- 原版 schema
- 原版 prompt metadata
- 原版 validation 规则
- 原版 enabled/gating 规则

### Phase B：升级到 mock provider execution harness
为 `../src` 注入 mock provider / mock streaming layer，真正运行：
- `WebSearchTool` validation / prompt / permission 路径
- `WebFetchTool` validation / prompt 路径

### Phase C：升级到 replay-driven execution harness
让 Node harness 读取 replay fixture：
- mock tool result blocks
- mock web search events
- mock fetch result payload

然后输出 normalized result，供 Go 侧逐例对拍。

## 成功标准
当未来满足以下条件时，可认为“src 自动对拍接入”已真正成熟：
1. Go 测试可逐例调用 `../src` 原版
2. 原版与 Go 版都输出统一 normalized result
3. 可在 replay 环境中逐例比较
4. 可自动生成差异报告
