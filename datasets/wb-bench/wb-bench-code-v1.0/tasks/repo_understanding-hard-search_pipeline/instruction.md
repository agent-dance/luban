我刚接手 ripgrep，想看懂从命令行参数到递归找文件、过滤、匹配、搜索、打印结果的大致流程。

请在仓库根目录生成 `analysis.json`，不要改业务代码。格式保持稳定：

```json
{
  "summary": "不超过 200 字",
  "facts": [
    {
      "id": "short-name",
      "claim": "关键事实",
      "details": "可选说明",
      "evidence": [{"path": "crates/core/main.rs", "line": 77}]
    }
  ],
  "request_flow": ["..."],
  "extension_points": ["..."]
}
```

重点说明：CLI parse 后怎么变成高层参数；单线程/多线程搜索怎么选择；目录遍历和 ignore/glob/type 过滤在哪里；haystack 怎么决定是否搜索；regex matcher、Searcher、SearchWorker、printer 怎么协作；binary/preprocessor/decompression 这类特殊路径在哪里处理。每个关键事实都要给真实 `path` 和 `line` 证据。
