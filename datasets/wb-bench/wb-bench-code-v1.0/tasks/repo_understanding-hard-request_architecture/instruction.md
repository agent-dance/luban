我刚接手这个 HTTPX 仓库，想先弄清楚一次请求从用户 API 进来以后，到 Client、auth、redirect、transport、httpcore 边界大概是怎么串起来的。

请你在仓库根目录生成一个 `analysis.json`，不要改业务代码。这个 JSON 后面会被脚本读取，所以格式尽量稳定：

```json
{
  "summary": "不超过 200 字的整体说明",
  "facts": [
    {
      "id": "short-name",
      "claim": "你确认的关键事实",
      "details": "可选，稍微展开说明",
      "evidence": [
        {"path": "httpx/_client.py", "line": 879}
      ]
    }
  ],
  "request_flow": ["..."],
  "async_flow": ["..."],
  "extension_points": ["..."]
}
```

我主要关心这些点：

- 顶层 `httpx.get()` / `httpx.request()` / `httpx.stream()` 和 `Client` 的关系；
- 同步请求从 `Client.request()` 到真正发出去的大致调用链；
- `AsyncClient` 和同步 Client 的对应关系，以及 async transport 的边界；
- transport 抽象在哪里，默认 transport 怎么接到 httpcore；
- proxy / mount 是怎么选 transport 的；
- auth flow 在请求过程中怎么参与；
- CLI 入口最终是不是也复用 Client 请求路径。

每条关键结论都要带 `evidence`，里面的 `path` 必须是仓库里的真实文件，`line` 是能支撑这条结论的行号。这里不需要你写长篇文章，我更想要一份能让后续维护者快速定位代码的结构化理解报告。
